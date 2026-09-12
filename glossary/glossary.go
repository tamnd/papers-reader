package glossary

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tamnd/papers-reader/corpus"
)

// A Term is one entry of the controlled vocabulary: one English term and one
// rendering per language.
//
// Keep says the English stands as it is in that language, which is the
// honest answer for a name like Transformer or MapReduce and not a gap in
// the table. A term with Keep set and no rendering is finished, and the
// coverage count says so.
type Term struct {
	En   string `yaml:"en"`
	Vi   string `yaml:"vi,omitempty"`
	Zh   string `yaml:"zh,omitempty"`
	Ja   string `yaml:"ja,omitempty"`
	Keep bool   `yaml:"keep,omitempty"`
	// Field scopes a term to one field of the corpus. A term with no field
	// is offered to every paper.
	//
	// Bourbaki is one subject and its glossary is global. This corpus is
	// twelve fields, and "reduction" means one thing in complexity theory
	// and another in compilers, "cache" one thing in architecture and
	// another in networks, and "model" four things. A rendering offered to
	// the wrong paper is worse than no rendering, because the translator
	// will take it.
	Field corpus.Field `yaml:"field,omitempty"`
	Note  string       `yaml:"note,omitempty"`
}

// Rendering is the term in one language, and false where there is none yet.
//
// A kept term renders as its English in every language, so that a caller
// building a prompt does not have to know about Keep.
func (t Term) Rendering(l corpus.Lang) (string, bool) {
	if t.Keep {
		return t.En, true
	}
	var s string
	switch l {
	case corpus.VI:
		s = t.Vi
	case corpus.ZH:
		s = t.Zh
	case corpus.JA:
		s = t.Ja
	case corpus.EN:
		s = t.En
	}
	s = strings.TrimSpace(s)
	return s, s != ""
}

// A Glossary is manifests/glossary.yaml.
//
// Version goes up by one whenever a term is added or a rendering changes.
// Every translated file records the version it was made under, so a change
// here makes the files it affects detectably stale rather than quietly
// inconsistent with the ones translated after it.
type Glossary struct {
	Version int    `yaml:"version"`
	Terms   []Term `yaml:"terms"`
}

// Load reads a glossary. A file that is not there is an empty glossary at
// version zero, which is what a corpus looks like before anybody has built
// one and is not an error to ask about.
func Load(path string) (*Glossary, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Glossary{}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Glossary
	if err := yaml.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &g, nil
}

// For is the terms offered to a paper in one field: the global terms and
// that field's, longest first.
//
// Longest first because a prompt is read in order and "hash table" has to be
// decided before "table" is, or the translator will render the two words of
// the phrase separately and the phrase will come out as neither.
func (g *Glossary) For(f corpus.Field) []Term {
	var out []Term
	for _, t := range g.Terms {
		if t.Field == "" || t.Field == f {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := len([]rune(out[i].En)), len([]rune(out[j].En))
		if li != lj {
			return li > lj
		}
		return out[i].En < out[j].En
	})
	return out
}

// Coverage is how many terms have a rendering in a language, out of how
// many there are.
//
// The translate command refuses to run for a language whose coverage is
// under a floor, because a body translated against a third of a glossary is
// a body that will have to be translated again.
func (g *Glossary) Coverage(l corpus.Lang) (have, total int) {
	for _, t := range g.Terms {
		total++
		if _, ok := t.Rendering(l); ok {
			have++
		}
	}
	return have, total
}

// Missing is the terms with no rendering in a language, in the order they
// sit in the file, which is the order somebody filling them in wants.
func (g *Glossary) Missing(l corpus.Lang) []Term {
	var out []Term
	for _, t := range g.Terms {
		if _, ok := t.Rendering(l); !ok {
			out = append(out, t)
		}
	}
	return out
}

// Has says whether a term is already in the glossary, matched the way the
// extractor counts: casefolded, on the English.
func (g *Glossary) Has(en string) bool {
	en = strings.ToLower(strings.TrimSpace(en))
	for _, t := range g.Terms {
		if strings.ToLower(strings.TrimSpace(t.En)) == en {
			return true
		}
	}
	return false
}
