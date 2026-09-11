package corpus

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Sources is manifests/sources.yaml: where each PDF was found and what the
// corpus is allowed to publish from it. It is the file that makes a public
// repository defensible, and it is written by the resolver rather than by
// hand.
type Sources struct {
	Sources []Source `yaml:"sources"`
}

// Source is the licence record of one paper.
type Source struct {
	ID     string `yaml:"id"`
	Access Access `yaml:"access"`
	// Licence is the licence as the publisher states it, not a guess. An open
	// paper that names only a URL fails audit rule S06, because "we could
	// download it" is not a licence.
	Licence string `yaml:"licence,omitempty"`
	URL     string `yaml:"url,omitempty"`
	Landing string `yaml:"landing,omitempty"`
	Fetched string `yaml:"fetched,omitempty"`
	SHA256  string `yaml:"sha256,omitempty"`
	Pages   int    `yaml:"pages,omitempty"`
	// TextLayer is what the file's own text is worth: native, digital, ocr or
	// none. It is measured by `papers classify`, not guessed from the year.
	TextLayer string `yaml:"text_layer,omitempty"`
	// FiguresLicence is recorded separately because a paper can be free to
	// read while its figures are reprinted from somewhere else.
	FiguresLicence string `yaml:"figures_licence,omitempty"`
	Note           string `yaml:"note,omitempty"`
}

// LoadSources reads manifests/sources.yaml.
func LoadSources(path string) (*Sources, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Sources
	if err := yaml.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

// LoadSources reads the licence records of this corpus.
func (c *Corpus) LoadSources() (*Sources, error) { return LoadSources(c.SourcesManifest()) }

// ByID finds the record of one paper.
func (s *Sources) ByID(id string) (*Source, bool) {
	for i := range s.Sources {
		if s.Sources[i].ID == id {
			return &s.Sources[i], true
		}
	}
	return nil, false
}

// Access is what the corpus may publish for a paper. A paper with no record
// at all is AccessUnknown, which publishes nothing, so the default of not having
// looked is the default of not publishing.
func (s *Sources) Access(id string) Access {
	rec, ok := s.ByID(id)
	if !ok || !rec.Access.Valid() {
		return AccessUnknown
	}
	return rec.Access
}

// Resolved is how many papers have an access class that is not unknown, which
// is the number milestone M1 finishes on.
func (s *Sources) Resolved() int {
	n := 0
	for _, rec := range s.Sources {
		if rec.Access.Valid() && rec.Access != AccessUnknown {
			n++
		}
	}
	return n
}
