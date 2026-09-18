package code

import (
	"strings"
	"testing"
)

// Every fixture here is typeset in the test. None of it is copied from a
// paper, because a test file is committed to a public repository and a page
// of somebody's paper is not ours to put there.

func TestUnmathTakesAVariableNameOutOfAListing(t *testing.T) {
	in := "```text\nprocedure add (a, b, c);\ncomment adds $b$ and $c$, putting the result in $a$.\n```"
	want := "```text\nprocedure add (a, b, c);\ncomment adds b and c, putting the result in a.\n```"
	if got := Unmath(in); got != want {
		t.Errorf("Unmath() = %q, want %q", got, want)
	}
}

func TestUnmathSpellsTheOperatorsAPagePrints(t *testing.T) {
	in := "```\nwhile $i \\leq n$ do\n  $v \\rightarrow w$\n```"
	want := "```\nwhile i ≤ n do\n  v → w\n```"
	if got := Unmath(in); got != want {
		t.Errorf("Unmath() = %q, want %q", got, want)
	}
}

func TestUnmathTakesTheFaceOffAndKeepsTheWord(t *testing.T) {
	in := "```text\nif $(X := \\text{decrypt}(Y, Key1)) = \\text{nonsense}$\n```"
	want := "```text\nif (X := decrypt(Y, Key1)) = nonsense\n```"
	if got := Unmath(in); got != want {
		t.Errorf("Unmath() = %q, want %q", got, want)
	}
}

// Real mathematics inside a listing is a different mistake with a different
// repair, and writing a sum as the word sum would be a lie about the page.
// M13 goes on reporting it.
func TestUnmathLeavesRealMathematicsForTheRuleToReport(t *testing.T) {
	for _, in := range []string{
		"```text\ntotal := $\\sum_{i=1}^{n} w_i$\n```",
		"```text\nx := $\\frac{a}{b}$\n```",
	} {
		if got := Unmath(in); got != in {
			t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
		}
	}
}

// A dollar in a named language is a character the page printed. Taking it
// out would change the program.
func TestUnmathLeavesAListingWhereTheDollarIsASigil(t *testing.T) {
	for _, in := range []string{
		"```fortran\nFORMAT(FILE NAME? $)\nCALL READB(ICHAN,$990,$990)\n```",
		"```sh\necho $HOME and $PATH\n```",
	} {
		if got := Unmath(in); got != in {
			t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
		}
	}
}

func TestUnmathLeavesTheProseAroundAListingAlone(t *testing.T) {
	in := "The bound is $O(n \\log n)$.\n\n```text\ncomment sorts $a$ in place.\n```\n\nand $b$ after it."
	want := "The bound is $O(n \\log n)$.\n\n```text\ncomment sorts a in place.\n```\n\nand $b$ after it."
	if got := Unmath(in); got != want {
		t.Errorf("Unmath() = %q, want %q", got, want)
	}
}

// An odd number of dollars means a span runs off the end of the line, and
// pairing the ones that are there would pair the wrong two.
func TestUnmathLeavesALineWithAnOddNumberOfDollars(t *testing.T) {
	in := "```text\ncomment the cost is $c and the gain is $g$\n```"
	if got := Unmath(in); got != in {
		t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
	}
}

// An unclosed fence is rule C01's finding and a worse problem than this one.
// Rewriting the tail of the body would make a broken listing look tidy.
func TestUnmathLeavesAnUnclosedFence(t *testing.T) {
	in := "```text\ncomment adds $b$ and $c$."
	if got := Unmath(in); got != in {
		t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
	}
}

func TestUnmathLeavesABodyWithNoListingInIt(t *testing.T) {
	in := "The dimension is $d_k$ and the bound is $O(n)$."
	if got := Unmath(in); got != in {
		t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
	}
}

// A subscripted name in a listing is a name the page could not type, and
// Unicode can. Tarjan's edge stack holds `(u_1, u_2)` and Floyd tests
// convergence over a subinterval of length `(b-a)/2^n`.
func TestUnmathSpellsAOneCharacterScript(t *testing.T) {
	in := "```text\nWHILE top edge $e = (u_1, u_2)$ on stack DO\nconverged when $(b-a)/2^n$ is small;\n```"
	want := "```text\nWHILE top edge e = (u₁, u₂) on stack DO\nconverged when (b-a)/2ⁿ is small;\n```"
	if got := Unmath(in); got != want {
		t.Errorf("Unmath() = %q, want %q", got, want)
	}
}

// Unicode has no subscript b, and `zb` for `z_b` reads as two letters, so
// the span keeps its dollars and M13 goes on reporting it.
func TestUnmathLeavesAScriptUnicodeCannotSpell(t *testing.T) {
	in := "```text\nx := $z_b$;\n```"
	if got := Unmath(in); got != in {
		t.Errorf("Unmath(%q) = %q, want it left alone", in, got)
	}
}

func TestSigilNamesTheLanguagesThatWriteADollar(t *testing.T) {
	for _, lang := range []string{"fortran", "sh", "bash", "FORTRAN", "tex"} {
		if !Sigil(lang) {
			t.Errorf("Sigil(%q) = false, want true", lang)
		}
	}
	// A fence with no tag on it has been past Label, so no answer means the
	// dollars in it are not the language's own.
	for _, lang := range []string{"", "text", "algol", "c", "go", "python"} {
		if Sigil(lang) {
			t.Errorf("Sigil(%q) = true, want false", lang)
		}
	}
}

func TestUnmathSpellsTheArrowOutsideAMathSpan(t *testing.T) {
	body := "```text\nstate \\leftarrow 0\nfor $i \\leftarrow 1$ until $k$ do\n```"
	got := Unmath(body)
	if want := "state ← 0"; !strings.Contains(got, want) {
		t.Errorf("the bare arrow is not spelled: %q", got)
	}
	if want := "for i ← 1 until k do"; !strings.Contains(got, want) {
		t.Errorf("the arrow in the span is not spelled: %q", got)
	}
	if strings.Contains(got, `\leftarrow`) {
		t.Errorf("a TeX arrow is left in the listing: %q", got)
	}
}

func TestUnmathSpellsALooseScriptButLeavesAnIdentifierAlone(t *testing.T) {
	body := "```text\nstate ← g(state, a_j)\nmax_value ← 0\nfirst_name_of ← x\n```"
	got := Unmath(body)
	if want := "g(state, aⱼ)"; !strings.Contains(got, want) {
		t.Errorf("the loose script is not spelled: %q", got)
	}
	for _, name := range []string{"max_value", "first_name_of"} {
		if !strings.Contains(got, name) {
			t.Errorf("the identifier %s was read as a subscript: %q", name, got)
		}
	}
}

func TestUnmathLeavesTheMathematicsInAMathSpanAlone(t *testing.T) {
	body := "```text\nconverges when $\\sum_{i=1}^{n} w_i \\leq 1$\n```"
	if got := Unmath(body); !strings.Contains(got, `\leq`) {
		t.Errorf("a span that kept its dollars was rewritten: %q", got)
	}
}
