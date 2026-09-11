package layout

import "testing"

func TestProseKeepsItsMathematics(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{`<p>plain prose</p>`, "plain prose"},
		{`<p>The error <span class="math">E_n</span> falls.</p>`, "The error $E_n$ falls."},
		{`<p>The <math>x</math>-axis</p>`, "The $x$-axis"},
		{`<p>Already wrapped: <span class="math">$y$</span>.</p>`, "Already wrapped: $y$."},
		{`<p>a < b and c > d</p>`, "a < b and c > d"},
		{`<p>first</p><p>second</p>`, "first second"},
		{`<p>broken<br>line</p>`, "broken line"},
		{`<p>&amp; and &lt;</p>`, "& and <"},
		{`<p>  lots   of\n  space </p>`, `lots of\n space`},
		{``, ""},
		{`   `, ""},
	} {
		if got := htmlText(tt.in); got != tt.want {
			t.Errorf("htmlText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAnEmptyMathSpanLeavesNothingBehind(t *testing.T) {
	if got, want := htmlText(`<p>before <span class="math"></span> after</p>`), "before after"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestASpanThatIsNotMathematicsIsStillProse(t *testing.T) {
	if got, want := htmlText(`<p>a <span class="italic">word</span> here</p>`), "a word here"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestATagThatNeverClosesDoesNotEatTheSentence(t *testing.T) {
	if got, want := htmlText(`<p>the rest of the sentence`), "the rest of the sentence"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMathematicsIsUnwrappedOnce(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{`x = y`, "x = y"},
		{`$x = y$`, "x = y"},
		{`$$x = y$$`, "x = y"},
		{`\[x = y\]`, "x = y"},
		{`\(x = y\)`, "x = y"},
		{`  $$ x = y $$  `, "x = y"},
		{`$`, "$"},
		{``, ""},
	} {
		if got := unwrapMath(tt.in); got != tt.want {
			t.Errorf("unwrapMath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAHeadingLevelComesFromTheTag(t *testing.T) {
	if got, want := htmlLevel(`<h3>a heading</h3>`), 3; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	// No tag means a section, and a section is level one. Guessing deeper
	// would bury it inside whatever came before it.
	if got, want := htmlLevel(`<p>a heading</p>`), 1; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestAPictureIsFoundByItsSource(t *testing.T) {
	if got, want := htmlImage(`<p><img src="_page_0_Figure_1.jpeg"></p>`), "_page_0_Figure_1.jpeg"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := htmlImage(`<p><img alt="a" src='one.png' width="10"></p>`), "one.png"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := htmlImage(`<p>no picture here</p>`); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestATableIsReadIntoRows(t *testing.T) {
	html := `<table>
	  <tr><th>model</th><th>score</th></tr>
	  <tr><td>first</td><td>1.0</td></tr>
	</table>`
	want := [][]string{{"model", "score"}, {"first", "1.0"}}
	got := tableRows(html)
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("cell %d,%d is %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestARowLabelStaysInFront(t *testing.T) {
	// A row that starts with a header cell and carries on in data cells is
	// the commonest shape in a results table, and reading the header cells
	// first would move the label to the front of every row but this one.
	got := tableRows(`<table><tr><th>first</th><td>1.0</td><td>2.0</td></tr></table>`)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	want := []string{"first", "1.0", "2.0"}
	for i := range want {
		if got[0][i] != want[i] {
			t.Fatalf("row is %v, want %v", got[0], want)
		}
	}
}

func TestACellSpanningColumnsIsRepeated(t *testing.T) {
	got := tableRows(`<table><tr><td colspan="3">one heading</td></tr></table>`)
	if len(got) != 1 || len(got[0]) != 3 {
		t.Fatalf("got %v, want the cell across three columns", got)
	}
	for _, c := range got[0] {
		if c != "one heading" {
			t.Errorf("cell is %q, want the heading repeated", c)
		}
	}
}

func TestAnAbsurdColspanIsOne(t *testing.T) {
	got := tableRows(`<table><tr><td colspan="9000">a</td></tr></table>`)
	if len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("got %v, want one cell: a colspan of nine thousand is a misread border", got)
	}
}

func TestAnUnclosedCellStillReads(t *testing.T) {
	got := tableRows(`<table><tr><td>a<td>b</tr></table>`)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != "a" || got[0][1] != "b" {
		t.Fatalf("got %v, want two cells", got)
	}
}

func TestABreakInACellIsASpace(t *testing.T) {
	got := tableRows(`<table><tr><td>3.1<br>0.2</td></tr></table>`)
	if len(got) != 1 || got[0][0] != "3.1 0.2" {
		t.Fatalf("got %v, want two numbers and not one wrong one", got)
	}
}

func TestATagNameIsMatchedWhole(t *testing.T) {
	// <tdata> is not a <td>, and a reader that matched on the prefix would
	// invent a cell out of it.
	if got := tableRows(`<table><tr><tdata>x</tdata><td>real</td></tr></table>`); len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("got %v, want one cell", got)
	}
}

func TestNoTableIsNoRows(t *testing.T) {
	if got := tableRows(""); got != nil {
		t.Errorf("got %v, want nothing", got)
	}
	if got := tableRows("<p>this was a picture</p>"); got != nil {
		t.Errorf("got %v, want nothing", got)
	}
}
