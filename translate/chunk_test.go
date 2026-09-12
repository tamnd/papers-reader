package translate

import (
	"strings"
	"testing"
)

// Every fixture in this file is typeset here rather than taken from a paper.
// The corpus is public and the papers in it are not all freely licensed, and
// a test file is the last place anybody would think to look for a copyright
// problem.

func TestABodyUnderBudgetIsOneChunk(t *testing.T) {
	body := "The model reads the page.\n\nThen it writes the page out again.\n"
	got := Chunks(body)
	if len(got) != 1 {
		t.Fatalf("%d chunks, want one", len(got))
	}
	if got[0].Text != "The model reads the page.\n\nThen it writes the page out again." {
		t.Errorf("the chunk is %q", got[0].Text)
	}
}

func TestAnEmptyBodyIsNoChunksAtAll(t *testing.T) {
	for _, body := range []string{"", "\n", "   \n\n\t\n"} {
		if got := Chunks(body); got != nil {
			t.Errorf("%q chunked into %d chunks", body, len(got))
		}
	}
}

func TestTheChunksJoinBackIntoTheBody(t *testing.T) {
	// The one property the whole design rests on: nothing is lost between
	// the blocks and joining the answers with a blank line puts the section
	// back together.
	body := strings.Repeat(paragraph(500)+"\n\n", 30)
	var parts []string
	for _, c := range Chunks(body) {
		parts = append(parts, c.Text)
	}
	if got, want := strings.Join(parts, "\n\n"), strings.TrimSpace(body); got != want {
		t.Errorf("the chunks join into %d characters and the body is %d", len(got), len(want))
	}
}

func TestAChunkStaysUnderTheCharacterBudget(t *testing.T) {
	body := strings.Repeat(paragraph(500)+"\n\n", 40)
	got := Chunks(body)
	if len(got) < 2 {
		t.Fatalf("%d chunks for %d characters", len(got), len(body))
	}
	for i, c := range got {
		if n := len([]rune(c.Text)); n > ChunkChars {
			t.Errorf("chunk %d is %d characters, over the %d budget", i+1, n, ChunkChars)
		}
	}
}

func TestABlockOverBudgetGoesAlone(t *testing.T) {
	// Cutting it would be worse. A paragraph of nine thousand characters is
	// one ask that might be refused, and half a paragraph is one ask that
	// will come back with a sentence finished the way the model felt like
	// finishing it.
	long := paragraph(ChunkChars + 3000)
	body := "Before.\n\n" + long + "\n\nAfter.\n"
	got := Chunks(body)
	if len(got) != 3 {
		t.Fatalf("%d chunks, want the long block on its own between two short ones", len(got))
	}
	if got[1].Text != long {
		t.Errorf("the long block was cut: %d characters of %d", len([]rune(got[1].Text)), len([]rune(long)))
	}
}

func TestAFenceIsNotCutHoweverLongItIs(t *testing.T) {
	// The blank line in the middle of the listing is the thing that would
	// cut it, and it is inside a protected span, so it is not a boundary.
	fence := "```go\nfunc main() {\n\tread()\n\n\twrite()\n}\n```"
	body := "The loop is this.\n\n" + fence + "\n\nAnd that is all of it.\n"
	got := Chunks(body)
	if len(got) != 1 {
		t.Fatalf("%d chunks, want one", len(got))
	}
	if !strings.Contains(got[0].Text, fence) {
		t.Errorf("the fence came apart:\n%s", got[0].Text)
	}
}

func TestAnIndentedFenceKeepsItsIndent(t *testing.T) {
	// A listing inside a list item is indented two spaces, and the indent is
	// what keeps it inside the item. Taking a block to start at its first
	// non-blank character rather than at the start of its first non-blank
	// line ate the indent, and the fence came back out of the list. Found on
	// the masking procedure in the BERT appendix.
	body := "- Replace the word with the\n\n  ```text\n  [MASK]\n  ```\n\n- Or leave it alone.\n"
	got := Chunks(body)
	if len(got) != 1 {
		t.Fatalf("%d chunks, want one", len(got))
	}
	if !strings.Contains(got[0].Text, "\n  ```text\n  [MASK]\n  ```") {
		t.Errorf("the indent was eaten:\n%q", got[0].Text)
	}
}

func TestADisplayEquationWithABlankLineInItIsOneBlock(t *testing.T) {
	display := "$$\n\\sum_{i=1}^{n} x_i\n\n= \\bar{x} n\n$$"
	got := Chunks("Add them up.\n\n" + display + "\n\nAnd divide.\n")
	if len(got) != 1 {
		t.Fatalf("%d chunks, want one", len(got))
	}
	if !strings.Contains(got[0].Text, display) {
		t.Errorf("the equation came apart:\n%s", got[0].Text)
	}
}

func TestAChunkStaysUnderTheSpanBudget(t *testing.T) {
	// Short paragraphs with a lot of mathematics in them. The character
	// budget is nowhere near, and the span budget is what closes the chunk.
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("The bound is $O(n \\log n)$ and the constant is $c_0$.\n\n")
	}
	got := Chunks(b.String())
	if len(got) < 2 {
		t.Fatalf("%d chunks for 80 spans, want the span budget to have closed one", len(got))
	}
	for i, c := range got {
		if c.Spans > ChunkSpans {
			t.Errorf("chunk %d holds %d spans, over the %d budget", i+1, c.Spans, ChunkSpans)
		}
		if n := len(Protect(c.Text)); n != c.Spans {
			t.Errorf("chunk %d says %d spans and has %d", i+1, c.Spans, n)
		}
	}
}

func TestABlockOverTheSpanBudgetGoesAlone(t *testing.T) {
	var b strings.Builder
	b.WriteString("A table of every bound in the paper.\n")
	for i := 0; i < ChunkSpans+10; i++ {
		b.WriteString("| $x_{" + itoa(i) + "}$ |\n")
	}
	body := "Before.\n\n" + b.String() + "\nAfter.\n"
	got := Chunks(body)
	if len(got) != 3 {
		t.Fatalf("%d chunks, want the table on its own", len(got))
	}
	if got[1].Spans <= ChunkSpans {
		t.Errorf("the table holds %d spans, so this is not testing what it says", got[1].Spans)
	}
}

func TestAChunkDoesNotEndOnAHeading(t *testing.T) {
	// "Results" on its own is two syllables with nothing around them, and a
	// translator given only that will pick the wrong word about as often as
	// the right one. It goes with the section it heads.
	// Eleven paragraphs of 500 is 5,520 characters, the heading takes it to
	// 5,532, and the paragraph after it would take it to 6,034. So the
	// heading fits and the chunk closes on it, which is the case this rule
	// is for.
	var b strings.Builder
	for i := 0; i < 11; i++ {
		b.WriteString(paragraph(500) + "\n\n")
	}
	b.WriteString("## Results\n\n")
	b.WriteString(paragraph(500) + "\n")
	got := Chunks(b.String())
	if len(got) != 2 {
		t.Fatalf("%d chunks, want two", len(got))
	}
	if strings.HasSuffix(got[0].Text, "## Results") {
		t.Errorf("the first chunk ends on the heading:\n...%s", last(got[0].Text, 80))
	}
	if !strings.HasPrefix(got[1].Text, "## Results\n\n") {
		t.Errorf("the heading did not move to the chunk it heads:\n%s", last(got[1].Text, 80))
	}
}

func TestABodyOfNothingButHeadingsIsStillChunkedEvenly(t *testing.T) {
	// The cap on moving headings forward. A paper whose own table of
	// contents lands in a section is the case. Without the cap every chunk
	// would hand itself to the next one, and the budget would be broken by
	// the rule that is meant to make the chunks readable.
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("## Section " + itoa(i) + " of the paper, at some length\n\n")
	}
	got := Chunks(b.String())
	if len(got) < 2 {
		t.Fatalf("%d chunks, want several", len(got))
	}
	for i, c := range got {
		if strings.TrimSpace(c.Text) == "" {
			t.Errorf("chunk %d is empty", i+1)
		}
		if n := len([]rune(c.Text)); n > ChunkChars {
			t.Errorf("chunk %d is %d characters, over the %d budget", i+1, n, ChunkChars)
		}
	}
}

func TestMovingAHeadingForwardDoesNotBreakTheBudget(t *testing.T) {
	// A heading right before a block that is itself over budget. Moving the
	// heading would put a chunk that is already too big further over, and
	// the heading is better left where it is than used to break the one
	// promise the chunker makes.
	body := paragraph(5900) + "\n\n## Results\n\n" + paragraph(ChunkChars+2000) + "\n"
	for i, c := range Chunks(body) {
		if n := len([]rune(c.Text)); n > ChunkChars && !strings.HasPrefix(c.Text, "The extractor") {
			t.Errorf("chunk %d is %d characters and is not the one long block", i+1, n)
		}
	}
}

func TestTheOffsetsPointAtTheChunkInTheBody(t *testing.T) {
	body := "First paragraph here.\n\nSecond paragraph here.\n"
	rs := []rune(body)
	for _, c := range Chunks(body) {
		if got := string(rs[c.Start:c.End]); !strings.HasPrefix(got, "First") {
			t.Errorf("the offsets cut out %q", got)
		}
	}
}

func TestTheOffsetsAreRunesAndNotBytes(t *testing.T) {
	// The corpus is translated into Vietnamese, Chinese and Japanese, so a
	// chunk of a translated body is full of runes that are three bytes long,
	// and package mathtex counts in runes. Anything else here would line the
	// spans up against the wrong part of the text.
	body := "Kết quả đầu tiên.\n\n" + strings.Repeat("Đoạn văn thứ hai. ", 400) + "\n"
	got := Chunks(body)
	if len(got) != 2 {
		t.Fatalf("%d chunks, want two", len(got))
	}
	rs := []rune(body)
	if string(rs[got[1].Start:got[1].End]) != got[1].Text {
		t.Errorf("the offsets do not cut out the chunk")
	}
}

// paragraph is n characters of plausible prose with no protected spans in it.
func paragraph(n int) string {
	const s = "The extractor reads one page at a time and writes what it sees. "
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(s)
	}
	return strings.TrimSpace(b.String()[:n])
}

func last(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[len(rs)-n:])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
