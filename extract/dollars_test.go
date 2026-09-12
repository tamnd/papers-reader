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

func TestADisplayWrittenOnOneLineBecomesTwoDollars(t *testing.T) {
	in := `\[ E = mc^2 \]`
	want := `$$ E = mc^2 $$`
	if got := Dollars(in); got != want {
		t.Errorf("Dollars(%q) = %q, want %q", in, got, want)
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
