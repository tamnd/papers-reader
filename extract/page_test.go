package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

func read(groups ...[]poppler.TextLine) Page {
	return Read(page(1, groups...), nil)
}

func TestTheLinesOfAParagraphAreJoinedIntoOne(t *testing.T) {
	p := read(column(60, 100,
		"The lines of a paragraph are set by the",
		"typesetter to fit the measure, and none",
		"of those breaks is the author's."))
	if len(p.Paragraphs) != 1 {
		t.Fatalf("read %d paragraphs, want 1: %q", len(p.Paragraphs), p.Text())
	}
	want := "The lines of a paragraph are set by the typesetter to fit the measure, and none of those breaks is the author's."
	if got := p.Paragraphs[0].Text; got != want {
		t.Errorf("read %q", got)
	}
	if p.Paragraphs[0].Lines != 3 {
		t.Errorf("counted %d lines, want 3", p.Paragraphs[0].Lines)
	}
	if p.Paragraphs[0].Continues {
		t.Error("a paragraph that ends in a full stop was marked as continuing")
	}
}

func TestAnIndentedFirstLineStartsANewParagraph(t *testing.T) {
	// What most of the hundred use, and the only mark they use: no extra
	// leading, just the indent.
	lines := column(60, 100, "the first paragraph runs to here and", "ends on this line without a stop")
	lines = append(lines, put(72, 124, "the second paragraph starts indented"))
	lines = append(lines, put(60, 136, "and carries on flush like this one"))

	p := read(lines)
	if len(p.Paragraphs) != 2 {
		t.Fatalf("read %d paragraphs, want 2: %q", len(p.Paragraphs), p.Text())
	}
	if !strings.HasPrefix(p.Paragraphs[1].Text, "the second paragraph") {
		t.Errorf("the second paragraph is %q", p.Paragraphs[1].Text)
	}
}

func TestExtraLeadingStartsANewParagraph(t *testing.T) {
	lines := column(60, 100, "the first paragraph is set", "in two lines like this")
	lines = append(lines, column(60, 150, "the second is set well below it", "with a line of space between")...)

	p := read(lines)
	if len(p.Paragraphs) != 2 {
		t.Fatalf("read %d paragraphs, want 2: %q", len(p.Paragraphs), p.Text())
	}
}

func TestAParagraphThatChangesColumnIsBrokenAndSaysSo(t *testing.T) {
	left := body(60, 100, 16, "a line at the foot of the left column")
	right := body(330, 100, 16, "a line at the head of the right column")
	p := Read(page(1, left, right), nil)
	if p.Columns != 2 {
		t.Fatalf("read %d columns, want 2", p.Columns)
	}
	var cols []int
	for _, par := range p.Paragraphs {
		cols = append(cols, par.Column)
	}
	if len(cols) < 2 || cols[0] != 0 || cols[len(cols)-1] != 1 {
		t.Errorf("the paragraphs are in columns %v, want the left one first", cols)
	}
	for _, par := range p.Paragraphs {
		if strings.Contains(par.Text, "left") && strings.Contains(par.Text, "right") {
			t.Errorf("a paragraph ran across the gutter: %q", par.Text)
		}
	}
}

func TestAWordBrokenAtALineEndIsHealed(t *testing.T) {
	p := read(column(60, 100,
		"the compiler emits an intermedi-",
		"ate form before it emits the code"))
	got := p.Paragraphs[0].Text
	if !strings.Contains(got, "intermediate form") {
		t.Errorf("read %q, want the word healed", got)
	}
	if strings.Contains(got, "-") {
		t.Errorf("read %q, want the hyphen gone", got)
	}
}

func TestACompoundAtALineEndIsLeftAlone(t *testing.T) {
	// A dash followed by a capital is a compound the author wrote, not a
	// break the typesetter made. Healing it would invent a word.
	p := read(column(60, 100,
		"the method is due to Newton-",
		"Raphson and converges quickly"))
	if got := p.Paragraphs[0].Text; !strings.Contains(got, "Newton-Raphson") {
		t.Errorf("read %q, want the compound kept", got)
	}
}

func TestACompoundBrokenAtItsOwnHyphenIsLeftAlone(t *testing.T) {
	// The capital test does not catch this one: what follows the break is
	// lower case and the hyphen is still the author's.
	p := read(column(60, 100,
		"the model was evaluated on the English-",
		"to-German translation task and did well"))
	if got := p.Paragraphs[0].Text; !strings.Contains(got, "English-to-German") {
		t.Errorf("read %q, want the compound kept", got)
	}
}

func TestAParagraphThatEndsMidSentenceIsMarkedAsContinuing(t *testing.T) {
	p := read(column(60, 100, "the sentence carries on past the foot of", "the page and is not finished here"))
	if !p.Paragraphs[0].Continues {
		t.Error("a paragraph with no terminal punctuation was not marked as continuing")
	}
	if p.Paragraphs[0].Hyphen {
		t.Error("a paragraph that does not end in a hyphen was marked as ending in one")
	}
}

func TestAParagraphThatEndsInAHyphenSaysSo(t *testing.T) {
	p := read(column(60, 100, "the word runs over the foot of the page and is hyphen-"))
	par := p.Paragraphs[0]
	if !par.Hyphen || !par.Continues {
		t.Errorf("Hyphen is %v and Continues is %v, want both true", par.Hyphen, par.Continues)
	}
}

func TestAClosingQuoteAfterTheStopStillEndsTheSentence(t *testing.T) {
	p := read(column(60, 100, `the author calls it "an accident of the machine."`))
	if p.Paragraphs[0].Continues {
		t.Error("a sentence that ends in a quoted stop was marked as continuing")
	}
}

func TestThePageFileIsOneLinePerParagraph(t *testing.T) {
	lines := column(60, 100, "the first paragraph.", "")
	lines = append(lines, put(60, 150, "The second paragraph."))
	got := Read(page(1, lines), nil).Text()
	if got != "the first paragraph.\n\nThe second paragraph.\n" {
		t.Errorf("the page file reads %q", got)
	}
}

func TestAPageWithNothingOnItIsEmpty(t *testing.T) {
	p := Read(page(4), nil)
	if !p.Empty() {
		t.Error("a page with no words on it was not empty")
	}
	if p.Number != 4 {
		t.Errorf("the page is numbered %d, want the number it came in with", p.Number)
	}
}

func TestTheFurnitureIsGoneBeforeTheParagraphsAreMade(t *testing.T) {
	pages := paper(8, func(int) []poppler.TextLine {
		return []poppler.TextLine{put(60, 30, "Journal of Nothing in Particular")}
	})
	f := FindFurniture(pages)
	got := Read(pages[0], f).Text()
	if strings.Contains(got, "Journal") {
		t.Errorf("the running head is in the page file: %q", got)
	}
}
