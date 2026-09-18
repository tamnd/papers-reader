package extract

import (
	"strings"

	"github.com/tamnd/papers-reader/mathtex"
)

// Unhash escapes the number signs the page prints inside its mathematics.
//
// The repair itself is mathtex.Hash and the reason for it is written there.
// What this adds is the fences. A number sign in a listing is a comment marker
// in half the languages a paper prints and the start of a directive in the
// other half, and none of it is TeX, so the lines inside a fence are handed
// over untouched.
//
// The page is passed to the repair in runs of the lines that are not fenced,
// joined as they stand, rather than a line at a time. A display is a span like
// any other and a display is usually three lines, so a pass that looked at one
// line would see a span that opens and never closes and would leave the whole
// of it alone. Neither the runs nor the repair adds or drops a newline, so what
// comes back has the lines of what went in, in order.
func Unhash(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	for i := 0; i < len(lines); i++ {
		if code[i] {
			continue
		}
		j := i
		for j < len(lines) && !code[j] {
			j++
		}
		fixed, n := mathtex.Hash(strings.Join(lines[i:j], "\n"))
		if n > 0 {
			copy(lines[i:j], strings.Split(fixed, "\n"))
		}
		i = j
	}
	return strings.Join(lines, "\n")
}
