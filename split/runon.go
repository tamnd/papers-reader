package split

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

// minRunOn is how much prose has to follow a heading on the next line before
// the two are taken for a heading and its section rather than for a list item
// and its continuation. Twelve words is longer than the second line of any
// list item in this corpus and shorter than the opening sentence of any
// section.
const minRunOn = 12

// Unrun separates a heading from the paragraph a reader ran it into.
//
// A paper prints a heading on a line of its own and the section under it
// starts on the next line. Nothing in that says whether there is a blank line
// between them, and a vision model answers either way from one page to the
// next: the Codd paper came back with a blank line after "1. Relational Model
// and Normal Form" and none after "1.1. Introduction". Without one the
// assembler is right to read the two lines as one paragraph, because that is
// what two adjacent lines of Markdown are, and then the heading is inside a
// paragraph of four hundred words where nothing will ever look for it.
//
// It is worth the trouble because of what fails and how quietly. That one
// paper lost its numbering scheme entirely: the chain needs three headings in
// sequence and only the first of its four was on a line by itself, so the
// scheme came out as none, the numbered pass found nothing, and a subheading
// in capitals was picked up by typography and published as the only section
// of the paper. A paper that numbers its sections perfectly well read as a
// paper that numbers nothing.
//
// The conditions are what keep a numbered list out of it. A list item is
// short, its continuation line is short, and its continuation carries on the
// sentence in lower case, and any one of the three is enough to leave the
// paragraph alone. None of this makes a list item safe on its own line, where
// it always looked exactly like a heading and where minChain is what stands
// between it and being read as one. This only declines to make that worse.
func Unrun(in []assemble.Paragraph) []assemble.Paragraph {
	out := make([]assemble.Paragraph, 0, len(in))
	for _, p := range in {
		head, rest, ok := runOn(p.Text)
		if !ok {
			out = append(out, p)
			continue
		}
		// The heading is on the page the paragraph began on. The rest of it is
		// what may have run across a page, so it keeps the span.
		out = append(out,
			assemble.Paragraph{Text: head, Page: p.Page, Pages: 1},
			assemble.Paragraph{Text: rest, Page: p.Page, Pages: p.Pages},
		)
	}
	return out
}

// runOn splits a paragraph into the heading it opens with and the prose under
// it, and says no for a paragraph that is not one.
//
// A heading does not have to carry a number to be one. The other thing that
// says a line is a heading is that it is one of the forty names a paper
// gives a section, and that turned out to be where most of this was needed:
// fourteen papers ran References into the first entry of the bibliography,
// which left the whole reference list inside the conclusion. The name table
// is stronger evidence than the leading digit numberedShape asks for, not
// weaker, because a line that reads exactly "References" and nothing else is
// not a line of anybody's prose.
func runOn(text string) (head, rest string, ok bool) {
	head, rest, found := strings.Cut(text, "\n")
	if !found {
		return "", "", false
	}
	if !candidate(head) {
		return "", "", false
	}
	_, byName := named(head)
	if !byName && !numberedShape(head) {
		return "", "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	if len(strings.Fields(rest)) < minRunOn {
		return "", "", false
	}
	// A section starts a sentence. A list item's second line carries one on.
	//
	// A bibliography does neither: it starts with its first entry, and an
	// entry starts with the label the paper numbers it by. That is allowed
	// only under a heading found by name, because a line of digits under
	// "3.2" is the numbered list this whole function is written to leave
	// alone.
	r := []rune(rest)
	if len(r) == 0 {
		return "", "", false
	}
	if !unicode.IsUpper(r[0]) && !(byName && label.MatchString(rest)) {
		return "", "", false
	}
	return head, rest, true
}

// label is what a bibliography prints in front of its first entry, either
// bracketed or as a number and a stop. The year is not asked for here the
// way bibliography.go asks for it, because the heading above the line has
// already said what the list is.
var label = regexp.MustCompile(`^(?:\[\d{1,3}\]|\d{1,3}\.)\s`)

// numberedShape says whether a line is written the way a numbered heading is,
// under any of the schemes. Which scheme the paper actually uses is not known
// yet and cannot be: DetectScheme reads the paragraphs, and these are the
// paragraphs it is about to read.
//
// Being generous here costs nothing. A line this accepts is a line offered to
// the scheme detector as a candidate, and the detector still has to find two
// more that continue its numbering before it believes any of them.
func numberedShape(line string) bool {
	return atx.MatchString(line) ||
		arabic.MatchString(line) ||
		roman.MatchString(line) ||
		sign.MatchString(line)
}
