package sources

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// A handful of the hundred live on sites with a stable URL shape and no API
// worth calling: USENIX, the RFC editor, the Engelbart Institute, bitcoin.org,
// llvm.org, MIT's DSpace. For those, what is missing is never the landing
// page, which Crossref or a search finds easily. What is missing is the PDF,
// and the step from one to the other is a string rewrite the site has not
// changed in fifteen years.
//
// So a publisher rule is that rewrite: landing page in, PDF URL and licence
// out. One function each, named, and a rule that stops working fails the one
// paper that uses it rather than quietly degrading every resolution.

// PublisherRule is one site's rewrite.
type PublisherRule struct {
	// Name is what gets written into reports/resolve.md when this rule is the
	// reason a paper resolved, so that a rule which breaks is findable by the
	// papers that stop resolving.
	Name string
	// Host is the landing page host this rule claims, without a leading www.
	Host string
	// PDF turns a landing page into a PDF URL, or returns "" if this
	// particular landing page is not one it recognises. Returning "" is the
	// right answer far more often than guessing.
	PDF func(u *url.URL) string
	// Licence is what the site publishes under, where the site publishes
	// everything under one thing. An empty licence means ask somebody else.
	Licence string
	Access  corpus.Access
}

// Publishers is the whole set, checked in order.
var Publishers = []PublisherRule{
	{
		Name:    "usenix",
		Host:    "usenix.org",
		PDF:     usenixPDF,
		Licence: "USENIX open access",
		Access:  corpus.AccessOpen,
	},
	{
		Name: "rfc-editor",
		Host: "rfc-editor.org",
		PDF:  rfcPDF,
		// An RFC is not public domain. It is published under the IETF Trust
		// legal provisions, which allow anyone to reproduce and distribute it
		// in full, and allow a translation as long as the notices survive.
		Licence: "IETF Trust legal provisions",
		Access:  corpus.AccessOpen,
	},
	{
		Name:    "ietf-datatracker",
		Host:    "datatracker.ietf.org",
		PDF:     rfcPDF,
		Licence: "IETF Trust legal provisions",
		Access:  corpus.AccessOpen,
	},
	{
		Name:    "engelbart-institute",
		Host:    "dougengelbart.org",
		PDF:     suffixPDF(".html"),
		Licence: "",
		Access:  corpus.AccessUnknown,
	},
	{
		Name: "bitcoin.org",
		Host: "bitcoin.org",
		PDF: func(u *url.URL) string {
			if strings.Contains(u.Path, "bitcoin-paper") || u.Path == "/bitcoin.pdf" {
				return "https://bitcoin.org/bitcoin.pdf"
			}
			return ""
		},
		Licence: "MIT",
		Access:  corpus.AccessOpen,
	},
	{
		Name:    "llvm.org",
		Host:    "llvm.org",
		PDF:     suffixPDF(".html", ".htm"),
		Licence: "",
		Access:  corpus.AccessUnknown,
	},
	{
		Name:    "acm",
		Host:    "dl.acm.org",
		PDF:     acmPDF,
		Licence: "",
		Access:  corpus.AccessUnknown,
	},
	{
		Name:    "springer",
		Host:    "link.springer.com",
		PDF:     springerPDF,
		Licence: "",
		Access:  corpus.AccessUnknown,
	},
}

// Publisher applies the first rule that claims the landing page's host and
// recognises its shape.
//
// It fills in the URL, and the licence only where the site really does
// publish everything under one thing. Where it does not, the licence is left
// empty and stays whatever the rest of the ladder found, because a URL shape
// is evidence about where a file is and no evidence at all about what may be
// done with it.
func Publisher(landing string) (Candidate, bool) {
	u, err := url.Parse(landing)
	if err != nil || u.Host == "" {
		return Candidate{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	for _, r := range Publishers {
		if host != r.Host {
			continue
		}
		pdf := r.PDF(u)
		if pdf == "" {
			continue
		}
		return Candidate{
			URL:     pdf,
			Landing: landing,
			Licence: r.Licence,
			Source:  "publisher:" + r.Name,
		}, true
	}
	return Candidate{}, false
}

// PublisherAccess is what a rule says about a site that publishes everything
// under one licence. It is separate from Publisher because most rules say
// nothing, and "nothing" must not overwrite what Unpaywall found.
func PublisherAccess(name string) (corpus.Access, bool) {
	name = strings.TrimPrefix(name, "publisher:")
	for _, r := range Publishers {
		if r.Name == name && r.Licence != "" {
			return r.Access, true
		}
	}
	return corpus.AccessUnknown, false
}

// publisherLicence is the licence string that goes with PublisherAccess.
func publisherLicence(name string) string {
	name = strings.TrimPrefix(name, "publisher:")
	for _, r := range Publishers {
		if r.Name == name {
			return r.Licence
		}
	}
	return ""
}

// usenixPDF turns a USENIX presentation page into the paper.
//
// USENIX serve the PDF from a different path than the page describing it, and
// the slug is the last element of the presentation URL. The conference part
// of the file path does not match the conference part of the page path for
// every year, so this asks for the redirect endpoint rather than guessing the
// file path.
func usenixPDF(u *url.URL) string {
	path := strings.TrimSuffix(u.Path, "/")
	if strings.HasSuffix(path, ".pdf") {
		return u.String()
	}
	i := strings.Index(path, "/presentation/")
	if i < 0 {
		return ""
	}
	return "https://www.usenix.org" + path + "/download"
}

var rfcNumber = regexp.MustCompile(`rfc(\d+)`)

// rfcPDF finds the RFC number in a path and asks the RFC editor for it
// directly, whichever of the two sites the landing page was on.
func rfcPDF(u *url.URL) string {
	m := rfcNumber.FindStringSubmatch(strings.ToLower(u.Path))
	if m == nil {
		return ""
	}
	return "https://www.rfc-editor.org/rfc/rfc" + m[1] + ".pdf"
}

// acmPDF turns an ACM landing page into the PDF endpoint.
//
// Whether that endpoint returns a PDF depends entirely on whether the paper
// is in ACM's open access programme, and this rule does not know. The fetcher
// checks the bytes and refuses anything that is not a PDF, which is what
// stops an ACM interstitial being committed as a paper.
func acmPDF(u *url.URL) string {
	path := strings.TrimSuffix(u.Path, "/")
	switch {
	case strings.HasPrefix(path, "/doi/pdf/"):
		return u.String()
	case strings.HasPrefix(path, "/doi/abs/"):
		return "https://dl.acm.org/doi/pdf/" + strings.TrimPrefix(path, "/doi/abs/")
	case strings.HasPrefix(path, "/doi/"):
		return "https://dl.acm.org/doi/pdf/" + strings.TrimPrefix(path, "/doi/")
	}
	return ""
}

// springerPDF turns a Springer article page into the content PDF.
func springerPDF(u *url.URL) string {
	path := strings.TrimSuffix(u.Path, "/")
	for _, p := range []string{"/article/", "/chapter/"} {
		if strings.HasPrefix(path, p) {
			return "https://link.springer.com/content/pdf/" + strings.TrimPrefix(path, p) + ".pdf"
		}
	}
	return ""
}

// suffixPDF is the rule for a site that puts the PDF beside the page, under
// the same name. It is the oldest URL shape on the web and several of these
// papers are still exactly where they were put in 1998.
func suffixPDF(suffixes ...string) func(*url.URL) string {
	return func(u *url.URL) string {
		path := u.Path
		if strings.HasSuffix(path, ".pdf") {
			return u.String()
		}
		for _, s := range suffixes {
			if strings.HasSuffix(path, s) {
				out := *u
				out.Path = strings.TrimSuffix(path, s) + ".pdf"
				out.RawQuery, out.Fragment = "", ""
				return out.String()
			}
		}
		return ""
	}
}
