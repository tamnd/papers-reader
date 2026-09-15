package translate

import (
	"fmt"
	"regexp"
	"strings"
)

// Hold takes the fenced listings out of a passage and leaves a marker where
// each one was. It reports the passage and the listings, in order.
//
// A listing is the one thing in a paper that a translation copies rather
// than writes: rule C07 says the fenced regions of a translation are the
// English ones byte for byte. Sending one to a model is therefore asking a
// question whose only right answer is the question, and paying for the
// answer twice, once going out and once coming back.
//
// Models are not good at it. The GPT-3 results table is columns held apart
// by spaces, and every answer for the chunk holding it came back with the
// table reflowed or shortened, so a run gave up on a section whose prose had
// come back correctly three times over. The BERT development set table did
// the same, and so did an inline code span the reader had mistaken for a
// fence. What is being asked for there is transcription, which is the one
// thing a translator has no reason to do.
//
// The marker is written in the shape of a citation, which the prompt already
// names as a span to be copied and which models carry through without
// thinking about it. Nothing else in the corpus is spelled that way: a real
// citation marker holds a paper id, and no paper id is the word listing and
// a number.
func Hold(text string) (string, []string) {
	blocks := fences(text)
	if len(blocks) == 0 {
		return text, nil
	}
	rs := []rune(text)
	var (
		b   strings.Builder
		out []string
		at  int
	)
	for _, f := range blocks {
		if f.start < at || f.end > len(rs) {
			continue
		}
		b.WriteString(string(rs[at:f.start]))
		b.WriteString(marker(len(out) + 1))
		out = append(out, string(rs[f.start:f.end]))
		at = f.end
	}
	b.WriteString(string(rs[at:]))
	return b.String(), out
}

// Unhold puts the listings back where their markers are.
//
// A marker the answer dropped is a listing the answer dropped, and a marker
// it invented has no listing to put there. Neither is repaired here. Verify
// sees the first as a span the source has and the answer has not, and the
// second as a marker the source never had, and refusing is right in both
// cases because what came back is not the same document.
func Unhold(text string, held []string) string {
	for i, block := range held {
		text = strings.ReplaceAll(text, marker(i+1), block)
	}
	return text
}

// marker is what stands in for the nth listing.
func marker(n int) string { return fmt.Sprintf("[[listing-%d]]", n) }

var markerShape = regexp.MustCompile(`\[\[listing-\d+\]\]`)

// Holding reports whether a passage already has something in it written the
// way a marker is written, in which case the passage has to be sent as it
// stands. No paper in this corpus does, and one that did would be a paper
// whose listings could not be told from its text.
func Holding(text string) bool { return markerShape.MatchString(text) }
