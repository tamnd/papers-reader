package figures

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// A Manifest is manifests/figures.yaml: every figure of every paper, with
// the provenance of each.
//
// One file for the whole corpus rather than one beside each paper's PNGs,
// which is how sources.yaml and papers.yaml are kept and is the shape the
// spec asks for. The entries are sorted by paper and then by figure, so a
// run that adds a paper adds one run of lines in one place and a run that
// redoes a paper rewrites that paper's lines and nothing else.
//
// The caption is here in English and is translated into the content files,
// which is the one thing in the corpus that is deliberately stored twice:
// the manifest is provenance and the content file is what a reader reads.
type Manifest struct {
	Figures []Figure `yaml:"figures"`
}

// Load reads the manifest. A corpus that has never cropped a figure has no
// file yet and gets an empty manifest, because the first run has to have
// something to add to.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s does not parse: %w", path, err)
	}
	return &m, nil
}

// Header is the comment the manifest is written with. It is part of the
// file rather than of the documentation because the person most likely to
// read it is the one looking at a diff of it in a pull request.
const Header = `# Every committed figure, and where it was cut from.
#
# Written by ` + "`papers figures`" + `, one entry per file under figures/.
#
# ` + "`page_fraction`" + ` is the area of the bounding box divided by the area of the page.
# Audit rule F06 refuses anything above 0.75, because a cropped diagram is a figure and a whole page is a scan of somebody else's paper, and to git those two look the same.
#
# ` + "`method`" + ` is ` + "`vector`" + ` when the region was re-rendered from the PDF, ` + "`raster`" + ` when an embedded bitmap was pulled out as it was, and ` + "`crop`" + ` when the region was cut out of a page image.

`

// Save writes the manifest back, sorted, under the header.
//
// It writes through a temporary file in the same directory and renames, so
// an interrupted run leaves either the old manifest or the new one and never
// half of either. This file is the whole corpus, so half of it is worse than
// none of it.
func (m *Manifest) Save(path string) error {
	m.sort()
	body, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".figures-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.WriteString(Header + string(body)); err != nil {
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

func (m *Manifest) sort() {
	sort.SliceStable(m.Figures, func(i, j int) bool {
		a, b := m.Figures[i], m.Figures[j]
		if a.Paper != b.Paper {
			return a.Paper < b.Paper
		}
		return a.ID < b.ID
	})
}

// Of is one paper's figures.
func (m *Manifest) Of(paper string) []Figure {
	var out []Figure
	for _, f := range m.Figures {
		if f.Paper == paper {
			out = append(out, f)
		}
	}
	return out
}

// Replace puts one paper's figures in place of whatever was recorded for it.
//
// Replace and not merge, because a second run of a paper is a correction:
// the detector changed, or the page range did, and the figures it did not
// produce this time are figures that should no longer be there. Merging
// would leave the old ones behind with nothing on disk to match them, which
// is exactly what audit rule F01 fails on.
func (m *Manifest) Replace(paper string, figures []Figure) {
	kept := m.Figures[:0]
	for _, f := range m.Figures {
		if f.Paper != paper {
			kept = append(kept, f)
		}
	}
	m.Figures = append(kept, figures...)
	m.sort()
}

// Has reports whether this paper already has a figure with these bytes.
//
// Deduplication is by hash and not by position, because the same diagram
// reprinted on two pages is one figure, and a corpus that committed it twice
// would translate its caption twice and give a reader two names for one
// picture. Within one paper and not across the corpus: two papers by the
// same authors reprinting the same diagram are two papers, and each of them
// should carry it.
func Has(figures []Figure, sha string) (Figure, bool) {
	for _, f := range figures {
		if f.SHA256 == sha {
			return f, true
		}
	}
	return Figure{}, false
}

// Next is the id for a figure about to be added: f01, f02 and so on.
//
// Positional and not the paper's own figure number, because a paper can
// print two figures numbered 3 and 3a, a paper can print figure 10 before
// figure 9 in the layout, and a paper can print a figure with no number at
// all. The number the paper used is kept in the entry, where it can be
// wrong without breaking a filename.
func Next(figures []Figure) string {
	return fmt.Sprintf("f%02d", len(figures)+1)
}
