package mathtex

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnEquationNumberComesOffTheMathematics(t *testing.T) {
	cases := []struct {
		name   string
		tex    string
		want   string
		number string
	}{
		{
			"a bare number at the right margin",
			`E = mc^2 \qquad (3)`,
			`E = mc^2`,
			"3",
		},
		{
			"no spacing command between them",
			`x = y (12)`,
			`x = y`,
			"12",
		},
		{
			"a tag, which is what a LaTeX source writes",
			`x = y \tag{3}`,
			`x = y`,
			"3",
		},
		{
			"a tag whose number is parenthesised",
			`x = y \tag{(3.2)}`,
			`x = y`,
			"3.2",
		},
		{
			"a section-numbered equation",
			`a = b \quad (3.2)`,
			`a = b`,
			"3.2",
		},
		{
			"a lettered equation, which is how a paper splits one display in two",
			`a = b \quad (12a)`,
			`a = b`,
			"12a",
		},
		{
			"a display set on its own lines keeps them",
			"\nx = y \\qquad (4)\n",
			"\nx = y\n",
			"4",
		},
		{
			// This is the case the whole pattern is narrowed for. Almost every
			// display in a paper about algorithms ends in a function call.
			"a function call at the end of a display is not a number",
			`T(n) = 2T(n/2) + O(n)`,
			`T(n) = 2T(n/2) + O(n)`,
			"",
		},
		{
			"a display that is nothing but a number is left for somebody to look at",
			`(7)`,
			`(7)`,
			"",
		},
		{
			"a number that is not at the end is part of the formula",
			`f(3) = 0 \text{ and } g = 1`,
			`f(3) = 0 \text{ and } g = 1`,
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, number := TakeNumber(c.tex)
			if got != c.want || number != c.number {
				t.Errorf("TakeNumber(%q) = %q, %q, want %q, %q", c.tex, got, number, c.want, c.number)
			}
		})
	}
}

func TestTakingTheNumberOffTwiceChangesNothing(t *testing.T) {
	// The extraction runs over a page that has already been extracted often
	// enough that this has to hold, and a second pass that ate the last term
	// of a formula would be invisible until somebody read the page.
	once, number := TakeNumber(`x = y \qquad (3)`)
	if number != "3" {
		t.Fatalf("the first pass found %q, want 3", number)
	}
	twice, again := TakeNumber(once)
	if twice != once || again != "" {
		t.Errorf("the second pass gave %q, %q, want %q and nothing", twice, again, once)
	}
}

// paper is a body in the shape the extraction writes: displays fenced on their
// own lines, prose around them, and the equation numbers already taken off.
const paper = `We begin from the definition

$$
E = mc^2
$$
{#p-eq-1 .equation tag=0001}

and the reader will recall that by (1) the energy is fixed. Substituting into

$$
a = b + c
$$
{#p-eq-2 .equation tag=0002}

gives the result. Note that (1) and (2) together imply the bound, while
$f(1)$ does not.
`

func TestAPaperThatWritesParenthesesGetsItsReferencesLinked(t *testing.T) {
	n := LearnNumbering(paper, []string{"1", "2"})
	if got := n.Forms(); !reflect.DeepEqual(got, []string{"paren"}) {
		t.Fatalf("learned %v, want just the parenthesised form", got)
	}

	out, count := n.Link(paper, func(number string) string { return "#p-eq-" + number })
	if count != 3 {
		t.Errorf("linked %d references, want 3", count)
	}
	if !strings.Contains(out, "by [(1)](#p-eq-1) the energy") {
		t.Error("the reference in the first sentence was not linked")
	}
	if !strings.Contains(out, "[(1)](#p-eq-1) and [(2)](#p-eq-2) together") {
		t.Error("the two references in the last sentence were not both linked")
	}
}

func TestANumberInsideAFormulaIsPartOfTheFormula(t *testing.T) {
	// $f(1)$ is a function evaluated at one. Linking it would put a Markdown
	// link inside a math span, which is an L01 refusal on every translation of
	// the paper for ever.
	n := LearnNumbering(paper, []string{"1", "2"})
	out, _ := n.Link(paper, func(number string) string { return "#p-eq-" + number })
	if !strings.Contains(out, "$f(1)$ does not") {
		t.Error("the mathematics was rewritten")
	}
	spans, _ := Split(out)
	before, _ := Split(paper)
	if len(spans) != len(before) {
		t.Errorf("%d spans after linking, %d before", len(spans), len(before))
	}
	for i := range spans {
		if spans[i].Text != before[i].Text {
			t.Errorf("span %d is now %q, was %q", i, spans[i].Text, before[i].Text)
		}
	}
}

func TestANumberThePaperNeverPrintedIsNotALink(t *testing.T) {
	// The defence against turning every parenthesis in the paper into a link
	// to nowhere. The paper numbers 1 and 2 and the prose mentions (9).
	body := paper + "\nand finally (9) is not an equation at all.\n"
	n := LearnNumbering(body, []string{"1", "2"})
	out, _ := n.Link(body, func(number string) string { return "#p-eq-" + number })
	if !strings.Contains(out, "finally (9) is not") {
		t.Error("a number the paper does not number was linked")
	}
}

func TestAListLabelIsNotAReference(t *testing.T) {
	// A paper that sets out its conditions as (1), (2), (3) down the left
	// margin is not referring to its equations, it is numbering a list, and
	// the definition of a condition must not point at a formula.
	body := paper + `
The conditions are these.

(1) The sequence is bounded.
(2) The limit exists.
`
	n := LearnNumbering(body, []string{"1", "2"})
	out, _ := n.Link(body, func(number string) string { return "#p-eq-" + number })
	if !strings.Contains(out, "\n(1) The sequence is bounded.") {
		t.Error("a list label at the start of a line was linked")
	}
	if !strings.Contains(out, "by [(1)](#p-eq-1) the energy") {
		t.Error("the real reference in the middle of a sentence was not linked")
	}
}

func TestAPaperThatSpellsItOutGetsBothSpellings(t *testing.T) {
	body := `The bound follows from Eq. 1 and from equation 2.

$$
x = y
$$

Later we use (2) again.
`
	n := LearnNumbering(body, []string{"1", "2"})
	forms := n.Forms()
	want := map[string]bool{"eq": true, "equation": true, "paren": true}
	if len(forms) != len(want) {
		t.Fatalf("learned %v, want eq, equation and paren", forms)
	}
	for _, f := range forms {
		if !want[f] {
			t.Errorf("learned %q, which this paper does not write", f)
		}
	}
	out, count := n.Link(body, func(number string) string { return "#eq-" + number })
	if count != 3 {
		t.Errorf("linked %d, want 3", count)
	}
	if !strings.Contains(out, "[Eq. 1](#eq-1)") {
		t.Error("the Eq. spelling was not linked whole")
	}
	if !strings.Contains(out, "[equation 2](#eq-2)") {
		t.Error("the spelled out form was not linked whole")
	}
}

func TestAPaperThatNumbersNothingLinksNothing(t *testing.T) {
	body := "There are (3) reasons for this, and none of them is an equation.\n"
	n := LearnNumbering(body, nil)
	out, count := n.Link(body, func(string) string { return "#nowhere" })
	if count != 0 || out != body {
		t.Errorf("linked %d references in a paper with no equations", count)
	}
}

func TestALineNumberInAListingIsNotAReference(t *testing.T) {
	// A paper about an algorithm prints a listing with the journal's line
	// numbers down the side, and the numbers 1 and 2 are in every one of them.
	body := "By (1) the loop terminates.\n\n```algol\n(1) procedure quicksort (A, M, N);\n(2)   integer M, N;\n```\n\nand that is all.\n"
	n := LearnNumbering(body, []string{"1", "2"})
	out, count := n.Link(body, func(number string) string { return "#eq-" + number })
	if count != 1 {
		t.Errorf("linked %d references, want just the one in the prose", count)
	}
	if !strings.Contains(out, "(1) procedure quicksort") {
		t.Error("a line number inside a fence was linked")
	}
}

func TestTheHrefCanRefuseANumber(t *testing.T) {
	// An equation whose anchor has not been assigned yet has nowhere to point,
	// and a link to an anchor that does not exist fails P02 on every page it
	// is on. Refusing it here leaves the text as the paper wrote it.
	n := LearnNumbering(paper, []string{"1", "2"})
	out, count := n.Link(paper, func(number string) string {
		if number == "2" {
			return ""
		}
		return "#p-eq-" + number
	})
	if count != 2 {
		t.Errorf("linked %d, want the two references to equation 1", count)
	}
	if !strings.Contains(out, "and (2) together") {
		t.Error("a number the caller refused was linked anyway")
	}
}
