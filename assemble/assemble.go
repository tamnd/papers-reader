// Package assemble joins the page files of one paper into a document.
//
// A page is where the paper ran out of room. Nothing a reader wants to know
// is expressed by it: a sentence runs across a page break, a word is broken
// across one, and a paragraph that starts at the top of page six started on
// page five. So the pages are joined and the joins are healed, and what
// comes out is a run of paragraphs with the page each one started on kept
// alongside, because the reading app says "page 6 of the PDF" and a person
// checking the extraction against the paper needs to find the page.
package assemble

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/code"
)

// A Page is one extracted page as it sits in work/<id>/pages/NNNN.txt:
// paragraphs one to a line with blank lines between them.
type Page struct {
	Number int
	Text   string
	// Model says the page came out of a model rather than out of the PDF's
	// own text layer, and so that the blank lines in it are a guess.
	//
	// It is what lets Join heal a paragraph that was broken in the middle of
	// the page. See Join for why the two kinds of page are treated
	// differently, and cmd/papers for where the answer comes from.
	Model bool
}

// A Paragraph is one paragraph of the assembled paper.
type Paragraph struct {
	Text string
	// Page is the page it began on, which is the page a reader is sent to.
	Page int
	// Pages is how many pages it ran across, and is one for almost all of
	// them. A paragraph that ran across three pages is worth a look.
	Pages int
}

// A Document is one paper, joined.
type Document struct {
	Paragraphs  []Paragraph
	First, Last int
}

// Join assembles the pages in the order they are given.
//
// The joining rule is the one Bourbaki uses and it is deliberately narrow: a
// page that ends without terminal punctuation and whose next page starts
// lower case is a continuation. Anything else starts a new paragraph.
//
// Narrow because the two mistakes are not equal. Two paragraphs wrongly run
// together read as one confused paragraph and the split point is gone; a
// paragraph wrongly broken in two reads as two paragraphs and a person can
// see where it happened. Over a hundred papers the second is the one to
// prefer.
//
// Where the rule is applied depends on Page.Model. On a page read natively
// the blank lines were put there from the coordinates of the lines, so a
// break inside the page is a fact and the rule is only asked about the join
// between one page and the next. On a page read by a model the blank lines
// are the model's guess at where the paragraphs are, and a model reading a
// two column page guesses wrong in one particular way: it reaches the foot
// of the left column in the middle of a sentence, and starts a new paragraph
// at the head of the right one. Page 9 of the MapReduce book had "using a
// partitioning function on" and then a paragraph break and then "the
// intermediate key", which is that. So on a model's page the rule is asked
// at every break, and it is the same narrow rule: the half above has to end
// unfinished and the half below has to start lower case.
//
// The one thing the two halves are allowed to have between them is a
// caption. See joinAt.
func Join(pages []Page) *Document {
	d := &Document{}
	for _, p := range pages {
		if d.First == 0 || p.Number < d.First {
			d.First = p.Number
		}
		if p.Number > d.Last {
			d.Last = p.Number
		}
		for i, text := range paragraphs(p.Text) {
			at := -1
			if i == 0 || p.Model {
				at = joinAt(d.Paragraphs, text)
			}
			// A listing that ran over the foot of the page. The halves
			// are not prose and JoinText would put a space between them,
			// which is the join that produced `} header vlan {`, but a
			// blank line between them is wrong too: there was no blank
			// line on the page, there was the edge of the paper. So they
			// go back together with the line break the page break stood
			// in for. Only at the head of a page, because a blank line
			// inside one is a blank line the listing really has.
			if at < 0 && i == 0 && len(d.Paragraphs) > 0 {
				last := &d.Paragraphs[len(d.Paragraphs)-1]
				if program(last.Text) && program(text) {
					last.Text += "\n" + text
					last.Pages++
					continue
				}
			}
			if at >= 0 {
				d.Paragraphs[at].Text = JoinText(d.Paragraphs[at].Text, text)
				// Only a join at the head of a page crossed a page break.
				// Pages is what tells a reader a paragraph ran across two
				// of them, and counting a repair inside one page would say
				// it ran across a break that is not there.
				if i == 0 {
					d.Paragraphs[at].Pages++
				}
				continue
			}
			d.Paragraphs = append(d.Paragraphs, Paragraph{Text: text, Page: p.Number, Pages: 1})
		}
	}
	return d
}

// Text is the document as one string, a paragraph to a line.
func (d *Document) Text() string {
	if len(d.Paragraphs) == 0 {
		return ""
	}
	parts := make([]string, len(d.Paragraphs))
	for i, p := range d.Paragraphs {
		parts[i] = p.Text
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// paragraphs cuts a page into paragraphs on its blank lines, except inside a
// block where a blank line is content.
//
// The native path writes prose and nothing else, and for prose a blank line
// is always a paragraph break. The layout path writes fenced listings and
// display equations as well, and both of those can hold a blank line: an
// algorithm with a gap between its setup and its loop, a two part derivation
// with a line between the halves. Cutting there produces half a fence, which
// renders as the rest of the paper in a code block.
func paragraphs(text string) []string {
	var out []string
	var cur []string
	verbatim := false
	fence, display := "", false

	flush := func() {
		if len(cur) == 0 {
			return
		}
		s := strings.Join(cur, "\n")
		if !verbatim {
			s = strings.TrimSpace(s)
		} else {
			s = strings.Trim(s, "\n")
		}
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
		cur, verbatim = nil, false
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			cur = append(cur, line)
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
				flush()
			}
		case display:
			cur = append(cur, line)
			if trimmed == "$$" {
				display = false
				flush()
			}
		case opens(trimmed) != "":
			flush()
			fence, verbatim = opens(trimmed), true
			cur = append(cur, line)
		case trimmed == "$$":
			flush()
			display, verbatim = true, true
			cur = append(cur, line)
		case trimmed == "":
			flush()
		default:
			cur = append(cur, line)
		}
	}
	// A fence or a display block that never closed. What is left is still
	// the end of the page and dropping it would lose it.
	flush()
	return out
}

// opens is the fence a line opens a code block with, or the empty string.
// The fence has to be at least three of the same character and the closing
// one has to be at least as long, which is how a listing that itself
// contains a fence is written.
func opens(trimmed string) string {
	for _, c := range []string{"`", "~"} {
		n := 0
		for n < len(trimmed) && string(trimmed[n]) == c {
			n++
		}
		if n >= 3 {
			return strings.Repeat(c, n)
		}
	}
	return ""
}

// joinAt is the paragraph already assembled that text belongs to the end of,
// or -1 if it starts a paragraph of its own.
//
// Usually the one before it. One further back if the one before it is a
// caption, because a figure floats into the middle of a column and the
// column carries on under it. Page 7 of the MapReduce paper reads "two
// 160GB IDE", then "Figure 2. Data transfer rate over time", then "disks,
// and a gigabit Ethernet link", and joining the caption to the words under
// it produced "Figure 2. Data transfer rate over time disks, and a gigabit
// Ethernet link", which is a sentence about nothing.
//
// The caption stays where it is, under the paragraph it interrupted, which
// is where the figure was and where the reading app wants it.
//
// Only a caption is stepped over. A heading is not: the paragraph under a
// heading belongs to the heading's section and never to the paragraph above
// it. A listing and a table are not, because neither has been seen to float
// into a column the way a figure does and stepping over one would join two
// paragraphs a page apart.
func joinAt(done []Paragraph, text string) int {
	n := len(done)
	if n > 0 && continues(done[n-1].Text, text) {
		return n - 1
	}
	if n > 1 && caption.MatchString(done[n-1].Text) && continues(done[n-2].Text, text) {
		return n - 2
	}
	return -1
}

// caption is the opening of a figure or table caption: the word, a number,
// and the punctuation the typesetter put after the number.
//
// The punctuation is what separates a caption from a sentence about a
// figure. "Figure 2 shows the progress of the computation over time" is
// prose and it opens the paragraph under this very caption in the MapReduce
// paper, so getting this wrong would take a real paragraph out of the flow.
// "Figure 2." and "Figure 2:" and "Figure 2)" are captions.
//
// Package figures has the same regexp with the same reasoning behind it, and
// this is not that one because figures imports assemble. Two copies of six
// words is the cheaper of the two prices.
var caption = regexp.MustCompile(`^(?:Fig(?:ure)?|FIG(?:URE)?|Table|TABLE|Algorithm|ALGORITHM|Listing|LISTING|Chart)\b\.?[ \t]*` +
	`(?:[0-9]+(?:[.\-][0-9a-zA-Z]+)*|[IVXLC]+|[A-Z])[ \t]*[.:)]`)

// block says whether a paragraph is one of the things that is not prose, and
// so can neither continue the paragraph before it nor be continued by the one
// after it. A heading is in the list for the same reason: it ends without
// terminal punctuation, which is exactly what the continuation rule looks
// for, so without this a heading swallows the paragraph under it. A caption
// is in the list for the same reason and it is the same mistake: "Figure 2.
// Data transfer rate over time" ends without terminal punctuation too.
func block(s string) bool {
	switch {
	case opens(s) != "":
		return true
	case caption.MatchString(s):
		return true
	case strings.HasPrefix(s, "$$"):
		return true
	case strings.HasPrefix(s, "#"):
		return true
	case strings.HasPrefix(s, "|"):
		return true
	case strings.HasPrefix(s, "!["):
		return true
	case program(s):
		return true
	}
	return false
}

// program says whether a paragraph is a listing the reader wrote without a
// fence round it.
//
// A listing is not prose and nothing above or below it continues into it, but
// without this the continuation rule reads it as prose and joins it, because
// a line of a program ends without terminal punctuation and the next one
// starts in lower case, which is the whole of what the rule looks for. The P4
// paper is the case: its header declarations came off the page correctly, one
// per paragraph, and the assembler wrote `} header vlan {` and then did it
// again for every parser in the section.
//
// The marks are code.Mark, the same ones audit rule C08 counts, and the
// threshold is different because the question is different. C08 is hunting a
// listing hidden in a body and wants three lines with two marks before it
// says anything. This is asking whether one paragraph is prose, and a
// paragraph where half the lines end in a semicolon or are a brace on their
// own is not prose whether it is two lines long or twenty.
//
// Two marks at the least, because one is a citation with a brace in it or a
// sentence that happened to end in a semicolon. More than half the lines,
// because a paragraph of prose with a couple of semicolons in it is still a
// paragraph of prose, and refusing to join one of those would leave a
// sentence cut in half at a page break. Half exactly is not enough: four
// lines of prose of which two end in a semicolon is a real paragraph and was
// the first thing this refused to join.
func program(s string) bool {
	lines := strings.Split(s, "\n")
	if marks := code.Marks(s); marks >= 2 && marks*2 > len(lines) {
		return true
	}
	// A paragraph whose last line ends in an opening brace is the head of a
	// listing whatever else is in it, and the P4 paper has one that is only
	// that: page 4 ends on `parser start{` and page 5 opens on the two lines
	// that close it. No count of marks finds a single line, and a sentence of
	// prose that ends in an opening brace has not turned up yet.
	return strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "{")
}

// continues says whether the second paragraph is the rest of the first.
func continues(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	if block(left) || block(right) {
		return false
	}
	r := []rune(right)[0]
	if !unicode.IsLower(r) && !strings.ContainsRune(",;", r) {
		return false
	}
	return !closed(left)
}

// JoinText puts two halves of a paragraph back together, healing the word
// that was broken at the break.
//
// Exported because a bibliography entry arrives as several paragraphs, one
// per printed line of the hanging indent, and package refs has to put those
// back together too. The two have to heal a word the same way or the same
// reference would be spelled one way in the body and another in the
// reference list.
func JoinText(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	l := []rune(left)
	if strings.ContainsRune(Hyphens, l[len(l)-1]) {
		word, _, _ := strings.Cut(right, " ")
		if !strings.ContainsAny(word, Hyphens) {
			return strings.TrimRight(left, Hyphens) + right
		}
		return left + right
	}
	return left + " " + right
}

// Hyphens is every character a line break hyphen is written with.
//
// One set for the whole toolchain, because a word broken across a page
// break is the same word broken at a line end and the same word broken
// between two lines of a bibliography entry. A reader who saw one healed
// and the others left would be right to wonder which spelling the corpus
// uses.
const Hyphens = "-\u2010\u2011\u00ad"

// Hyphenated says whether a piece of text ends in a line break hyphen, and
// so wants the text after it joined on without a space.
func Hyphenated(s string) bool {
	r := []rune(s)
	return len(r) > 0 && strings.ContainsRune(Hyphens, r[len(r)-1])
}

// closed says whether a paragraph ends where a sentence ends.
//
// A full stop after a single capital is an initial and not the end of
// anything, which is a real case at a page break: a bibliography entry
// broken after "C." is one entry and not two.
func closed(s string) bool {
	s = strings.TrimRight(s, `"'”’)]}`)
	if s == "" {
		return false
	}
	r := []rune(s)
	last := r[len(r)-1]
	if last == '.' && len(r) >= 2 && unicode.IsUpper(r[len(r)-2]) &&
		(len(r) == 2 || r[len(r)-3] == ' ') {
		return false
	}
	switch last {
	case '.', '!', '?', ':', ';':
		return true
	}
	return false
}
