package corpus

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Collections is manifests/collections.yaml, the named reading lists over the
// corpus. They are the reading paths written down as data instead of as prose
// nobody can follow.
type Collections struct {
	Collections []Collection `yaml:"collections"`
}

// Collection is one reading list.
type Collection struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	// Order is how the members are presented: number, manual, year or
	// topological. A topological order sorts by prerequisites and fails on a
	// cycle, which is a real possibility once prerequisites are edited by hand.
	Order string `yaml:"order,omitempty"`
	// Members is either a list of paper ids or one of the two words "all" and
	// "all-with-number", so a collection over everything does not have to be
	// re-edited every time a paper is added.
	Members Members `yaml:"members"`
}

// Members is a list of paper ids or a word standing for one.
type Members struct {
	All        bool
	WithNumber bool
	IDs        []string
}

// UnmarshalYAML accepts either the word or the list.
func (m *Members) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		switch n.Value {
		case "all":
			m.All = true
		case "all-with-number":
			m.All, m.WithNumber = true, true
		default:
			return fmt.Errorf("line %d: members is %q, want a list, all, or all-with-number", n.Line, n.Value)
		}
		return nil
	}
	return n.Decode(&m.IDs)
}

// MarshalYAML writes back what was read.
func (m Members) MarshalYAML() (any, error) {
	switch {
	case m.WithNumber:
		return "all-with-number", nil
	case m.All:
		return "all", nil
	}
	return m.IDs, nil
}

// LoadCollections reads manifests/collections.yaml.
func LoadCollections(path string) (*Collections, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Collections
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// LoadCollections reads the reading lists of this corpus.
func (c *Corpus) LoadCollections() (*Collections, error) {
	return LoadCollections(c.CollectionsManifest())
}

// ByID finds one collection.
func (c *Collections) ByID(id string) (*Collection, bool) {
	for i := range c.Collections {
		if c.Collections[i].ID == id {
			return &c.Collections[i], true
		}
	}
	return nil, false
}

// Resolve turns a collection into the list of paper ids it names, in the
// order it asks for.
func (col Collection) Resolve(m *Papers) ([]string, error) {
	var ids []string
	switch {
	case col.Members.WithNumber:
		for _, p := range m.Numbered() {
			ids = append(ids, p.ID)
		}
	case col.Members.All:
		ids = m.IDs()
	default:
		for _, id := range col.Members.IDs {
			if _, ok := m.ByID(id); !ok {
				return nil, fmt.Errorf("collection %s names %s, which is not a paper in this corpus", col.ID, id)
			}
			ids = append(ids, id)
		}
	}

	switch col.Order {
	case "", "manual":
		return ids, nil
	case "number":
		byID := make(map[string]Paper, len(m.Papers))
		for _, p := range m.Papers {
			byID[p.ID] = p
		}
		ps := make([]Paper, 0, len(ids))
		for _, id := range ids {
			ps = append(ps, byID[id])
		}
		sortPapers(ps)
		out := make([]string, 0, len(ps))
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out, nil
	case "topological":
		return m.ReadingOrder(ids)
	default:
		return nil, fmt.Errorf("collection %s asks for order %q, which is not number, manual or topological", col.ID, col.Order)
	}
}
