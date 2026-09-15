package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/mathtex"
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

// pieces is one row of a figure that is drawn out of type. pdftotext gives
// back a row like this in as many lines as there are sizes set in it, all of
// them at the same height on the page, and the tops of those lines then
// differ by a rounding error rather than by a line.
//
// The sizes go up by more than double each time so that the pieces stay
// separate lines: a piece whose middle falls inside the piece before it is
// the same row as far as the row builder is concerned, which is the right
// answer for a superscript and the wrong one here.
func pieces(y float64) []poppler.TextLine {
	var out []poppler.TextLine
	for i, h := range []float64{4, 9, 19} {
		top := y + float64(i)*0.002
		box := poppler.Box{XMin: 60, YMin: top, XMax: 120, YMax: top + h}
		out = append(out, poppler.TextLine{
			Box:   box,
			Words: []poppler.Word{{Box: box, Text: "piece"}},
		})
	}
	return out
}

// A page that is mostly picture still reads its prose. Before this the pitch
// came off the pieces of the picture rather than off the text, every real
// line of text looked like a jump, and a caption came out as one paragraph
// per line with everything after the first line lost to whoever reads the
// first paragraph and stops.
func TestARowThatCameBackInPiecesIsNotALinePitch(t *testing.T) {
	var lines []poppler.TextLine
	for _, y := range []float64{100, 240, 380} {
		lines = append(lines, pieces(y)...)
	}
	lines = append(lines, column(60, 520,
		"Figure 1: a caption of three lines, of",
		"which the second and the third are the",
		"ones a pitch read off a picture loses.")...)

	p := read(lines)
	var caption string
	for _, par := range p.Paragraphs {
		if strings.HasPrefix(par.Text, "Figure 1:") {
			caption = par.Text
		}
	}
	if !strings.Contains(caption, "a picture loses.") {
		t.Errorf("the caption reads %q, want all three of its lines", caption)
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

// A ligature is the typesetter's decision about two letters that collide,
// and a Type 1 era journal puts it in the text layer. A search for "filing"
// should find it and a translator should not be handed a glyph it has no
// word for.
func TestALigatureIsWrittenOutAsItsLetters(t *testing.T) {
	p := read([]poppler.TextLine{put(60, 100, "Found behind a ﬁling cabinet in the editorial oﬃce.")})
	if got := p.Text(); !strings.Contains(got, "filing cabinet") || !strings.Contains(got, "editorial office") {
		t.Errorf("the ligatures survived: %q", got)
	}
	if got := Ligatures("no ligatures here"); got != "no ligatures here" {
		t.Errorf("Ligatures rewrote text with none in it: %q", got)
	}
	if got := Ligatures("Ærø and œuvre"); got != "Ærø and œuvre" {
		t.Errorf("Ligatures folded letters rather than typesetting: %q", got)
	}
}

// The native path produces no TeX, so a dollar sign in the text is one
// somebody printed. Left bare it opens a math span: the copyright line at the
// foot of the Paxos paper's first page failed acceptance rule A2 over one.
func TestADollarSignOnANativePageIsMoneyAndNotADelimiter(t *testing.T) {
	p := read([]poppler.TextLine{put(60, 100, "Permission to copy is granted for a fee of $00.00 per copy.")})
	text := p.Text()
	if !strings.Contains(text, `\$00.00`) {
		t.Errorf("the dollar sign was left bare: %q", text)
	}
	if _, unclosed := mathtex.Split(text); unclosed != nil {
		t.Errorf("a printed dollar sign opened a math span: %q", text)
	}
}

// A heading sits in the same column as the prose under it, at the same size,
// and is marked off by the space above and below it and by nothing else. The
// step from the heading to the first line of the section is a third wider
// than the step inside a paragraph on page 3 of the Paxos paper, which is
// under the 1.4 the pitch rule wants, so before this the heading was the
// first few words of the paragraph under it. Three of that paper's headings
// in a row went that way, the splitter needs three in a row to believe a
// numbering scheme, and a thirty three page paper published as three
// sections.
func TestAHeadingSetOffByItsWhiteIsItsOwnParagraph(t *testing.T) {
	var lines []poppler.TextLine
	lines = append(lines, put(60, 100, "1 The Problem"))
	lines = append(lines, put(60, 116, "1.1 The Island of Paxos"))
	lines = append(lines, column(60, 132,
		"Early in this millennium the Aegean",
		"island of Paxos was a thriving centre",
		"of the mercantile trade of its day, and",
		"wealth led to political sophistication",
		"and the Paxons replaced their ancient",
		"theocracy with a parliament of its own.")...)

	p := read(lines)
	if len(p.Paragraphs) != 3 {
		t.Fatalf("read %d paragraphs, want 3: %q", len(p.Paragraphs), p.Text())
	}
	for i, want := range []string{"1 The Problem", "1.1 The Island of Paxos"} {
		if got := p.Paragraphs[i].Text; got != want {
			t.Errorf("paragraph %d is %q, want %q", i, got, want)
		}
	}
}

// The white above a line is measured against what the paper itself does, so
// a paper set loose is not read as one paragraph per line. This is the same
// three lines set on double the leading, and they are still one paragraph.
func TestALooselyLeadedParagraphIsStillOneParagraph(t *testing.T) {
	var lines []poppler.TextLine
	for i, s := range []string{
		"A thesis is set on double leading and",
		"every line of it stands as far from the",
		"line above as a paragraph break would.",
	} {
		lines = append(lines, put(60, 100+float64(i)*24, s))
	}

	p := read(lines)
	if len(p.Paragraphs) != 1 {
		t.Fatalf("read %d paragraphs, want 1: %q", len(p.Paragraphs), p.Text())
	}
}

// A line that is taller than its neighbours moves the step without moving
// the space. Line 4 of page 3 of the Paxos paper carries a footnote marker
// and is three points taller than the rest, so the step onto the line under
// it is six points longer than the step onto the line above it and nothing
// has happened. The white is the same on both sides and the paragraph holds
// together.
func TestATallLineDoesNotBreakTheParagraphUnderIt(t *testing.T) {
	tall := put(60, 112, "a line with a footnote marker set above the baseline")
	tall.YMax = tall.YMin + fontSize + 3

	lines := []poppler.TextLine{
		put(60, 100, "the first line of the paragraph runs to here"),
		tall,
		put(60, 127, "and the third carries the sentence to its end."),
		put(60, 139, "and a fourth line to measure the leading by."),
	}

	p := read(lines)
	if len(p.Paragraphs) != 1 {
		t.Fatalf("read %d paragraphs, want 1: %q", len(p.Paragraphs), p.Text())
	}
}
