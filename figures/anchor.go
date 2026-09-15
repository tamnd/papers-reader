package figures

import (
	"math"
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
//
// Find will not look at a page carrying less than minLines of text, because
// on such a page the white space is the page rather than a hole in a column
// and the band around the caption is the whole sheet. This pass has no such
// floor. It is not reading white space, it is reading a caption, and a page
// of two charts and six lines of type is exactly the page that needs it:
// the TPU paper has three of them and Find sees nothing on any of them. The
// plate is guarded against directly instead, by refusing a band that is
// most of the sheet.
func Anchor(p poppler.Layout, f *extract.Furniture, frame Frame, caps []Caption, found []Found) []Found {
	lines := f.Lines(p)
	if len(lines) == 0 || p.Width <= 0 || p.Height <= 0 {
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
		if at >= 0 && out[at].Box.YMax <= c.Box.YMin && !sibling(caps, c) {
			// The first pass found a region above this caption, which is
			// where a figure's caption says its figure is. That region is a
			// hole in the column and this would be a guess, so it wins.
			//
			// Unless there is a second caption beside this one. Then the
			// hole is the hole both figures left and the first pass has
			// found the pair of them as one region, so the guess is the
			// better answer: it is bounded sideways by the caption, and the
			// caption of one of two figures set side by side is under that
			// figure and not under both.
			continue
		}
		band, ok := above(p, lines, cuts, frame, caps, c, head)
		if !ok || len(keep([]Candidate{band}, p)) == 0 || claimed(out, c, band.Box) {
			continue
		}
		// Most of the sheet is a plate and not a figure, and the difference
		// matters because a plate is somebody else's page republished. It is
		// refused again after the render, on the ink rather than on the box,
		// and that is the check that counts. This one is here because a page
		// with nothing on it but a caption is the one page this pass has no
		// evidence about, and rendering it to find that out is work done to
		// reach a foregone conclusion.
		if Fraction(band.Box, p) > MaxFraction {
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

// room is how far out the region for c may be widened before it reaches
// the figure next to it.
//
// Two figures set side by side have nothing between them but white space, so
// the clipping against type does not see them and each of the two widens
// across the other. Both regions then hold both pictures, they render to the
// same bytes, and the second is thrown out as a repeat: page 3 and page 4 of
// the TPU paper and page 64 of the GPT-3 paper each lost a figure that way.
//
// Their captions are side by side too, and a caption sits under its own
// figure, so the caption beside this one says where the other figure is. The
// line between them is drawn halfway across the white between the two
// captions, because neither picture is the width of the caption under it and
// stopping at the other caption's edge leaves each region holding a slice of
// its neighbour. On page 64 of the GPT-3 paper the captions of H.2 and H.3
// are 88 points apart and both charts run into that gap, so the halfway line
// at 261.9 is the only place either of them can be cut.
//
// Where there is no caption beside this one the limits are open and the
// region widens to the measure.
func room(caps []Caption, c *Caption) (lo, hi float64) {
	lo, hi = math.Inf(-1), math.Inf(1)
	for i := range caps {
		o := &caps[i]
		if o == c || o.Page != c.Page {
			continue
		}
		if o.Box.YMin >= c.Box.YMax || o.Box.YMax <= c.Box.YMin {
			continue
		}
		if o.Box.XMax < c.Box.XMin {
			lo = max(lo, (o.Box.XMax+c.Box.XMin)/2)
		}
		if o.Box.XMin > c.Box.XMax {
			hi = min(hi, (c.Box.XMax+o.Box.XMin)/2)
		}
	}
	return lo, hi
}

// sibling says another caption is set beside this one, level with it and
// clear of it, which is two figures across the measure rather than one.
//
// The same shape as alone reads a row of panel labels with, and for the same
// reason: type at one height with air between it is two things and not one.
// What it means here is that the region over the pair belongs to neither of
// them whole.
func sibling(caps []Caption, c *Caption) bool {
	for i := range caps {
		o := &caps[i]
		if o == c || o.Page != c.Page {
			continue
		}
		if o.Box.YMin < c.Box.YMax && o.Box.YMax > c.Box.YMin && (o.Box.XMin > c.Box.XMax || o.Box.XMax < c.Box.XMin) {
			return true
		}
	}
	return false
}

// inside says a region is accounted for by what this pass has just found.
//
// Added up over the regions rather than taken one at a time, because the
// region over two figures set side by side is not mostly either of them and
// is entirely the two of them together.
func inside(b poppler.Box, add []Found) bool {
	share := 0.0
	for _, f := range add {
		share += overlap(b, f.Box)
	}
	return share > half
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
func above(p poppler.Layout, lines []poppler.TextLine, cuts []float64, frame Frame, caps []Caption, c *Caption, head float64) (Candidate, bool) {
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
	body := bodyHeight(mine, c)

	// Every column's own left edge, because a line is prose or picture by
	// the measure of the column it is set in and not by the measure of the
	// one the caption is in.
	byColumn := make(map[int][]poppler.TextLine)
	for _, l := range lines {
		at := extract.Column(l, cuts)
		byColumn[at] = append(byColumn[at], l)
	}
	edges := make(map[int]float64, len(byColumn))
	widths := make(map[int]float64, len(byColumn))
	for at, in := range byColumn {
		e, ok := margin0(in)
		if !ok {
			continue
		}
		edges[at] = e
		if r, ok := margin1(in); ok && r > e {
			widths[at] = r - e
		}
	}
	edge := c.Box.XMin
	if e, ok := edges[col]; ok {
		edge = min(edge, e)
	}

	// Half a line above the caption, because a caption's first line and the
	// bottom row of the picture can round to the same point.
	//
	// A line of this paper's own text, not the median gap between the lines
	// of this page. The pages this pass is for are nearly all picture, and on
	// one of them the median gap is not a line of anything. Page 67 of the
	// GPT-3 paper is two charts and two captions, and pdftotext reads four
	// lines on it whose median gap is 425 points, more than half the sheet.
	// A margin of that is a margin wider than the figure: the region for
	// figure H.11 started 170 points below where the chart does and stopped
	// 170 points above its own caption, and what was left was too small to
	// keep. The caption is set in the paper's text face, so the height of a
	// word of it is a line of the paper wherever the pass runs.
	//
	// Over the whole page and not only the caption's own column, because a
	// column is where pdftotext put a line and not where the paper set it.
	// Page 5 of the ResNet paper runs the caption of table 1 across both
	// columns and the run comes back as two lines, the first the width of
	// the page and the second two thirds of it, and the two are read into
	// different columns. Reading the caption's column alone, the region for
	// figure 4 began three points above the second line and committed it.
	//
	// What the line does have to do is stand over the caption. A column of
	// prose beside the figure is not above it and has no business saying
	// where it begins: page 10 of the MapReduce paper sets figure 4 in the
	// right column with section 5.4 running down the left, and every line of
	// 5.4 that happened to fall in a gap between two of the chart's axis
	// labels stopped the region dead. Those lines are dealt with lower down,
	// by bringing the region's side in, which is the answer to where a thing
	// beside the figure leaves off.
	ceiling := c.Box.YMin - body/2
	stop := head
	for _, l := range lines {
		if l.YMax > ceiling || !alone(l, lines) {
			continue
		}
		if l.XMax <= c.Box.XMin || l.XMin >= c.Box.XMax {
			continue
		}
		e, ok := edges[extract.Column(l, cuts)]
		if !ok {
			e = edge
		}
		if bottom, ok := prose(l, e, body); ok {
			stop = max(stop, bottom)
		}
	}
	top, floor := stop+margin*body, c.Box.YMin-margin*body

	// The gap over the caption is there so the caption does not end up in
	// its own figure, and it is a gap and not a cut. A figure's last row is
	// often set right on top of its caption, and taking a fixed margin off
	// the bottom takes half of that row with it: figure 7 of the ResNet
	// paper is labelled "layer index (sorted by magnitude)" half a point
	// above the caption, and what was committed had the lettering sheared
	// through the middle.
	//
	// So the floor comes back down to the foot of anything standing wholly
	// above the caption and over the figure. Prose is left out of it, since
	// a line of the paper above the caption is the paper and not the figure,
	// and the whole point of the gap is to keep it out.
	for _, l := range lines {
		if l.YMax > c.Box.YMin || l.YMax <= floor || l.YMin < top {
			continue
		}
		if l.XMax <= c.Box.XMin || l.XMin >= c.Box.XMax {
			continue
		}
		e, ok := edges[extract.Column(l, cuts)]
		if !ok {
			e = edge
		}
		if _, is := prose(l, e, body); is {
			continue
		}
		floor = max(floor, l.YMax)
	}

	// How wide, taken from the lines that are wholly inside the region and
	// from the caption. Two things it is not taken from, and each of them
	// cost a paper a readable figure.
	//
	// Not the column's whole extent. A page read with a title or an author
	// list across the top of its columns has that line in one of them, and a
	// region as wide as that reaches into the column next door and renders
	// its prose into the PNG. The ResNet paper's first page is that page.
	//
	// Not the lines the region merely overlaps either, because the line the
	// region stopped at is one of those. Its descenders hang into the top of
	// the region, and on that same ResNet page the line is the authors'
	// email addresses, which reach 113 points further left than the plot
	// under them.
	//
	// The caption is in, because it is often the widest thing under a narrow
	// figure and because it is certainly the right column.
	left, right := c.Box.XMin, c.Box.XMax
	for _, l := range mine {
		if l.YMin < top || l.YMax > floor {
			continue
		}
		left = min(left, l.XMin)
		right = max(right, l.XMax)
	}

	// And then out to the paper's own measure, as far as there is room for
	// it. A picture is not obliged to be as wide as the caption under it and
	// is usually wider: the eleven charts in appendix H of the GPT-3 paper
	// are the full measure of the page under captions a third of it, and a
	// box the width of the caption cuts two thirds of every one of them off.
	//
	// It costs nothing to ask for more than the picture. The render is
	// trimmed back to the ink, so a box wider than what is drawn in it comes
	// out the same size either way. What it must not reach is type that
	// belongs to something else, so it stops short of anything set level
	// with it, which on a two column page is the column next door.
	if !frame.Empty() {
		l, r := min(left, frame.Left), max(right, frame.Right)
		for _, o := range lines {
			if o.YMax <= top || o.YMin >= floor {
				continue
			}
			if o.XMax <= left {
				l = max(l, o.XMax+margin*body)
			}
			if o.XMin >= right {
				r = min(r, o.XMin-margin*body)
			}
		}
		lo, hi := room(caps, c)
		left, right = min(left, max(l, lo)), max(right, min(r, hi))
	}

	// Last, off anything beside the region that is the paper talking.
	//
	// The stop above the region ignores a line with something set next to
	// it, because a column sets one line at a time and two pieces of type at
	// the same height are not one sentence. That is the right answer to the
	// question of where the region ends, and it leaves the other question
	// open: the thing beside the figure is still the paper, and the region
	// must not be drawn over it.
	//
	// Page 4 of the ResNet paper is the page that says so. Figure 3 is three
	// network diagrams down the left two fifths of the sheet, section 3.4 is
	// set in a column beside them, and the caption runs the full measure
	// under both. The caption is the only thing the width can be seeded from
	// there, so the region started out the width of the page, and every line
	// of section 3.4 was rendered into the PNG.
	//
	// Which side to come in from is settled by the caption's middle, because
	// a figure is set over the caption that names it. A line that straddles
	// the middle is not something the region can be brought in past, and
	// there is nothing to do about it here.
	//
	// And the line has to run its column's measure before the region is
	// brought in off it. Coming in is the destructive answer here: a stop too
	// low leaves white at the top of a PNG and a cut too far in takes a third
	// of the picture away, so this wants better evidence than the vertical
	// stop does and the measure is it. A paragraph of the paper reaches the
	// right hand edge of its column on every line but the last, and a figure
	// drawn out of type reaches it on none.
	//
	// Without that the test is only that the line is short and flush, which a
	// figure made of type satisfies everywhere. The formatted dataset examples
	// in appendix G of the GPT-3 paper are a label in a narrow left column and
	// the example itself in a wide right one, and the short last line of an
	// example, "the sun was rising." under figure G.5 on page 52, took the
	// region's left edge from 179.2 to 272.2 and cut the first third off every
	// line of the example above it. It is 89 points in a column that measures
	// 329, and section 3.4 on page 4 of the ResNet paper, which this has to go
	// on cutting, runs all 236 points of its own.
	mid := (c.Box.XMin + c.Box.XMax) / 2
	for _, o := range lines {
		// Inside the region rather than touching it. The line the region
		// stopped at is still hanging into the top of it by the depth of
		// whatever the tallest thing on that line was, and it is not beside
		// the figure, it is over it. On page 1 of the ResNet paper that line
		// is the authors' email addresses, set in braces that make the line
		// twice the height of its words, and coming in off it took the left
		// half of figure 1 away.
		if (o.YMin+o.YMax)/2 <= top || (o.YMin+o.YMax)/2 >= floor {
			continue
		}
		e, ok := edges[extract.Column(o, cuts)]
		if !ok {
			continue
		}
		if _, is := prose(o, e, body); !is {
			continue
		}
		if w, ok := widths[extract.Column(o, cuts)]; !ok || o.Width() < filled*w {
			continue
		}
		if o.XMax <= mid {
			left = max(left, o.XMax+margin*body)
		}
		if o.XMin >= mid {
			right = min(right, o.XMin-margin*body)
		}
	}
	b := poppler.Box{
		XMin: left,
		YMin: top,
		XMax: right,
		YMax: floor,
	}
	if b.Height() <= 0 || b.Width() <= 0 {
		return Candidate{}, false
	}
	return Candidate{Page: p.Number, Box: b, Method: Vector}, true
}

// alone says nothing else in the column is set beside this line, which is
// the plainest thing there is to say about a column of text: it sets one
// line at a time, top to bottom, and two pieces of type at the same height
// with clear air between them are not two halves of a sentence.
//
// What they are is the row of labels under a figure of several panels. The
// MapReduce paper's figure 3 is six plots and the row under them reads "(a)
// Normal execution", "(b) No backup tasks", "(c) 200 tasks killed", in the
// paper's own face at the paper's own size, and the first of them begins at
// the column's left edge. Every test prose has says prose. This one says
// there are two more of it across the page.
//
// Clear air matters. A line pdftotext split in two because a tall symbol
// sat in the middle of it leaves two boxes that overlap each other sideways
// as well, and that is one line of the paper being read twice rather than
// two things side by side.
func alone(l poppler.TextLine, lines []poppler.TextLine) bool {
	for _, o := range lines {
		if o.Box == l.Box {
			continue
		}
		if o.YMin < l.YMax && o.YMax > l.YMin && (o.XMin > l.XMax || o.XMax < l.XMin) {
			return false
		}
	}
	return true
}

// bodyHeight is how tall a word of this paper's text is, measured off the
// caption, which is the one thing on the page that is certainly the paper
// and is set in the paper's own face at the paper's own size.
//
// It is measured here rather than taken from the page as a whole because on
// the pages this pass is for, most of the lines are the picture.
//
// Words rather than lines, because a line is as tall as the tallest thing in
// it and one tall thing is all it takes. The TPU paper's captions carry a
// zero width space between the figure number and the title, set 15.6 points
// tall in a caption whose words are 10. Read off the lines the paper's text
// height came out half again too big, every word on the page was then the
// wrong size to be text, and the region above the caption grew to the whole
// sheet because there was nothing left to stop it. What prose compares
// against is a word, so what this measures is a word.
func bodyHeight(lines []poppler.TextLine, c *Caption) float64 {
	var hs []float64
	for _, l := range lines {
		if l.YMin >= c.Box.YMin-1 && l.YMax <= c.Box.YMax+1 {
			for _, w := range l.Words {
				hs = append(hs, w.Height())
			}
		}
	}
	if len(hs) == 0 {
		for _, l := range lines {
			for _, w := range l.Words {
				hs = append(hs, w.Height())
			}
		}
	}
	if len(hs) == 0 {
		return 0
	}
	sort.Float64s(hs)
	return hs[len(hs)/2]
}

// margin1 is the right hand edge of a column of type, read off the type the
// same way margin0 reads the left. It is the point the most lines of the
// column end on, which on justified text is every line but the last of each
// paragraph, and on a column of ragged setting or of a figure's labels is
// nothing in particular.
//
// Three lines have to agree before it is believed, and a column where none do
// has no measure to speak of, which is the answer the caller wants there.
func margin1(lines []poppler.TextLine) (float64, bool) {
	at := make(map[int]int)
	for _, l := range lines {
		at[int(l.XMax+0.5)]++
	}
	best, count := 0, 0
	for x, n := range at {
		if n > count || (n == count && x > best) {
			best, count = x, n
		}
	}
	if count < 3 {
		return 0, false
	}
	return float64(best), true
}

// filled is how much of its column's measure a line has to run before the
// region beside it is brought in off it, as a share. Three fifths is well
// past the short last line of a paragraph and well short of a full one.
const filled = 0.6

// margin0 is the left edge of a column of type, read off the type itself.
//
// prose asks how far a line starts from the column's edge, and until this
// was written it asked how far the line started from the caption. That is
// the same question only while the caption is flush left. A centred caption
// is not, and the MapReduce paper centres its captions: figure 2 on page 8
// is a chart whose y axis is labelled 30000, 20000, 10000 and 0 in the
// paper's own face and size, and every one of those numbers is further left
// than the caption under them. Each read as a line of the paper, the region
// stopped at the last of them, and what was committed was the bottom fifth
// of the chart with the y axis title sliced in half. Against the column's
// real edge they are 32 points in and are what they are, which is the inside
// of a picture.
//
// The edge is the one the most lines of the column share, because a column
// of set type puts every line but an indent on exactly the same point, and
// a picture's parts land wherever the picture puts them. Three lines
// agreeing is asked for before any of this is believed, so a column holding
// nothing but a caption and a chart falls back to the caption and behaves as
// it did before.
func margin0(lines []poppler.TextLine) (float64, bool) {
	at := make(map[int]int)
	for _, l := range lines {
		at[int(l.XMin+0.5)]++
	}
	best, count := 0, 0
	for x, n := range at {
		if n > count || (n == count && x < best) {
			best, count = x, n
		}
	}
	if count < 3 {
		return 0, false
	}
	return float64(best), true
}

// prose says whether a line is the paper talking rather than part of a
// picture, and if it is, how far down the page it reaches.
//
// Two ways it can be. It is another caption, which is a different figure and
// must never be swallowed. Or it opens flush with the column at the size the
// paper sets its text in and reads on from there, which a paragraph's last
// line does and a section heading does and nothing inside a picture does.
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
// The three tests need each other and none of them is enough on its own. A
// word turned on its side comes back as a box as tall as the word is long,
// so a short one is the height of a line of text by accident; and the
// lettering of a figure that starts at the column edge is not rare. The
// MapReduce paper's figure 1 is drawn with type and labelled across the
// bottom in the paper's own face, and the labels begin at the edge of the
// column the centred caption under them was read into. Size and position
// both say prose. What gives them away is the white between them: the gap
// from one label to the next is the width of the picture, and prose does
// not have a hole in it.
func prose(l poppler.TextLine, edge, body float64) (float64, bool) {
	if body <= 0 {
		return 0, false
	}
	if caption.MatchString(strings.TrimSpace(l.Text())) {
		return l.YMax, true
	}
	var mine []poppler.Word
	bottom := 0.0
	for _, w := range l.Words {
		if h := w.Height(); h < body/sizeSlack || h > body*sizeSlack {
			continue
		}
		mine = append(mine, w)
		bottom = max(bottom, w.YMax)
	}
	if len(mine) == 0 {
		return 0, false
	}
	// One line height off the column edge, which is about one em. A paper
	// that indents the first line of a paragraph indents it by that much,
	// and the lines after it are flush anyway.
	//
	// The leftmost word rather than any word, because a word further along
	// the line is at the measure only by accident.
	if mine[0].XMin-edge >= body {
		return bottom, false
	}
	return bottom, run(mine, body)
}

// run says these words read on from one another rather than being scattered
// across the page, which is the difference between a line of the paper and a
// row of a picture's labels. A line with one word on it and nothing else is
// the end of a paragraph and reads on by default.
func run(words []poppler.Word, body float64) bool {
	for i, w := range words[1:] {
		if w.XMin-words[i].XMax > spread*body {
			return false
		}
	}
	return true
}

// spread is the widest gap there may be between two words of one line before
// the line is a row of labels, in ems. Justified text stretches its spaces
// and does not stretch them anything like this far, and the gaps a picture
// leaves between its labels are the width of the picture.
const spread = 4

// sizeSlack is how far a word's height may differ from the paper's own, as a
// ratio either way. A quarter covers a heading set a size or two up and
// stops short of the half again that is the nearest a picture's lettering
// has come to the text size on this corpus.
const sizeSlack = 1.25
