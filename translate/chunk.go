package translate

import (
	"strings"
)

// The two budgets a chunk is closed on.
//
// ChunkChars is a target and not a limit, because a block is never split and
// one block can be over budget on its own. The arithmetic behind it: the
// prompt with no body in it is about 5,000 characters and the glossary is
// another 8,000 to 11,000, so a 6,000 character body is a question of about
// 20,000 characters and an answer of about the same. That is a size every
// model on the fleet returns in one piece.
//
// ChunkSpans is the interesting one. It was 15 on Bourbaki, because a 3,503
// character appendix with 45 mathematical spans in it was refused four times
// running, on the span count and then on span 23 coming back wrong, while
// the prose was fine every time. Fifteen turned 479 chunks into 1,552 and
// made one slip cost a quarter of a section instead of all of it. Then it
// became 60, and what changed was not the models: a third to a half of the
// asks in a day were lost before the question was read at all, out of quota
// or answering 403, which turns the bill from per character into per message
// and makes three times the asks not worth paying for. 60 is the current
// answer and it is a number to re-measure, not a principle.
const (
	ChunkChars = 6000
	ChunkSpans = 60
)

// A Chunk is one ask's worth of a section body.
//
// Start and End are rune offsets into the body it was cut from, counted the
// way package mathtex counts, so that the spans a chunk holds can be lined
// up with the spans Protect found in the whole body.
type Chunk struct {
	Text  string
	Spans int
	Start int
	End   int
}

// Chunks cuts a section body into pieces small enough to translate in one
// ask.
//
// A block is never split. A chunk is blocks added until the next one would
// take it past a budget, and a block that is over budget on its own goes
// alone rather than being cut in half. The boundary between blocks is a
// blank line, which the Markdown already has, so joining the answers back
// together with a blank line between them reproduces the block structure
// exactly and there is no reassembly to get wrong.
//
// The body is the body. Front matter is the caller's problem, because the
// fields in it are translated one at a time against their own rules and a
// translator handed a YAML block will reformat it.
func Chunks(body string) []Chunk {
	bs := blocks(body)
	if len(bs) == 0 {
		return nil
	}
	var (
		out  []Chunk
		cur  []block
		size int
		held int
	)
	flush := func() {
		if len(cur) == 0 {
			return
		}
		out = append(out, gather(body, cur))
		cur, size, held = nil, 0, 0
	}
	for _, b := range bs {
		// The blank line that will be put back between two blocks counts
		// against the budget, because it will be in the ask.
		grown := size + b.chars
		if len(cur) > 0 {
			grown += 2
		}
		if len(cur) > 0 && (grown > ChunkChars || held+b.spans > ChunkSpans) {
			// A heading is not left at the end of a chunk. On its own it is
			// two or three words with nothing around them, and "Results" or
			// "Setup" translated with no sight of what follows is the kind of
			// wrong that no rule catches and every reader sees. It goes with
			// the blocks it heads instead.
			moved := trailingHeadings(cur, b)
			cur = cur[:len(cur)-len(moved)]
			flush()
			for _, h := range moved {
				if len(cur) > 0 {
					size += 2
				}
				cur = append(cur, h)
				size += h.chars
				held += h.spans
			}
			grown = size + b.chars
			if len(cur) > 0 {
				grown += 2
			}
		}
		cur = append(cur, b)
		size, held = grown, held+b.spans
	}
	flush()
	return out
}

// A block is one paragraph, fence, display equation, table or list item run,
// with where it sits in the body and how much of it is protected.
type block struct {
	start, end int
	chars      int
	spans      int
	heading    bool
}

// blocks cuts the body at its blank lines.
//
// A blank line inside a protected span is not a boundary, which is what
// keeps a fenced listing with a paragraph break in it, or a display equation
// written over several lines with a gap in the middle, in one piece. This is
// the whole reason the spans are found before the blocks and not after.
func blocks(body string) []block {
	rs := []rune(body)
	inside := make([]bool, len(rs)+1)
	spans := Protect(body)
	for _, s := range spans {
		for i := s.Start; i < s.End && i < len(inside); i++ {
			inside[i] = true
		}
	}

	var (
		out   []block
		start = -1
	)
	shut := func(end int) {
		if start < 0 {
			return
		}
		for end > start && isSpace(rs[end-1]) {
			end--
		}
		if end > start {
			out = append(out, block{start: start, end: end,
				chars:   end - start,
				heading: isHeading(rs[start:end])})
		}
		start = -1
	}
	// A block starts at the beginning of its first non-blank line and not at
	// its first non-blank character. The indent is part of the text: a
	// fenced listing inside a list item is indented two spaces, and a chunk
	// that dropped the indent would come back as a fence that had escaped
	// the list.
	line := 0
	for i := 0; i <= len(rs); i++ {
		if i == len(rs) {
			shut(i)
			break
		}
		if rs[i] != '\n' {
			if start < 0 && !isSpace(rs[i]) {
				start = line
			}
			continue
		}
		if !inside[i] {
			// A boundary is this newline and the next one with nothing but
			// whitespace between them.
			j := i + 1
			for j < len(rs) && rs[j] != '\n' && isSpace(rs[j]) {
				j++
			}
			if j < len(rs) && rs[j] == '\n' && !inside[j] {
				shut(i)
			}
		}
		line = i + 1
	}

	// The spans are handed out to the blocks they fall in. A span that
	// straddles a boundary cannot happen, because a boundary inside a span
	// is not a boundary.
	at := 0
	for i := range out {
		for at < len(spans) && spans[at].Start < out[i].start {
			at++
		}
		for at < len(spans) && spans[at].End <= out[i].end {
			out[i].spans++
			at++
		}
	}
	return out
}

// gather turns a run of blocks into the chunk that will be asked about.
func gather(body string, bs []block) Chunk {
	rs := []rune(body)
	parts := make([]string, 0, len(bs))
	c := Chunk{Start: bs[0].start, End: bs[len(bs)-1].end}
	for _, b := range bs {
		parts = append(parts, string(rs[b.start:b.end]))
		c.Spans += b.spans
	}
	c.Text = strings.Join(parts, "\n\n")
	return c
}

// headingRun is how many headings in a row will move to the next chunk.
//
// Three, because a heading stack in a paper is at most "4 Results" over
// "4.1 Experimental setup" over "4.1.1 Data", and a body that is nothing but
// headings is a real thing when a paper's own table of contents lands in a
// section. Without a cap that body would hand every chunk to the next one
// and the last chunk would hold the lot.
const headingRun = 3

// trailingHeadings is the run of headings at the end of a chunk that will
// move to the next one to sit in front of the block they head.
//
// It moves nothing it cannot afford. A heading that arrives in a chunk that
// is already full has bought nothing and cost the budget, so the run is
// trimmed until it fits in front of next, and a next that is over budget on
// its own takes no headings with it.
func trailingHeadings(bs []block, next block) []block {
	n := len(bs)
	for n > 1 && len(bs)-n < headingRun && bs[n-1].heading {
		n--
	}
	moved := bs[n:]
	for len(moved) > 0 && !fits(moved, next) {
		moved = moved[1:]
	}
	return moved
}

// fits says whether a run of blocks and the block after it are one chunk.
func fits(bs []block, next block) bool {
	chars, spans := next.chars, next.spans
	for _, b := range bs {
		chars += b.chars + 2
		spans += b.spans
	}
	return chars <= ChunkChars && spans <= ChunkSpans
}

// isHeading says whether a block is a heading and nothing else.
//
// Only the ATX form is recognised. The setext form, a line underlined with
// equals signs, does not survive extraction: the extractor writes headings
// with hashes and the corpus has none of the other kind.
func isHeading(rs []rune) bool {
	if len(rs) == 0 || rs[0] != '#' {
		return false
	}
	for _, r := range rs {
		if r == '\n' {
			return false
		}
	}
	return true
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }
