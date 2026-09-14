package extract

import "testing"

// Every table here is typeset in the test. The shapes are the ones that came
// back from the reader, a heading that spans two columns and a label that
// spans four rows, but the numbers and the words are made up, because a test
// file is committed to a public repository and a table out of somebody's
// paper is not ours to put there.

func TestAPlainTableBecomesAPipeTable(t *testing.T) {
	in := "Before.\n\n" +
		"<table>\n" +
		"<tr><th>Model</th><th>Score</th></tr>\n" +
		"<tr><td>base</td><td>27.3</td></tr>\n" +
		"</table>\n\n" +
		"After."
	want := "Before.\n\n" +
		"| Model | Score |\n" +
		"| --- | --- |\n" +
		"| base | 27.3 |\n\n" +
		"After."
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Every row of the table is on its own line in the answers that prompted
// this, with the cells on lines of their own under it.
func TestATableSpreadOverManyLinesIsStillOneTable(t *testing.T) {
	in := "<table>\n" +
		"<tr>\n<th>Model</th>\n<th>Score</th>\n</tr>\n" +
		"<tr>\n<td>base</td>\n<td>27.3</td>\n</tr>\n" +
		"</table>"
	want := "| Model | Score |\n| --- | --- |\n| base | 27.3 |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A cell that spans rows is written once and the rows under it get an empty
// cell in that column, because GitHub Flavored Markdown has no way to write a
// span and every renderer flattens one anyway.
func TestARowspanLeavesAnEmptyCellUnderneath(t *testing.T) {
	in := "<table>\n" +
		`<tr><td rowspan="2">A</td><td>one</td></tr>` + "\n" +
		"<tr><td>two</td></tr>\n" +
		"</table>"
	want := "| A | one |\n| --- | --- |\n|  | two |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A cell that spans columns is followed by as many empty cells as it covers,
// which keeps every row the same width and keeps the numbers under the
// heading they belong to.
func TestAColspanIsFollowedByEmptyCells(t *testing.T) {
	in := "<table>\n" +
		`<tr><th rowspan="2">Model</th><th colspan="2">Score</th></tr>` + "\n" +
		"<tr><th>one</th><th>two</th></tr>\n" +
		"<tr><td>base</td><td>27.3</td><td>38.1</td></tr>\n" +
		"</table>"
	want := "| Model | Score |  |\n" +
		"| --- | --- | --- |\n" +
		"|  | one | two |\n" +
		"| base | 27.3 | 38.1 |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A subscript is mathematics wherever it turns up in these tables, and
// leaving it as a tag would hide it from mathtex and from the M rules as well
// as from the renderer.
func TestSubscriptsAndSuperscriptsBecomeMathematics(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>d<sub>model</sub></th><th>params ×10<sup>6</sup></th></tr>\n" +
		"<tr><td>512</td><td>65</td></tr>\n" +
		"</table>"
	want := "| $d_{model}$ | params ×$10^{6}$ |\n| --- | --- |\n| 512 | 65 |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBoldAndItalicAndLineBreaksSurvive(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Model</th><th>Score</th></tr>\n" +
		"<tr><td>base<br>large</td><td><b>41.29</b> <i>ours</i></td></tr>\n" +
		"</table>"
	want := "| Model | Score |\n| --- | --- |\n| base large | **41.29** *ours* |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A pipe in a cell is escaped once. grid.Pipe does the escaping, and doing it
// here as well would put a backslash in the table.
func TestAPipeInACellIsEscapedOnce(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Operator</th><th>Meaning</th></tr>\n" +
		"<tr><td>a | b</td><td>either</td></tr>\n" +
		"</table>"
	want := "| Operator | Meaning |\n| --- | --- |\n| a \\| b | either |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The whole point of saying no. A table with a tag in it that this code does
// not understand comes back exactly as it went in, so that rule T11 reports
// it and a person looks at it.
func TestATableWithATagItCannotReadIsLeftAlone(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Figure</th></tr>\n" +
		`<tr><td><img src="plot.png"></td></tr>` + "\n" +
		"</table>"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

// A row of boxes is a table with one row in it, which is what the Spanner
// paper draws its example schema as, and a header with nothing under it is a
// table GitHub Flavored Markdown can write.
func TestATableWithOneRowIsAHeaderWithNothingUnderIt(t *testing.T) {
	in := "<table>\n<tr><th>Model</th><th>Score</th></tr>\n</table>"
	want := "| Model | Score |\n| --- | --- |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestATableWithARowThatHasNoCellsIsLeftAlone(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Model</th></tr>\n" +
		"<tr></tr>\n" +
		"<tr><td>base</td></tr>\n" +
		"</table>"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

// A table that runs off the end of the page is a page that went wrong
// somewhere else, and finishing it here would be guessing where it ended.
func TestAnUnclosedTableIsLeftAlone(t *testing.T) {
	in := "<table>\n<tr><td>base</td></tr>\n<tr><td>large</td></tr>"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

// A paper about HTML has the markup in a listing and none of it is a table.
func TestATableInsideAFenceIsLeftAlone(t *testing.T) {
	in := "```html\n" +
		"<table>\n" +
		"<tr><th>Model</th></tr>\n" +
		"<tr><td>base</td></tr>\n" +
		"</table>\n" +
		"```"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

// A table that opens outside a fence and appears to close inside one is not a
// table, it is two different things that happen to be next to each other.
func TestATableThatWouldCloseInsideAFenceIsLeftAlone(t *testing.T) {
	in := "<table>\n" +
		"<tr><td>base</td></tr>\n" +
		"```html\n" +
		"</table>\n" +
		"```"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

func TestTwoTablesOnAPageAreBothConverted(t *testing.T) {
	in := "<table>\n<tr><th>A</th></tr>\n<tr><td>1</td></tr>\n</table>\n" +
		"\n" +
		"<table>\n<tr><th>B</th></tr>\n<tr><td>2</td></tr>\n</table>"
	want := "| A |\n| --- |\n| 1 |\n\n| B |\n| --- |\n| 2 |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAPageWithNoTableIsUntouched(t *testing.T) {
	in := "One paragraph.\n\nAnother one, with a `<table>` mentioned in it."
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it untouched", got)
	}
}

// Tidy does the conversion in the same pass as the rest of the unwrapping,
// and it does it before Dollars so that TeX inside a cell is treated like TeX
// anywhere else on the page.
func TestTidyConvertsTablesAndThenTheMathematics(t *testing.T) {
	in := "Here is the transcription of the page:\n" +
		"\n" +
		"<table>\n" +
		"<tr><th>Term</th><th>Value</th></tr>\n" +
		`<tr><td>\(d_k\)</td><td>64</td></tr>` + "\n" +
		"</table>\n" +
		"\n" +
		"Let me know if you need anything else."
	want := "| Term | Value |\n| --- | --- |\n| $d_k$ | 64 |"
	if got := Tidy(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A cell that is bare TeX gets dollars round it. The whole reason for
// converting these tables is that TeX the splitter cannot see is mathematics
// the audit cannot check, and leaving a cell like this would be exactly that
// with the angle brackets taken off.
func TestBareTeXInACellGetsDollars(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Model</th><th>Cost</th></tr>\n" +
		"<tr><td>base</td><td>1.0 \\cdot 10^{20}</td></tr>\n" +
		"<tr><td>large</td><td><b>3.3 \\cdot 10^{18}</b></td></tr>\n" +
		"</table>"
	want := "| Model | Cost |\n| --- | --- |\n" +
		"| base | $1.0 \\cdot 10^{20}$ |\n" +
		"| large | **$3.3 \\cdot 10^{18}$** |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A cell the reader already marked up is left alone. Guessing at the rest of
// it would put dollars inside dollars.
func TestACellThatAlreadyHasDollarsIsLeftAsItIs(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Layer</th><th>Complexity</th></tr>\n" +
		"<tr><td>self-attention</td><td>$O(n^2 \\cdot d)$</td></tr>\n" +
		"</table>"
	want := "| Layer | Complexity |\n| --- | --- |\n| self-attention | $O(n^2 \\cdot d)$ |"
	if got := Untable(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The rectangle test, and the reason the whole thing says no more often than
// it looks like it should.
//
// A reader that writes spans writes them from looking at a picture. This is
// the shape that came back for the Transformer paper's third table: a
// rowspan over one more row than it claims, so the rows come out at different
// widths and every number after it is under the wrong heading. A table whose
// numbers are under the wrong headings is worse than no table, because it
// reads like a table.
func TestATableWhoseSpansDoNotAddUpIsLeftAlone(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Group</th><th>N</th><th>Score</th></tr>\n" +
		`<tr><td rowspan="2">(A)</td><td>1</td><td>24.9</td></tr>` + "\n" +
		"<tr><td>2</td><td>25.5</td></tr>\n" +
		"<tr><td>4</td><td>25.8</td></tr>\n" +
		"</table>"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

// A span that runs off the bottom of the table is the same failure seen from
// the other end.
func TestATableWithASpanPastItsLastRowIsLeftAlone(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>Group</th><th>Score</th></tr>\n" +
		`<tr><td rowspan="4">(A)</td><td>24.9</td></tr>` + "\n" +
		"</table>"
	if got := Untable(in); got != in {
		t.Errorf("got:\n%s\nwant it left alone", got)
	}
}

func TestAScriptInProseIsMathematics(t *testing.T) {
	in := "Here n<sub>params</sub> is the count and d<sub>ff</sub> = 4 * d<sub>model</sub> throughout.\n"
	want := "Here $n_{params}$ is the count and $d_{ff}$ = 4 * $d_{model}$ throughout.\n"
	if got := Unscript(in); got != want {
		t.Errorf("Unscript gave\n%q\nand wanted\n%q", got, want)
	}
}

func TestASuperscriptInProseIsMathematics(t *testing.T) {
	in := "The bound is 2<sup>n</sup> in the worst case.\n"
	want := "The bound is $2^{n}$ in the worst case.\n"
	if got := Unscript(in); got != want {
		t.Errorf("Unscript gave %q and wanted %q", got, want)
	}
}

func TestAScriptInsideMathematicsIsLeftAlone(t *testing.T) {
	// Already inside dollars, so nesting would close the span early.
	in := "The value $x<sub>i</sub>$ and the count n<sub>k</sub>.\n"
	want := "The value $x<sub>i</sub>$ and the count $n_{k}$.\n"
	if got := Unscript(in); got != want {
		t.Errorf("Unscript gave %q and wanted %q", got, want)
	}
}

func TestAScriptInAListingIsPartOfTheListing(t *testing.T) {
	in := "```html\n<p>x<sub>1</sub></p>\n```\n"
	if got := Unscript(in); got != in {
		t.Errorf("Unscript reached into a fence: %q", got)
	}
}

func TestAScriptWithNothingInFrontOfItIsATableNote(t *testing.T) {
	// The GPT-3 caption. The cell above it reads 45.6<sup>a</sup> and comes
	// out as $45.6^{a}$, so this has to come out as the same mark or the
	// caption stops explaining the table.
	in := "Table 3.2: Performance on cloze tasks. <sup>a</sup>[Tur20] <sup>b</sup>[RWC+19]\n"
	want := "Table 3.2: Performance on cloze tasks. $^{a}$[Tur20] $^{b}$[RWC+19]\n"
	if got := Unscript(in); got != want {
		t.Errorf("Unscript gave\n%q\nand wanted\n%q", got, want)
	}
}

func TestATableNoteAndAScriptOnASymbolOnOneLine(t *testing.T) {
	in := "The score 45.6<sup>a</sup> is the best. <sup>a</sup>[Tur20]\n"
	want := "The score $45.6^{a}$ is the best. $^{a}$[Tur20]\n"
	if got := Unscript(in); got != want {
		t.Errorf("Unscript gave\n%q\nand wanted\n%q", got, want)
	}
}

func TestALineWithAnOddDelimiterIsLeftForTheAcceptanceRules(t *testing.T) {
	in := "The value of $x is unclear and n<sub>k</sub> too.\n"
	if got := Unscript(in); got != in {
		t.Errorf("Unscript guessed at an unpaired line: %q", got)
	}
}

func TestAPageWithNoScriptsIsUntouched(t *testing.T) {
	in := "Plain prose with $x_i$ and a table.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"
	if got := Unscript(in); got != in {
		t.Errorf("Unscript changed a page with nothing to do: %q", got)
	}
}

func TestATableWhoseSpansDoNotAddUpBecomesAFence(t *testing.T) {
	in := "Before.\n\n" +
		"<table>\n" +
		"<tr><th>Run</th><th>Depth</th><th>Score</th></tr>\n" +
		"<tr><td rowspan=\"4\">(A)</td><td colspan=\"7\">3</td><td>27.3</td></tr>\n" +
		"</table>\n\n" +
		"After."
	want := "Before.\n\n" +
		"```text\n" +
		"Run  Depth  Score\n" +
		"(A)  3  27.3\n" +
		"```\n\n" +
		"After."
	if got := Untable(in); got != in {
		t.Fatalf("Untable should have left this table alone, and gave:\n%s", got)
	}
	if got := Fence(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAFenceKeepsASubscriptAsSomethingAPersonWouldType(t *testing.T) {
	in := "<table>\n" +
		"<tr><th>d<sub>model</sub></th><th>params &times;10<sup>6</sup></th></tr>\n" +
		"<tr><td rowspan=\"3\">512</td><td>65</td></tr>\n" +
		"</table>"
	want := "```text\n" +
		"d_model  params &times;10^6\n" +
		"512  65\n" +
		"```"
	if got := Fence(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAFenceDropsTheMarkupInsideACell(t *testing.T) {
	in := "<table>\n" +
		"<tr><td><b>base</b></td><td>a<br/>b</td><td><span class=\"x\">7</span></td></tr>\n" +
		"<tr><td colspan=\"9\">only one</td></tr>\n" +
		"</table>"
	want := "```text\n" +
		"base  a b  7\n" +
		"only one\n" +
		"```"
	if got := Fence(in); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A table Untable can read is a table Fence never sees, because the page it
// was on passed the acceptance rules and never reached the end of the ladder.
// This says so anyway, because the two run over the same pages in the tests
// above and a fence where a pipe table belongs would be a silent loss.
func TestAFenceLeavesAPipeTableAlone(t *testing.T) {
	in := "| Model | Score |\n| --- | --- |\n| base | 27.3 |\n"
	if got := Fence(in); got != in {
		t.Errorf("Fence touched a table that was already a table: %q", got)
	}
}

func TestAFenceLeavesATableInsideAListingAlone(t *testing.T) {
	in := "```html\n<table>\n<tr><td>a</td></tr>\n</table>\n```\n"
	if got := Fence(in); got != in {
		t.Errorf("Fence rewrote a listing: %q", got)
	}
}

func TestAnUnclosedTableIsNotFenced(t *testing.T) {
	in := "<table>\n<tr><td>a</td></tr>\n"
	if got := Fence(in); got != in {
		t.Errorf("Fence guessed where a table ended: %q", got)
	}
}

func TestATableWithNoRowsIsNotFenced(t *testing.T) {
	in := "<table>\n<caption>Nothing here</caption>\n</table>"
	if got := Fence(in); got != in {
		t.Errorf("Fence wrote an empty fence: %q", got)
	}
}
