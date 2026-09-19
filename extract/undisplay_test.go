package extract

import "testing"

// Every fixture here is typeset in the test. The sentences say nothing and
// are written to have the shape of a page that broke in the middle, because
// a page of somebody's paper is not ours to put in a public repository.

func TestAnUnclosedDisplayOfProseIsAParagraph(t *testing.T) {
	in := `\[
\mu(g) \text{ for all } f, g \in F_n \text{ and the rest of the sentence carries on here, which is what a page that broke in the middle looks like.}
`
	want := `$\mu(g)$ for all $f, g \in F_n$ and the rest of the sentence carries on here, which is what a page that broke in the middle looks like.
`
	if got := Undisplay(in); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

// A full stop set in a math font at the end of a sentence is a full stop in
// the wrong font, so the punctuation comes out of the formula.
func TestThePunctuationComesOutOfTheFormula(t *testing.T) {
	in := "$$\nf \\in F_n. \\text{ The next sentence starts here and runs on far enough to be prose.}"
	want := "$f \\in F_n$. The next sentence starts here and runs on far enough to be prose."
	if got := Undisplay(in); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestADisplayThatClosesIsLeftAlone(t *testing.T) {
	in := "$$\n\\text{maximize } c^T x \\text{ subject to } Ax \\leq b\n$$\n"
	if got := Undisplay(in); got != in {
		t.Errorf("got\n%q\nwant it unchanged", got)
	}
}

// The two unclosed displays in the corpus that are really mathematics have
// no words in them at all, and an unclosed display this cannot read is left
// for rule M11 to report.
func TestAnUnclosedDisplayOfMathematicsIsLeftAlone(t *testing.T) {
	in := "\\[\nx_{n+1} = \\frac{x_n + a}{2}\n"
	if got := Undisplay(in); got != in {
		t.Errorf("got\n%q\nwant it unchanged", got)
	}
}

// The closing delimiter of a display in dollars is the same two characters
// as the opening one, so without the pairing the paragraph under a display
// was read as the body of a display of its own.
func TestTheParagraphAfterADisplayIsNotADisplay(t *testing.T) {
	in := "$$\nx = y + 1\n$$\n\nThe paragraph under it is prose and stays where it is, whatever the delimiters above it look like.\n"
	if got := Undisplay(in); got != in {
		t.Errorf("got\n%q\nwant it unchanged", got)
	}
}

func TestADisplayInsideACodeFenceIsLeftAlone(t *testing.T) {
	in := "```text\n\\[\na := pattern[1];\n```\n"
	if got := Undisplay(in); got != in {
		t.Errorf("got\n%q\nwant it unchanged", got)
	}
}
