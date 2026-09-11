package mathtex

import "testing"

// The shape chapter I of Theory of Sets is written in. Three runs on one line,
// read in the order the line writes them.
func TestTheWordsOfALogicalFormulaAreRead(t *testing.T) {
	got := TextRuns(`((\text{not } A) \text{ or } B) \Rightarrow ((\text{not not } A) \text{ or } B)`)
	want := []string{"not ", " or ", "not not ", " or "}
	if len(got) != len(want) {
		t.Fatalf("%d runs, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Text != want[i] {
			t.Errorf("run %d is %q, want %q", i+1, got[i].Text, want[i])
		}
		if got[i].Macro != `\text` {
			t.Errorf("run %d was opened by %q", i+1, got[i].Macro)
		}
	}
}

// The offsets are what a caller puts a different word back through, so they
// have to name the words and not the braces.
func TestARunSaysWhereItSits(t *testing.T) {
	span := `x \text{ and } y`
	got := TextRuns(span)
	if len(got) != 1 {
		t.Fatalf("%d runs, want 1", len(got))
	}
	rs := []rune(span)
	if s := string(rs[got[0].Start:got[0].End]); s != " and " {
		t.Errorf("the offsets cut out %q, want %q", s, " and ")
	}
}

// \textit sets the same prose and the corpus has nine of them, all in the
// exercises of chapter III.
func TestTheItalicMacroIsARunToo(t *testing.T) {
	got := TextRuns(`(\textit{for all } x)(\textit{whenever } y)`)
	if len(got) != 2 || got[0].Macro != `\textit` || got[0].Text != "for all " {
		t.Fatalf("read %v", got)
	}
}

// A macro whose argument is not the prose must not be read as one. \textcolor
// takes a colour first, and \text is a prefix of both it and \textit.
func TestAMacroThatMerelyStartsWithTextIsNotARun(t *testing.T) {
	if got := TextRuns(`\textcolor{red}{x}`); len(got) != 0 {
		t.Errorf("read %v, want nothing", got)
	}
}

// Braces nest, and the run ends where its own brace closes rather than at the
// first closing brace on the line.
func TestARunEndsAtItsOwnBrace(t *testing.T) {
	got := TextRuns(`\text{if $m$ is odd, so ${a}$}`)
	if len(got) != 1 {
		t.Fatalf("%d runs, want 1: %v", len(got), got)
	}
	if want := `if $m$ is odd, so ${a}$`; got[0].Text != want {
		t.Errorf("run is %q, want %q", got[0].Text, want)
	}
}

// An escaped brace closes nothing.
func TestAnEscapedBraceIsNotABrace(t *testing.T) {
	got := TextRuns(`\text{a \} b} c`)
	if len(got) != 1 || got[0].Text != `a \} b` {
		t.Fatalf("read %v", got)
	}
}

// The mask is what compares two printings of the same formula. The words differ
// and nothing else may, so the two mask to the same string and the unmasked
// strings are not equal.
func TestTheMaskHidesTheWordsAndNothingElse(t *testing.T) {
	en := `(\text{not } A) \text{ or } B`
	vi := `(\text{không } A) \text{ hoặc } B`
	if MaskText(en) != MaskText(vi) {
		t.Errorf("masked apart:\n%q\n%q", MaskText(en), MaskText(vi))
	}
	if en == vi {
		t.Fatal("the two spans are the same, so the test proves nothing")
	}
	moved := `(\text{không } A) \text{ hoặc } C`
	if MaskText(en) == MaskText(moved) {
		t.Error("a changed variable survived the mask")
	}
}

// A span with no words in it is its own mask, which keeps the comparison exact
// for the 18,000 spans of the corpus that hold no prose at all.
func TestASpanWithNoWordsIsUnchanged(t *testing.T) {
	span := `\sum_{i\in I} x_i^2 \leqslant \mathfrak{a}`
	if MaskText(span) != span {
		t.Errorf("masked to %q", MaskText(span))
	}
}
