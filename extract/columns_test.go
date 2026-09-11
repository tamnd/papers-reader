package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// The fixtures here are typeset rather than transcribed: a page is built by
// putting words at points, the way the PDF that pdftotext reads holds them.
// No page of any paper in the corpus appears in this file, and none should.
// What is being tested is geometry, and geometry needs no prose.
const (
	pageWidth  = 612
	pageHeight = 792
	fontSize   = 10
	charWidth  = 5
	spaceWidth = 2.5
	linePitch  = 12
)

// put sets one line of words with its left edge at x and its top at y.
func put(x, y float64, s string) poppler.TextLine {
	l := poppler.TextLine{Box: poppler.Box{XMin: x, YMin: y, XMax: x, YMax: y + fontSize}}
	for _, word := range strings.Fields(s) {
		w := poppler.Word{
			Box:  poppler.Box{XMin: x, YMin: y, XMax: x + float64(len(word))*charWidth, YMax: y + fontSize},
			Text: word,
		}
		l.Words = append(l.Words, w)
		l.XMax = w.XMax
		x = w.XMax + spaceWidth
	}
	return l
}

// column sets a run of lines down the page from y, all starting at x.
func column(x, y float64, lines ...string) []poppler.TextLine {
	var out []poppler.TextLine
	for i, s := range lines {
		out = append(out, put(x, y+float64(i)*linePitch, s))
	}
	return out
}

// page puts lines on a page. They all go in one block, because the block
// grouping is pdftotext's guess and nothing in this package reads it.
func page(number int, groups ...[]poppler.TextLine) poppler.Layout {
	p := poppler.Layout{Number: number, Width: pageWidth, Height: pageHeight}
	var b poppler.Block
	for _, g := range groups {
		b.Lines = append(b.Lines, g...)
	}
	p.Blocks = []poppler.Block{b}
	return p
}

// body is enough lines to make a page look like a page. Gutter refuses to
// measure a page with less text on it than this, so every fixture that wants
// to be read as two columns has to be at least this long.
func body(x, y float64, n int, s string) []poppler.TextLine {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = s
	}
	return column(x, y, lines...)
}

func texts(lines []poppler.TextLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text()
	}
	return out
}

func TestATwoColumnPageIsReadDownOneColumnAndThenTheOther(t *testing.T) {
	left := column(60, 100,
		"the first line of the left column here",
		"the second line of the left column here",
		"the third line of the left column here")
	right := column(330, 100,
		"the first line of the right column ok",
		"the second line of the right column ok",
		"the third line of the right column ok")
	left = append(left, body(60, 160, 12, "more of the left column carries on here")...)
	right = append(right, body(330, 160, 12, "more of the right column carries on here")...)

	p := page(1, left, right)
	if got := Gutters(p); len(got) != 1 {
		t.Fatalf("found %d gutters on a two column page, want 1: %v", len(got), got)
	}
	got := texts(Lines(p))
	if len(got) != len(left)+len(right) {
		t.Fatalf("read %d lines, set %d", len(got), len(left)+len(right))
	}
	// The whole point: every left line before every right one.
	for i, s := range got {
		wantLeft := i < len(left)
		if strings.Contains(s, "left") != wantLeft {
			t.Fatalf("line %d is %q, and the columns are interleaved", i, s)
		}
	}
}

func TestASingleColumnPageWithNumbersInTheMarginIsOneColumn(t *testing.T) {
	// The case that a line centre clustering gets wrong. The section numbers
	// hang out to the left of the text, so the centres fall into two clusters
	// with a clear gap between them, and the page is still one column.
	var lines []poppler.TextLine
	for i := 0; i < 20; i++ {
		y := 100 + float64(i)*linePitch
		if i%5 == 0 {
			lines = append(lines, put(60, y, "3.1"))
			continue
		}
		lines = append(lines, put(100, y, "a line of the body that runs the width of the page here"))
	}
	if got := Gutters(page(1, lines)); len(got) != 0 {
		t.Errorf("found a gutter at %v on a single column page", got)
	}
}

func TestAThreeColumnPageIsReadInThree(t *testing.T) {
	one := body(50, 100, 14, "first column line")
	two := body(240, 100, 14, "second column line")
	three := body(430, 100, 14, "third column line")
	p := page(1, one, two, three)
	if got := Gutters(p); len(got) != 2 {
		t.Fatalf("found %d gutters on a three column page, want 2: %v", len(got), got)
	}
	got := texts(Lines(p))
	for i, s := range got {
		var want string
		switch {
		case i < len(one):
			want = "first"
		case i < len(one)+len(two):
			want = "second"
		default:
			want = "third"
		}
		if !strings.Contains(s, want) {
			t.Fatalf("line %d is %q, want one of the %s column", i, s, want)
		}
	}
}

func TestATitleAcrossTheTopStaysOneLineAndComesFirst(t *testing.T) {
	// A title spans the gutter, so the row that holds it crosses the gutter
	// the same way an interleaved pair of column lines does. What tells them
	// apart is the gap at the crossing, and here there is none: the title is
	// set as one line of words with ordinary spaces.
	title := []poppler.TextLine{put(150, 60, "A Title Set Across The Whole Page Width")}
	left := body(60, 120, 14, "left column line of the body")
	right := body(330, 120, 14, "right column line of the body")

	got := texts(Lines(page(1, left, right, title)))
	if len(got) == 0 {
		t.Fatal("nothing was read")
	}
	if got[0] != "A Title Set Across The Whole Page Width" {
		t.Errorf("the page starts %q, want the title whole and first", got[0])
	}
}

func TestAHeadingInOneColumnDoesNotSwallowTheLineBesideIt(t *testing.T) {
	// The failure this package exists to prevent. A heading at the top of
	// the right column sits on the same row as a line of the left column,
	// and pdftotext hands the two back as one line of text.
	left := body(60, 100, 14, "the sentence in the left column ends here")
	right := append([]poppler.TextLine{put(330, 100, "3.1 A Heading")}, body(330, 112, 13, "the right column body")...)

	for _, s := range texts(Lines(page(1, left, right))) {
		if strings.Contains(s, "left") && strings.Contains(s, "Heading") {
			t.Fatalf("a line of the left column was joined to the heading beside it: %q", s)
		}
	}
}

func TestAPageWithAlmostNoTextIsNotMeasured(t *testing.T) {
	// A plate, a part title or a mostly blank verso. Two columns of four
	// lines is not evidence of anything, and guessing costs more than it
	// saves.
	p := page(1, body(60, 100, 3, "a few words"), body(330, 100, 3, "a few words"))
	if got := Gutters(p); got != nil {
		t.Errorf("measured a nearly empty page and found %v", got)
	}
}

func TestAMarginalNoteIsNotAColumn(t *testing.T) {
	// A note in the margin leaves a wide clear channel beside the body, and
	// there is not enough text in it for it to be a column.
	note := column(40, 100, "note", "here")
	text := body(150, 100, 20, "the body of the page runs on for a good many words")
	if got := Gutters(page(1, note, text)); len(got) != 0 {
		t.Errorf("read a marginal note as a column: gutters %v", got)
	}
}
