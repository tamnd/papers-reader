package extract

import (
	"cmp"
	"slices"
	"strings"

	"github.com/tamnd/papers-reader/poppler"
)

// proseWidth is how wide a block has to be, as a fraction of one column,
// before this counts it as prose.
//
// Half a column, and the gap it sits in is wide. Page 3 of the MapReduce
// paper is the page it was measured on. Its columns are 225 points and every
// paragraph on it is 225 points, because the paper is justified and a
// paragraph is as wide as its widest line. The twenty eight blocks inside
// Figure 1 run from 19 points to 63, which is 0.08 to 0.28 of a column, so
// the widest label on the page is not half way to this from below and the
// narrowest paragraph is twice it from above.
//
// The one thing on that page in between is the section heading, 99 points,
// which this drops along with the figure. That is a real loss and a small
// one: A9 is for a reader that stopped at a figure or forgot a column, and
// neither of those loses a heading and nothing else.
const proseWidth = 0.5

// Prose is the page's text layer with the words the reading is not expected
// to have left out. Pass the paper's furniture, or nil to keep it.
//
// It exists for rule A9, which compares a reading of a page against the words
// the file was typeset from and fails a page that does not account for them.
// Two kinds of word on a page are in that layer and are not in a correct
// reading, and on a page with enough of either the rule reads a correct
// answer as a page with a hole in it.
//
// The first is what is inside a figure. A figure is described and not
// transcribed, so its labels never appear. Page 3 of the MapReduce paper is
// 0.15 covered and correct: the twenty words of its worst stretch are user,
// program, fork, fork, fork, master, which are the labels in Figure 1.
//
// The second is the running head and foot, which is why this wants the
// furniture. The prompt does ask for the head and the readers do not give it,
// and on the same page of the same paper, with the figure already out of the
// way, what was left was usenix, association, osdi, symposium, operating,
// systems, which is the line the journal prints along the bottom of every
// page. Furniture.Is is the same judgement the native path makes, made once
// over the whole paper, because a line is a running head by repeating and no
// single page can tell.
//
// What separates a label from a paragraph here is width. A paragraph is as
// wide as its column and a label is a word or two, and the two do not come
// within a factor of three of each other on the pages this was measured on.
// It is a proxy for being inside a figure rather than a test of it, and it is
// the one the geometry supports without finding the figure first: finding it
// means walking up from the caption, which is the figures package's work and
// needs the page assembled, and this runs while the page is still being read.
//
// What it buys is the whole point of the rule, which is telling a page that
// is fine from a page with a paragraph missing. Over the 111 pages of the
// corpus where A9 runs at all, 79 score higher with this than without it and
// two score lower. One page is under MinCoverage with this where five were
// without it, and that page is a real defect: ResNet page 1 at 0.25, missing
// have, series, breakthroughs, naturally, integrate, highlevel, which is a
// stretch of the introduction the reader dropped. The two ends of the rest:
//
//	                     without      with
//	bitcoin page 5          0.20      1.00
//	  same, last paragraph dropped
//	                        0.20      0.30
//	attention page 5        0.55      1.00
//	  same, last paragraph dropped
//	                        0.30      0.35
//
// Bitcoin page 5 is the page A9 was written for, and the row above is the
// reason to bother. Without this the rule cannot tell the two readings of it
// apart at all, 0.20 against 0.20, and it only ever caught the defect because
// it was failing the good reading too. With it the gap is 0.70, and the words
// it reports missing from the bad one are should, noted, where, depends,
// several, those, which is the dropped paragraph and nothing else.
//
// Taking words out of the layer does not only make the rule quieter, which
// was the first guess and is wrong. A9 looks for the worst run of twenty
// consecutive words, so dropping words that were scattered through a bad run
// packs what is left of it into one window and scores lower than before. That
// is what the second row of each pair is, and it is the behaviour to want.
func Prose(p poppler.Layout, f *Furniture) string {
	if len(p.Blocks) == 0 {
		return ""
	}
	least := proseWidth * columnWidth(p)
	var out []string
	// The rebuilt lines rather than pdftotext's, because those are the ones
	// Furniture learned from and asking it about the others finds nothing:
	// a running foot with the journal across the middle and the page number
	// at the right is one line to pdftotext and two to the column reader.
	for _, l := range f.Lines(p) {
		if inProse(p, l, least) {
			out = append(out, l.Text())
		}
	}
	return strings.Join(out, "\n")
}

// inProse reports whether a line is in a block at least as wide as least.
//
// The line is measured by the block it is in rather than by itself, because
// the last line of a paragraph is short and is prose, and a paragraph is as
// wide as its widest line. A line that crosses two blocks counts if either
// of them is wide, which is what a title set across the top of a two column
// page needs.
func inProse(p poppler.Layout, l poppler.TextLine, least float64) bool {
	for _, b := range p.Blocks {
		if b.Width() >= least && meets(b.Box, l.Box) {
			return true
		}
	}
	return false
}

// meets reports whether two boxes overlap at all.
func meets(a, b poppler.Box) bool {
	return a.XMin < b.XMax && b.XMin < a.XMax && a.YMin < b.YMax && b.YMin < a.YMax
}

// columnWidth is how wide one column of the page is, in points.
//
// Two ways of asking, and the narrower answer wins. Each is wrong on a page
// the other is right about, and they are wrong in opposite directions, so the
// smaller of the two is right on both.
//
// Being wrong small and being wrong large do not cost the same, which is the
// other reason to take the smaller. Too small keeps a figure label, and the
// rule reports a word a reader was never going to give it. Too large drops a
// paragraph, and on a page where it drops all of them Prose comes back empty
// and the rule stops running at all. The first is noise and the second is a
// rule that has quietly switched itself off.
func columnWidth(p poppler.Layout) float64 {
	if w, ok := repeatedWidth(p); ok {
		return min(w, spannedWidth(p))
	}
	return spannedWidth(p)
}

// spannedWidth divides the type area by the number of columns.
//
// The type area rather than the paper, because the margins are not type and a
// paper's margins are a fifth of its width. Dividing the area by the columns
// rather than measuring a column keeps this to the one thing Gutters already
// knows, which is where the channels are and not what is either side of them.
//
// Where it is wrong is a page whose figure crosses the channel. Gutters wants
// a clear vertical strip and the labels scattered through a diagram fill it,
// so a two column page reads as one and a column comes back twice its width.
// Page 3 of the MapReduce paper is that page: no gutter found, 503 points of
// type area, 503 points of column, and its nine real 225 point paragraphs all
// measured against half of 503 and thrown away.
func spannedWidth(p poppler.Layout) float64 {
	left, right := p.Blocks[0].XMin, p.Blocks[0].XMax
	for _, b := range p.Blocks[1:] {
		left, right = min(left, b.XMin), max(right, b.XMax)
	}
	return (right - left) / float64(len(Gutters(p))+1)
}

// repeatedWidth is the widest block width that two blocks share, and whether
// any width is shared at all.
//
// A paragraph is as wide as its column and a page has several paragraphs, so
// a column's width is a width that repeats. The things wider than a column
// are the running foot, a title and a figure set across the page, and a page
// has one of each at most, so the widest repeated width is the column and not
// one of those. On MapReduce page 3 the widths in order are 408 for the foot
// and then nine paragraphs between 224.7 and 224.9, and this returns 224.9.
//
// Where it is wrong is a page that repeats something wider than a column, a
// title over a full width table, and a page with no two blocks alike at all,
// which is a plate with one caption under it. The first is why the caller
// takes the smaller of this and the span, and the second is what ok is for.
func repeatedWidth(p poppler.Layout) (float64, bool) {
	w := make([]float64, len(p.Blocks))
	for i, b := range p.Blocks {
		w[i] = b.Width()
	}
	slices.SortFunc(w, func(a, b float64) int { return cmp.Compare(b, a) })
	for i := 0; i+1 < len(w); i++ {
		// Within a twentieth, because a justified paragraph is as wide as
		// its widest line and the last line of one is not justified, so two
		// paragraphs of the same column differ by a fraction of a point.
		if w[i+1] >= 0.95*w[i] {
			return w[i], true
		}
	}
	return 0, false
}
