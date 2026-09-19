package refs

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest is manifests/refs/<id>.yaml: one paper's bibliography, parsed.
//
// One file per paper rather than one file for the corpus. A hundred
// bibliographies in one document would be a file nobody can read and a diff
// nobody can review, and the point of keeping raw is that a person can go
// and look at an entry the parser got wrong.
type Manifest struct {
	Paper   string  `yaml:"paper"`
	Style   Style   `yaml:"style"`
	Entries []Entry `yaml:"entries"`
	// Notes are what the parse could not do, carried into the file so that
	// the next person to read it does not have to re-run the parser to find
	// out.
	Notes []string `yaml:"notes,omitempty"`
	// Gaps are the entries the page does not give back. See Gap.
	Gaps []Gap `yaml:"gaps,omitempty"`
}

// A Gap is an entry the paper cites and the page will not give back.
//
// It is written by hand and nothing generates it, which is the point of it.
// A citation pointing at nothing is rule R02 and nearly always a fault in
// this toolchain rather than in the paper, and every one of them found so
// far has been: a heading the journal printed on every page, a label with
// no space after it, an array index read as a reference. A rule that let
// the parser excuse its own misses would have hidden all four.
//
// So a gap is a person saying, in the corpus and in a diff somebody can
// review, that they went and looked and the entry is not there to be had.
// Mohan's entry 67 is the one this was written for: the page is a scan with
// no text layer, the reference list runs 66 then 68 in four separate reads
// of it, and the paper cites 67 three times.
//
// It is carried across a rebuild of the manifest, since the reason it was
// written down does not change when the parser does.
type Gap struct {
	Key string `yaml:"key"`
	Why string `yaml:"why"`
}

// Missing says whether an entry is one the manifest records as a gap.
func (m *Manifest) Missing(key string) bool {
	for _, g := range m.Gaps {
		if g.Key == key {
			return true
		}
	}
	return false
}

// NewManifest is the parse of one paper's bibliography, ready to save.
func NewManifest(id string, r *Result) *Manifest {
	return &Manifest{Paper: id, Style: r.Style, Entries: r.Entries, Notes: r.Notes}
}

// Load reads one manifests/refs/<id>.yaml.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Save writes the manifest, creating the directory if it is the first one.
//
// It writes through a temporary file in the same directory and renames, so
// an interrupted run leaves either the old manifest or the new one and never
// half of either.
func (m *Manifest) Save(path string) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".refs-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Entry finds one reference by its key.
func (m *Manifest) Entry(key string) (Entry, bool) {
	for _, e := range m.Entries {
		if e.Key == key {
			return e, true
		}
	}
	return Entry{}, false
}

// Resolved is how many entries name a paper in the corpus.
func (m *Manifest) Resolved() int {
	n := 0
	for _, e := range m.Entries {
		if e.ResolvesTo != "" {
			n++
		}
	}
	return n
}

// Links is the key to paper id map the citation rewriter works from.
func (m *Manifest) Links() map[string]string {
	out := make(map[string]string, len(m.Entries))
	for _, e := range m.Entries {
		if e.ResolvesTo != "" {
			out[e.Key] = e.ResolvesTo
		}
	}
	return out
}
