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

func TestAPageThatOpensWithAFenceIsNotRefused(t *testing.T) {
	// The layout path writes a listing as the first block of a page on any
	// paper whose page starts with an algorithm, and then the opening fence
	// has no newline in front of it.
	var c Checker
	text := "```text\nfor i in 1..n\n```\n\nand the prose under it.\n"
	if faults := c.Check(1, text); len(faults) > 0 {
		t.Errorf("the page was refused: %v", faults)
	}
}

func TestAFenceThatIsNeverClosedIsStillRefused(t *testing.T) {
	var c Checker
	text := "```text\nfor i in 1..n\n\nand the prose under it.\n"
	if faults := c.Check(1, text); len(faults) == 0 {
		t.Error("a fence that was never closed was accepted")
	}
}

// layerOf is a Checker.Layer over one page, which is all any of these need.
func layerOf(page int, text string) func(int) (string, bool) {
	return func(p int) (string, bool) {
		if p != page {
			return "", false
		}
		return text, true
	}
}

func TestAPageThatDroppedAParagraphIsRefused(t *testing.T) {
	first, second, ok := strings.Cut(layer, "\n\n")
	if !ok {
		t.Fatal("the fixture no longer has two paragraphs in it")
	}
	c := Checker{Layer: layerOf(1, layer)}
	faults := c.Check(1, first+"\n")
	if !has(faults, A9) {
		t.Fatalf("a page with a paragraph missing was accepted: %v", faults)
	}
	// The fault has to name enough of the passage for somebody reading the
	// queue to find it in the paper, so the words it names come out of it.
	for _, f := range faults {
		if !strings.HasPrefix(f.Rule, "A9") {
			continue
		}
		_, named, _ := strings.Cut(f.Detail, "missing ")
		if named == "" {
			t.Fatalf("the fault names no missing words: %q", f.Detail)
		}
		for _, w := range strings.Split(named, ", ") {
			if !strings.Contains(strings.ToLower(second), w) {
				t.Errorf("the fault names %q, which is not in the dropped paragraph", w)
			}
		}
	}
}

func TestAPageReadCorrectlyPassesA9(t *testing.T) {
	c := Checker{Layer: layerOf(1, layer)}
	if faults := c.Check(1, layer+"\n"); has(faults, A9) {
		t.Errorf("a page read correctly was refused: %v", faults)
	}
}

func TestA9SaysNothingAboutAPageWithNoLayer(t *testing.T) {
	// Most pages of most papers. The rule is off by default and off for any
	// page the caller has nothing to compare against.
	var c Checker
	first, _, _ := strings.Cut(layer, "\n\n")
	if faults := c.Check(1, first+"\n"); has(faults, A9) {
		t.Errorf("A9 ran with no layer at all: %v", faults)
	}
	c = Checker{Layer: layerOf(2, layer)}
	if faults := c.Check(1, first+"\n"); has(faults, A9) {
		t.Errorf("A9 ran on a page the layer function declined: %v", faults)
	}
}

func TestFaultsAppliesTheRulesWithoutCountingThePage(t *testing.T) {
	// Rechecking pages already on disk runs over each of them twice, once to
	// give A5 its statistics and once to ask. Counting them both times would
	// narrow the variance A5 works from and it would stop firing.
	var c Checker
	text := "A page of quite ordinary length, long enough to have an opinion about.\n"
	if faults := c.Faults(1, text); len(faults) != 0 {
		t.Fatalf("a sound page was refused: %v", faults)
	}
	if c.Pages() != 0 {
		t.Errorf("Faults counted %d pages toward the statistics, want 0", c.Pages())
	}
	c.Check(1, text)
	before := c.Pages()
	c.Faults(1, text)
	if c.Pages() != before {
		t.Errorf("Faults changed the page count from %d to %d", before, c.Pages())
	}
}
