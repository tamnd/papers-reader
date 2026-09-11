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
