// Package pagemap relates the page of a file to the page of a paper.
//
// A paper pulled out of a journal starts at page 483 and a preprint starts
// at 1, and neither of them starts where the PDF does, because the PDF of a
// journal article often carries a cover sheet the library added. Everything
// the corpus records about where something came from is in pages of the
// file, because that is the only number the toolchain can see, and everything
// the paper says about itself is in pages of the paper, because that is what
// the author wrote. This is the table between the two.
//
// It also carries the size of each page and the figures cut from it. A
// bounding box in figures.yaml is four numbers in points, and four numbers in
// points mean nothing without the page they were measured on: a box 300
// points wide is half of a letter page and a fifth of a poster. The figure
// list is the other direction of the same question, which page a crop came
// off, and it is the one a person asks when a figure looks wrong and they
// want the page to compare it against.
package pagemap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/extract"
	"gopkg.in/yaml.v3"
)

// A Map is manifests/pages/<id>.yaml: one paper's pages, in file order.
//
// One file per paper, like the bibliographies and unlike figures.yaml. The
// bibliographies are one per paper because a hundred of them in one document
// is a diff nobody can review, and a page list is longer than a bibliography.
type Map struct {
	Paper string `yaml:"paper"`
	// Offset is what to add to a page of the file to get the page the paper
	// prints. It is nil, and written as null, for a paper whose pages print
	// no numbers to learn it from, which is not the same as an offset of
	// zero: a preprint numbered from one has an offset of zero and knows it.
	Offset *int   `yaml:"offset"`
	Pages  []Page `yaml:"pages"`
}

// A Page is one page of the file.
type Page struct {
	// PDF is the page of the file, counting the cover sheet, from one.
	PDF int `yaml:"pdf"`
	// Printed is the folio the page itself printed, exactly as it printed
	// it, and empty for the many pages that print none. It is text and not a
	// number because the front matter of a thesis prints roman numerals.
	//
	// A page that printed nothing is left empty rather than filled in from
	// the offset. The offset is right there at the top of the file for
	// anybody who wants to work it out, and a manifest that cannot be read
	// back as what was observed is a manifest nobody can check.
	Printed string `yaml:"printed,omitempty"`
	// Size is the width and the height of the page in points.
	Size Size `yaml:"size,flow"`
	// Columns is how many columns of text the page was set in, and is zero
	// for a page nothing could be read off.
	Columns int `yaml:"columns,omitempty"`
	// Figures are the figures cut from this page, by their id in
	// figures.yaml.
	Figures []string `yaml:"figures,flow,omitempty"`
}

// Size is a page's width and height in points.
type Size [2]float64

// Width and Height are the two halves of a Size, named so that a caller does
// not have to remember which way round the pair is.
func (s Size) Width() float64  { return s[0] }
func (s Size) Height() float64 { return s[1] }

// Build assembles a map and learns the offset from the folios the pages
// printed.
//
// The offset is learned by the same median rule the acceptance rules use,
// through the same code, because two opinions about where a paper's numbering
// starts would mean extraction refusing pages the manifest says are fine.
func Build(paper string, pages []Page) *Map {
	folios := &extract.Folios{}
	for _, p := range pages {
		// A printed folio is a line that is a number and nothing else, which
		// is what a one line text of just the folio is.
		folios.Add(p.PDF, p.Printed)
	}
	m := &Map{Paper: paper, Pages: pages}
	if learned := folios.Map(); learned.Known {
		offset := learned.Offset
		m.Offset = &offset
	}
	return m
}

// Printed is the number this page of the file prints, as text.
//
// A page that printed a folio gets the folio it printed. A page that printed
// none gets the one the offset says it would have printed, which is how a
// reader is told what page of the paper they are looking at on a page whose
// number the journal set in the gutter and the scanner cut off. It is empty
// when the paper never numbered itself at all.
func (m *Map) Printed(pdf int) string {
	if p, ok := m.Page(pdf); ok && p.Printed != "" {
		return p.Printed
	}
	if m.Offset == nil {
		return ""
	}
	return strconv.Itoa(pdf + *m.Offset)
}

// PDF is the page of the file that carries a printed page number. The bool
// is false when no page of this paper does.
//
// The pages that printed the number are searched first and the offset is
// used only when none did, so a paper with a page misnumbered in the journal
// itself resolves to the page that really carries the number.
func (m *Map) PDF(printed int) (int, bool) {
	want := strconv.Itoa(printed)
	for _, p := range m.Pages {
		if p.Printed == want {
			return p.PDF, true
		}
	}
	if m.Offset == nil {
		return 0, false
	}
	pdf := printed - *m.Offset
	if _, ok := m.Page(pdf); !ok {
		return 0, false
	}
	return pdf, true
}

// Page finds one page of the file.
func (m *Map) Page(pdf int) (Page, bool) {
	for _, p := range m.Pages {
		if p.PDF == pdf {
			return p, true
		}
	}
	return Page{}, false
}

// Span is the pages the paper prints, as the front matter writes a range: a
// single number for a one page span and first-last for the rest. It is empty
// for a paper that prints no numbers.
//
// Only the pages that printed a number count towards it. A cover sheet the
// library added is page zero by the offset and is not a page of anybody's
// paper, and a span that started at zero would be this program inventing a
// page rather than reading one.
//
// The decoration comes off, because a journal that sets its folios as "-2-"
// and "-3-" prints pages 2 to 3 and not "-2--3-".
func (m *Map) Span() string {
	var first, last string
	for _, p := range m.Pages {
		if p.Printed == "" {
			continue
		}
		printed := strings.Trim(p.Printed, "-[]() ")
		if first == "" {
			first = printed
		}
		last = printed
	}
	if first == "" || first == last {
		return first
	}
	return first + "-" + last
}

// Header is the comment the manifest is written with, for the person who
// meets one of these in a pull request rather than in the documentation.
const Header = `# Which page of the file is which page of the paper.
#
# Written by ` + "`papers pagemap`" + `, one file per paper.
#
# ` + "`offset`" + ` is what to add to a page of the file to get the page the paper prints. It is null for a paper whose pages print no numbers, and zero for a preprint that starts at one.
# ` + "`printed`" + ` is the folio the page itself printed, and is absent for a page that printed none.
# ` + "`size`" + ` is the width and the height of the page in points, which is what makes the bounding boxes in figures.yaml mean something.

`

// Load reads one manifests/pages/<id>.yaml.
func Load(path string) (*Map, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Map
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// Bytes is the file as it would be written, which is what a caller compares
// against the file on disk to see whether a run has anything to do.
func (m *Map) Bytes() ([]byte, error) {
	b, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append([]byte(Header), b...), nil
}

// Save writes the manifest, creating the directory if it is the first one.
//
// Through a temporary file in the same directory and a rename, so an
// interrupted run leaves either the old manifest or the new one and never
// half of either.
func (m *Map) Save(path string) error {
	b, err := m.Bytes()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pagemap-*")
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

// String is the one line summary a run prints per paper.
func (m *Map) String() string {
	parts := []string{fmt.Sprintf("%d pages", len(m.Pages))}
	if span := m.Span(); span != "" {
		parts = append(parts, "printed "+span)
	} else {
		parts = append(parts, "unnumbered")
	}
	figures := 0
	for _, p := range m.Pages {
		figures += len(p.Figures)
	}
	if figures > 0 {
		parts = append(parts, fmt.Sprintf("%d figures", figures))
	}
	return strings.Join(parts, ", ")
}
