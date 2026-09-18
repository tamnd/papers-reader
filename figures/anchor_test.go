package figures

import (
	"testing"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// tall is a word turned on its side, which is what an attention
// visualisation is made of. pdftotext gives it a box as tall as the word is
// long, which is what tells it apart from the paper's own type.
func tall(x, y0, y1 float64, text string) poppler.TextLine {
	box := poppler.Box{XMin: x, YMin: y0, XMax: x + 14, YMax: y1}
	return poppler.TextLine{Box: box, Words: []poppler.Word{{Box: box, Text: text}}}
}

// picture is one row of a figure drawn with type. It stands off the left
// edge of the column, because the paper's own lines start at that edge and
// a picture's lettering does not.
func picture(y0, y1 float64) []poppler.TextLine {
	var out []poppler.TextLine
	for i, x := 0, colLeft+28; x+14 <= colRight; i, x = i+1, x+30 {
		out = append(out, tall(x, y0, y1, marker(9, i, x)))
	}
	return out
}

// drawn stacks the picture down the page, in rows tall enough that no
// whitespace detector sees a hole between them.
func drawn(from, to float64) []poppler.TextLine {
	var out []poppler.TextLine
	for y := from; y+25 <= to; y += 27 {
		out = append(out, picture(y, y+25)...)
	}
	return out
}

// prosePage is the paper's own type from the top of the frame down to the
// height given, with whatever the test wants under it.
func prosePage(to float64, extra ...poppler.TextLine) poppler.Layout {
	lines := column(9, colLeft, colRight, []poppler.Box{
		{XMin: colLeft, YMin: to, XMax: colRight, YMax: pageTall},
	})
	return pageOf(4, append(lines, extra...))
}

// anchored runs both detectors over one page the way the command does.
func anchored(t *testing.T, pages []poppler.Layout, want int) []Found {
	t.Helper()
	f := extract.FindFurniture(pages)
	frame := FindFrame(pages, f)
	p := pages[want]
	caps := Captions(extract.Read(p, f), f.Lines(p))
	pitch := pitchOf(f.Lines(p))
	return Anchor(p, f, frame, caps, Pair(Find(p, f, frame), caps, pitch))
}

// captioned is the regions of a page that something named, which are the
// only ones that ever become a figure.
func captioned(got []Found) []Found {
	var out []Found
	for _, f := range got {
		if f.Caption != nil {
			out = append(out, f)
		}
	}
	return out
}

func only(t *testing.T, got []Found) Found {
	t.Helper()
	out := captioned(got)
	if len(out) != 1 {
		t.Fatalf("found %d captioned regions, want 1: %+v", len(out), out)
	}
	return out[0]
}

// typeset is the page the whole second pass exists for: a few lines of the
// paper at the top, a section heading, a figure made of sideways words, and
// the caption under it. There is no hole anywhere on it.
func typeset() poppler.Layout {
	head := textLine(colLeft, 204, 220, "6 Attention Visualizations")
	legend := textLine(colLeft, 680, colRight,
		"Figure 3: an invented diagram of an invented thing, drawn with words")
	return prosePage(198, append(append([]poppler.TextLine{head}, drawn(240, 640)...), legend)...)
}

// The figure is the text, so the whitespace detector has nothing to work
// with and the caption is the only evidence the figure is there.
func TestAFigureDrawnWithTypeIsFoundFromItsCaption(t *testing.T) {
	pages := append(plain(), typeset())
	if got := found(t, pages, 3); len(got) != 0 {
		t.Fatalf("the whitespace detector found %d regions on a page with no holes in it, so this fixture is not the case the second pass is for: %+v", len(got), got)
	}
	got := only(t, anchored(t, pages, 3))
	if got.Caption.Number != "3" {
		t.Fatalf("the region is paired with figure %q, want 3", got.Caption.Number)
	}
	if got.Box.YMax > 680 || got.Box.YMax < 655 {
		t.Fatalf("the region ends at y=%.0f, which is not just above the caption at 680", got.Box.YMax)
	}
	if got.Box.XMin > colLeft || got.Box.XMax < colRight {
		t.Fatalf("the region is %.0f to %.0f across, which does not cover the column", got.Box.XMin, got.Box.XMax)
	}
}

// The appendix of the Transformer paper opens with a heading directly above
// the figure. A region that ran past it would put a section heading inside a
// PNG, where no reader can search it and no translator can translate it.
func TestTheHeadingAboveTheFigureIsNotSwallowed(t *testing.T) {
	got := only(t, anchored(t, append(plain(), typeset()), 3))
	// The heading is set at 204 and every line of the fixture is ten points
	// tall, so it ends at 214 and the region has to start below that.
	if got.Box.YMin < 214 {
		t.Fatalf("the region starts at y=%.0f, above the heading that ends at 214", got.Box.YMin)
	}
	if got.Box.YMin > 240 {
		t.Fatalf("the region starts at y=%.0f, which is inside the figure", got.Box.YMin)
	}
}

// Growth stops at the paper's own prose, whether or not there is a heading
// in between.
func TestTheParagraphAboveTheFigureIsNotSwallowed(t *testing.T) {
	legend := textLine(colLeft, 680, colRight, "Figure 3: an invented caption")
	page := prosePage(230, append(drawn(240, 640), legend)...)
	got := only(t, anchored(t, append(plain(), page), 3))
	// The last line of the paragraph sits at 216 and ends at 226.
	if got.Box.YMin < 226 {
		t.Fatalf("the region starts at y=%.0f, inside the paragraph that ends at 226", got.Box.YMin)
	}
}

// A figure drawn with type has short lines in it, and a short line set
// flush with the column is the one thing the sideways clip cannot tell from
// the paper by shape alone. What tells them apart is the measure: a
// paragraph of the paper runs the width of its column and a picture's
// lettering does not.
func TestAShortLineInsideTheFigureDoesNotBringTheRegionIn(t *testing.T) {
	stray := textLine(colLeft, 300, 200, "the sun was rising")
	legend := textLine(colLeft, 680, colRight, "Figure 3: an invented caption")
	page := prosePage(230, append(drawn(240, 640), stray, legend)...)
	got := only(t, anchored(t, append(plain(), page), 3))
	if got.Box.XMin > colLeft {
		t.Fatalf("the region starts at x=%.0f, so the clip came in off a line that is part of the figure", got.Box.XMin)
	}
}

// The right hand edge of a column is where most of its lines end, which on
// justified prose is every line but the last of a paragraph.
func TestTheRightEdgeOfAColumnIsWhereMostLinesEnd(t *testing.T) {
	lines := column(9, colLeft, colRight, nil)
	got, ok := margin1(lines)
	if !ok {
		t.Fatal("a column of justified prose has no right hand edge")
	}
	if got != colRight {
		t.Fatalf("the right hand edge is %.0f, want %.0f", got, colRight)
	}
}

// Ragged setting has no such edge, and a caller that asks for one is told
// there is none rather than given the longest line.
func TestRaggedSettingHasNoRightHandEdge(t *testing.T) {
	var lines []poppler.TextLine
	for i, y := 0, 100.0; y < 300; i, y = i+1, y+leading {
		lines = append(lines, textLine(colLeft, y, colLeft+40+float64(i)*7, marker(9, i, colLeft)))
	}
	if got, ok := margin1(lines); ok {
		t.Fatalf("ragged setting came back with a right hand edge at %.0f", got)
	}
}

// Two charts set across the measure with a caption under each. The lettering
// drawn into them is one row of type running across both, so a region seeded
// from it is as wide as the pair whichever caption it was grown for, and the
// gutter between the two captions is the only thing on the page that says
// where one picture ends and the next begins.
func TestTwoFiguresSideBySideAreNotOnePictureTwice(t *testing.T) {
	mid := (colLeft + colRight) / 2
	extra := append(drawn(240, 640),
		textLine(colLeft, 680, colLeft+48, "Figure 12"),
		textLine(mid+40, 680, mid+88, "Figure 13"))
	got := captioned(anchored(t, append(plain(), prosePage(198, extra...)), 3))
	if len(got) != 2 {
		t.Fatalf("found %d captioned regions, want one for each of the two captions: %+v", len(got), got)
	}
	left, right := got[0], got[1]
	if left.Box.XMin > right.Box.XMin {
		left, right = right, left
	}
	if left.Box.XMax > right.Box.XMin {
		t.Fatalf("the two regions overlap, %v and %v, so both of them hold both charts", left.Box, right.Box)
	}
	if left.Caption.Number != "12" || right.Caption.Number != "13" {
		t.Fatalf("the left region is figure %q and the right one is figure %q, want 12 then 13",
			left.Caption.Number, right.Caption.Number)
	}
}

// A caption above is another figure. Swallowing it would put one figure's
// caption inside the other figure's PNG and lose a caption from the corpus.
func TestACaptionAboveStopsTheGrowth(t *testing.T) {
	var extra []poppler.TextLine
	extra = append(extra, drawn(210, 330)...)
	extra = append(extra, textLine(colLeft, 340, colRight, "Figure 4: the first invented diagram"))
	extra = append(extra, drawn(370, 662)...)
	extra = append(extra, textLine(colLeft, 680, colRight, "Figure 5: the second invented diagram"))
	pages := append(plain(), prosePage(198, extra...))

	got := captioned(anchored(t, pages, 3))
	if len(got) != 2 {
		t.Fatalf("found %d captioned regions, want one for each caption: %+v", len(got), got)
	}
	if got[0].Caption.Number != "4" || got[1].Caption.Number != "5" {
		t.Fatalf("the regions came back as figures %q and %q, want 4 then 5", got[0].Caption.Number, got[1].Caption.Number)
	}
	if got[1].Box.YMin < 350 {
		t.Fatalf("figure 5 starts at y=%.0f, which is over the caption of figure 4 at 340", got[1].Box.YMin)
	}
}

// A table is a Markdown table or it is a hole in the extraction somebody has
// to fill. Growing a region for one only produces a picture of a table that
// nobody should commit.
func TestATableCaptionGrowsNothing(t *testing.T) {
	legend := textLine(colLeft, 680, colRight, "Table 2: an invented table of invented numbers")
	page := prosePage(198, append(drawn(240, 640), legend)...)
	if got := captioned(anchored(t, append(plain(), page), 3)); len(got) != 0 {
		t.Fatalf("%d regions were grown for a table caption: %+v", len(got), got)
	}
}

// The first pass is the better one where it works, because a hole in a
// column is where the picture is and a band grown from a caption is only
// where the picture might be. So a caption it already paired is left alone.
func TestACaptionThatAlreadyHasAFigureIsLeftAlone(t *testing.T) {
	lines := column(9, colLeft, colRight, []poppler.Box{
		{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 492},
	})
	lines = append(lines, textLine(colLeft, 480, colRight, "Figure 3: an invented diagram"))
	pages := append(plain(), pageOf(4, lines))

	first := found(t, pages, 3)
	if len(first) != 1 {
		t.Fatalf("the whitespace detector found %d regions, want the 1 hole that was cut: %+v", len(first), first)
	}
	got := only(t, anchored(t, pages, 3))
	if got.Box != first[0].Box {
		t.Fatalf("the region is %v, want the hole at %v that the first pass had already found", got.Box, first[0].Box)
	}
}

// A figure caption sits under its figure, so a region below it is the first
// pass falling back rather than answering. On the appendix pages of the
// Transformer paper that region is the empty bottom half of the sheet, and
// leaving the caption on it loses the figure and commits nothing.
func TestACaptionIsTakenBackFromTheEmptyRegionBelowIt(t *testing.T) {
	var extra []poppler.TextLine
	extra = append(extra, textLine(colLeft, 204, 220, "6 Attention Visualizations"))
	extra = append(extra, drawn(240, 390)...)
	extra = append(extra, textLine(colLeft, 400, colRight, "Figure 3: an invented diagram"))
	pages := append(plain(), prosePage(198, extra...))

	var below poppler.Box
	for _, f := range Pair(found(t, pages, 3), nil, 12) {
		below = f.Box
	}
	if below.YMin < 400 {
		t.Fatalf("the first pass found no region under the caption, so this fixture does not test what it says it does")
	}

	got := anchored(t, pages, 3)
	one := only(t, got)
	if one.Box.YMax > 400 {
		t.Fatalf("the caption stayed on the empty region at %v", one.Box)
	}
	for _, f := range got {
		if f.Box == below && f.Caption != nil {
			t.Fatal("the empty region below the caption kept it, so one caption names two figures")
		}
	}
}

// A figure drawn with type does leave a little white in it, and that white
// came back from the first pass as a region nobody captioned. Reporting it
// once the figure has been found whole is reporting the same thing twice.
func TestTheWhiteInsideTheFigureIsNotReportedTwice(t *testing.T) {
	var extra []poppler.TextLine
	extra = append(extra, textLine(colLeft, 204, 220, "6 Attention Visualizations"))
	extra = append(extra, drawn(240, 362)...)
	// The space between two panels of the one figure, which is a hole as
	// far as the first pass can tell.
	extra = append(extra, drawn(500, 640)...)
	extra = append(extra, textLine(colLeft, 680, colRight, "Figure 3: an invented diagram"))
	pages := append(plain(), prosePage(198, extra...))

	if len(found(t, pages, 3)) == 0 {
		t.Fatal("the first pass found no hole, so this fixture does not test what it says it does")
	}
	got := anchored(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("got %d regions, want the one figure: %+v", len(got), got)
	}
	if got[0].Caption == nil {
		t.Fatal("the region that survived is the sliver rather than the figure")
	}
}

// A caption with the paper's own text right above it has no room for a
// figure, and the only thing a region there could hold is the white between
// two paragraphs.
func TestACaptionWithNoRoomAboveItGrowsNothing(t *testing.T) {
	lines := column(9, colLeft, colRight, []poppler.Box{
		{XMin: colLeft, YMin: 296, XMax: colRight, YMax: 310},
	})
	lines = append(lines, textLine(colLeft, 300, colRight, "Figure 3: an invented caption"))
	pages := append(plain(), pageOf(4, lines))
	if got := captioned(anchored(t, pages, 3)); len(got) != 0 {
		t.Fatalf("%d regions were grown into solid type: %+v", len(got), got)
	}
}

// A plate with a caption on it and nothing else is a page image. The first
// pass stands down on it, and the second must not walk in and claim the
// whole page.
func TestTheSecondPassLeavesAPlateAlone(t *testing.T) {
	lines := []poppler.TextLine{textLine(colLeft, 700, colRight, "Figure 3: an invented caption")}
	pages := append(plain(), pageOf(4, lines))
	if got := anchored(t, pages, 3); len(got) != 0 {
		t.Fatalf("got %d regions on a page with one line on it: %+v", len(got), got)
	}
}

// The caption of a figure on another page says nothing about this one.
func TestACaptionFromAnotherPageIsIgnored(t *testing.T) {
	pages := append(plain(), typeset())
	f := extract.FindFurniture(pages)
	frame := FindFrame(pages, f)
	p := pages[3]
	caps := Captions(extract.Read(p, f), f.Lines(p))
	if len(caps) == 0 {
		t.Fatal("the fixture has no caption on it")
	}
	for i := range caps {
		caps[i].Page = 99
	}
	if got := Anchor(p, f, frame, caps, nil); len(got) != 0 {
		t.Fatalf("got %d regions from a caption on another page: %+v", len(got), got)
	}
}
