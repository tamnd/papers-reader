package split

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/assemble"
)

var (
	// marker is a footnote marker in the prose of a paper.
	marker = regexp.MustCompile(`\[\^(\d{1,3})\]`)

	// opener is the number an endnote begins with. The separator after it
	// is optional because the reader transcribes what the page sets, and a
	// page that sets the number raised gives back "9We should note" as
	// often as it gives back "9. We should note".
	opener = regexp.MustCompile(`^(\d{1,3})[.)]?[ \t]*`)
)

// Markers is the footnote markers a paper's prose carries, by number.
func Markers(paragraphs []assemble.Paragraph) map[int]bool {
	out := map[int]bool{}
	for _, p := range paragraphs {
		for _, m := range marker.FindAllStringSubmatch(p.Text, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out[n] = true
			}
		}
	}
	return out
}

// Endnotes writes the notes of an endnotes section as footnote definitions,
// and says how many it wrote.
//
// A paper that prints its notes at the back has them as numbered paragraphs
// and refers to them with a raised number in the prose, which the reader
// gives back as a Markdown footnote marker. The two halves are the same
// thing and nothing was joining them: Saltzer carries [^1], [^2] and [^3]
// in its second and third sections and its Notes section opens with the
// three paragraphs they refer to, and all three markers were set as a bare
// number linking to nothing. That is rule P02 on that paper.
//
// The run has to start at 1 and count up. That is nearly the whole of the
// safety here, because a numbered paragraph is a common enough shape and a
// list of steps or of design principles would be read as notes without it.
// The run ends where the counting does, so the prose Hoare has after his
// three notes stays prose.
//
// Nothing is rewritten unless the paper marks at least one of the notes.
// Two other papers in the corpus print a Notes section and neither refers
// to it from anywhere, and for those the numbered paragraphs are the whole
// of what the section has to show. Turning those into definitions would
// move them to the foot of the paper and leave the section empty, which is
// a worse page and fixes nothing.
func Endnotes(body string, marked map[int]bool) (string, int) {
	paras := strings.Split(strings.TrimSpace(body), "\n\n")
	want, hit := 1, false
	for _, p := range paras {
		n, _, ok := note(p, want)
		if !ok {
			break
		}
		hit = hit || marked[n]
		want++
	}
	if !hit {
		return body, 0
	}
	count := 0
	for i, p := range paras {
		n, text, ok := note(p, i+1)
		if !ok {
			break
		}
		paras[i] = "[^" + strconv.Itoa(n) + "]: " + text
		count++
	}
	return strings.Join(paras, "\n\n") + "\n", count
}

// note reads a paragraph as the endnote numbered want, and returns the text
// of it with the number taken off the front.
//
// A digit after the number is not a separator, so a paragraph opening "1975
// was the year" is not note 197. The number has to be the one the count is
// up to, which is what the run is.
func note(p string, want int) (int, string, bool) {
	p = strings.TrimSpace(p)
	loc := opener.FindStringSubmatchIndex(p)
	if loc == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(p[loc[2]:loc[3]])
	if err != nil || n != want {
		return 0, "", false
	}
	text := p[loc[1]:]
	if text == "" || (text[0] >= '0' && text[0] <= '9') {
		return 0, "", false
	}
	return n, text, true
}
