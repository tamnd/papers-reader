package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Crossref is the registration agency for most of the DOIs here. It is on the
// ladder for the canonical DOI, the venue and the author list rather than for
// the PDF, because the link it gives is usually the publisher's landing page
// and behind whatever the publisher puts in front of it.

type crossrefWork struct {
	DOI       string     `json:"DOI"`
	Title     []string   `json:"title"`
	Abstract  string     `json:"abstract"`
	Container []string   `json:"container-title"`
	Author    []crAuthor `json:"author"`
	Issued    crDate     `json:"issued"`
	Published crDate     `json:"published"`
	PrintDate crDate     `json:"published-print"`
	Link      []crLink   `json:"link"`
	Licence   []crLicnce `json:"license"`
	URL       string     `json:"URL"`
}

type crAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
	Name   string `json:"name"`
	// Raw is the whole name in one string. Crossref never sends it, but
	// Unpaywall copies its author list from several upstreams and for some
	// records this is the only field there is. A resolver that read the
	// split fields alone would see a paper with no authors at all, and the
	// author predicate would then refuse a candidate that is plainly right.
	Raw string `json:"raw_author_name"`
}

// name is the author written out, whichever of the shapes the record uses.
func (a crAuthor) name() string {
	switch {
	case a.Family != "" && a.Given != "":
		return a.Given + " " + a.Family
	case a.Family != "":
		return a.Family
	case a.Name != "":
		return a.Name
	}
	return a.Raw
}

// crDate is Crossref's date-parts, which is a list of lists of numbers where
// only the first number is reliably present.
type crDate struct {
	Parts [][]int `json:"date-parts"`
}

func (d crDate) year() int {
	if len(d.Parts) == 0 || len(d.Parts[0]) == 0 {
		return 0
	}
	return d.Parts[0][0]
}

type crLink struct {
	URL         string `json:"URL"`
	ContentType string `json:"content-type"`
	Application string `json:"intended-application"`
}

type crLicnce struct {
	URL     string `json:"URL"`
	Version string `json:"content-version"`
}

// CrossrefByDOI looks one DOI up.
func CrossrefByDOI(ctx context.Context, c *Client, doi string) (Candidate, error) {
	doi = CleanDOI(doi)
	if doi == "" {
		return Candidate{}, errNotFound
	}
	raw := c.withMailto(c.Base.Crossref + "/" + url.PathEscape(doi))
	body, err := c.Get(ctx, raw)
	if err != nil {
		return Candidate{}, err
	}
	return parseCrossrefWork(body)
}

func parseCrossrefWork(body []byte) (Candidate, error) {
	var out struct {
		Message crossrefWork `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return Candidate{}, fmt.Errorf("the Crossref response does not parse: %w", err)
	}
	cand := crossrefCandidate(out.Message)
	if cand.Title == "" {
		return Candidate{}, errNotFound
	}
	return cand, nil
}

// CrossrefByTitle searches, bracketed to the year and the one either side of
// it. The bracket is what stops a title search from wandering off into a
// paper with the same name published thirty years later.
func CrossrefByTitle(ctx context.Context, c *Client, title string, year int) ([]Candidate, error) {
	q := url.Values{}
	q.Set("query.bibliographic", title)
	q.Set("rows", "10")
	q.Set("select", "DOI,title,abstract,container-title,author,issued,published,published-print,link,license,URL")
	if year > 0 {
		q.Set("filter", fmt.Sprintf("from-pub-date:%d-01-01,until-pub-date:%d-12-31", year-YearSlack, year+YearSlack))
	}
	body, err := c.Get(ctx, c.withMailto(c.Base.Crossref+"?"+q.Encode()))
	if err != nil {
		return nil, err
	}
	var out struct {
		Message struct {
			Items []crossrefWork `json:"items"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("the Crossref response does not parse: %w", err)
	}
	var cands []Candidate
	for _, w := range out.Message.Items {
		if cand := crossrefCandidate(w); cand.Title != "" {
			cands = append(cands, cand)
		}
	}
	return cands, nil
}

func crossrefCandidate(w crossrefWork) Candidate {
	cand := Candidate{
		DOI:      CleanDOI(w.DOI),
		Source:   "crossref",
		Landing:  w.URL,
		Abstract: stripJATS(w.Abstract),
	}
	if len(w.Title) > 0 {
		cand.Title = collapse(w.Title[0])
	}
	for _, a := range w.Author {
		if name := a.name(); name != "" {
			cand.Authors = append(cand.Authors, name)
		}
	}
	// The print date first, because the manifest records the year the canon
	// lists and that is the year the paper appeared in its venue. "issued" can
	// be the online-first date, which is sometimes a year earlier.
	for _, d := range []crDate{w.PrintDate, w.Published, w.Issued} {
		if y := d.year(); y > 0 {
			cand.Year = y
			break
		}
	}
	for _, l := range w.Link {
		if strings.Contains(l.ContentType, "pdf") && l.Application != "text-mining" {
			cand.URL = l.URL
			break
		}
	}
	// Crossref lists every licence a work has ever had, including the one the
	// publisher applies before an embargo lifts. The most permissive wins,
	// because the question being asked is what may be done today.
	best := corpusUnknownRank
	for _, l := range w.Licence {
		name, access := Licence(l.URL)
		if r := rank(access); r < best {
			best, cand.Licence = r, name
		}
	}
	if cand.Landing == "" && cand.DOI != "" {
		cand.Landing = "https://doi.org/" + cand.DOI
	}
	return cand
}

// CleanDOI strips the prefixes a DOI arrives wrapped in and lowercases it. A
// DOI is case insensitive by specification and comparing two of them by bytes
// without this is how the same paper ends up recorded twice.
func CleanDOI(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			s = s[len(p):]
			break
		}
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "10.") {
		return ""
	}
	return s
}

// stripJATS turns a Crossref abstract into plain text. Crossref stores them as
// JATS XML and a corpus that pasted that straight into a markdown file would
// be publishing tags.
func stripJATS(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	out := collapse(b.String())
	// Almost every JATS abstract opens with the word "Abstract" as its title
	// element, and a heading repeated inside the text it heads is noise.
	return strings.TrimSpace(strings.TrimPrefix(out, "Abstract"))
}
