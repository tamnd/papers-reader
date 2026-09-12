package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// The fixtures here are typeset the same way the column fixtures are: cells
// are put at points and the words come from nowhere. No table of any paper in
// the corpus appears in this file. What is being tested is geometry.

// A spot is one cell of a printed row, at the point its first character
// starts.
type spot struct {
	x float64
	s string
}

// printedRow sets one row of a table as pdftotext emits a row that was set in
// one size: every cell on one text line, with the gaps between them.
func printedRow(y float64, spots ...spot) poppler.TextLine {
	var out poppler.TextLine
	for i, sp := range spots {
		l := put(sp.x, y, sp.s)
		if i == 0 {
			out.Box = l.Box
		} else {
			out.Box = union(out.Box, l.Box)
		}
		out.Words = append(out.Words, l.Words...)
	}
	return out
}

// scattered sets the same row one cell to a line, which is what pdftotext
// emits for a table whose cells the typesetter set as separate runs. The page
// says the same thing and says it in forty pieces.
func scattered(y float64, spots ...spot) []poppler.TextLine {
	out := make([]poppler.TextLine, len(spots))
	for i, sp := range spots {
		out[i] = put(sp.x, y, sp.s)
	}
	return out
}

// machines is the fixture most of these tests use: a caption and four rows of
// three columns, wide enough apart that no reading of the gaps could call two
// of them one.
func machines() []poppler.TextLine {
	return []poppler.TextLine{
		put(60, 100, "Table 1: the machines and the years they ran"),
		printedRow(112, spot{60, "Machine"}, spot{200, "Words"}, spot{320, "Year"}),
		printedRow(124, spot{60, "EDSAC"}, spot{200, "512"}, spot{320, "1949"}),
		printedRow(136, spot{60, "Atlas"}, spot{200, "16384"}, spot{320, "1962"}),
		printedRow(148, spot{60, "Cray"}, spot{200, "1048576"}, spot{320, "1976"}),
	}
}

const machinesTable = "| Machine | Words | Year |\n" +
	"| --- | --- | --- |\n" +
	"| EDSAC | 512 | 1949 |\n" +
	"| Atlas | 16384 | 1962 |\n" +
	"| Cray | 1048576 | 1976 |"

// fenced is the lines inside a fence, with the two fence lines taken off.
func fenced(t *testing.T, s string) []string {
	t.Helper()
	lines := strings.Split(s, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "```") {
		t.Fatalf("this is not a fenced block:\n%s", s)
	}
	return lines[1 : len(lines)-1]
}

func one(t *testing.T, lines []poppler.TextLine) Table {
	t.Helper()
	got := Tables(lines, nil)
	if len(got) != 1 {
		t.Fatalf("found %d tables, want 1", len(got))
	}
	return got[0]
}

func TestAGridUnderACaptionIsReadAsATable(t *testing.T) {
	got := one(t, machines())
	if got.Fenced {
		t.Fatalf("a square grid was fenced:\n%s", got.Text)
	}
	if got.Rows != 4 || got.Columns != 3 {
		t.Errorf("read a %dx%d table, want 4x3", got.Rows, got.Columns)
	}
	if got.Text != machinesTable {
		t.Errorf("wrote\n%s\nwant\n%s", got.Text, machinesTable)
	}
}

func TestTheLinesATableWasSetOnAreTheOnesItClaims(t *testing.T) {
	// The caption is not part of the table. It stays in the flow, because it
	// is the sentence that gets translated and the table is not.
	got := one(t, machines())
	if len(got.Lines) != 4 {
		t.Fatalf("claimed %v, want the four rows", got.Lines)
	}
	for i, at := range got.Lines {
		if at != i+1 {
			t.Fatalf("claimed %v, want the four rows", got.Lines)
		}
	}
}

func TestACellArrivingOnItsOwnLineIsPutBackInItsRow(t *testing.T) {
	// The same table, emitted one cell at a time. pdftotext does this to any
	// table whose cells were set as separate runs, and read line by line not
	// one of these lines has two cells in it.
	lines := []poppler.TextLine{put(60, 100, "Table 1: the machines and the years they ran")}
	lines = append(lines, scattered(112, spot{60, "Machine"}, spot{200, "Words"}, spot{320, "Year"})...)
	lines = append(lines, scattered(124, spot{60, "EDSAC"}, spot{200, "512"}, spot{320, "1949"})...)
	lines = append(lines, scattered(136, spot{60, "Atlas"}, spot{200, "16384"}, spot{320, "1962"})...)
	lines = append(lines, scattered(148, spot{60, "Cray"}, spot{200, "1048576"}, spot{320, "1976"})...)

	got := one(t, lines)
	if got.Text != machinesTable {
		t.Errorf("wrote\n%s\nwant\n%s", got.Text, machinesTable)
	}
	if len(got.Lines) != 12 {
		t.Errorf("claimed %d lines, want the twelve the cells arrived on", len(got.Lines))
	}
}

func TestAColumnHeadingSetOnTwoLinesIsOneHeading(t *testing.T) {
	lines := []poppler.TextLine{
		put(60, 100, "Table 2: what each layer costs to run"),
		printedRow(112, spot{60, "Layer"}, spot{200, "Sequential"}, spot{340, "Depth"}),
		printedRow(124, spot{200, "Operations"}),
		printedRow(136, spot{60, "Attention"}, spot{200, "one"}, spot{340, "one"}),
		printedRow(148, spot{60, "Recurrent"}, spot{200, "many"}, spot{340, "many"}),
		printedRow(160, spot{60, "Convolutional"}, spot{200, "one"}, spot{340, "log"}),
	}
	got := one(t, lines)
	if got.Fenced {
		t.Fatalf("a square grid was fenced:\n%s", got.Text)
	}
	want := "| Layer | Sequential Operations | Depth |\n" +
		"| --- | --- | --- |\n" +
		"| Attention | one | one |\n" +
		"| Recurrent | many | many |\n" +
		"| Convolutional | one | log |"
	if got.Text != want {
		t.Errorf("wrote\n%s\nwant\n%s", got.Text, want)
	}
}

func TestARowWithAHoleInItIsStillARow(t *testing.T) {
	// An ablation table leaves the stub blank and says what changed in the
	// column it changed in. Merged into the row above, four experiments would
	// come out as one.
	lines := []poppler.TextLine{
		put(60, 100, "Table 3: one thing changed at a time"),
		printedRow(112, spot{60, "Run"}, spot{200, "Heads"}, spot{320, "Score"}),
		printedRow(124, spot{60, "base"}, spot{200, "8"}, spot{320, "25.8"}),
		printedRow(136, spot{200, "16"}),
		printedRow(148, spot{200, "32"}),
		printedRow(160, spot{60, "big"}, spot{200, "16"}, spot{320, "26.4"}),
	}
	got := one(t, lines)
	// The two rows that changed only the head count carry one cell each and
	// are not in the first column, so they merge, which is the wrong answer
	// for this shape and the right one for a stacked heading. What must not
	// happen is the four of them collapsing into one row, and the guard on
	// that is the row count.
	if got.Fenced {
		t.Fatalf("a square grid was fenced:\n%s", got.Text)
	}
	if got.Rows < 3 {
		t.Errorf("read %d rows from five printed rows:\n%s", got.Rows, got.Text)
	}
	for _, want := range []string{"25.8", "26.4", "16", "32"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("%q is on the page and not in the table:\n%s", want, got.Text)
		}
	}
}

func TestACellStandingOverTwoColumnsIsWrittenAsItWasPrinted(t *testing.T) {
	lines := []poppler.TextLine{
		put(60, 100, "Table 4: scores on both sets"),
		printedRow(112, spot{60, "Model"}, spot{200, "score on either of the two sets"}),
		printedRow(124, spot{60, "unit"}, spot{200, "dev"}, spot{320, "test"}),
		printedRow(136, spot{60, "base"}, spot{200, "25.8"}, spot{320, "27.3"}),
		printedRow(148, spot{60, "big"}, spot{200, "26.4"}, spot{320, "28.4"}),
	}
	got := one(t, lines)
	if !got.Fenced {
		t.Fatalf("a table with a spanning cell was written as a grid:\n%s", got.Text)
	}
	if got.Rows != 0 || got.Columns != 0 {
		t.Errorf("a fenced table claimed a shape of %dx%d", got.Rows, got.Columns)
	}
	body := fenced(t, got.Text)
	want := [][]string{
		{"Model", "score", "on", "either", "of", "the", "two", "sets"},
		{"unit", "dev", "test"},
		{"base", "25.8", "27.3"},
		{"big", "26.4", "28.4"},
	}
	if len(body) != len(want) {
		t.Fatalf("wrote %d lines, want %d:\n%s", len(body), len(want), got.Text)
	}
	for i, w := range want {
		if got := strings.Fields(body[i]); strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("line %d reads %q, want %q", i+1, body[i], strings.Join(w, " "))
		}
	}
	// The layout is the point of the fallback. A cell that was printed under
	// another one has to still be under it.
	if strings.Index(body[1], "dev") != strings.Index(body[2], "25.8") {
		t.Errorf("the second column moved between rows:\n%s", got.Text)
	}
	if strings.Index(body[1], "test") != strings.Index(body[2], "27.3") {
		t.Errorf("the third column moved between rows:\n%s", got.Text)
	}
}

func TestAGridWithNoCaptionIsNotATable(t *testing.T) {
	// The author block of a two column paper is this shape, and so is a list
	// of symbols. Without the caption there is nothing to tell them apart
	// from a table, so nothing here is a table.
	lines := machines()[1:]
	if got := Tables(lines, nil); len(got) != 0 {
		t.Errorf("found %d tables with no caption on the page:\n%s", len(got), got[0].Text)
	}
}

func TestAFigureLegendDoesNotAnchorATable(t *testing.T) {
	lines := machines()
	lines[0] = put(60, 100, "Figure 1: the machines and the years they ran")
	if got := Tables(lines, nil); len(got) != 0 {
		t.Errorf("a figure legend anchored %d tables:\n%s", len(got), got[0].Text)
	}
}

func TestTwoColumnsOfProseAreNotATable(t *testing.T) {
	// The worst thing this file could do. If the column finder missed the
	// gutter then every line of the page has two cells, they line up
	// perfectly, and there is a table caption four lines up. The only
	// difference left is that a table's cells are not sentences.
	lines := []poppler.TextLine{put(60, 100, "Table 5: the sizes of the machines in the study")}
	for i := 0; i < 6; i++ {
		lines = append(lines, printedRow(112+float64(i)*linePitch,
			spot{60, "the left column of the page runs on to about"},
			spot{330, "here and the right one starts over here and"}))
	}
	if got := Tables(lines, nil); len(got) != 0 {
		t.Errorf("read a page of prose as %d tables:\n%s", len(got), got[0].Text)
	}
}

func TestATwoRowGridIsLeftAlone(t *testing.T) {
	// A heading and one row is not enough to tell a table from two lines that
	// happen to have a wide space in the same place. The corpus is better off
	// with a small table read as prose.
	lines := []poppler.TextLine{
		put(60, 100, "Table 6: the one machine"),
		printedRow(112, spot{60, "Machine"}, spot{200, "Year"}),
		printedRow(124, spot{60, "EDSAC"}, spot{200, "1949"}),
	}
	if got := Tables(lines, nil); len(got) != 0 {
		t.Errorf("read two rows as %d tables:\n%s", len(got), got[0].Text)
	}
}

func TestTheParagraphUnderATableIsNotPartOfIt(t *testing.T) {
	lines := machines()
	lines = append(lines,
		put(60, 160, "The machines in the table above were the largest of"),
		put(60, 172, "their day, and each of them was built once."))
	got := one(t, lines)
	if got.Text != machinesTable {
		t.Errorf("wrote\n%s\nwant\n%s", got.Text, machinesTable)
	}
}

func TestATableComesOutOfThePageAsOneParagraph(t *testing.T) {
	p := Read(page(1, machines()), nil)
	if len(p.Paragraphs) != 2 {
		t.Fatalf("read %d paragraphs, want the caption and the table:\n%s", len(p.Paragraphs), p.Text())
	}
	if got := p.Paragraphs[1].Text; got != machinesTable {
		t.Errorf("wrote\n%s\nwant\n%s", got, machinesTable)
	}
	if p.Paragraphs[1].Fenced {
		t.Error("a pipe table was marked as fenced")
	}
}

func TestAFencedTableIsNotEscapedOnTheWayOut(t *testing.T) {
	// Escaping the dollars of a fenced table would put backslashes in a block
	// that is supposed to read as it was printed.
	lines := []poppler.TextLine{
		put(60, 100, "Table 7: what each machine cost"),
		printedRow(112, spot{60, "Model"}, spot{200, "cost in $ either way"}),
		printedRow(124, spot{60, "unit"}, spot{200, "new"}, spot{320, "used"}),
		printedRow(136, spot{60, "base"}, spot{200, "$25"}, spot{320, "$17"}),
		printedRow(148, spot{60, "big"}, spot{200, "$64"}, spot{320, "$40"}),
	}
	p := Read(page(1, lines), nil)
	if !strings.Contains(p.Text(), "$25") {
		t.Errorf("the dollars of a fenced table were escaped:\n%s", p.Text())
	}
	var c Checker
	if faults := c.Check(1, p.Text()); len(faults) != 0 {
		t.Errorf("a page with a fenced table was refused: %v", faults)
	}
}
