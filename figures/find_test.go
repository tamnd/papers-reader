package figures

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// filler is a line of the fixture papers. Invented, because a test that
// quotes a paper is a copy of one.
const filler = "the quick brown fox jumps over the lazy dog and then does it again"

// textLine is one line of type at a given place on the page.
func textLine(x0, y, x1 float64, text string) poppler.TextLine {
	box := poppler.Box{XMin: x0, YMin: y, XMax: x1, YMax: y + 10}
	words := strings.Fields(text)
	step := (x1 - x0) / float64(max(len(words), 1))
	out := poppler.TextLine{Box: box}
	for i, w := range words {
		out.Words = append(out.Words, poppler.Word{
			Box: poppler.Box{
				XMin: x0 + float64(i)*step, YMin: y,
				XMax: x0 + float64(i+1)*step, YMax: y + 10,
			},
			Text: w,
		})
	}
	return out
}

// column sets solid type down one column, leaving out any line that would
// fall in one of the holes.
//
// Every line is given a marker of its own, because the furniture detector
// deletes any line in the margin that says the same thing on every page and
// a fixture of identical pages would lose its first and last lines to it.
func column(n int, x0, x1 float64, holes []poppler.Box) []poppler.TextLine {
	var out []poppler.TextLine
	for i, y := 0, topLine; y+10 <= botLine; i, y = i+1, y+leading {
		if hollow(y, y+10, x0, x1, holes) {
			continue
		}
		out = append(out, textLine(x0, y, x1, marker(n, i, x0)+" "+filler))
	}
	return out
}

// marker is a word no other line on any other page of the fixture has.
func marker(n, i int, x0 float64) string {
	return fmt.Sprintf("%c%c%c", 'a'+n%26, 'a'+i%26, 'a'+int(x0)%26)
}

func hollow(y0, y1, x0, x1 float64, holes []poppler.Box) bool {
	for _, h := range holes {
		if y1 > h.YMin && y0 < h.YMax && x1 > h.XMin && x0 < h.XMax {
			return true
		}
	}
	return false
}

// onePage is a single column page of solid type with holes cut in it.
func onePage(number int, holes ...poppler.Box) poppler.Layout {
	return pageOf(number, column(number, colLeft, colRight, holes))
}

func pageOf(number int, lines ...[]poppler.TextLine) poppler.Layout {
	p := poppler.Layout{Number: number, Width: pageWide, Height: pageTall}
	for _, set := range lines {
		if len(set) == 0 {
			continue
		}
		p.Blocks = append(p.Blocks, poppler.Block{Box: hull(set), Lines: set})
	}
	return p
}

func hull(lines []poppler.TextLine) poppler.Box {
	out := lines[0].Box
	for _, l := range lines[1:] {
		out.XMin = min(out.XMin, l.XMin)
		out.YMin = min(out.YMin, l.YMin)
		out.XMax = max(out.XMax, l.XMax)
		out.YMax = max(out.YMax, l.YMax)
	}
	return out
}

// found runs the detector over a paper the way the command does: the
// furniture and the frame are learned from every page, then each page is
// scanned.
func found(t *testing.T, pages []poppler.Layout, want int) []Candidate {
	t.Helper()
	f := extract.FindFurniture(pages)
	frame := FindFrame(pages, f)
	got := Find(pages[want], f, frame)
	return got
}

// plain is three pages of solid type, which is what the frame is measured
// from. A one page fixture would have no frame and would exercise the
// fallback rather than the path every real paper takes.
func plain() []poppler.Layout {
	return []poppler.Layout{onePage(1), onePage(2), onePage(3)}
}

func TestAHoleInAColumnIsACandidate(t *testing.T) {
	hole := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 420}
	pages := append(plain(), onePage(4, hole))
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want 1: %+v", len(got), got)
	}
	if got[0].Box.YMin < 295 || got[0].Box.YMax > 425 {
		t.Fatalf("the candidate is %v, which is not the hole that was cut", got[0].Box)
	}
}

func TestSolidTypeHasNoFigureInIt(t *testing.T) {
	pages := plain()
	if got := found(t, pages, 1); len(got) != 0 {
		t.Fatalf("found %d candidates on a page of solid type: %+v", len(got), got)
	}
}

// The gap above a section heading is two lines and is not a figure. This is
// the whole reason there is a floor on the height of a band.
func TestTheGapRoundAHeadingIsNotAFigure(t *testing.T) {
	gap := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 324}
	pages := append(plain(), onePage(4, gap))
	if got := found(t, pages, 3); len(got) != 0 {
		t.Fatalf("found %d candidates in a two line gap: %+v", len(got), got)
	}
}

// A figure at the head of a page leaves no hole measured from that page's
// own text, because there is no text above it. It is found against the
// frame the paper keeps on its other pages.
func TestAFigureAtTheTopOfAPageIsFound(t *testing.T) {
	hole := poppler.Box{XMin: colLeft, YMin: 0, XMax: colRight, YMax: 300}
	pages := append(plain(), onePage(4, hole))
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want 1: %+v", len(got), got)
	}
	// It runs up to the top of the page and not only to the top of the
	// frame, because a figure is not obliged to stay inside the type area
	// and the render is trimmed back to the ink afterwards.
	if got[0].Box.YMin > 10 {
		t.Fatalf("the candidate starts at y=%.0f, so the top of the figure is cut off", got[0].Box.YMin)
	}
}

func TestAFigureAtTheFootOfAPageIsFound(t *testing.T) {
	hole := poppler.Box{XMin: colLeft, YMin: 500, XMax: colRight, YMax: pageTall}
	pages := append(plain(), onePage(4, hole))
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want 1: %+v", len(got), got)
	}
	if got[0].Box.YMax < botLine {
		t.Fatalf("the candidate ends at y=%.0f, so the foot of the figure is cut off", got[0].Box.YMax)
	}
}

// A figure whose panels are titled has the titles inside it, and a detector
// that stops at every line of text crops them out of the middle of the one
// figure that is really there.
func TestAFigureIsNotCutInHalfByItsOwnTitle(t *testing.T) {
	holes := []poppler.Box{
		{XMin: colLeft, YMin: 200, XMax: colRight, YMax: 320},
		{XMin: colLeft, YMin: 332, XMax: colRight, YMax: 460},
	}
	lines := column(4, colLeft, colRight, holes)
	// The title sits in the gap between the two holes, set in from both
	// edges of the column the way a label over a panel is.
	lines = append(lines, textLine(200, 320, 400, "left panel"))
	pages := append(plain(), pageOf(4, lines))
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want the two holes joined into 1: %+v", len(got), got)
	}
	if got[0].Box.YMin > 205 || got[0].Box.YMax < 455 {
		t.Fatalf("the join came to %v, which does not cover both holes", got[0].Box)
	}
}

// A section heading between two figures is not lettering, and joining
// across it would put a line of the paper inside a PNG.
func TestTwoFiguresAreNotJoinedAcrossAHeading(t *testing.T) {
	holes := []poppler.Box{
		{XMin: colLeft, YMin: 200, XMax: colRight, YMax: 320},
		{XMin: colLeft, YMin: 332, XMax: colRight, YMax: 460},
	}
	lines := column(4, colLeft, colRight, holes)
	lines = append(lines, textLine(colLeft, 320, 260, "3 Method"))
	pages := append(plain(), pageOf(4, lines))
	if got := found(t, pages, 3); len(got) != 2 {
		t.Fatalf("found %d candidates, want 2 kept apart by the heading: %+v", len(got), got)
	}
}

// The caption is the one line that is never swallowed, whatever it looks
// like. It is translated separately, and a caption inside the PNG is a
// caption no reader can read in their own language.
func TestTwoFiguresAreNotJoinedAcrossACaption(t *testing.T) {
	holes := []poppler.Box{
		{XMin: colLeft, YMin: 200, XMax: colRight, YMax: 320},
		{XMin: colLeft, YMin: 332, XMax: colRight, YMax: 460},
	}
	lines := column(4, colLeft, colRight, holes)
	lines = append(lines, textLine(180, 320, 430, "Figure 4: the first of the two"))
	pages := append(plain(), pageOf(4, lines))
	if got := found(t, pages, 3); len(got) != 2 {
		t.Fatalf("found %d candidates, want 2 kept apart by the caption: %+v", len(got), got)
	}
}

// A figure set across both columns of a two column page is a hole in both
// of them at the same height, and committing it as two half figures would
// cut the diagram down the middle.
func TestABandAcrossTwoColumnsIsOneFigure(t *testing.T) {
	const gutter = 306.0
	hole := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 430}
	twoUp := func(n int, holes ...poppler.Box) poppler.Layout {
		return pageOf(n,
			column(n, colLeft, gutter-6, holes),
			column(n, gutter+6, colRight, holes),
		)
	}
	pages := []poppler.Layout{twoUp(1), twoUp(2), twoUp(3), twoUp(4, hole)}
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want the two halves merged into 1: %+v", len(got), got)
	}
	if got[0].Box.Width() < 400 {
		t.Fatalf("the merged band is %.0f points wide, so only one column of it was kept", got[0].Box.Width())
	}
}

// lower moves a run of lines down the page. The two columns of a real paper
// do not share baselines, and a fixture where they do is read as one column:
// the words of a row are gathered left to right whatever column they are in,
// so two columns set level fill the gutter and there is no gutter to find.
func lower(lines []poppler.TextLine, by float64) []poppler.TextLine {
	out := make([]poppler.TextLine, 0, len(lines))
	for _, l := range lines {
		l.Box.YMin, l.Box.YMax = l.Box.YMin+by, l.Box.YMax+by
		words := make([]poppler.Word, len(l.Words))
		for i, w := range l.Words {
			w.Box.YMin, w.Box.YMax = w.Box.YMin+by, w.Box.YMax+by
			words[i] = w
		}
		l.Words = words
		out = append(out, l)
	}
	return out
}

// A column is assigned to a line by where the middle of the line falls, so a
// title set across the page lands in whichever column its middle happens to
// be in. It is not that column's measure and a figure in that column must not
// be committed at the width of the sheet.
func TestATitleAcrossThePageIsNotAColumnsMeasure(t *testing.T) {
	const gutter = 306.0
	head := poppler.Box{XMin: colLeft, YMin: topLine - 1, XMax: colRight, YMax: topLine + 1}
	hole := poppler.Box{XMin: gutter, YMin: 300, XMax: colRight, YMax: 430}
	titled := func(n int, holes ...poppler.Box) poppler.Layout {
		holes = append(holes, head)
		return pageOf(n,
			[]poppler.TextLine{textLine(colLeft, topLine, colRight, marker(n, 0, colLeft)+" a title across the page")},
			column(n, colLeft, gutter-18, holes),
			lower(column(n, gutter+18, colRight, holes), leading/2),
		)
	}
	pages := []poppler.Layout{titled(1), titled(2), titled(3), titled(4, hole)}
	got := found(t, pages, 3)
	if len(got) != 1 {
		t.Fatalf("found %d candidates, want 1: %+v", len(got), got)
	}
	if got[0].Box.XMin < gutter {
		t.Fatalf("the band runs from %.0f, which is left of the gutter and into the other column", got[0].Box.XMin)
	}
}

// A plate with one line on it is a page image. The rule that keeps this
// package away from it is that a page has to carry some text before a hole
// in it means anything.
func TestAPageWithAlmostNoTextOnItIsLeftAlone(t *testing.T) {
	bare := pageOf(4, []poppler.TextLine{textLine(180, 700, 430, "Figure 9: one line and nothing else")})
	pages := append(plain(), bare)
	if got := found(t, pages, 3); len(got) != 0 {
		t.Fatalf("found %d candidates on a page that is one caption: %+v", len(got), got)
	}
}

// The band test counts lines of the paper's own text, which is the wrong
// unit on a page whose figure is drawn with words in it: the lines are the
// figure, the gaps between them are tiny, and four of those is a sliver.
func TestASliverIsNotAFigure(t *testing.T) {
	hole := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 330}
	pages := append(plain(), onePage(4, hole))
	if got := found(t, pages, 3); len(got) != 0 {
		t.Fatalf("found %d candidates in a 30 point hole: %+v", len(got), got)
	}
}

func TestTheFrameIsWhereThePaperPutsItsType(t *testing.T) {
	pages := plain()
	f := extract.FindFurniture(pages)
	frame := FindFrame(pages, f)
	if frame.Empty() {
		t.Fatal("three pages of solid type produced no frame")
	}
	if frame.Top < topLine-1 || frame.Top > topLine+1 {
		t.Fatalf("frame top = %.1f, want about %.1f", frame.Top, topLine)
	}
	if frame.Bottom < botLine-leading-1 || frame.Bottom > botLine+1 {
		t.Fatalf("frame bottom = %.1f, want about %.1f", frame.Bottom, botLine)
	}
}

func TestAnUnmeasuredFrameIsEmpty(t *testing.T) {
	if !(Frame{}).Empty() {
		t.Fatal("the zero frame says it was measured")
	}
}

func TestThePitchIsTheMedianGapBetweenLines(t *testing.T) {
	lines := column(1, colLeft, colRight, nil)
	if got := pitchOf(lines); got != leading {
		t.Fatalf("pitch = %v, want %v", got, leading)
	}
}
