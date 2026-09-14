package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// The fixtures here are typeset the same way as the rest of this package's,
// by putting words at points. No page of any paper appears in this file.

// blocked puts each group of lines in a block of its own, which is what
// pdftotext does and what this reads. The block's box is the box around its
// lines, because that is the only thing Prose asks a block for.
func blocked(groups ...[]poppler.TextLine) poppler.Layout {
	p := poppler.Layout{Number: 1, Width: pageWidth, Height: pageHeight}
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		b := poppler.Block{Box: g[0].Box, Lines: g}
		for _, l := range g[1:] {
			b.XMin = min(b.XMin, l.XMin)
			b.YMin = min(b.YMin, l.YMin)
			b.XMax = max(b.XMax, l.XMax)
			b.YMax = max(b.YMax, l.YMax)
		}
		p.Blocks = append(p.Blocks, b)
	}
	return p
}

// marker is one word of a diagram, set where the picture wanted it.
func marker(x, y float64, s string) []poppler.TextLine {
	return []poppler.TextLine{put(x, y, s)}
}

func hasWords(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("the prose does not have %q in it:\n%s", want, got)
	}
}

func lacksWords(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("the prose still has %q in it:\n%s", want, got)
	}
}

// The bug this was written for. Page 3 of the MapReduce paper is half a
// diagram, the words inside the diagram are in the file's text layer, a
// correct reading describes the diagram instead of transcribing it, and rule
// A9 read the correct reading as a page with a hole in it.
func TestTheWordsInsideADiagramAreNotProse(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
		marker(120, 300, "Master"),
		marker(200, 320, "worker"),
		marker(260, 340, "shuffle"),
	)
	got := Prose(p, nil)
	hasWords(t, got, "left column")
	hasWords(t, got, "right column")
	lacksWords(t, got, "Master")
	lacksWords(t, got, "worker")
	lacksWords(t, got, "shuffle")
}

// A page with nothing on it but text comes back with all of it. This is the
// common case and the one that must not lose a word.
func TestAPageOfNothingButTextIsUnchanged(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
	)
	if got := len(strings.Fields(Prose(p, nil))); got != 2*12*9 {
		t.Errorf("the prose has %d words, want %d", got, 2*12*9)
	}
}

// A caption is prose and is set to the width of its column, so it stays even
// though what it is captioning does not. The reading is expected to have it.
func TestACaptionStaysWithTheProse(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
		marker(120, 300, "Master"),
		column(60, 360,
			"Figure 1: the parts of the system and what",
			"each of them is holding while it runs."),
	)
	got := Prose(p, nil)
	hasWords(t, got, "Figure 1")
	lacksWords(t, got, "Master")
}

// The column and not the page. A paragraph of a two column paper is half the
// width of the type area, so a threshold measured against the page would
// throw away every paragraph in the paper.
func TestTheWidthIsMeasuredAgainstAColumnAndNotThePage(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
	)
	if len(Gutters(p)) != 1 {
		t.Fatalf("the fixture is not being read as two columns: gutters %v", Gutters(p))
	}
	hasWords(t, Prose(p, nil), "right column")
}

// A one column paper has no channel down it and its paragraphs are the whole
// type area, which is the other end of the same sum.
func TestAOneColumnPageKeepsItsParagraphs(t *testing.T) {
	p := blocked(
		body(60, 100, 14, "a line of the one column this paper is set in ok"),
		marker(300, 320, "Nonce"),
	)
	if len(Gutters(p)) != 0 {
		t.Fatalf("the fixture is not being read as one column: gutters %v", Gutters(p))
	}
	hasWords(t, Prose(p, nil), "one column")
	lacksWords(t, Prose(p, nil), "Nonce")
}

// The known loss, written down so that changing it is a decision. A section
// heading is a short line in a block of its own and goes with the labels.
func TestASectionHeadingGoesWithTheLabels(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
		column(60, 260, "3 Implementation"),
	)
	lacksWords(t, Prose(p, nil), "Implementation")
}

// The second half of the same bug. With Figure 1 out of the way, what was
// left of page 3 of the MapReduce paper was the line the journal prints
// along the bottom of every page, which no reader transcribes and which is
// as wide as the type area, so the width test keeps it.
func TestTheRunningFootIsNotProse(t *testing.T) {
	var pages []poppler.Layout
	for i := 1; i <= 6; i++ {
		p := blocked(
			body(60, 100, 12, "a line of the left column of the paper"),
			body(330, 100, 12, "a line of the right column of the paper"),
			column(60, 740, "Proceedings of the Symposium on Operating Systems"),
		)
		p.Number = i
		pages = append(pages, p)
	}
	f := FindFurniture(pages)
	got := Prose(pages[1], f)
	hasWords(t, got, "left column")
	lacksWords(t, got, "Proceedings")
}

// A running head is learned from the paper and not from the page, so a paper
// handed over one page at a time keeps its head. The alternative is a rule
// that quietly stops looking at the top and the bottom of every page.
func TestOnePageOnItsOwnKeepsItsHead(t *testing.T) {
	p := blocked(
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
		column(60, 740, "Proceedings of the Symposium on Operating Systems"),
	)
	hasWords(t, Prose(p, FindFurniture([]poppler.Layout{p})), "Proceedings")
}

// The defect the width sum was rewritten for. A figure whose labels are
// scattered across the channel leaves Gutters nothing clear to find, so a two
// column page reads as one, a column comes back at twice its width and every
// real paragraph is measured against half of that and thrown away. Prose came
// back empty on MapReduce page 3 and rule A9 stopped running on the one page
// it was written for.
func TestAFigureAcrossTheChannelDoesNotTakeTheParagraphsWithIt(t *testing.T) {
	groups := [][]poppler.TextLine{
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
	}
	// Set across the channel and stacked down it. One label to a bin is not
	// enough: a bin counts as clear while it holds under a share of the ink
	// in the busiest bin on the page, and one label against twelve lines of
	// body is under it. A diagram crosses a channel repeatedly.
	for _, x := range []float64{230, 260, 290, 320} {
		for i := range 6 {
			groups = append(groups, marker(x, 300+float64(i)*14, "worker"))
		}
	}
	p := blocked(groups...)
	if len(Gutters(p)) != 0 {
		t.Fatalf("the fixture no longer hides the channel: gutters %v", Gutters(p))
	}
	got := Prose(p, nil)
	hasWords(t, got, "left column")
	hasWords(t, got, "right column")
	lacksWords(t, got, "worker")
}

// The other direction, and the reason the two sums are combined by taking the
// smaller. A page that repeats something wider than a column, a title set over
// a full width table, would have the repeated width believe the page is one
// column wide. The span knows better here, because the channel is clear.
func TestSomethingWiderThanAColumnRepeatingDoesNotWidenTheColumn(t *testing.T) {
	wide := "a full width line running right across the type area of this page"
	p := blocked(
		column(60, 60, wide),
		column(60, 740, wide),
		body(60, 100, 12, "a line of the left column of the paper"),
		body(330, 100, 12, "a line of the right column of the paper"),
		marker(200, 400, "Master"),
	)
	got := Prose(p, nil)
	hasWords(t, got, "left column")
	hasWords(t, got, "right column")
	lacksWords(t, got, "Master")
}

// A plate with one caption repeats nothing, so the span is all there is.
func TestAPageWhereNoTwoBlocksAreAlikeFallsBackToTheSpan(t *testing.T) {
	p := blocked(
		marker(200, 200, "Nonce"),
		column(60, 700, "Figure 4: the whole of the apparatus as it was built."),
	)
	if _, ok := repeatedWidth(p); ok {
		t.Fatal("the fixture has two blocks of the same width after all")
	}
	hasWords(t, Prose(p, nil), "Figure 4")
}

func TestAPageWithNoBlocksIsNoProse(t *testing.T) {
	if got := Prose(poppler.Layout{Number: 1, Width: pageWidth, Height: pageHeight}, nil); got != "" {
		t.Errorf("an empty page has prose %q", got)
	}
}

// A full page plate has a caption and nothing else, and the caption is the
// widest thing on it, so it is what one column means and it stays. Coverage
// reads a page with almost nothing to compare as covered, which is right: a
// picture is not missing prose.
func TestAFullPagePlateKeepsItsCaption(t *testing.T) {
	p := blocked(
		marker(200, 200, "Nonce"),
		marker(280, 240, "Hash"),
		column(60, 700, "Figure 4: the whole of the apparatus as it was built."),
	)
	got := Prose(p, nil)
	hasWords(t, got, "Figure 4")
	lacksWords(t, got, "Nonce")
}

func TestThePublishersPermissionNoticeIsNotProse(t *testing.T) {
	// No reading transcribes it and it is not the paper, so leaving it in the
	// layer is forty five consecutive words that A9 will score at nothing.
	// Both wordings, because ACM has used both and the corpus has both.
	for _, notice := range []string{
		"Permission to make digital or hard copies of all or part of this " +
			"work for personal or classroom use is granted without fee " +
			"provided that copies are not made or distributed for profit",
		"Permission to copy without fee all or part of this material is " +
			"granted provided that the copies are not made or distributed " +
			"for direct commercial advantage",
	} {
		p := page(1,
			body(60, 100, 20, "a line of the body of the paper that runs on here"),
			column(60, 400, strings.Fields(notice)...))
		p.Blocks = []poppler.Block{{Lines: p.Blocks[0].Lines[:20]}, {Lines: p.Blocks[0].Lines[20:]}}
		for i := range p.Blocks {
			p.Blocks[i].Box = box(p.Blocks[i].Lines)
		}
		got := Prose(p, FindFurniture([]poppler.Layout{p}))
		if strings.Contains(strings.ToLower(got), "permission") {
			t.Errorf("the permission notice is still in the prose:\n%s", got)
		}
		if !strings.Contains(got, "a line of the body") {
			t.Errorf("the body went with it:\n%s", got)
		}
	}
}

// box is the smallest box holding every line, which is what pdftotext reports
// for a block and what the fixtures here have to supply themselves.
func box(lines []poppler.TextLine) poppler.Box {
	b := lines[0].Box
	for _, l := range lines[1:] {
		b.XMin, b.YMin = min(b.XMin, l.XMin), min(b.YMin, l.YMin)
		b.XMax, b.YMax = max(b.XMax, l.XMax), max(b.YMax, l.YMax)
	}
	return b
}
