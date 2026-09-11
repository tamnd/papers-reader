package layout

import (
	"fmt"
	"sort"
	"strings"
)

// Tool is the program that read the page.
//
// It goes in the front matter of every file the paper produces, because a
// reader who finds a mangled table has a right to know which program made it
// and a maintainer re-running the paper needs to know what to re-run it with.
type Tool string

// The three programs this package can read. The name is the one the program
// calls itself, which is also the command line to run it.
const (
	MinerU  Tool = "mineru"
	Marker  Tool = "marker"
	Docling Tool = "docling"
)

// Tools is every program, in the order `papers doctor` reports them: the one
// to prefer first.
var Tools = []Tool{MinerU, Marker, Docling}

// Kind is what a block is.
//
// It is a short list on purpose. A layout model has thirty or so labels, most
// of which exist to tell page furniture apart from other page furniture, and
// the corpus does exactly two things with that distinction: it drops the
// furniture and it keeps everything else. A finer list here would be a finer
// list nothing reads.
type Kind string

// The kinds the corpus renders differently.
const (
	// Heading is a section heading, with Level saying how deep.
	Heading Kind = "heading"
	// Text is a paragraph. So is a list item, because a list that survived
	// two columns and a PDF is a run of paragraphs and pretending otherwise
	// invents structure.
	Text Kind = "text"
	// Equation is a display equation, which becomes $$..$$ on its own.
	// Inline mathematics is not a block; it arrives inside the text of one.
	Equation Kind = "equation"
	// Figure is a picture with, one hopes, a caption.
	Figure Kind = "figure"
	// Table is a table, which becomes a Markdown table and never an image.
	Table Kind = "table"
	// Code is a listing, and is the one place in the corpus where whitespace
	// is content.
	Code Kind = "code"
	// Furniture is a running head, a footer or a folio: read so that it can
	// be dropped, and dropped here rather than downstream, because it is here
	// that something knows it is furniture.
	Furniture Kind = "furniture"
)

// A Box is a rectangle on the page, in the tool's own coordinates.
//
// Kept because the figure pipeline crops from it and because rule F06, the
// page fraction cap, is computed from it against the page size. Nothing else
// reads it, and a tool that does not report one leaves it zero.
type Box struct {
	X0, Y0, X1, Y1 float64
}

// Empty reports whether a box was never filled in.
func (b Box) Empty() bool { return b == Box{} }

// Area is the box in square units of whatever the page is measured in.
func (b Box) Area() float64 {
	w, h := b.X1-b.X0, b.Y1-b.Y0
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// A Block is one thing on one page.
type Block struct {
	Kind Kind
	// Page is one based, as a person counts pages and as poppler numbers
	// them. The tools count from zero and the parsers add the one, in one
	// place each, so that nothing downstream has to remember.
	Page int
	Box  Box
	// Text is the block's content with the corpus's own inline conventions
	// already applied: mathematics in $..$ and nothing else.
	Text string
	// Level is 1 for a top level heading and 2 for a subsection. Zero on
	// anything that is not a heading.
	Level int
	// Lang is the language of a code block, and is "text" for pseudocode,
	// which is the honest answer and is used freely. A wrong language tag is
	// worse than none, because a renderer colours it and a reader believes it.
	Lang string
	// Image is where the tool wrote the picture it cut out, relative to the
	// directory the JSON is in. The figure stage reads it; this package only
	// carries it.
	Image string
	// Caption is the caption the tool paired with a figure or a table. An
	// uncaptioned figure keeps an empty one and is reported rather than
	// quietly numbered.
	Caption string
	// Rows is a table, the first row being the header. A table that the tool
	// could only give as an image has no rows and is reported: a table
	// committed as a picture is a table nobody can read, translate or search.
	Rows [][]string
}

// A Page is one page of blocks.
type Page struct {
	Number int
	// Width and Height are the page in the tool's coordinates, for the page
	// fraction cap. Zero where the tool did not say.
	Width, Height float64
	Blocks        []Block
}

// A Document is one paper as a layout tool read it.
type Document struct {
	Tool  Tool
	Pages []Page
	// Notes is what was odd about the file: a table that came back as a
	// picture, a figure with no caption, a block type nothing here knows.
	// They end up in reports/extract.md, because the alternative is a corpus
	// that silently drops what it did not understand.
	Notes []string
}

// Page finds one page. The bool is false for a page the tool did not read.
func (d *Document) Page(number int) (Page, bool) {
	for _, p := range d.Pages {
		if p.Number == number {
			return p, true
		}
	}
	return Page{}, false
}

// First and Last are the page range the tool covered.
func (d *Document) First() int {
	if len(d.Pages) == 0 {
		return 0
	}
	return d.Pages[0].Number
}

func (d *Document) Last() int {
	if len(d.Pages) == 0 {
		return 0
	}
	return d.Pages[len(d.Pages)-1].Number
}

// Blocks is every block of the document in reading order.
func (d *Document) Blocks() []Block {
	var out []Block
	for _, p := range d.Pages {
		out = append(out, p.Blocks...)
	}
	return out
}

// Figures is every picture the tool found, in reading order. It is what the
// figure stage works from.
func (d *Document) Figures() []Block {
	var out []Block
	for _, b := range d.Blocks() {
		if b.Kind == Figure {
			out = append(out, b)
		}
	}
	return out
}

// note records something odd, once. A file where the same unknown block type
// appears four hundred times should say so once and not four hundred times.
func (d *Document) note(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	for _, have := range d.Notes {
		if have == s {
			return
		}
	}
	d.Notes = append(d.Notes, s)
}

// sortPages puts the pages in order and, within a page, leaves the blocks in
// the order the tool gave them.
//
// The tool's order is the reading order and it is the thing the tool is for:
// it is what knows that a two column page is read down the left column first,
// and sorting the blocks again by their boxes here would throw that away and
// get it wrong.
func (d *Document) sortPages() {
	sort.SliceStable(d.Pages, func(i, j int) bool { return d.Pages[i].Number < d.Pages[j].Number })
}

// touch makes sure a page exists, whether or not anything is on it.
func (d *Document) touch(page int, width, height float64) *Page {
	for i := range d.Pages {
		if d.Pages[i].Number != page {
			continue
		}
		if d.Pages[i].Width == 0 {
			d.Pages[i].Width, d.Pages[i].Height = width, height
		}
		return &d.Pages[i]
	}
	d.Pages = append(d.Pages, Page{Number: page, Width: width, Height: height})
	return &d.Pages[len(d.Pages)-1]
}

// add puts a block on its page, making the page if this is the first block
// to land on it.
func (d *Document) add(page int, width, height float64, b Block) {
	b.Page = page
	p := d.touch(page, width, height)
	p.Blocks = append(p.Blocks, b)
}

// collapse is whitespace flattened to single spaces. Everything but a code
// block goes through it: a PDF's text arrives broken at the line ends the
// typesetter chose, and those line ends mean nothing to a reader of a
// reflowing page.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
