package extract

import (
	"context"
	"sort"

	"github.com/tamnd/papers-reader/poppler"
)

// A Native is one paper read by pdftotext and nothing else.
//
// This is the path whose output no model ever guessed at, and it is worth
// keeping that property in front of the reader: a sentence on a native page
// is the sentence the author wrote, and a sentence on a vision page is a
// reading of a photograph of it. Which path a paper took is in its front
// matter for exactly this reason.
//
// The whole range is read in one pdftotext run rather than a run per page,
// because the furniture has to be learned from the paper before any page of
// it can be written, and because one process for a twenty page paper beats
// twenty.
type Native struct {
	Layouts   []poppler.Layout
	Furniture *Furniture
}

// ReadNative runs pdftotext over a range of pages and learns the paper's
// furniture from them. Pass the whole paper where that is affordable: a
// sample from the middle learns the running head correctly and learns
// nothing about the first page, which is the page that has a different one.
func ReadNative(ctx context.Context, path string, first, last int) (*Native, error) {
	layouts, err := poppler.Layouts(ctx, path, first, last)
	if err != nil {
		return nil, err
	}
	return &Native{Layouts: layouts, Furniture: FindFurniture(layouts)}, nil
}

// Page reads one page. The bool is false for a page outside the range that
// was read.
func (n *Native) Page(number int) (Page, bool) {
	for _, l := range n.Layouts {
		if l.Number == number {
			return Read(l, n.Furniture), true
		}
	}
	return Page{}, false
}

// All reads every page of the range, in order.
func (n *Native) All() []Page {
	out := make([]Page, 0, len(n.Layouts))
	for _, l := range n.Layouts {
		out = append(out, Read(l, n.Furniture))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// Map is how the number printed on a page relates to the page of the file.
//
// A paper pulled out of a journal starts at page 483 and a preprint starts at
// 1, and neither of them starts where the PDF does, because the PDF of the
// journal article often carries a cover sheet the library added. The offset
// is learned from the pages that print a number and is then required of the
// ones that do, which is acceptance rule A6.
type Map struct {
	Offset int
	Known  bool
}

// LearnMap works out the offset from the folios the pages printed.
//
// The median and not the mean, and not the first page either. A scanned paper
// misreads a folio now and then, a cover sheet prints nothing, and one page
// in the hundred carries a figure number where the folio should be. The
// median survives all three; a mean does not, and a paper whose offset is
// off by one refuses every page it has.
func LearnMap(pages []Page) Map {
	var offsets []int
	for _, p := range pages {
		if n, ok := p.PrintedNumber(); ok {
			offsets = append(offsets, n-p.Number)
		}
	}
	// Two pages agreeing is the least that means anything: one page's folio
	// read wrong is one page's folio read wrong, and there is no way to tell
	// which of two disagreeing pages is the good one.
	if len(offsets) < 2 {
		return Map{}
	}
	sort.Ints(offsets)
	return Map{Offset: offsets[len(offsets)/2], Known: true}
}

// Printed is the number that should be printed on a page of the file. The
// bool is false when the paper never said.
func (m Map) Printed(page int) (int, bool) {
	if !m.Known {
		return 0, false
	}
	return page + m.Offset, true
}
