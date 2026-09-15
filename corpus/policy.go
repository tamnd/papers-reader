package corpus

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Policy is manifests/policy.yaml: what this corpus publishes, as against
// what each paper's licence lets it publish.
//
// The two are kept apart on purpose. sources.yaml records what the publisher
// says, found by the resolver and never edited to suit us, and the access
// class on each record stays true whatever is decided here. This file records
// the decision taken about those facts by whoever owns the corpus, in one
// place, in the open, where a reader can see it and a maintainer can change
// it back.
//
// A corpus with no policy file publishes by licence, so the cautious
// behaviour is the one you get by doing nothing.
type Policy struct {
	// Body publishes the whole text of every paper, and the figures cropped
	// from it, whatever the access class says. Without it a restricted paper
	// gets front matter and a short abstract and nothing else, which is what
	// the first hundred papers of this corpus got: 88 of them resolve to
	// restricted, mostly because only 17 of the hundred state a licence
	// anywhere a resolver can read it, so "restricted" there means "nobody
	// told us" far more often than it means "we were told no".
	//
	// Turning it on is a claim about the corpus, not about any one paper: it
	// says the person publishing it has decided to carry the whole text of
	// work they do not hold the rights to. It does not touch the rule against
	// committing PDFs, which holds under every licence and every policy.
	Body bool `yaml:"body"`
	// Note is why the policy is what it is, for the next person to read. It
	// is not used by anything.
	Note string `yaml:"note,omitempty"`
}

// PolicyManifest is manifests/policy.yaml.
func (c *Corpus) PolicyManifest() string { return filepath.Join(c.Manifests(), "policy.yaml") }

// LoadPolicy reads manifests/policy.yaml. A corpus that has no such file gets
// the zero policy, which publishes by licence.
func LoadPolicy(path string) (Policy, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Policy{}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := yaml.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Publishes reports whether the body text of a paper in this access class may
// be published here. It is Access.Body asked of a corpus rather than of a
// licence, and it is the call every command should make: the access class
// answers what the publisher allows, this answers what this corpus does.
func (c *Corpus) Publishes(a Access) bool {
	if c != nil && c.Policy.Body {
		return true
	}
	return a.Body()
}

// PublishesFigures reports whether cropped figures from a paper in this
// access class may be committed here. Figures follow the body: a policy that
// carries somebody's text has already made the decision that a diagram from
// the same page would raise again.
func (c *Corpus) PublishesFigures(a Access) bool { return c.Publishes(a) }

// PublishesWhole reports whether the corpus publishes every paper in full,
// which is the question the audit asks when it decides whether a rule about
// how much of a restricted paper is on the page has anything to check.
func (c *Corpus) PublishesWhole() bool { return c != nil && c.Policy.Body }
