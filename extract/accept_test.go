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

// layered is a checker whose pages the file itself can be asked about, which
// is what a born digital paper gives A5. Each page's layer is as long as the
// share of a full page the caller asks for, and a share over one is a page
// the file says holds more than an ordinary one.
func layered(share map[int]float64) *Checker {
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	return &Checker{Model: true, Layer: func(page int) (string, bool) {
		s, ok := share[page]
		if !ok {
			s = 1
		}
		text := strings.Repeat(full, int(s)+1)
		return text[:int(float64(len(full))*s)], true
	}}
}

// Issue #47. Pages 13, 14 and 15 of the Transformer paper are a full page
// figure and a caption, they came back at about 270 characters against a
// paper average of 3542, and A5 refused all three and had them read again at
// 400 dpi and again at 600. Nine asks of a rationed reader, and the answer
// was right the first time. The file says how much prose is on the page, so
// the rule can ask whether the reading is short or the page is.
func TestAPageTheFileItselfSaysIsShortIsNotRefused(t *testing.T) {
	c := layered(map[int]float64{9: 0.15})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	short := full[:int(0.15*float64(len(full)))]
	if faults := c.Check(9, short); has(faults, A5) {
		t.Errorf("a short page on a short page of the file gave %v", rules(faults))
	}
}

// The other half of it, and the reason this is a narrowing rather than a
// hole. A reading that stopped part way down a full page is short while the
// file says the page is full, so the expectation stays where it was.
func TestATruncatedPageIsStillRefusedWhenTheFileSaysThePageIsFull(t *testing.T) {
	c := layered(nil)
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, "a sentence.\n"); !has(faults, A5) {
		t.Errorf("a truncated page on a full page of the file gave %v", rules(faults))
	}
}

// Page 11 of the ResNet paper. Prose is a floor under what is on a page and
// not an estimate of it, because it drops the cells of a table by design and
// a reader transcribes a table. The page came back at 6062 characters of
// correct reading where the layer on it is worth 1848, so a reading longer
// than the paper's average is measured against the paper and not the file.
func TestAPageThatReadsLongerThanItsLayerIsMeasuredAgainstThePaper(t *testing.T) {
	c := layered(map[int]float64{9: 0.15})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, full); has(faults, A5) {
		t.Errorf("a full length reading of a page whose layer is thin gave %v", rules(faults))
	}
}

// Page 63 of the GPT-3 paper, which is the other way about. The page is one
// appendix table of every score in the paper, the layer on it is worth 8744
// against a paper that averages 3188, and a correct reading of 11556
// characters was refused nine times against the average. A floor can raise
// what is expected of a reading and it must never lower it.
func TestAPageTheFileItselfSaysIsLongIsNotRefusedForBeingLong(t *testing.T) {
	c := layered(map[int]float64{9: 3})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, strings.Repeat(full, 3)); has(faults, A5) {
		t.Errorf("a long reading of a page the file calls long gave %v", rules(faults))
	}
}

// And the teeth are still in it. A page the file calls long that comes back
// at four times what even the file says is on it is a reading that ran away
// with itself.
func TestAReadingFarPastWhatTheFileSaysIsOnThePageIsStillRefused(t *testing.T) {
	c := layered(map[int]float64{9: 3})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, strings.Repeat(full, 12)); !has(faults, A5) {
		t.Errorf("a runaway reading gave %v", rules(faults))
	}
}

// What the rule says has to send a person to the right place. When the
// expectation came from the file, the complaint is that the reading is
// unlike the page and not that the page is unlike the paper.
func TestTheLengthRuleNamesWhateverItMeasuredAgainst(t *testing.T) {
	c := layered(nil)
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		c.Check(i, full)
	}
	faults := c.Check(9, "a sentence.\n")
	if len(faults) == 0 {
		t.Fatal("the truncated page was accepted")
	}
	if !strings.Contains(faults[0].Detail, "text layer") {
		t.Errorf("A5 says %q, which does not say what it measured against", faults[0].Detail)
	}
}

// inked is a checker whose pages have been measured for ink and have no text
// layer at all, which is what a scan gives A5. An ordinary page is five and a
// half per cent ink, which is what Cook's pages measure.
func inked(share map[int]float64) *Checker {
	return &Checker{Model: true, Ink: func(page int) (float64, bool) {
		s, ok := share[page]
		if !ok {
			s = 1
		}
		return 0.055 * s, true
	}}
}

// Page 8 of Cook's paper. It is the end of the bibliography, two references
// and a folio on an otherwise empty sheet, and it came back at 287 characters
// against a paper averaging 3679. It was read three times at three
// resolutions, it was correct all three times, and A5 refused it all three
// times because a scan has no text layer to say the page is empty. The ink on
// the page says it.
func TestAPageWithAlmostNoInkOnItIsNotRefusedForBeingShort(t *testing.T) {
	c := inked(map[int]float64{9: 0.1})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, full[:int(0.1*float64(len(full)))]); has(faults, A5) {
		t.Errorf("a short reading of a nearly empty page gave %v", rules(faults))
	}
}

// And the teeth are still in it. A reading that stopped part way down a page
// with as much ink on it as every other page is a truncated reading.
func TestATruncatedPageIsStillRefusedWhenThePageIsFullOfInk(t *testing.T) {
	c := inked(nil)
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, "a sentence.\n"); !has(faults, A5) {
		t.Errorf("a truncated page on a full page of ink gave %v", rules(faults))
	}
}

// Ink is a ceiling and never a floor. A page covered in a half tone
// photograph is nearly all ink and holds no words at all, so a page with
// three times the ink of its neighbours is still measured against the paper
// and a truncated reading of it is still refused.
func TestAPageCoveredInInkDoesNotRaiseWhatIsExpectedOfIt(t *testing.T) {
	c := inked(map[int]float64{9: 3})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	if faults := c.Check(9, full); has(faults, A5) {
		t.Errorf("an ordinary reading of a page heavy with ink gave %v", rules(faults))
	}
}

// A paper that has both is measured against its text layer, because prose on
// the page is a better account of what a reading should hold than ink is: ink
// counts a rule, a folio and the black of a photograph the same as a letter.
func TestTheTextLayerIsPreferredToTheInk(t *testing.T) {
	c := layered(nil)
	c.Ink = func(int) (float64, bool) { return 0.001, true }
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		if faults := c.Check(i, full); len(faults) != 0 {
			t.Fatalf("page %d of the run in was refused: %v", i, faults)
		}
	}
	faults := c.Check(9, "a sentence.\n")
	if !has(faults, A5) {
		t.Fatalf("a truncated page on a full page of the file gave %v", rules(faults))
	}
	if !strings.Contains(faults[0].Detail, "text layer") {
		t.Errorf("A5 says %q, which is not what it should have measured against", faults[0].Detail)
	}
}

// What the rule says has to send a person to the right place, and on a scan
// the place is the page and not the paper.
func TestTheLengthRuleSaysWhenItMeasuredAgainstTheInk(t *testing.T) {
	c := inked(map[int]float64{9: 0.5})
	full := strings.Repeat("a sentence of the paper that runs to a reasonable length. ", 20)
	for i := 1; i <= 8; i++ {
		c.Check(i, full)
	}
	faults := c.Check(9, "a sentence.\n")
	if len(faults) == 0 {
		t.Fatal("the truncated page was accepted")
	}
	if !strings.Contains(faults[0].Detail, "the ink on it") {
		t.Errorf("A5 says %q, which does not say what it measured against", faults[0].Detail)
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

func TestAPageOfHTMLTheTidierCouldNotConvertIsRefused(t *testing.T) {
	// The TPU paper's first table, whose header spans four columns over five
	// sub-columns. Untable will not guess at it, so it arrives here as
	// markup and the page goes back to be asked again.
	var c Checker
	text := "Table 1 shows the benchmarks.\n\n<table>\n  <tr>\n    <th colspan=\"4\">Layers</th>\n  </tr>\n</table>\n"
	faults := c.Check(1, text)
	if !has(faults, A10) {
		t.Fatalf("a page of HTML was accepted: %v", rules(faults))
	}
	if !strings.Contains(faults[0].Detail, "<table>") {
		t.Errorf("the fault does not name the tag: %v", faults[0].Detail)
	}
}

func TestAPageWithNoMarkupOnItIsNotRefusedForMarkup(t *testing.T) {
	var c Checker
	text := "The bound is $2^n$ and the table follows.\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a clean page was refused for markup: %v", faults)
	}
}

func TestATokenInAngleBracketsIsNotMarkup(t *testing.T) {
	// The Transformer paper prints these and means the tokens, and an author
	// block prints an address the same way.
	var c Checker
	text := "The sequence ends with <EOS> and is filled with <pad>.\n\nWrite to <ada@example.org> for the data.\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a page of content in angle brackets was refused: %v", faults)
	}
}

// An ordered pair is not a tag, however much the letters in it look like
// one. Karp writes an edge as an ordered pair of vertices, u is the
// underline tag, and page 18 of the reducibility paper was refused as HTML
// three times over and then lost. Every letter used here is on the tag list.
func TestAnOrderedPairIsNotMarkup(t *testing.T) {
	var c Checker
	text := "An edge <u,v> joins two vertices, a path runs from <s,t>, and the\n" +
		"entry at <i, j> is one where the predicates <p,q> agree.\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a page of ordered pairs was refused as HTML: %v", faults)
	}
}

// The other half of the same change. Tightening what follows a tag name has
// to leave every shape a reader really writes still being caught.
func TestTheTagShapesAReaderWritesAreStillMarkup(t *testing.T) {
	for _, tag := range []string{
		"<table>", "</table>", "<br/>", "<br />", "<td colspan=\"2\">",
		"<sub>", "<body>", "<a href=\"https://example.org\">", "< table >",
	} {
		var c Checker
		if faults := c.Check(1, "Some prose and then "+tag+" on the page.\n"); !has(faults, A10) {
			t.Errorf("%s was not read as markup", tag)
		}
	}
}

// The two shapes the reading actually lost its place in, a column specifier
// that never ended and a pattern string of one letter, cut down to size.
func TestAPageThatRepeatsItselfIsRefused(t *testing.T) {
	for _, text := range []string{
		"The table is laid out like this:\n\n\\[\n\\begin{array}{" + strings.Repeat("c", 900),
		"The worst case for the pattern:\n\n" + strings.Repeat("a ", 600),
		"A row of the table:\n\n" + strings.Repeat("| --- ", 200) + "|\n",
	} {
		var c Checker
		faults := c.Check(1, text)
		if !has(faults, A12) {
			t.Errorf("a page that repeats itself was accepted: %v", rules(faults))
		}
	}
}

// What a paper really prints has to survive it. A wide table holds its
// columns apart with spaces, a heading gets a rule of hyphens under it, and
// a paper about string matching writes out strings of one letter on purpose.
func TestTheRepetitionAPaperPrintsIsKept(t *testing.T) {
	for _, text := range []string{
		"Throughput" + strings.Repeat(" ", 159) + "4.1\n",
		"Results\n" + strings.Repeat("-", 127) + "\n",
		"The pattern cccccccccc occurs in the text accaccaccacc twice.\n",
		"| a | b |\n|" + strings.Repeat(" --- |", 20) + "\n",
	} {
		var c Checker
		if faults := c.Check(1, text); has(faults, A12) {
			t.Errorf("%q was read as a model repeating itself", text)
		}
	}
}

func TestMarkupInAListingIsPartOfTheListing(t *testing.T) {
	var c Checker
	text := "The template is:\n\n```html\n<table><tr><td>1</td></tr></table>\n```\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a listing about HTML was refused: %v", faults)
	}
}

func TestMarkupInInlineCodeIsPartOfTheProse(t *testing.T) {
	var c Checker
	text := "A paper about the web writes `<table>` in prose and means the word.\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a page that names a tag was refused: %v", faults)
	}
}

// A page of set theory that came back with no dollar sign in it passed
// every math rule there was, because they all read the spans and it had
// none. Six pages of the Paxos paper went into the corpus that way.
func TestA11RefusesMathematicsFlattenedIntoTheProse(t *testing.T) {
	c := &Checker{}
	text := "Condition B3(B) has the form for every B ∈ B: the decree of B ≤ the decree of every earlier ballot, and the set of voters is a subset Q ⊆ P of the priests.\n"
	if !has(c.Faults(3, text), A11) {
		t.Errorf("A11 passed a page with three signs and no span: %v", c.Faults(3, text))
	}
}

// One sign is a glyph that wandered into a sentence and is not a paper's
// mathematics going missing.
func TestA11LeavesOneStraySignAlone(t *testing.T) {
	c := &Checker{}
	text := "The running time is at most n ≤ 400 for every input we tried, which was enough to settle the question.\n"
	if has(c.Faults(3, text), A11) {
		t.Error("A11 refused a page for one sign in a sentence")
	}
}

// A page that wrote its mathematics as mathematics is what the rule wants
// and it says nothing about it, however much notation is inside the spans.
func TestA11SaysNothingWhereThereIsASpan(t *testing.T) {
	c := &Checker{}
	text := "The condition is $B \\in \\mathcal{B}$ and the quorum is $Q \\subseteq P$ with $|Q| \\geq 3$.\n"
	if has(c.Faults(3, text), A11) {
		t.Errorf("A11 refused a page whose mathematics is in spans: %v", c.Faults(3, text))
	}
}

// A listing is code. A shell session full of redirections and comparisons
// is not this paper's mathematics going missing.
func TestA11DoesNotReadAListingAsMathematics(t *testing.T) {
	c := &Checker{}
	text := "The driver is run like this.\n\n```\nsolve --tol ≤0.5 --set ∈A --mode ⊆B\n```\n\nThat is the whole interface.\n"
	if has(c.Faults(3, text), A11) {
		t.Errorf("A11 refused a page for the notation inside a listing: %v", c.Faults(3, text))
	}
}

// A token of one letter in angle brackets lands on the tag list by
// accident. The GNMT mixed word and character model marks the beginning,
// the middle and the end of a word with these, b is the bold tag, and page
// 8 of the paper was refused as HTML three times over and then lost.
func TestAOneLetterMarkerIsNotMarkup(t *testing.T) {
	var c Checker
	text := "There are three prefixes: <B>, <M> and <E>, and Miki becomes <B>M <M>i <M>k <E>i.\n"
	if faults := c.Check(1, text); has(faults, A10) {
		t.Errorf("a page of word markers was refused as HTML: %v", faults)
	}
}

// The other half of it. A tag of one letter in lower case is the tag.
func TestAOneLetterTagInLowerCaseIsStillMarkup(t *testing.T) {
	for _, tag := range []string{"<b>", "</b>", "<i>", "<u>", "<p>"} {
		var c Checker
		if faults := c.Check(1, "Some prose and then "+tag+" on the page.\n"); !has(faults, A10) {
			t.Errorf("%s was not read as markup", tag)
		}
	}
}
