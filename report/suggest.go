package report

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/refs"
)

// A Suggestion is a paper the corpus does not hold that several papers it
// does hold cite.
//
// It is the reading list, and it is the only part of growing the corpus that
// the corpus can do for itself. Everything else about adding a paper is a
// judgement: whether it is canonical, whether it is readable, whether the
// licence allows any of this. Which papers the ones already here keep
// pointing at is a fact, and it is sitting in the bibliographies.
type Suggestion struct {
	// Title is the entry's title as the citing papers printed it, taken from
	// the longest of them. The longest because a bibliography that ran out of
	// room truncates, and one of the two readings is a prefix of the other.
	Title string
	// Authors and Year are from the first entry that had them. A reference
	// list prints one author and "et al." as often as not, so these are a
	// hint for a person deciding whether to add the paper and not something
	// to write into the manifest.
	Authors []string
	Year    int
	// DOI and ArXiv are whatever any of the citing entries carried, and are
	// what papers add wants. Most references carry neither.
	DOI   string
	ArXiv string
	// By is the papers of the corpus that cite it, in id order.
	By []string
}

// Command is the line to run to act on a suggestion, and the empty string
// where there is nothing to run: a reference with no identifier in it cannot
// be added by identifier, and telling somebody to run papers add on a title
// would be telling them to run something that does not work.
func (s Suggestion) Command() string {
	switch {
	case s.ArXiv != "":
		return "papers add -arxiv " + s.ArXiv
	case s.DOI != "":
		return "papers add -doi " + s.DOI
	}
	return ""
}

// Suggest is every paper cited by at least min papers of the corpus and not
// held by it, most cited first.
//
// Entries are grouped by title and then the groups that share an identifier
// are merged, in that order and not the other way round. Doing it by
// identifier alone loses the great majority of references, which carry none;
// doing it by title alone splits a paper whose title two bibliographies
// printed differently, and one of the two commonest ways to cite the RSA
// paper drops the subtitle.
func Suggest(c *corpus.Corpus, min int) ([]Suggestion, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	held := holdingsOf(papers)

	groups := map[string]*Suggestion{}
	for _, p := range papers.Papers {
		m, err := refs.Load(c.Refs(p.ID))
		if os.IsNotExist(err) {
			// A paper whose bibliography nobody has read yet cites nothing
			// here. That is the graph's coverage problem and not this one.
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range m.Entries {
			if e.ResolvesTo != "" || held.has(e) {
				continue
			}
			key := normalTitle(e.Title)
			if key == "" {
				// A reference the parser could not find a title in is a
				// reference nobody could act on, whoever cites it.
				continue
			}
			g := groups[key]
			if g == nil {
				g = &Suggestion{}
				groups[key] = g
			}
			g.absorb(e, p.ID)
		}
	}

	var out []Suggestion
	for _, g := range merged(groups) {
		g.By = unique(g.By)
		if len(g.By) < min {
			continue
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].By) != len(out[j].By) {
			return len(out[i].By) > len(out[j].By)
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

// absorb folds one bibliography entry into the group.
func (s *Suggestion) absorb(e refs.Entry, by string) {
	if len(e.Title) > len(s.Title) {
		s.Title = strings.TrimSpace(e.Title)
	}
	if s.Year == 0 {
		s.Year = e.Year
	}
	if len(s.Authors) == 0 {
		s.Authors = e.Authors
	}
	if s.DOI == "" {
		s.DOI = e.DOI
	}
	if s.ArXiv == "" {
		s.ArXiv = e.ArXiv
	}
	s.By = append(s.By, by)
}

// merged is the groups with the ones that share an identifier joined up.
func merged(groups map[string]*Suggestion) []*Suggestion {
	byKey := map[string]*Suggestion{}
	var out []*Suggestion
	for _, key := range groupKeys(groups) {
		g := groups[key]
		var into *Suggestion
		for _, k := range []string{"arxiv:" + g.ArXiv, "doi:" + strings.ToLower(g.DOI)} {
			if strings.HasSuffix(k, ":") {
				continue
			}
			if had := byKey[k]; had != nil {
				into = had
				break
			}
		}
		if into == nil {
			out = append(out, g)
			into = g
		} else {
			if len(g.Title) > len(into.Title) {
				into.Title = g.Title
			}
			into.By = append(into.By, g.By...)
		}
		if into.ArXiv != "" {
			byKey["arxiv:"+into.ArXiv] = into
		}
		if into.DOI != "" {
			byKey["doi:"+strings.ToLower(into.DOI)] = into
		}
	}
	return out
}

// groupKeys is the keys of a group table in a fixed order, so that two runs
// over the same corpus merge the same way round and produce the same file.
func groupKeys(groups map[string]*Suggestion) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// holdings is what the corpus already has, by every handle a reference might
// name it with.
type holdings struct {
	titles map[string]bool
	arxiv  map[string]bool
	doi    map[string]bool
}

func (h holdings) has(e refs.Entry) bool {
	switch {
	case e.ArXiv != "" && h.arxiv[strings.ToLower(e.ArXiv)]:
		return true
	case e.DOI != "" && h.doi[strings.ToLower(e.DOI)]:
		return true
	}
	return h.titles[normalTitle(e.Title)]
}

// holdingsOf reads the manifest rather than the resolved edges, so that a paper
// the corpus holds and whose citation the resolver did not match is still not
// suggested. The resolver missing an edge is rule R01's problem; suggesting
// that somebody add a paper that is three lines further up the same manifest
// is this one's.
func holdingsOf(m *corpus.Papers) holdings {
	h := holdings{titles: map[string]bool{}, arxiv: map[string]bool{}, doi: map[string]bool{}}
	for _, p := range m.Papers {
		if t := normalTitle(p.Title); t != "" {
			h.titles[t] = true
		}
		for _, a := range p.AKA {
			if t := normalTitle(a); t != "" {
				h.titles[t] = true
			}
		}
		if p.ArXiv != "" {
			h.arxiv[strings.ToLower(p.ArXiv)] = true
		}
		if p.DOI != "" {
			h.doi[strings.ToLower(p.DOI)] = true
		}
	}
	return h
}

// normalTitle is a title reduced to the letters and digits in it, lowercased.
//
// Everything else goes, because the same title comes out of two
// bibliographies with different punctuation more often than not: a colon
// where the other has a comma, a hyphen where the other has an en dash, and
// the ACM style capitalises words the IEEE style does not.
func normalTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SuggestMarkdown is the reading list as reports/suggest.md.
func SuggestMarkdown(all []Suggestion, min int) string {
	var b strings.Builder
	b.WriteString("# Worth adding\n\n")
	if len(all) == 0 {
		fmt.Fprintf(&b, "Nothing outside the corpus is cited by %s of its papers.\n", atLeast(min))
		return b.String()
	}
	fmt.Fprintf(&b, "%d works outside the corpus are cited by %s of its papers.\n\n", len(all), atLeast(min))
	b.WriteString("A work here is a candidate and not a decision. It says the papers already in the corpus keep pointing at it, which is a fact, and it says nothing about whether the paper is readable, whether it is the canonical version, or whether its licence allows any of what happens next.\n\n")
	for _, s := range all {
		fmt.Fprintf(&b, "## %s\n\n", heading(s))
		fmt.Fprintf(&b, "Cited by %d: %s.\n\n", len(s.By), strings.Join(s.By, ", "))
		if cmd := s.Command(); cmd != "" {
			fmt.Fprintf(&b, "    %s\n\n", cmd)
		} else {
			b.WriteString("No identifier in any of the references, so it has to be looked up by hand.\n\n")
		}
	}
	return b.String()
}

// heading is the heading for one suggestion: the title, and the year where a
// reference gave one.
func heading(s Suggestion) string {
	if s.Year > 0 {
		return fmt.Sprintf("%s (%d)", s.Title, s.Year)
	}
	return s.Title
}

// atLeast is "at least three" written the way a sentence wants it, and "two
// or more" is the same thing and reads worse in the middle of a line.
func atLeast(min int) string {
	if min == 1 {
		return "any"
	}
	return fmt.Sprintf("at least %d", min)
}
