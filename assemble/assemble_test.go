package assemble

import (
	"strings"
	"testing"
)

func join(pages ...string) *Document {
	in := make([]Page, len(pages))
	for i, text := range pages {
		in[i] = Page{Number: i + 1, Text: text}
	}
	return Join(in)
}

func TestAParagraphContinuedOnTheNextPageIsOneParagraph(t *testing.T) {
	d := join(
		"the argument is set out at the foot of the page and\n",
		"carries on at the head of the next one.\n",
	)
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
	want := "the argument is set out at the foot of the page and carries on at the head of the next one."
	if got := d.Paragraphs[0].Text; got != want {
		t.Errorf("assembled %q", got)
	}
	if d.Paragraphs[0].Page != 1 {
		t.Errorf("the paragraph is on page %d, want the page it began on", d.Paragraphs[0].Page)
	}
	if d.Paragraphs[0].Pages != 2 {
		t.Errorf("the paragraph ran across %d pages, want 2", d.Paragraphs[0].Pages)
	}
}

func TestASentenceThatEndsAtTheFootOfThePageIsNotJoined(t *testing.T) {
	d := join(
		"the section ends here.\n",
		"The next one starts on its own page.\n",
	)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestOnlyTheFirstParagraphOfAPageCanContinueThePageBefore(t *testing.T) {
	// The second paragraph of a page also starts lower case, and the break
	// before it was decided by the geometry, which knows more than the
	// punctuation does.
	d := join(
		"the first page ends mid sentence and\n",
		"continues here at the top\n\nand this is a second paragraph that also starts lower case\n",
	)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
	if strings.Contains(d.Paragraphs[0].Text, "second paragraph") {
		t.Errorf("the second paragraph of the page was swallowed: %q", d.Paragraphs[0].Text)
	}
}

func TestAWordBrokenAcrossAPageBreakIsHealed(t *testing.T) {
	d := join(
		"the compiler emits an intermedi-\n",
		"ate form before it emits the code\n",
	)
	got := d.Paragraphs[0].Text
	if !strings.Contains(got, "intermediate form") {
		t.Errorf("assembled %q, want the word healed", got)
	}
	if strings.Contains(got, "-") {
		t.Errorf("assembled %q, want the hyphen gone", got)
	}
}

func TestACompoundBrokenAcrossAPageBreakIsLeftAlone(t *testing.T) {
	d := join(
		"the model was evaluated on the English-\n",
		"to-German task and did well\n",
	)
	if got := d.Paragraphs[0].Text; !strings.Contains(got, "English-to-German") {
		t.Errorf("assembled %q, want the compound kept", got)
	}
}

func TestABibliographyEntryBrokenAfterAnInitialIsOneEntry(t *testing.T) {
	// A full stop after a single capital is an initial and not the end of
	// anything, and a bibliography is where that happens at a page break.
	d := join(
		"[14] Shannon, C. E.\n",
		"a mathematical theory of communication. Bell System Technical Journal, 1948.\n",
	)
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
}

func TestAParagraphThatStartsWithACapitalStartsANewParagraph(t *testing.T) {
	d := join(
		"the page ends without a stop\n",
		"The next page opens a new sentence\n",
	)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestTheFirstAndLastPagesAreRecorded(t *testing.T) {
	d := Join([]Page{{Number: 3, Text: "a.\n"}, {Number: 4, Text: "b.\n"}, {Number: 9, Text: "c.\n"}})
	if d.First != 3 || d.Last != 9 {
		t.Errorf("the document runs from page %d to %d, want 3 to 9", d.First, d.Last)
	}
}

func TestEveryParagraphKnowsThePageItBeganOn(t *testing.T) {
	d := Join([]Page{
		{Number: 6, Text: "the first paragraph.\n\nthe second paragraph.\n"},
		{Number: 7, Text: "The third paragraph.\n"},
	})
	want := []int{6, 6, 7}
	if len(d.Paragraphs) != len(want) {
		t.Fatalf("assembled %d paragraphs, want %d", len(d.Paragraphs), len(want))
	}
	for i, n := range want {
		if d.Paragraphs[i].Page != n {
			t.Errorf("paragraph %d is on page %d, want %d", i, d.Paragraphs[i].Page, n)
		}
	}
}

func TestAnEmptyPageDoesNotBreakTheJoin(t *testing.T) {
	// A plate or a blank verso between two halves of a sentence. The page
	// carried no text, so it has nothing to say about the join.
	d := join(
		"the argument carries on past\n",
		"\n",
		"the blank page in the middle of it.\n",
	)
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
}

func TestTheDocumentIsOneLinePerParagraph(t *testing.T) {
	d := join("the first.\n", "The second.\n")
	if got := d.Text(); got != "the first.\n\nThe second.\n" {
		t.Errorf("the document reads %q", got)
	}
}

func TestNoPagesMakeAnEmptyDocument(t *testing.T) {
	d := Join(nil)
	if len(d.Paragraphs) != 0 {
		t.Errorf("assembled %d paragraphs from no pages", len(d.Paragraphs))
	}
	if d.Text() != "" {
		t.Errorf("an empty document reads %q, want nothing", d.Text())
	}
}

// The pages the layout path writes hold more than prose, and a blank line
// inside a fence or a display equation is content rather than a paragraph
// break.

func TestAListingWithABlankLineStaysOneBlock(t *testing.T) {
	page := "```python\ndef f(x):\n    y = x + 1\n\n    return y\n```\n\nand the prose after it.\n"
	d := join(page)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
	want := "```python\ndef f(x):\n    y = x + 1\n\n    return y\n```"
	if got := d.Paragraphs[0].Text; got != want {
		t.Errorf("the listing came out as %q", got)
	}
}

func TestAListingKeepsItsIndentation(t *testing.T) {
	page := "```text\nline one\n        deeply indented\n```\n"
	d := join(page)
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
	if !strings.Contains(d.Paragraphs[0].Text, "\n        deeply indented") {
		t.Errorf("the indentation was lost: %q", d.Paragraphs[0].Text)
	}
}

func TestALongerFenceIsNeededToCloseALongerOne(t *testing.T) {
	page := "````markdown\n```\nnested\n```\n````\n\nprose.\n"
	d := join(page)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestADisplayEquationWithABlankLineStaysOneBlock(t *testing.T) {
	page := "$$\na = b\n\nc = d\n$$\n\nand the prose after it.\n"
	d := join(page)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
	if got, want := d.Paragraphs[0].Text, "$$\na = b\n\nc = d\n$$"; got != want {
		t.Errorf("the equation came out as %q", got)
	}
}

func TestAFenceThatNeverClosesIsStillKept(t *testing.T) {
	d := join("```text\nthe page ran out here\n")
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
	if !strings.Contains(d.Paragraphs[0].Text, "the page ran out here") {
		t.Errorf("the unclosed block was dropped: %q", d.Paragraphs[0].Text)
	}
}

func TestAHeadingDoesNotSwallowTheParagraphUnderIt(t *testing.T) {
	// A heading ends without terminal punctuation and the paragraph under
	// it can start lower case, which is exactly what the continuation rule
	// looks for.
	d := join("## 3.1 Gated units\n", "the paragraph under the heading.\n")
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestABlockIsNeverJoinedAcrossAPageBreak(t *testing.T) {
	for _, right := range []string{
		"```text\nlisting\n```\n",
		"$$\na = b\n$$\n",
		"| a | b |\n| --- | --- |\n",
		"![figure](images/one.png)\n",
	} {
		d := join("the sentence that ran out of room and\n", right)
		if len(d.Paragraphs) != 2 {
			t.Errorf("%q was joined onto the prose before it: %q", right, d.Text())
		}
	}
}

// model is one page read by a model, which is the case where the blank
// lines in the page are a guess rather than a measurement.
func model(text string) *Document {
	return Join([]Page{{Number: 1, Text: text, Model: true}})
}

func TestAModelsBreakInTheMiddleOfASentenceIsHealed(t *testing.T) {
	// The foot of the left column and the head of the right one, which is
	// where a model reading two columns puts a paragraph break it invented.
	d := model("using a partitioning function on\n\nthe intermediate key.\n")
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
	want := "using a partitioning function on the intermediate key."
	if got := d.Paragraphs[0].Text; got != want {
		t.Errorf("assembled %q, want %q", got, want)
	}
	if d.Paragraphs[0].Pages != 1 {
		t.Errorf("the paragraph ran across %d pages, want 1: it never left the page", d.Paragraphs[0].Pages)
	}
}

func TestANativeBreakInTheMiddleOfThePageStands(t *testing.T) {
	// The same text with the break measured rather than guessed. The
	// coordinates said there were two paragraphs here, so there are two.
	d := join("using a partitioning function on\n\nthe intermediate key.\n")
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestTheRuleIsNoWiderInsideAModelsPage(t *testing.T) {
	for _, text := range []string{
		// Finished above.
		"the section ends here.\n\nThe next one starts under it.\n",
		// Upper case below.
		"a list of the things that follow\n\nThe first of them.\n",
		// A block on either side.
		"the sentence that ran out of room and\n\n```text\nlisting\n```\n",
		"## 3.1 Gated units\n\nthe paragraph under the heading.\n",
	} {
		if d := model(text); len(d.Paragraphs) != 2 {
			t.Errorf("%q assembled as %d paragraphs, want 2: %q", text, len(d.Paragraphs), d.Text())
		}
	}
}

func TestAFigureFloatsIntoTheMiddleOfAColumn(t *testing.T) {
	// Page 7 of the MapReduce paper, where the figure sits between two
	// halves of the same sentence.
	d := model("Each machine had two 160GB IDE\n\nFigure 2. Data transfer rate over time\n\ndisks, and a gigabit Ethernet link.\n")
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
	want := "Each machine had two 160GB IDE disks, and a gigabit Ethernet link."
	if got := d.Paragraphs[0].Text; got != want {
		t.Errorf("assembled %q, want %q", got, want)
	}
	if got := d.Paragraphs[1].Text; got != "Figure 2. Data transfer rate over time" {
		t.Errorf("the caption came out as %q", got)
	}
}

func TestACaptionIsNotTheStartOfTheSentenceUnderIt(t *testing.T) {
	// Without the caption in the way the second half would join the first
	// half, so this is the case the step over has to get right and not the
	// case where it joins the caption itself to something.
	d := model("Figure 2. Data transfer rate over time\n\ndisks, and a gigabit Ethernet link.\n")
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want 2: %q", len(d.Paragraphs), d.Text())
	}
}

func TestASentenceAboutAFigureIsNotACaption(t *testing.T) {
	// "Figure 2 shows" opens the paragraph under the caption of Figure 2 in
	// the MapReduce paper. A caption has punctuation after the number and
	// this does not, which is the whole of the difference.
	for _, s := range []string{
		"Figure 2 shows the progress of the computation over time",
		"Table 1 lists the counters the library provides",
	} {
		if caption.MatchString(s) {
			t.Errorf("%q was read as a caption", s)
		}
	}
	for _, s := range []string{
		"Figure 2. Data transfer rate over time",
		"Figure 3: Data transfer rates for the sort program",
		"Table 1) Counters",
		"Fig. 4. The execution overview",
		"Algorithm 1. Backup tasks",
	} {
		if !caption.MatchString(s) {
			t.Errorf("%q was not read as a caption", s)
		}
	}
}

func TestOnlyOneCaptionIsSteppedOver(t *testing.T) {
	d := model("the sentence that ran out of room and\n\n## 3.1 Gated units\n\ncarries on here.\n")
	if len(d.Paragraphs) != 3 {
		t.Fatalf("assembled %d paragraphs, want 3: %q", len(d.Paragraphs), d.Text())
	}
}

// A listing that came off the page without a fence is not prose, and the
// continuation rule has no business running one brace into the next. Every
// line here is typeset in the test and the shape is the P4 paper's header
// declarations, which is where this was found.
func TestAListingWithNoFenceIsNotJoinedToTheOneAfterIt(t *testing.T) {
	page := "header first {\n" +
		"    fields {\n" +
		"        a : 8;\n" +
		"        b : 8;\n" +
		"    }\n" +
		"}\n\n" +
		"header second {\n" +
		"    fields {\n" +
		"        c : 8;\n" +
		"    }\n" +
		"}\n"
	d := Join([]Page{{Number: 1, Text: page, Model: true}})
	if len(d.Paragraphs) != 2 {
		t.Fatalf("the two declarations came out as %d paragraph(s):\n%s", len(d.Paragraphs), d.Text())
	}
	if strings.Contains(d.Text(), "} header second {") {
		t.Errorf("one declaration was run into the next:\n%s", d.Text())
	}
}

// A listing does run over the foot of a page, and the halves of it belong
// back together. This is the P4 parser: page 4 ends on the line that opens the
// block and page 5 holds the two that close it.
func TestAListingSplitAcrossAPageBreakIsPutBackTogether(t *testing.T) {
	d := Join([]Page{
		{Number: 4, Text: "The mTag state machine is written as follows.\n\nparser start{\n", Model: true},
		{Number: 5, Text: "ethernet;\n}\n\nparser ethernet {\n    switch(ethertype) {\n        case 0x8100: vlan;\n    }\n}\n", Model: true},
	})
	want := "parser start{\nethernet;\n}"
	if !strings.Contains(d.Text(), want) {
		t.Errorf("the halves of the parser did not come back together:\n%s", d.Text())
	}
	if strings.Contains(d.Text(), "parser start{ ethernet;") {
		t.Errorf("the halves were joined as prose, with a space:\n%s", d.Text())
	}
}

// The short form of the same thing. Two lines, both of them program, and the
// run is shorter than the one audit rule C08 needs before it says anything.
func TestATwoLineListingIsNotProseEither(t *testing.T) {
	page := "parser start{ ethernet;\n}\n\nparser ethernet {\n    switch(x) {\n        case 1: vlan;\n    }\n}\n"
	d := Join([]Page{{Number: 1, Text: page, Model: true}})
	if len(d.Paragraphs) != 2 {
		t.Fatalf("the two parsers came out as %d paragraph(s):\n%s", len(d.Paragraphs), d.Text())
	}
}

// The other side of the threshold. A paragraph of prose with a couple of
// semicolons in it is still prose, and a sentence cut in half at a page
// break still has to be put back together.
func TestAParagraphWithAFewSemicolonsIsStillProse(t *testing.T) {
	left := "The scheduler has three parts;\n" +
		"the first reads the queue, the second sorts it;\n" +
		"and the third hands the work out to the lanes, one\n" +
		"item at a time, until the queue is empty and the run is"
	right := "finished and the report is written."
	d := Join([]Page{
		{Number: 1, Text: left + "\n", Model: true},
		{Number: 2, Text: right + "\n", Model: true},
	})
	if len(d.Paragraphs) != 1 {
		t.Fatalf("a sentence broken at a page break came out as %d paragraphs:\n%s", len(d.Paragraphs), d.Text())
	}
}
