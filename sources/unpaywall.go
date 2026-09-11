package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Unpaywall is the rung that answers the question the others cannot: not
// "where was this published" but "is there a copy anybody may fetch". It
// indexes repository deposits, publisher open access and author pages, and it
// is the only one of these that reliably knows an ACM DOI is free.

type unpaywallWork struct {
	DOI      string        `json:"doi"`
	Title    string        `json:"title"`
	Year     int           `json:"year"`
	IsOA     bool          `json:"is_oa"`
	Authors  []crAuthor    `json:"z_authors"`
	Best     *unpaywallOA  `json:"best_oa_location"`
	Location []unpaywallOA `json:"oa_locations"`
}

type unpaywallOA struct {
	URLForPDF     string `json:"url_for_pdf"`
	URLForLanding string `json:"url_for_landing_page"`
	Licence       string `json:"license"`
	HostType      string `json:"host_type"`
	Version       string `json:"version"`
}

// Unpaywall asks about one DOI.
//
// Unpaywall requires an email and refuses the request without one, which is
// the only place on the ladder where the politeness parameter is not optional.
func Unpaywall(ctx context.Context, c *Client, doi string) (Candidate, error) {
	doi = CleanDOI(doi)
	if doi == "" {
		return Candidate{}, errNotFound
	}
	if c.Mailto == "" {
		return Candidate{}, fmt.Errorf("Unpaywall needs an email address and none is configured")
	}
	raw := c.Base.Unpaywall + "/" + url.PathEscape(doi) + "?email=" + url.QueryEscape(c.Mailto)
	body, err := c.Get(ctx, raw)
	if err != nil {
		return Candidate{}, err
	}
	return parseUnpaywall(body)
}

func parseUnpaywall(body []byte) (Candidate, error) {
	var w unpaywallWork
	if err := json.Unmarshal(body, &w); err != nil {
		return Candidate{}, fmt.Errorf("the Unpaywall response does not parse: %w", err)
	}

	cand := Candidate{
		Title:  collapse(w.Title),
		Year:   w.Year,
		DOI:    CleanDOI(w.DOI),
		Source: "unpaywall",
	}
	for _, a := range w.Authors {
		switch {
		case a.Family != "" && a.Given != "":
			cand.Authors = append(cand.Authors, a.Given+" "+a.Family)
		case a.Family != "":
			cand.Authors = append(cand.Authors, a.Family)
		case a.Name != "":
			cand.Authors = append(cand.Authors, a.Name)
		}
	}
	if best := pickOA(w); best != nil {
		cand.URL = best.URLForPDF
		cand.Landing = best.URLForLanding
		cand.Licence = best.Licence
		if a := CleanArXivID(best.URLForPDF); strings.Contains(best.URLForPDF, "arxiv.org") && a != "" {
			cand.ArXiv = a
		}
	}
	if cand.Title == "" && cand.URL == "" {
		return Candidate{}, errNotFound
	}
	return cand, nil
}

// pickOA chooses which open copy to use.
//
// Unpaywall's own best_oa_location is preferred, and it is right nearly
// always. Where it is not, it is because the best copy by its ranking has no
// PDF link, only a landing page, and a landing page cannot be extracted. So
// the fallback walks the other locations for one that does, preferring the
// published version over an accepted manuscript, because the corpus records
// page numbers and an author manuscript has different ones.
func pickOA(w unpaywallWork) *unpaywallOA {
	if w.Best != nil && w.Best.URLForPDF != "" {
		return w.Best
	}
	var accepted *unpaywallOA
	for i := range w.Location {
		loc := &w.Location[i]
		if loc.URLForPDF == "" {
			continue
		}
		if loc.Version == "publishedVersion" {
			return loc
		}
		if accepted == nil {
			accepted = loc
		}
	}
	if accepted != nil {
		return accepted
	}
	return w.Best
}
