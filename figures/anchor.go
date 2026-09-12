package figures

import (
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// Anchor is the second detector, and it works from the caption rather than
// from the white space.
//
// Find looks for a hole in a column, which is what a picture leaves. Some
// figures are not pictures. The attention visualisations in the appendix of
// the Transformer paper are drawn out of the sentence being visualised, with
// coloured lines between the words, and pdftotext reads that page as a dozen
// lines of text scattered down it. There is no hole anywhere on the page big
// enough to be a figure, so the first pass comes back with four slivers and
// no figure.
//
// What is left to work from is the caption. A page that says "Figure 3:" has
// a figure 3 on it, and the only question is where it ends. So the region is
// grown upwards from the caption until it reaches a line that is the paper
// talking rather than part of the picture.
//
// Upwards only. A caption below its figure is the convention and the
// exception is a table, which this does not run for. Growing both ways would
// put the paragraph after the figure inside the PNG on every paper that
// follows the convention, and that is most of them.
//
// The regions it finds are generous, because the stop is a line of text and
// the picture ends somewhere above that. That costs little: the render is
// trimmed back to its ink, a region that was really white space renders as
// nothing and is thrown out, and the page fraction is measured on what is
// left rather than on what was asked for.
func Anchor(p poppler.Layout, f *extract.Furniture, frame Frame, caps []Caption, found []Found) []Found {
	lines := f.Lines(p)
	if len(lines) < minLines || p.Width <= 0 || p.Height <= 0 {
		return found
	}
	pitch := pitchOf(lines)
	if pitch <= 0 {
		return found
	}
	top, bottom := block(lines, frame)
	head, _ := clear(p, f, top, bottom)
	cuts := extract.Gutters(p)

	// The caller's slice is left alone, because a caption moved off one
	// region and onto another is a change this may yet decide not to make.
	out := append([]Found(nil), found...)

	var add []Found
	for i := range caps {
		c := &caps[i]
		// Tables only ever come back as a Markdown table, so growing a
		// region for one would be work in aid of a file nobody should
		// commit.
		if c.Page != p.Number || c.Kind != "figure" {
			continue
		}
		at := holder(out, c)
		if at >= 0 && out[at].Box.YMax <= c.Box.YMin {
			// The first pass found a region above this caption, which is
			// where a figure's caption says its figure is. That region is a
			// hole in the column and this would be a guess, so it wins.
			continue
		}
		band, ok := above(p, lines, cuts, c, pitch, head)
		if !ok || len(keep([]Candidate{band}, p)) == 0 || claimed(out, c, band.Box) {
			continue
		}
		// A figure caption paired with a region below it is the first pass
		// falling back rather than answering: on the appendix pages of the
		// Transformer paper the region below the caption is the empty
		// bottom half of the sheet. Now that there is something above to
		// grow into, the caption belongs to that.
		if at >= 0 {
			out[at].Caption = nil
		}
		add = append(add, Found{Candidate: band, Caption: c})
	}
	if len(add) == 0 {
		return found
	}

	// A figure drawn with type does leave a little white in it, and that
	// white came back from the first pass as a region nobody captioned.
	// Those are pieces of the figure this pass has just found whole, so
	// reporting them would be reporting the same thing twice.
	kept := make([]Found, 0, len(out)+len(add))
	for _, was := range out {
		if was.Caption == nil && inside(was.Box, add) {
			continue
		}
		kept = append(kept, was)
	}
	kept = append(kept, add...)

	// Back into the order they sit on the page, because that is the order
	// the figure ids are handed out in and a reader expects figure 3 to be
	// numbered before figure 4.
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].Box.YMin != kept[j].Box.YMin {
			return kept[i].Box.YMin < kept[j].Box.YMin
		}
		return kept[i].Box.XMin < kept[j].Box.XMin
	})
	return kept
}

// holder is the region the first pass gave this caption to, or -1.
func holder(found []Found, c *Caption) int {
	for i, f := range found {
		if f.Caption == c {
			return i
		}
	}
	return -1
}

// claimed says whether some other caption's region is where this band wants
// to be. Two captions over one picture is a figure this has misread, and the
// one found by its own white space is the one to believe.
func claimed(found []Found, c *Caption, b poppler.Box) bool {
	for _, f := range found {
		if f.Caption != nil && f.Caption != c && overlap(f.Box, b) > half {
			return true
		}
	}
	return false
}

func inside(b poppler.Box, add []Found) bool {
	for _, f := range add {
		if overlap(b, f.Box) > half {
			return true
		}
	}
	return false
}

// overlap is how much of the first box is inside the second, as a share of
// the first.
func overlap(a, b poppler.Box) float64 {
	w := min(a.XMax, b.XMax) - max(a.XMin, b.XMin)
	h := min(a.YMax, b.YMax) - max(a.YMin, b.YMin)
	if w <= 0 || h <= 0 || a.Width() <= 0 || a.Height() <= 0 {
		return 0
	}
	return w * h / (a.Width() * a.Height())
}

const half = 0.5

// above grows a region upwards from a caption to the last line over it that
// belongs to the paper.
func above(p poppler.Layout, lines []poppler.TextLine, cuts []float64, c *Caption, pitch, head float64) (Candidate, bool) {
	col := extract.Column(poppler.TextLine{Box: c.Box}, cuts)
	var mine []poppler.TextLine
	for _, l := range lines {
		if extract.Column(l, cuts) == col {
			mine = append(mine, l)
		}
	}
	if len(mine) == 0 {
		return Candidate{}, false
	}
	// The column's extent, taken from every line in it including the
	// caption's own. A caption runs the measure and the picture over it
	// rarely does, so the caption is usually what sets the width.
	left, right := c.Box.XMin, c.Box.XMax
	for _, l := range mine {
		left = min(left, l.XMin)
		right = max(right, l.XMax)
	}
	body := bodyHeight(mine, c)

	// Half a line above the caption, because a caption's first line and the
	// bottom row of the picture can round to the same point.
	ceiling := c.Box.YMin - pitch/2
	stop := head
	for _, l := range mine {
		if l.YMax > ceiling {
			continue
		}
		if bottom, ok := prose(l, c, body); ok {
			stop = max(stop, bottom)
		}
	}
	b := poppler.Box{
		XMin: left,
		YMin: stop + margin*pitch,
		XMax: right,
		YMax: c.Box.YMin - margin*pitch,
	}
	if b.Height() <= 0 || b.Width() <= 0 {
		return Candidate{}, false
	}
	return Candidate{Page: p.Number, Box: b, Method: Vector}, true
}

// bodyHeight is how tall a line of this paper's text is, measured off the
// caption, which is the one thing on the page that is certainly the paper
// and is set in the paper's own face at the paper's own size.
//
// It is measured here rather than taken from the page as a whole because on
// the pages this pass is for, most of the lines are the picture.
func bodyHeight(lines []poppler.TextLine, c *Caption) float64 {
	var hs []float64
	for _, l := range lines {
		if l.YMin >= c.Box.YMin-1 && l.YMax <= c.Box.YMax+1 {
			hs = append(hs, l.Height())
		}
	}
	if len(hs) == 0 {
		for _, l := range lines {
			hs = append(hs, l.Height())
		}
	}
	if len(hs) == 0 {
		return 0
	}
	sort.Float64s(hs)
	return hs[len(hs)/2]
}

// prose says whether a line is the paper talking rather than part of a
// picture, and if it is, how far down the page it reaches.
//
// Two ways it can be. It is another caption, which is a different figure and
// must never be swallowed. Or some word of it is set flush with the column
// at the size the paper sets its text in, which the first word of a
// paragraph's last line is and the first word of a section heading is and
// nothing inside a picture is.
//
// Words rather than lines, because pdftotext puts everything at one height
// on the page into one line whether or not it is one line. The appendix of
// the Transformer paper opens with the heading "Attention Visualizations"
// and the figure's own "Input-Input Layer5" label at the same height, in one
// line 23.8 points tall that is two thirds picture. Read as a line there is
// nothing there to stop at and the heading goes into the PNG. Read as words
// the heading is two words of 10.75 and the label is two of 22.77.
//
// The size test is worth more than it looks. pdftotext gives a word the
// height of its font's ascent and descent rather than the height of its ink,
// so every word of a given size comes back exactly as tall as every other,
// and a word of another height was set in another size. The slack is for a
// heading, which is set a size or two up from the text under it. It has to
// stay well under the step to the sizes a picture letters with, and those
// are two and three times the text size rather than a quarter up.
//
// The two tests need each other. A word turned on its side comes back as a
// box as tall as the word is long, so a short one is the height of a line of
// text by accident; and the lettering of a figure that starts at the column
// edge is not rare. Together they have nothing to catch on: on the three
// appendix pages the only flush words are the heading and the label, and the
// label is twice the size.
func prose(l poppler.TextLine, c *Caption, body float64) (float64, bool) {
	if body <= 0 {
		return 0, false
	}
	if caption.MatchString(strings.TrimSpace(l.Text())) {
		return l.YMax, true
	}
	bottom, flush := 0.0, false
	for _, w := range l.Words {
		h := w.Height()
		if h < body/sizeSlack || h > body*sizeSlack {
			continue
		}
		bottom = max(bottom, w.YMax)
		// One line height off the column edge, which is about one em. A
		// paper that indents the first line of a paragraph indents it by
		// that much, and the lines after it are flush anyway.
		if w.XMin-c.Box.XMin < body {
			flush = true
		}
	}
	return bottom, flush
}

// sizeSlack is how far a word's height may differ from the paper's own, as a
// ratio either way. A quarter covers a heading set a size or two up and
// stops short of the half again that is the nearest a picture's lettering
// has come to the text size on this corpus.
const sizeSlack = 1.25
