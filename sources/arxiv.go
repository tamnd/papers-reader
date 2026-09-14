package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// arXiv is the first service on the ladder because it is the only one that is
// unambiguous. An arXiv id names one submission, the metadata is the author's
// own, and the licence is stated per submission rather than per journal.

// arxivFeed is as much of the Atom response as this needs.
type arxivFeed struct {
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	ID        string `xml:"id"`
	Title     string `xml:"title"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Summary   string `xml:"summary"`
	DOI       string `xml:"doi"`
	Authors   []struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Links []struct {
		Href  string `xml:"href,attr"`
		Rel   string `xml:"rel,attr"`
		Type  string `xml:"type,attr"`
		Title string `xml:"title,attr"`
	} `xml:"link"`
	// Primary is the one category the author picked out of however many they
	// cross listed under, and it is the only one worth reading. A transformer
	// paper is cross listed under cs.CL, cs.LG and stat.ML and the author
	// chose the first, which is the closest thing to a person saying what
	// field this is.
	Primary struct {
		Term string `xml:"term,attr"`
	} `xml:"primary_category"`
}

// ArXivByID looks a submission up by its identifier.
func ArXivByID(ctx context.Context, c *Client, id string) (Candidate, error) {
	id = CleanArXivID(id)
	if id == "" {
		return Candidate{}, errNotFound
	}
	raw := c.Base.ArXiv + "?id_list=" + url.QueryEscape(id) + "&max_results=1"
	body, err := c.Get(ctx, raw)
	if err != nil {
		return Candidate{}, err
	}
	entries, err := parseArXiv(body)
	if err != nil {
		return Candidate{}, err
	}
	if len(entries) == 0 {
		return Candidate{}, errNotFound
	}
	return entries[0], nil
}

// ArXivByTitle searches by title. Everything it returns still has to survive
// Verify, because a title search is a suggestion and not an answer.
func ArXivByTitle(ctx context.Context, c *Client, title string) ([]Candidate, error) {
	q := `ti:"` + strings.ReplaceAll(title, `"`, "") + `"`
	raw := c.Base.ArXiv + "?search_query=" + url.QueryEscape(q) + "&max_results=10"
	body, err := c.Get(ctx, raw)
	if err != nil {
		return nil, err
	}
	return parseArXiv(body)
}

func parseArXiv(body []byte) ([]Candidate, error) {
	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("the arXiv response does not parse: %w", err)
	}
	var out []Candidate
	for _, e := range feed.Entries {
		id := CleanArXivID(e.ID)
		if id == "" {
			continue
		}
		cand := Candidate{
			Title:  collapse(e.Title),
			Year:   yearOf(e.Published),
			DOI:    e.DOI,
			Source: "arxiv",
			ArXiv:  id,
			// The abstract page rather than the PDF, because that is the page
			// a reader should be sent to and the PDF is derived from it.
			Landing:  "https://arxiv.org/abs/" + id,
			URL:      "https://arxiv.org/pdf/" + id,
			Abstract: collapse(e.Summary),
			Category: strings.TrimSpace(e.Primary.Term),
			// A preprint's venue is arXiv until somebody knows better. The
			// journal reference is in the Atom feed for maybe a third of
			// submissions and is free text when it is there, so it is not
			// something to write into a manifest unread.
			Venue: "arXiv",
		}
		for _, a := range e.Authors {
			cand.Authors = append(cand.Authors, collapse(a.Name))
		}
		for _, l := range e.Links {
			if l.Title == "pdf" && l.Href != "" {
				cand.URL = strings.Replace(l.Href, "http://", "https://", 1)
			}
		}
		out = append(out, cand)
	}
	return out, nil
}

// arxivOAIRecord is the licence, which the Atom API does not carry.
type arxivOAIRecord struct {
	Licence string `xml:"GetRecord>record>metadata>arXiv>license"`
	Error   string `xml:"error"`
}

// ArXivLicence asks the OAI-PMH interface what licence a submission carries.
//
// A submission with no licence element has not chosen one, and arXiv's
// deposit agreement then gives arXiv a non-exclusive licence to distribute it.
// That is a real, documented permission and it is what this returns, with a
// note saying it was the default rather than the author's choice, because the
// two are not the same claim and the corpus should not print them the same
// way.
func ArXivLicence(ctx context.Context, c *Client, id string) (licence string, stated bool, err error) {
	id = CleanArXivID(id)
	if id == "" {
		return "", false, errNotFound
	}
	raw := c.Base.ArXivOAI + "?verb=GetRecord&metadataPrefix=arXiv&identifier=" + url.QueryEscape("oai:arXiv.org:"+id)
	body, err := c.Get(ctx, raw)
	if err != nil {
		return "", false, err
	}
	return parseOAILicence(body)
}

func parseOAILicence(body []byte) (licence string, stated bool, err error) {
	var rec arxivOAIRecord
	if err := xml.Unmarshal(body, &rec); err != nil {
		return "", false, fmt.Errorf("the arXiv OAI response does not parse: %w", err)
	}
	if rec.Error != "" {
		return "", false, errNotFound
	}
	if l := strings.TrimSpace(rec.Licence); l != "" {
		return l, true, nil
	}
	return "http://arxiv.org/licenses/nonexclusive-distrib/1.0/", false, nil
}

// arxivFields is the arXiv primary category a paper was filed under and the
// group of the manifest it belongs in.
//
// The two taxonomies are not the same shape and this is not an attempt to
// make them one. arXiv has no category for a database paper and files them
// all under cs.DB alongside data management, and it has one category for
// graphics and none for interaction, so cs.HC has to carry both. What this
// table is for is saving a person from typing -field on the two thirds of
// submissions where there is only one sensible answer, and leaving them to
// say for the rest. A category not listed here is not a mistake, it is a
// category where arXiv's answer and the corpus's answer come apart often
// enough that guessing would be worse than asking.
var arxivFields = map[string]corpus.Field{
	"cs.CC":   corpus.Theory,
	"cs.LO":   corpus.Theory,
	"cs.FL":   corpus.Theory,
	"math.LO": corpus.Theory,
	"cs.DS":   corpus.Algorithms,
	"cs.PL":   corpus.Languages,
	"cs.OS":   corpus.Systems,
	"cs.DC":   corpus.Systems,
	"cs.NI":   corpus.Networks,
	"cs.DB":   corpus.Databases,
	"cs.AR":   corpus.Architecture,
	"cs.CR":   corpus.Security,
	"cs.LG":   corpus.AIML,
	"cs.CL":   corpus.AIML,
	"cs.CV":   corpus.AIML,
	"cs.AI":   corpus.AIML,
	"cs.NE":   corpus.AIML,
	"stat.ML": corpus.AIML,
	"cs.GR":   corpus.Graphics,
	"cs.HC":   corpus.HCI,
	"cs.SE":   corpus.Software,
}

// FieldOf is the manifest group an arXiv category belongs in, and false where
// the corpus would rather be told than guess. See arxivFields.
func FieldOf(category string) (corpus.Field, bool) {
	f, ok := arxivFields[strings.TrimSpace(category)]
	return f, ok
}

// CleanArXivID pulls the bare identifier out of whatever form it arrived in:
// a URL, an "arXiv:" prefix, a version suffix, or the identifier itself.
//
// The version goes. A version is a different file and the corpus pins a file
// by its hash rather than by its version number, so recording v7 here would
// be recording something that the hash already says better.
func CleanArXivID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "/abs/"); i >= 0 {
		s = s[i+len("/abs/"):]
	} else if i := strings.LastIndex(s, "/pdf/"); i >= 0 {
		s = s[i+len("/pdf/"):]
	}
	s = strings.TrimSuffix(s, ".pdf")
	s = strings.TrimPrefix(strings.ToLower(s), "arxiv:")
	// An old style id is a category and a number, as in cs/9901001, and the
	// slash is part of it. A new style id is digits, a dot and digits.
	if i := strings.LastIndexByte(s, 'v'); i > 0 {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			s = s[:i]
		}
	}
	return s
}

// collapse turns the whitespace of an XML text node into single spaces. Atom
// wraps titles across lines and a title with a newline in it fails every
// comparison it is put through.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// yearOf reads the year off an ISO date.
func yearOf(s string) int {
	if len(s) < 4 {
		return 0
	}
	n, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0
	}
	return n
}
