package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/katex"
)

func rules(faults []Fault) []string {
	out := make([]string, len(faults))
	for i, f := range faults {
		out[i] = strings.Fields(f.Rule)[0]
	}
	return out
}

func has(faults []Fault, rule string) bool {
	for _, r := range rules(faults) {
		if r == strings.Fields(rule)[0] {
			return true
		}
	}
	return false
}

func TestAGoodPageIsAccepted(t *testing.T) {
	var c Checker
	text := "The transform is defined for every $x$ in the domain, and the\nsum $\\sum_{i=1}^{n} a_i$ converges.\n"
	if faults := c.Check(1, text); len(faults) != 0 {
		t.Fatalf("a sound page was refused: %v", faults)
	}
	if c.Pages() != 1 {
		t.Errorf("the page was not counted toward the statistics")
	}
}

func TestAnEmptyPageIsRefused(t *testing.T) {
	var c Checker
	if faults := c.Check(1, "   \n\n"); !has(faults, A1) {
		t.Errorf("an empty page gave %v", rules(faults))
	}
}

func TestARefusalIsNotAPage(t *testing.T) {
	var c Checker
	for _, text := range []string{
		"I'm sorry, I can't help with that.",
		"As an AI language model, I am unable to read this image.",
		"<!DOCTYPE html><html><body>502 Bad Gateway</body></html>",
	} {
		if faults := c.Check(1, text); !has(faults, A1) {
			t.Errorf("%q was accepted as a page: %v", text, rules(faults))
		}
	}
}

func TestAPageThatDiscussesRefusalsIsStillAPage(t *testing.T) {
	// The corpus is about machine learning and one of these papers will
	// quote a refusal. A refusal is the whole answer or the start of it, so
	// only the head of the page is looked at.
	var c Checker
	text := strings.Repeat("The model was trained on a corpus of dialogue and the evaluation follows the protocol above. ", 6) +
		"The most common failure was a response beginning I'm sorry, I cannot help with that."
	if faults := c.Check(1, text); len(faults) != 0 {
		t.Errorf("a page about refusals was refused: %v", faults)
	}
}

func TestAnUnclosedMathDelimiterIsRefused(t *testing.T) {
	var c Checker
	if faults := c.Check(1, "the value of $x is not known\n"); !has(faults, A2) {
		t.Errorf("an unclosed span gave %v", rules(faults))
	}
	if faults := c.Check(1, "a display $$ a = b\n\nand then some prose\n"); !has(faults, A2) {
		t.Errorf("an unclosed display gave %v", rules(faults))
	}
}

func TestAMathSpanKaTeXCannotReadIsRefused(t *testing.T) {
	r, err := katex.New()
	if err != nil {
		t.Skipf("no KaTeX build here: %v", err)
	}
	c := Checker{Math: r}
	if faults := c.Check(1, "the bound is $\\frac{1}{$ and no more\n"); len(faults) == 0 {
		t.Error("a span that does not parse was accepted")
	}
	if faults := c.Check(2, "the bound is $\\frac{1}{2}$ and no more\n"); len(faults) != 0 {
		t.Errorf("a span that does parse was refused: %v", faults)
	}
}

func TestKaTeXIsNotAskedWhenThereIsNoRenderer(t *testing.T) {
	// A caller with no KaTeX build gets the other seven rules rather than a
	// crash, and the report says A4 did not run.
	var c Checker
	if faults := c.Check(1, "the bound is $\\notacommand{x}$ here\n"); has(faults, A4) {
		t.Error("A4 ran without a renderer")
	}
}

func TestATruncatedPageIsRefused(t *testing.T) {
	c := Checker{Model: true}
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, "a sentence.\n"); !has(faults, A5) {
		t.Errorf("a page at a tenth of the usual length gave %v", rules(faults))
	}
	// And the short page did not widen the variance for the next one.
	if faults := c.Check(10, "another sentence.\n"); !has(faults, A5) {
		t.Errorf("the second short page gave %v, so the first one was counted", rules(faults))
	}
}

func TestTheLengthRuleSaysNothingUntilItHasSeenEnoughPages(t *testing.T) {
	c := Checker{Model: true}
	long := strings.Repeat("a sentence of the paper. ", 40)
	for i := 1; i < MinLengthSample; i++ {
		c.Check(i, long)
	}
	if faults := c.Check(MinLengthSample, "short.\n"); has(faults, A5) {
		t.Errorf("A5 fired on page %d, before it had a sample", MinLengthSample)
	}
}

func TestAShortPageIsNotRefusedWhenNoModelReadIt(t *testing.T) {
	// The three pages of attention visualisations at the end of the
	// Transformer paper are short because they are pictures. pdftotext read
	// them correctly and there is nothing to ask again.
	var c Checker
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		c.Check(i, full)
	}
	if faults := c.Check(9, "Figure 4: Attention at layer 5.\n"); len(faults) != 0 {
		t.Errorf("a short page of a native extraction was refused: %v", faults)
	}
}

func TestAPageWithTheWrongPrintedNumberIsRefused(t *testing.T) {
	c := Checker{Map: Map{Offset: 482, Known: true}}
	if faults := c.Check(1, "the body of the page.\n\n483\n"); len(faults) != 0 {
		t.Errorf("the right folio was refused: %v", faults)
	}
	if faults := c.Check(2, "the body of the page.\n\n491\n"); !has(faults, A6) {
		t.Errorf("the wrong folio gave %v", rules(faults))
	}
	if faults := c.Check(3, "a page that prints no number at all.\n"); has(faults, A6) {
		t.Error("a page that prints no number was refused for printing the wrong one")
	}
}

func TestAnUnclosedCodeFenceIsRefused(t *testing.T) {
	var c Checker
	text := "here is the loop:\n\n```c\nfor (i = 0; i < n; i++) {\n"
	if faults := c.Check(1, text); !has(faults, A7) {
		t.Errorf("an unclosed fence gave %v", rules(faults))
	}
	closed := "here is the loop:\n\n```c\nfor (i = 0; i < n; i++) { }\n```\n"
	if faults := c.Check(2, closed); len(faults) != 0 {
		t.Errorf("a closed fence was refused: %v", faults)
	}
}

func TestAnIllegibleMarkerIsRefusedUnlessThePageIsRecordedAsDamaged(t *testing.T) {
	c := Checker{Damaged: map[int]bool{7: true}}
	if faults := c.Check(3, "the value is [?] in the original.\n"); !has(faults, A8) {
		t.Errorf("an undeclared illegible marker gave %v", rules(faults))
	}
	if faults := c.Check(7, "the value is [?] in the original.\n"); has(faults, A8) {
		t.Error("a page recorded as damaged was refused for its one marker")
	}
	if faults := c.Check(7, "the value is [?] and the other is [?].\n"); !has(faults, A8) {
		t.Errorf("a damaged page with two markers gave %v", rules(faults))
	}
}

func TestAcceptNamesEveryRuleThePageBroke(t *testing.T) {
	var c Checker
	err := c.Accept(4, "the value of $x is [?] here\n")
	if err == nil {
		t.Fatal("a page that broke two rules was accepted")
	}
	for _, want := range []string{"page 4", "A2", "A8"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s: %v", want, err)
		}
	}
}
