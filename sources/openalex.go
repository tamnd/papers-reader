package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// OpenAlex is last on the ladder, and it is last for a reason. It has the
// widest coverage of anything here and the loosest matching, which is a good
// trade for the question "does a record exist at all" and a bad one for the
// question "is this the paper". It is also the only source of a citation
// count, which the reports use.

type openalexWork struct {
	ID          string `json:"id"`
	DOI         string `json:"doi"`
	Title       string `json:"display_name"`
	Year        int    `json:"publication_year"`
	CitedBy     int    `json:"cited_by_count"`
	Authorships []struct {
		Author struct {
			Name string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	BestOA   *openalexLocation `json:"best_oa_location"`
	Primary  *openalexLocation `json:"primary_location"`
	Abstract map[string][]int  `json:"abstract_inverted_index"`
}

type openalexLocation struct {
	PDFURL      string `json:"pdf_url"`
	LandingPage string `json:"landing_page_url"`
	Licence     string `json:"license"`
	Version     string `json:"version"`
}

// OpenAlexByDOI looks one DOI up.
func OpenAlexByDOI(ctx context.Context, c *Client, doi string) (Candidate, error) {
	doi = CleanDOI(doi)
	if doi == "" {
		return Candidate{}, errNotFound
	}
	body, err := c.Get(ctx, c.withMailto(c.Base.OpenAlex+"/doi:"+url.PathEscape(doi)))
	if err != nil {
		return Candidate{}, err
	}
	return parseOpenAlexWork(body)
}

func parseOpenAlexWork(body []byte) (Candidate, error) {
	var w openalexWork
	if err := json.Unmarshal(body, &w); err != nil {
		return Candidate{}, fmt.Errorf("the OpenAlex response does not parse: %w", err)
	}
	cand := openalexCandidate(w)
	if cand.Title == "" {
		return Candidate{}, errNotFound
	}
	return cand, nil
}

// OpenAlexByTitle searches by title.
//
// This is the query that, while this was being written, answered "Attention
// Is All You Need" with a 2025 preprint carrying a fabricated DOI. Everything
// it returns is a suggestion and Verify decides.
func OpenAlexByTitle(ctx context.Context, c *Client, title string, year int) ([]Candidate, error) {
	filter := "title.search:" + strings.ReplaceAll(title, ",", " ")
	if year > 0 {
		filter += fmt.Sprintf(",from_publication_date:%d-01-01,to_publication_date:%d-12-31", year-YearSlack, year+YearSlack)
	}
	q := url.Values{}
	q.Set("filter", filter)
	q.Set("per-page", "10")
	body, err := c.Get(ctx, c.withMailto(c.Base.OpenAlex+"?"+q.Encode()))
	if err != nil {
		return nil, err
	}
	var out struct {
		Results []openalexWork `json:"results"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("the OpenAlex response does not parse: %w", err)
	}
	var cands []Candidate
	for _, w := range out.Results {
		if cand := openalexCandidate(w); cand.Title != "" {
			cands = append(cands, cand)
		}
	}
	return cands, nil
}

func openalexCandidate(w openalexWork) Candidate {
	cand := Candidate{
		Title:    collapse(w.Title),
		Year:     w.Year,
		DOI:      CleanDOI(w.DOI),
		Source:   "openalex",
		Abstract: deinvert(w.Abstract),
	}
	for _, a := range w.Authorships {
		if n := collapse(a.Author.Name); n != "" {
			cand.Authors = append(cand.Authors, n)
		}
	}
	for _, loc := range []*openalexLocation{w.BestOA, w.Primary} {
		if loc == nil {
			continue
		}
		if cand.URL == "" {
			cand.URL = loc.PDFURL
		}
		if cand.Landing == "" {
			cand.Landing = loc.LandingPage
		}
		if cand.Licence == "" {
			cand.Licence = loc.Licence
		}
	}
	if a := CleanArXivID(cand.URL); strings.Contains(cand.URL, "arxiv.org") && a != "" {
		cand.ArXiv = a
	}
	return cand
}

// CitationCount is the one thing only OpenAlex has. It is used by the reports
// and never by the resolver, because how often a paper is cited says nothing
// about whether this record is that paper.
func CitationCount(ctx context.Context, c *Client, doi string) (int, error) {
	doi = CleanDOI(doi)
	if doi == "" {
		return 0, errNotFound
	}
	body, err := c.Get(ctx, c.withMailto(c.Base.OpenAlex+"/doi:"+url.PathEscape(doi)))
	if err != nil {
		return 0, err
	}
	var w openalexWork
	if err := json.Unmarshal(body, &w); err != nil {
		return 0, fmt.Errorf("the OpenAlex response does not parse: %w", err)
	}
	return w.CitedBy, nil
}

// deinvert rebuilds an abstract from OpenAlex's inverted index, which is a
// map from each word to the positions it appears at.
//
// They store it that way to avoid shipping the publisher's abstract verbatim.
// Putting it back together is exactly what they expect callers to do, and the
// corpus only keeps the result for a restricted paper, where an abstract
// under 250 words is the whole of what gets published.
func deinvert(index map[string][]int) string {
	if len(index) == 0 {
		return ""
	}
	high := -1
	for _, positions := range index {
		for _, p := range positions {
			if p > high {
				high = p
			}
		}
	}
	if high < 0 || high > 100000 {
		return ""
	}
	words := make([]string, high+1)
	for word, positions := range index {
		for _, p := range positions {
			if p >= 0 && p <= high {
				words[p] = word
			}
		}
	}
	return collapse(strings.Join(words, " "))
}
