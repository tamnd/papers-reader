package extract

import (
	"regexp"
	"strings"
)

// inlineLink is a Markdown inline link that is not an image. Images are
// Unlink's business and are a different mistake with a different repair.
var inlineLink = regexp.MustCompile(`(^|[^!\\])\[([^\]\n]*)\]\(([^)\n]*)\)`)

// inlineSpan is a run of backticks with something between them, which is a
// line of prose quoting markup rather than writing it.
var inlineSpan = regexp.MustCompile("`[^`\n]*`")

// Delink writes a link back as the text the page printed.
//
// The corpus has no links in it. A reference is a numbered entry, a cross
// reference is an anchor, a paper is [[an id]] and a figure is a manifest
// entry. So a link on a page is a reader adding markup, and it always adds
// it in the same place: the paper prints a web address as prose and the
// reader, having read a great deal of Markdown, writes the address as a
// link with itself for the text.
//
// Reference 10 of the MapReduce paper is the one that showed it up. The page
// prints "Jim Gray. Sort benchmark home page. http://research.microsoft.com/
// barc/SortBenchmark/." and the second reading came back with the address in
// brackets and in parentheses, which broke rule R08 as well, because the
// bibliography no longer matched the text the paper printed.
//
// What is kept is the text and not the target. Where the two are the same
// that is the same thing said twice; where they differ, the text is what the
// page prints and the target is what the reader supplied, and the page is
// what this is transcribing. A link with nothing for its text is the one
// case where the target is all there is, and it keeps the target.
//
// A fenced block is left alone, and so is anything between backticks, which
// is a paper writing about Markdown rather than in it.
func Delink(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	for i, line := range lines {
		if code[i] || !strings.Contains(line, "](") {
			continue
		}
		lines[i] = rewrite(line)
	}
	return strings.Join(lines, "\n")
}

// rewrite works on one line, around the inline code spans rather than
// through them.
func rewrite(line string) string {
	var b strings.Builder
	at := 0
	for _, s := range inlineSpan.FindAllStringIndex(line, -1) {
		b.WriteString(flatten(line[at:s[0]]))
		b.WriteString(line[s[0]:s[1]])
		at = s[1]
	}
	b.WriteString(flatten(line[at:]))
	return b.String()
}

func flatten(s string) string {
	return inlineLink.ReplaceAllStringFunc(s, func(m string) string {
		g := inlineLink.FindStringSubmatch(m)
		text := strings.TrimSpace(g[2])
		if text == "" {
			text = strings.TrimSpace(g[3])
		}
		return g[1] + text
	})
}
