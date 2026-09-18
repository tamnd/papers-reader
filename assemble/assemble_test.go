package assemble

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/code"
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
	if strings.Contains(d.Text(), "} header second {") {
		t.Errorf("one declaration was run into the next:\n%s", d.Text())
	}
	// One fence with the blank line the page had inside it, because the two
	// declarations were one listing on the page. See fenced.
	if !strings.Contains(d.Text(), "}\n\nheader second {") {
		t.Errorf("the blank line between the declarations is gone:\n%s", d.Text())
	}
	if len(d.Paragraphs) != 1 {
		t.Fatalf("the listing came out as %d paragraph(s):\n%s", len(d.Paragraphs), d.Text())
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
	if !strings.Contains(d.Text(), "}\n\nparser ethernet {") {
		t.Errorf("the two parsers were run together:\n%s", d.Text())
	}
}

// A listing that came off the page without a fence gets one, because a
// listing written as prose loses its indentation in every renderer and is
// refused by every translator. This is the P4 paper's section 4, eighteen
// times over.
func TestAListingComesOutFenced(t *testing.T) {
	d := model("The header is declared as follows:\n\n" +
		"header mTag {\n    fields {\n        up1 : 8;\n        up2 : 8;\n    }\n}\n")
	if len(d.Paragraphs) != 2 {
		t.Fatalf("assembled %d paragraphs, want a sentence and a listing:\n%s", len(d.Paragraphs), d.Text())
	}
	got := d.Paragraphs[1].Text
	if !strings.HasPrefix(got, "```\n") || !strings.HasSuffix(got, "\n```") {
		t.Errorf("the listing is not in a fence:\n%s", got)
	}
	if !strings.Contains(got, "\n        up1 : 8;\n") {
		t.Errorf("the indentation did not survive:\n%s", got)
	}
	if blocks, unclosed := code.Blocks(d.Text()); len(blocks) != 1 || unclosed != nil {
		t.Errorf("the document holds %d blocks and %v unclosed, want one closed one", len(blocks), unclosed)
	}
}

// The fence says nothing about the language, because naming one belongs to
// code.Label, which papers split runs over every paragraph and which answers
// `text` when it is not sure. A tag written here would be written twice.
func TestTheFenceNamesNoLanguage(t *testing.T) {
	d := model("header mTag {\n    fields {\n        up1 : 8;\n        up2 : 8;\n    }\n}\n")
	if got := firstLineOf(d.Paragraphs[0].Text); got != "```" {
		t.Errorf("the fence opens with %q, want ```", got)
	}
}

// Prose stays prose. This is the whole risk of fencing anything at all: a
// paragraph wrongly fenced is a paragraph a reader cannot read.
func TestProseIsNotFenced(t *testing.T) {
	for _, s := range []string{
		"The scheduler has three parts; the first reads the queue.\n",
		"We evaluate on ImageNet [12], CIFAR-10 [13] and MNIST [14].\n",
		"Figure 3. Data transfer rate over time\n",
		"| lanes | rate |\n| --- | --- |\n| 4 | 12 |\n",
	} {
		if got := model(s).Text(); strings.Contains(got, "```") {
			t.Errorf("prose was fenced:\n%s", got)
		}
	}
}

// The numbered clauses of a definition end in semicolons and are not a
// program. Fenced they render as a wall of monospace with the mathematics
// in them read as listing text, which is what happened to two paragraphs of
// the Rabin and Scott paper.
func TestNumberedClausesOfADefinitionAreNotFenced(t *testing.T) {
	d := model("(i) $U$ is in $T$;\n(ii) $U$ is the union of some of the classes;\n(iii) the relation $E$ has finite index.\n")
	if got := d.Text(); strings.Contains(got, "```") {
		t.Errorf("a definition was fenced:\n%s", got)
	}
}

// A row of a formal proof has an assignment in it and every row of the
// table has one, which is enough marks to look like a program line by line.
// Table 1 of Hoare's paper went into the corpus fenced on that evidence.
func TestATableOfAssignmentsIsNotFenced(t *testing.T) {
	d := model("| Line | Proof | Rule |\n| --- | --- | --- |\n| 1 | $x = y \\{ r := x \\}$ | D0 |\n| 2 | $y = z \\{ q := 0 \\}$ | D0 |\n")
	if got := d.Text(); strings.Contains(got, "```") {
		t.Errorf("a table was fenced:\n%s", got)
	}
}

// A paragraph that arrived fenced is left as it is. The layout path writes
// its own fences and fencing one twice would put the opening fence inside
// the listing.
func TestAFencedListingIsNotFencedAgain(t *testing.T) {
	page := "```p4\nheader mTag {\n    fields {\n        up1 : 8;\n    }\n}\n```\n"
	d := Join([]Page{{Number: 1, Text: page, Model: true}})
	if got := d.Text(); strings.Count(got, "```") != 2 {
		t.Errorf("the listing was fenced twice:\n%s", got)
	}
}

// A listing with a fence of its own in it needs a longer one round it, which
// is what CommonMark says and what the shell examples of a paper that quotes
// a Markdown file would need.
func TestAListingHoldingAFenceGetsALongerOne(t *testing.T) {
	page := "print(x);\nprint(y);\n// the fence below is ```\n"
	d := Join([]Page{{Number: 1, Text: page, Model: true}})
	if got := firstLineOf(d.Paragraphs[0].Text); got != "````" {
		t.Errorf("the fence is %q, want one longer than the run inside it", got)
	}
	if blocks, unclosed := code.Blocks(d.Text()); len(blocks) != 1 || unclosed != nil {
		t.Errorf("the document holds %d blocks and %v unclosed, want one closed one", len(blocks), unclosed)
	}
}

// A break inside a block is the reader guessing at the foot of a column and
// the halves go back together. A break between two declarations is a blank
// line the paper set and it stays. The P4 paper has both.
func TestABreakInsideABlockIsClosedUp(t *testing.T) {
	inside := model("table mTag_table {\n\nreads {\n        vlan.vid : exact;\n    }\n    max_size : 20000;\n}\n")
	if got := inside.Text(); strings.Contains(got, "{\n\nreads") {
		t.Errorf("the column break inside the table is still a blank line:\n%s", got)
	}
	between := model("header first {\n    fields {\n        a : 8;\n    }\n}\n\n" +
		"header second {\n    fields {\n        b : 8;\n    }\n}\n")
	if got := between.Text(); !strings.Contains(got, "}\n\nheader second {") {
		t.Errorf("the line the paper set between two declarations is gone:\n%s", got)
	}
}

// A listing that ran over a page break is one listing, and the page count
// says it crossed one. That is what sends a reader to the right page.
func TestAFencedListingKeepsItsPages(t *testing.T) {
	d := Join([]Page{
		{Number: 4, Text: "parser start{\n", Model: true},
		{Number: 5, Text: "ethernet;\n}\n", Model: true},
	})
	if len(d.Paragraphs) != 1 {
		t.Fatalf("assembled %d paragraphs, want one listing:\n%s", len(d.Paragraphs), d.Text())
	}
	if p := d.Paragraphs[0]; p.Page != 4 || p.Pages != 2 {
		t.Errorf("the listing begins on page %d and runs across %d, want page 4 across 2", p.Page, p.Pages)
	}
}

func firstLineOf(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
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

// A page that sets a theorem in the same indented block as the algorithm it
// is about hands the assembler one paragraph, and the theorem must not end up
// inside the fence.
func TestAProseTailIsCutOffAListing(t *testing.T) {
	d := model("    x := 1;\n    WHILE x < n DO x := x + 1;\n    END;\n    THEOREM 13. *The algorithm takes linear time.*\n    *Proof.* It visits each point once.\n")
	got := d.Text()
	if !strings.Contains(got, "```") {
		t.Fatalf("the listing lost its fence:\n%s", got)
	}
	fence := strings.LastIndex(got, "```")
	if strings.Index(got, "THEOREM 13.") < fence {
		t.Errorf("the theorem was fenced with the listing:\n%s", got)
	}
}

// Emphasis all the way down was never a listing, so nothing is cut off it.
func TestAParagraphOfProseIsNotCutUp(t *testing.T) {
	d := model("*Proof.* The bound holds because each point is visited once.\n")
	if len(d.Paragraphs) != 1 {
		t.Errorf("assembled %d paragraphs, want 1: %q", len(d.Paragraphs), d.Text())
	}
}

// An asterisk in a listing is multiplication, not italics.
func TestArithmeticIsNotReadAsEmphasis(t *testing.T) {
	d := model("    a := b * c;\n    d := e * f;\n")
	if got := d.Text(); !strings.Contains(got, "```") {
		t.Errorf("a listing of multiplications lost its fence:\n%s", got)
	}
}

func TestACaptionIsCutOffTheFrontOfAListing(t *testing.T) {
	d := model(`Algorithm 4. Construction of the automaton.
begin
    queue ← empty
    for each symbol a do
        begin
            next(0, a) ← goto(0, a)
        end
end`)
	if len(d.Paragraphs) != 2 {
		t.Fatalf("got %d paragraphs, want 2: %v", len(d.Paragraphs), d.Paragraphs)
	}
	if got := d.Paragraphs[0].Text; got != "Algorithm 4. Construction of the automaton." {
		t.Errorf("first paragraph is %q, want the caption on its own", got)
	}
	body := d.Paragraphs[1].Text
	if !strings.HasPrefix(body, "```") || !strings.HasSuffix(body, "```") {
		t.Errorf("the listing is not fenced: %q", body)
	}
	if strings.Contains(body, "Algorithm 4") {
		t.Errorf("the caption went inside the fence: %q", body)
	}
}

func TestACaptionOverProseIsLeftWhereItIs(t *testing.T) {
	d := model(`Algorithm 4. Construction of the automaton.
The construction reads the goto function once and writes the next move
function as it goes, so the whole of it is one pass over the states.`)
	if len(d.Paragraphs) != 1 {
		t.Fatalf("got %d paragraphs, want 1: %v", len(d.Paragraphs), d.Paragraphs)
	}
}
