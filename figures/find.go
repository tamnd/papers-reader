package figures

import (
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// Find is the whitespace island detector: the regions of a page that hold no
// text.
//
// The spec lists three detectors and this is the third of them, the one that
// works everywhere. The first reads the bounding boxes out of a layout
// model's JSON and is better when there is one, and the second lists the
// embedded bitmaps with pdfimages and is better when the figure is a
// photograph. Neither is available on a born digital paper read by the
// native path, which is most of the hundred, and both of them still need
// this one's caption pairing.
//
// What it looks for is a band of a column with no text lines in it. That is
// a crude test and it is the right crude test: a page of a paper is set
// solid, and the only things that leave a hole in a column are a figure, a
// table, a display equation and the space above a section heading. The first
// three are large and the last is not, so a floor on the height of the band
// separates them, and the caption pairing throws away whatever is left that
// nothing captioned.
//
// Boxes come back in the toolchain's coordinates: points, origin top left.
func Find(p poppler.Layout, f *extract.Furniture, frame Frame) []Candidate {
	lines := f.Lines(p)
	if len(lines) < minLines || p.Width <= 0 || p.Height <= 0 {
		return nil
	}
	cuts := extract.Gutters(p)
	pitch := pitchOf(lines)
	if pitch <= 0 {
		return nil
	}
	top, bottom := block(lines, frame)
	head, foot := clear(p, f, top, bottom)

	// Per column, because a figure that takes one column of a two column
	// page leaves the other column full of text, and a page wide scan for
	// empty rows would never see it.
	var out []Candidate
	for col := 0; col <= len(cuts); col++ {
		out = append(out, inColumn(p, lines, cuts, col, pitch, top, bottom, head, foot)...)
	}
	return keep(merge(out, pitch), p)
}

// block is the top and the bottom of the paper's text block on this page, so
// that a figure at the head of a page and one at its foot are found too. A
// page whose text starts an inch below where the paper's normally does has
// something in that inch.
//
// The frame is where the paper puts its type across the whole run, and it is
// the better answer where there is one. Falling back to this page's own
// lines is right for a paper with no consistent frame and wrong for a page
// that opens with a figure, which is why the frame is looked at first.
func block(lines []poppler.TextLine, frame Frame) (top, bottom float64) {
	if !frame.Empty() {
		return frame.Top, frame.Bottom
	}
	top, bottom = lines[0].YMin, lines[0].YMax
	for _, l := range lines {
		top = min(top, l.YMin)
		bottom = max(bottom, l.YMax)
	}
	return top, bottom
}

// clear is how far a band at the head or the foot of a page may run past the
// text block: to the last piece of furniture above it and the first below,
// and to the paper's edge where there is none.
//
// The frame is where a paper puts its type on a normal page, and a figure is
// not obliged to stay inside it. The Transformer paper's figure 1 starts
// about a line above the frame, so cropping at the frame cuts the top word
// of the diagram in half. Running to the edge instead costs nothing, because
// the render is trimmed back to the ink afterwards, and the only thing that
// must not be swept up on the way is a running head or a page number, which
// is what the furniture is.
func clear(p poppler.Layout, f *extract.Furniture, top, bottom float64) (head, foot float64) {
	head, foot = 0, p.Height
	for _, l := range extract.Lines(p) {
		if len(l.Words) == 0 || !f.Is(p, l) {
			continue
		}
		if l.YMax <= top {
			head = max(head, l.YMax)
		}
		if l.YMin >= bottom {
			foot = min(foot, l.YMin)
		}
	}
	return head, foot
}

// keep drops the regions that are too small to be a diagram.
//
// The band test is in lines of the paper's own text, which is the right
// unit for a hole in a column and is the wrong unit for a page that is
// mostly picture. The attention visualisations at the end of the
// Transformer paper are drawn with words in them, so the lines are the
// figure, the median gap between them is a fraction of a normal line, and
// four of those is a sliver a centimetre high. Committing that gives a
// reader a strip of somebody's diagram with the top and bottom cut off.
//
// So there is a floor in the page's own units as well. A figure is a
// substantial part of a page or it is not a figure, and a paper whose
// figures this loses is a paper for the layout path, which reads the
// figure boxes out of the model rather than inferring them from the holes.
func keep(in []Candidate, p poppler.Layout) []Candidate {
	var out []Candidate
	for _, c := range in {
		if c.Box.Height() < minHeight*p.Height || c.Box.Width() < minWidth*p.Width {
			continue
		}
		out = append(out, c)
	}
	return out
}

// minHeight and minWidth are the floor as a share of the page. A twentieth
// of the height is about half an inch on letter paper, which is smaller
// than any real diagram and larger than any gap.
const (
	minHeight = 0.06
	minWidth  = 0.15
)

// minLines is how much text a page has to carry before a hole in it means
// anything. A plate with a single caption line on it is a page image, and
// the band around the caption is the whole page.
const minLines = 8

// minBand is how tall a hole has to be, in lines of this paper's own text,
// before it is a candidate. Three lines is the gap a section heading leaves
// with its space above and below, so the floor is above that.
const minBand = 4

// margin is how much of the surrounding white space is kept on each side of
// a band, as a share of the line pitch. A crop taken exactly at the last
// baseline above and the first below cuts the descenders off the top row of
// the figure.
const margin = 0.4

func inColumn(p poppler.Layout, lines []poppler.TextLine, cuts []float64, col int, pitch, top, bottom, head, foot float64) []Candidate {
	var mine []poppler.TextLine
	for _, l := range lines {
		if extract.Column(l, cuts) == col {
			mine = append(mine, l)
		}
	}
	if len(mine) < minBand {
		return nil
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].YMin < mine[j].YMin })

	// The column's own extent, which is the width a figure in it can have.
	// Taken from the text and not from the gutters, because a gutter is
	// halfway between two columns and half a gutter of white margin on each
	// side of every figure would be white the reader has to look past.
	left, right := mine[0].XMin, mine[0].XMax
	for _, l := range mine {
		left = min(left, l.XMin)
		right = max(right, l.XMax)
	}

	// The hole above the column's first line, the holes between its lines,
	// and the hole below its last. A figure at the head of a column with its
	// caption under it leaves a hole above the text rather than inside it,
	// and one at the foot leaves a hole below. A hole with no text above it
	// runs up to the furniture and one with no text below it runs down to
	// it, because the render is trimmed back to the ink either way.
	var holes []hole
	floor := top
	for _, l := range mine {
		if l.YMin-floor >= minBand*pitch {
			holes = append(holes, hole{edge(floor, top, head), l.YMin})
		}
		floor = max(floor, l.YMax)
	}
	if bottom-floor >= minBand*pitch {
		holes = append(holes, hole{edge(floor, top, head), foot})
	}

	holes = stretch(join(holes, mine, left, right), mine, head, foot, left, right)

	var out []Candidate
	for _, h := range holes {
		out = append(out, band(p, left, right, h.from, h.to, pitch))
	}
	return out
}

// A hole is a stretch of a column with no text in it, before anything has
// decided whether it is a figure.
type hole struct{ from, to float64 }

// edge widens a hole that begins at the top of the text block out to the top
// of the page. Anywhere else the hole begins at a line of text, and the line
// is where it has to stop.
func edge(from, top, head float64) float64 {
	if from == top {
		return head
	}
	return from
}

// join puts back together a figure that its own lettering cut in half.
//
// A figure is not only pictures. The Transformer paper's figure 2 is two
// panels with a title set over each of them, so the page has a hole, a line
// of text, and another hole, and taking the holes as two figures crops the
// titles out of the middle of the one figure that is really there. The same
// thing happens to the (a) and (b) under a pair of plots.
//
// What separates that lettering from the paper's own text is where it sits.
// Body text runs the measure and a section heading is flush left, so both
// touch the left edge of the column. A figure's lettering is placed over the
// thing it names and touches neither edge. That is a narrow test and it is
// deliberately narrow: a hole in a column is cheap to find and a wrong join
// puts a paragraph of somebody's paper inside a PNG.
func join(in []hole, lines []poppler.TextLine, left, right float64) []hole {
	if len(in) < 2 {
		return in
	}
	out := []hole{in[0]}
	for _, h := range in[1:] {
		last := &out[len(out)-1]
		if lettering(lines, last.to, h.from, left, right) {
			last.to = h.to
			continue
		}
		out = append(out, h)
	}
	return out
}

// stretch runs the first hole up over a title set above the figure and the
// last one down over a label set under it.
//
// join only ever sees the lettering between two holes, and lettering at the
// top of a page has no hole above it to join to. The Transformer paper's
// figure 2 is exactly that: two panel titles at the very top of the page
// with the picture under them, so the page has one hole and the titles are
// not in it.
func stretch(in []hole, lines []poppler.TextLine, head, foot, left, right float64) []hole {
	if len(in) == 0 {
		return in
	}
	if first := &in[0]; lettering(lines, head, first.from, left, right) {
		first.from = head
	}
	if last := &in[len(in)-1]; lettering(lines, last.to, foot, left, right) {
		last.to = foot
	}
	return in
}

// lettering reports whether everything between two holes belongs to the
// figure rather than to the paper.
func lettering(lines []poppler.TextLine, from, to, left, right float64) bool {
	width := right - left
	if width <= 0 {
		return false
	}
	n := 0
	for _, l := range lines {
		if l.YMax <= from || l.YMin >= to {
			continue
		}
		n++
		if n > maxLabel {
			return false
		}
		// A caption is never swallowed, whatever it looks like. It is the
		// thing that says which figure this is, it is translated separately,
		// and a caption inside the PNG is a caption no reader can read in
		// their own language.
		if caption.MatchString(strings.TrimSpace(l.Text())) {
			return false
		}
		if l.XMin-left < indent*width || right-l.XMax < indent*width {
			return false
		}
	}
	return n > 0
}

// maxLabel is how many lines of lettering a figure may have in the middle of
// it. Two covers a title over each panel and a row of (a) (b) (c) under
// them, and anything more is a paragraph.
const maxLabel = 2

// indent is how far a line has to stand off both edges of the column before
// it counts as part of a figure rather than part of the paper, as a share of
// the column width. Three percent is a couple of characters, which is more
// than the nothing a heading stands off by and less than the inch a panel
// title does.
const indent = 0.03

func band(p poppler.Layout, left, right, from, to, pitch float64) Candidate {
	b := poppler.Box{
		XMin: left,
		YMin: from + margin*pitch,
		XMax: right,
		YMax: to - margin*pitch,
	}
	return Candidate{Page: p.Number, Box: b, Method: Vector}
}

// merge joins the bands of two columns that are the same band.
//
// A figure set across both columns of a two column page is a hole in both of
// them at the same height, and committing it as two half figures would cut
// the diagram down the middle.
func merge(in []Candidate, pitch float64) []Candidate {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Box.YMin != in[j].Box.YMin {
			return in[i].Box.YMin < in[j].Box.YMin
		}
		return in[i].Box.XMin < in[j].Box.XMin
	})
	var out []Candidate
	for _, c := range in {
		n := len(out)
		if n > 0 && overlaps(out[n-1].Box, c.Box, pitch) {
			out[n-1].Box.XMin = min(out[n-1].Box.XMin, c.Box.XMin)
			out[n-1].Box.XMax = max(out[n-1].Box.XMax, c.Box.XMax)
			out[n-1].Box.YMin = min(out[n-1].Box.YMin, c.Box.YMin)
			out[n-1].Box.YMax = max(out[n-1].Box.YMax, c.Box.YMax)
			continue
		}
		out = append(out, c)
	}
	return out
}

// overlaps says whether two bands are at the same height. Most of each has
// to be inside the other: a figure in the left column and a shorter one
// beside it in the right are one figure often enough that the loose test
// would be wrong, and a figure across both columns is the same band in both
// to within a line.
func overlaps(a, b poppler.Box, pitch float64) bool {
	top := max(a.YMin, b.YMin)
	bottom := min(a.YMax, b.YMax)
	shared := bottom - top
	if shared <= 0 {
		return false
	}
	return shared > a.Height()-pitch && shared > b.Height()-pitch
}

// pitchOf is this paper's line spacing on this page, which is the unit every
// vertical measurement here is in. A gap of twelve points is a paragraph
// break in a 1967 proceedings and half a line in a modern preprint.
func pitchOf(lines []poppler.TextLine) float64 {
	if len(lines) < 2 {
		return 0
	}
	sorted := make([]poppler.TextLine, len(lines))
	copy(sorted, lines)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].YMin < sorted[j].YMin })
	var gaps []float64
	for i := 1; i < len(sorted); i++ {
		if g := sorted[i].YMin - sorted[i-1].YMin; g > 0 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return 0
	}
	sort.Float64s(gaps)
	return gaps[len(gaps)/2]
}
