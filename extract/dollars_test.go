package extract

import "testing"

// Every fixture here is typeset in the test. None of it is copied from a
// paper, because a test file is committed to a public repository and a page of
// somebody's paper is not ours to put there.

func TestAnInlineFormulaInTeXDelimitersBecomesDollars(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{
			name: "one formula",
			in:   `The input consists of keys of dimension \( d_k \).`,
			want: `The input consists of keys of dimension $d_k$.`,
		},
		{
			name: "two on a line",
			in:   `We project \( Q \) and \( K \) alike.`,
			want: `We project $Q$ and $K$ alike.`,
		},
		{
			name: "the spaces inside the delimiters are the delimiters and not the formula",
			in:   `A value \(   x^2   \) here.`,
			want: `A value $x^2$ here.`,
		},
		{
			name: "dollars already there are left alone",
			in:   `Both $a$ and \( b \) appear.`,
			want: `Both $a$ and $b$ appear.`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Dollars(c.in); got != c.want {
				t.Errorf("Dollars(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestADisplayInTeXDelimitersBecomesTwoDollars(t *testing.T) {
	in := "We compute the outputs as:\n\\[\nE = mc^2\n\\]\nThe two attentions differ."
	want := "We compute the outputs as:\n$$\nE = mc^2\n$$\nThe two attentions differ."
	if got := Dollars(in); got != want {
		t.Errorf("Dollars() = %q, want %q", got, want)
	}
}

// The formula comes out tight against its dollars, the way the inline
// rewrite has always written one and the way the rest of the corpus is
// written. Rule M12 reports the loose spelling.
func TestADisplayWrittenOnOneLineBecomesTwoDollars(t *testing.T) {
	in := `\[ E = mc^2 \]`
	want := `$$E = mc^2$$`
	if got := Dollars(in); got != want {
		t.Errorf("Dollars(%q) = %q, want %q", in, got, want)
	}
}

// olmOCR leaves the equation number where the page prints it, after the
// closing delimiter. It goes inside the display as a tag, which is how the
// rest of the corpus writes a numbered equation and the only way KaTeX sets
// the number beside it.
func TestAnEquationNumberAfterTheCloserBecomesATag(t *testing.T) {
	in := "\\[\nW = \\sum_i w_i\n\\] (1)"
	want := "$$\nW = \\sum_i w_i\n\\tag{1}\n$$"
	if got := Dollars(in); got != want {
		t.Errorf("Dollars() = %q, want %q", got, want)
	}
}

func TestAnEquationNumberAfterAOneLineDisplayBecomesATag(t *testing.T) {
	in := `\[ W = \sum_i w_i \] (3a)`
	want := `$$W = \sum_i w_i \tag{3a}$$`
	if got := Dollars(in); got != want {
		t.Errorf("Dollars(%q) = %q, want %q", in, got, want)
	}
}

// A page already read in dollars gets the same treatment, because the number
// lands after the closer whichever dialect the reader answered in.
func TestANumberAfterADollarCloserBecomesATag(t *testing.T) {
	in := "$$\nW = \\sum_i w_i\n$$ (12)"
	want := "$$\nW = \\sum_i w_i\n\\tag{12}\n$$"
	if got := Dollars(in); got != want {
		t.Errorf("Dollars() = %q, want %q", got, want)
	}
}

// Hoare's Table I sets the name of each axiom beside the axiom, and olmOCR
// transcribes it that way. A display in the middle of a line is not a
// display, so it comes out in one dollar rather than two.
func TestALabelledFormulaKeepsItsLabelAndTakesOneDollar(t *testing.T) {
	in := `A5 \[(r - y) + y \times (1 + q)\]`
	want := `A5 $(r - y) + y \times (1 + q)$`
	if got := Dollars(in); got != want {
		t.Errorf("Dollars(%q) = %q, want %q", in, got, want)
	}
}

func TestAFormulaWithProseOnBothSidesOfItTakesOneDollar(t *testing.T) {
	in := `A10 \[\forall x \quad (x \leq \max)\] where max is the largest integer.`
	want := `A10 $\forall x \quad (x \leq \max)$ where max is the largest integer.`
	if got := Dollars(in); got != want {
		t.Errorf("Dollars(%q) = %q, want %q", in, got, want)
	}
}

// A label ahead of an opener gets a line of its own. `A9 $$` on one line
// reads as a paragraph of text to the assembler, which joins it to the
// display under it and then breaks the display in half with a blank line.
func TestALabelAheadOfAnOpenerGetsALineOfItsOwn(t *testing.T) {
	in := "A9 \\[\n(x \\leq y) \\land (y \\leq x) \\supset (x = y)\n\\]"
	want := "A9\n$$\n(x \\leq y) \\land (y \\leq x) \\supset (x = y)\n$$"
	if got := Dollars(in); got != want {
		t.Errorf("Dollars() = %q, want %q", got, want)
	}
}

// A range of references is escaped the same way a single one is, and it is
// still not mathematics.
func TestAnEscapedRangeOfCitationsIsNotADisplay(t *testing.T) {
	in := `The idea is older than the paper \[2, 11-14\] and was never analysed.`
	if got := Dollars(in); got != in {
		t.Errorf("Dollars(%q) = %q, want it left alone", in, got)
	}
}

// The one this function exists to get right. A reader that escapes the
// brackets of a citation writes the same two characters a display opens with,
// and a rewrite that could not tell them apart would turn a reference into a
// formula on every page of every bibliography in the corpus.
func TestAnEscapedCitationIsNotADisplay(t *testing.T) {
	for _, in := range []string{
		`Additive attention \[2\] computes the compatibility function.`,
		`See \[12\], \[13\] and the survey \[14\].`,
		`The bound is due to Cook \[7\].`,
	} {
		if got := Dollars(in); got != in {
			t.Errorf("Dollars(%q) = %q, want it left alone", in, got)
		}
	}
}

func TestAnOpenerWithNoCloserIsLeftAlone(t *testing.T) {
	for _, in := range []string{
		`The value \( d_k is never closed.`,
		"\\[\nE = mc^2\nand the page ends here",
	} {
		if got := Dollars(in); got != in {
			t.Errorf("Dollars(%q) = %q, want it left alone", in, got)
		}
	}
}

func TestProgramTextKeepsItsBackslashes(t *testing.T) {
	in := "A listing:\n\n```text\nmatch \\( group \\) end\n```\n\nand \\( x \\) after it."
	want := "A listing:\n\n```text\nmatch \\( group \\) end\n```\n\nand $x$ after it."
	if got := Dollars(in); got != want {
		t.Errorf("Dollars() = %q, want %q", got, want)
	}
}

func TestAFenceThatIsNeverClosedProtectsTheRestOfThePage(t *testing.T) {
	// An unclosed fence is rule A7's finding to report. Rewriting the tail of
	// the page would change what A7 is looking at, so nothing after the fence
	// is touched.
	in := "```c\nif \\( a \\) then\n\nand \\( x \\) after it."
	if got := Dollars(in); got != in {
		t.Errorf("Dollars() = %q, want it left alone", got)
	}
}

func TestAPageWithNoMathematicsOnItComesBackUnchanged(t *testing.T) {
	in := "## 3.1 Encoder and Decoder Stacks\n\nThe encoder is a stack of six layers.\n"
	if got := Dollars(in); got != in {
		t.Errorf("Dollars() = %q, want it left alone", got)
	}
}

// Tidy is where the vision path meets this, so the wiring is worth one test of
// its own: a fenced answer in the other dialect comes out unfenced and in
// dollars in a single pass.
func TestTidyUnwrapsAndConvertsInOnePass(t *testing.T) {
	in := "```markdown\nThe dimension is \\( d_k \\).\n```"
	want := `The dimension is $d_k$.`
	if got := Tidy(in); got != want {
		t.Errorf("Tidy() = %q, want %q", got, want)
	}
}
