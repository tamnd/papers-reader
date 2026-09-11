package refs

import (
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/sources"
)

// Resolve fills in resolves_to for every entry that names a paper the corpus
// already has, and returns how many it placed.
//
// Resolution runs against papers.yaml and never against a service, so it is
// offline, deterministic and cheap enough to re-run on every build. That
// matters more than it sounds: a paper added today retroactively resolves
// references in papers extracted last month, and nothing has to be
// re-extracted for it to happen.
//
// The bar is the one the acquisition resolver uses, three predicates at
// once, and it is deliberately high. A wrong edge in the citation graph is
// not a missing edge, it is a claim that one paper cited another, and the
// graph is the thing a reader is most likely to take on trust.
func Resolve(entries []Entry, papers []corpus.Paper, self string) int {
	n := 0
	for i := range entries {
		id := match(entries[i], papers, self)
		entries[i].ResolvesTo = id
		if id != "" {
			n++
		}
	}
	return n
}

// match finds the corpus paper one reference names.
func match(e Entry, papers []corpus.Paper, self string) string {
	// An identifier is not a guess. If the reference prints the DOI or the
	// arXiv id of a paper in the corpus then it is that paper, whatever the
	// title parse made of the rest of the line, and this catches the entries
	// whose authors the parser could not read.
	if id := byIdentifier(e, papers, self); id != "" {
		return id
	}
	best, score := "", 0.0
	for _, p := range papers {
		if p.ID == self {
			// Rule R05. An entry in a paper's own bibliography is never that
			// paper, and a self loop in the citation graph would break the
			// one data-quality check the graph exists to support.
			continue
		}
		want := sources.Want{Title: p.Title, Authors: p.Authors, Year: p.Year}
		got := sources.Candidate{Title: e.Title, Authors: e.Authors, Year: e.Year}
		v := sources.Verify(want, got)
		if v.OK && v.TitleScore > score {
			best, score = p.ID, v.TitleScore
		}
	}
	return best
}

func byIdentifier(e Entry, papers []corpus.Paper, self string) string {
	for _, p := range papers {
		if p.ID == self {
			continue
		}
		if e.ArXiv != "" && sameArXiv(e.ArXiv, p.ArXiv) {
			return p.ID
		}
		if e.DOI != "" && p.DOI != "" && strings.EqualFold(bareDOI(e.DOI), bareDOI(p.DOI)) {
			return p.ID
		}
	}
	return ""
}

// sameArXiv compares two arXiv identifiers, ignoring the version. A
// reference to v1 and a manifest entry for v3 are the same paper, and the
// corpus has one record per paper and not one per revision.
func sameArXiv(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(bareArXiv(a), bareArXiv(b))
}

func bareArXiv(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "arXiv:"), "arxiv:")
	if i := strings.LastIndex(s, "v"); i > 0 {
		if rest := s[i+1:]; rest != "" && isDigits(rest) {
			s = s[:i]
		}
	}
	return s
}

// bareDOI drops the resolver prefix a reference often prints in front of a
// DOI, so that doi.org/10.1145/359340.359342 and 10.1145/359340.359342 are
// the same identifier.
func bareDOI(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi.org/", "doi:", "DOI:"} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strings.TrimRight(s, ".")
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
