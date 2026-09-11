package corpus

import (
	"fmt"
	"os"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// Papers is manifests/papers.yaml, the list of what the corpus is for.
//
// There is deliberately no Save. The file carries a header and per group
// comments that yaml.v3 does not round trip, and re-marshalling it would throw
// them away silently. When `papers add` arrives it edits the file as a
// yaml.Node so the comments survive.
type Papers struct {
	Papers []Paper `yaml:"papers"`
}

// Paper is one entry in the manifest. It is what the corpus knows about a
// paper before anything has been fetched.
type Paper struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Authors []string `yaml:"authors"`
	// AuthorsPartial marks a seed entry whose author list was cut short by
	// hand. The resolver replaces the list with the published one and clears
	// this, so a true here means nobody has resolved the paper yet.
	AuthorsPartial bool   `yaml:"authors_partial,omitempty"`
	Year           int    `yaml:"year"`
	Venue          string `yaml:"venue"`
	Field          Field  `yaml:"field"`
	// Number is the position in the seed list of a hundred, and is absent on
	// every paper added afterwards. That is how the hundred stay a hundred
	// while the corpus grows.
	Number int      `yaml:"number,omitempty"`
	AKA    []string `yaml:"aka,omitempty"`
	// ArXiv, DOI and URL are pinned identifiers. A pinned identifier beats
	// anything an API says, because a person who has looked knows more than a
	// search index.
	ArXiv         string   `yaml:"arxiv,omitempty"`
	DOI           string   `yaml:"doi,omitempty"`
	URL           string   `yaml:"url,omitempty"`
	Difficulty    int      `yaml:"difficulty,omitempty"`
	Prerequisites []string `yaml:"prerequisites,omitempty"`
	CoreIdea      string   `yaml:"core_idea,omitempty"`
	// Expect is a guess at the access class made before resolution. Nothing is
	// fetched on the strength of it; the fact lives in sources.yaml.
	Expect Access `yaml:"expect,omitempty"`
	Status Status `yaml:"status"`
}

// LoadPapers reads manifests/papers.yaml.
func LoadPapers(path string) (*Papers, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Papers
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Papers reads the corpus manifest.
func (c *Corpus) LoadPapers() (*Papers, error) { return LoadPapers(c.PapersManifest()) }

// ByID finds one paper.
func (m *Papers) ByID(id string) (*Paper, bool) {
	for i := range m.Papers {
		if m.Papers[i].ID == id {
			return &m.Papers[i], true
		}
	}
	return nil, false
}

// IDs is every paper id in manifest order.
func (m *Papers) IDs() []string {
	ids := make([]string, 0, len(m.Papers))
	for _, p := range m.Papers {
		ids = append(ids, p.ID)
	}
	return ids
}

// Field is every paper in one field, in the order they will be listed:
// numbered papers first by number, then everything added later by year.
func (m *Papers) Field(f Field) []Paper {
	var out []Paper
	for _, p := range m.Papers {
		if p.Field == f {
			out = append(out, p)
		}
	}
	sortPapers(out)
	return out
}

// Numbered is the seed list, in its own numbering.
func (m *Papers) Numbered() []Paper {
	var out []Paper
	for _, p := range m.Papers {
		if p.Number > 0 {
			out = append(out, p)
		}
	}
	sortPapers(out)
	return out
}

// Counts is how many papers each field holds, for the catalogue and for
// coverage.md.
func (m *Papers) Counts() map[Field]int {
	counts := make(map[Field]int, len(Fields))
	for _, p := range m.Papers {
		counts[p.Field]++
	}
	return counts
}

// sortPapers puts the numbered papers first in their own order and the rest
// after them oldest first, so a listing is stable whatever order the file is
// edited into.
func sortPapers(ps []Paper) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		switch {
		case a.Number > 0 && b.Number > 0:
			return a.Number < b.Number
		case a.Number > 0:
			return true
		case b.Number > 0:
			return false
		case a.Year != b.Year:
			return a.Year < b.Year
		}
		return a.ID < b.ID
	})
}

// ReadingOrder sorts the given papers so that no paper comes before one it
// lists as a prerequisite, keeping the input order otherwise.
//
// A prerequisite that is not in the corpus is skipped rather than treated as
// an error: the manifest records those as free text on purpose, because "you
// should read Bahdanau first and we have not got it" is worth writing down.
func (m *Papers) ReadingOrder(ids []string) ([]string, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	const (
		fresh = iota
		open
		done
	)
	state := make(map[string]int, len(ids))
	var order []string
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case done:
			return nil
		case open:
			return fmt.Errorf("prerequisites form a cycle: %s", cycle(path, id))
		}
		state[id] = open
		if p, ok := m.ByID(id); ok {
			for _, req := range p.Prerequisites {
				if !want[req] {
					continue
				}
				if err := visit(req, append(path, id)); err != nil {
					return err
				}
			}
		}
		state[id] = done
		order = append(order, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// cycle prints the loop starting from where it closes, which is the part
// somebody has to edit.
func cycle(path []string, id string) string {
	if i := slices.Index(path, id); i >= 0 {
		path = path[i:]
	}
	out := ""
	for _, p := range path {
		out += p + " -> "
	}
	return out + id
}
