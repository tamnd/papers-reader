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
	"github.com/tamnd/papers-reader/markdown"
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
	d.Paragraphs = fenced(uncaptioned(unprosed(d.Paragraphs)))
	return d
}

// fenced puts a fence round the listings that came off the page without one.
//
// The assembler already has to know which paragraphs are program text, or it
// joins them into the prose around them. Knowing that and then writing them
// out as prose anyway is half a job. It is the P4 paper that says so: its
// section 4 is eighteen listings and not one of them was fenced, so the
// declarations rendered as run-together prose with the indentation collapsed,
// audit rule C08 reported all eighteen, and the Vietnamese translation of the
// section was refused three times and given up on because the model quite
// reasonably fenced the listings itself and the source had no fence to match.
// Eighteen of the twenty C08 findings in the corpus are that one file.
//
// A run of program paragraphs goes into one fence rather than a fence each.
// They were one listing on the page and what is between them is either a
// blank line the paper set or a break the reader guessed at, and the braces
// say which. A break with a brace still open is inside a block, so it is the
// reader guessing at the foot of a column and the two halves go back together
// with one newline, the same repair Join makes at a page break and for the
// same reason. A break with the braces balanced is between one declaration
// and the next and the blank line is the paper's. Both are in the P4 paper:
// `table mTag_table {` and the `reads {` under it are one table, and `header
// ethernet` and `header vlan` are two declarations with a line between them.
//
// Anything that is not program text ends the run, so two listings with a
// sentence between them stay two listings.
//
// The fence carries no language, because naming one is not this package's
// job. papers split runs code.Label over every paragraph it writes, which
// puts a tag on a bare fence where the answer is not in doubt and `text`
// where it is, and that is the same treatment a fence off the layout path
// gets. The P4 listings come out `text`, which is right: nothing in the
// signature list is P4 and colouring it as something else would be a lie
// about what a reader is looking at.
//
// Nothing here fences a paragraph that arrived fenced. The layout path writes
// its own fences and this is for the native path, which writes prose and
// nothing else.
// unprosed cuts the prose off the end of a listing that has some stuck to it.
//
// A paragraph is a run of lines between blank ones, and the page decides
// where the blank ones are. Tarjan's page 12 ends the strongly connected
// components algorithm with END; and sets THEOREM 13 on the next line, at the
// same indentation and with no line between, because the theorem and the
// algorithm it is about are one indented block on the page. So the theorem,
// its statement and the whole of its proof arrive as part of the listing
// paragraph, and every one of them goes inside the fence: two thirds of a
// page of English set in monospace with no line wrapping.
//
// Emphasis is what tells them apart. The page sets a theorem statement in
// italics and the reader writes that as *...*, and a program does not carry
// emphasis: an asterisk in a listing is multiplication or a pointer and it
// does not come in a matched pair around a phrase of English. So a trailing
// run of lines that carry emphasis is where the listing stopped and the prose
// started, and it becomes a paragraph of its own.
//
// Only the tail is cut, and only when there is a listing left in front of it.
// Emphasis in the middle of a listing is a reader marking up a comment, which
// is a different mistake with a different repair, and a paragraph that is
// emphasis all the way down was never a listing to begin with.
func unprosed(ps []Paragraph) []Paragraph {
	out := make([]Paragraph, 0, len(ps))
	for _, p := range ps {
		lines := strings.Split(p.Text, "\n")
		cut := len(lines)
		for cut > 0 && emphasised(lines[cut-1]) {
			cut--
		}
		if cut == 0 || cut == len(lines) || !listing(strings.Join(lines[:cut], "\n")) {
			out = append(out, p)
			continue
		}
		out = append(out,
			Paragraph{Text: strings.Join(lines[:cut], "\n"), Page: p.Page, Pages: p.Pages},
			Paragraph{Text: strings.TrimSpace(strings.Join(lines[cut:], "\n")), Page: p.Page, Pages: p.Pages},
		)
	}
	return out
}

// uncaptioned cuts the caption off the front of a listing that has one stuck
// to it.
//
// This is the other end of the same fault unprosed repairs, and the Aho and
// Corasick paper is where it shows. Its four algorithms are each printed as
// one block with the caption at the top of it, "Algorithm 4. Construction of
// a deterministic finite automaton.", and no blank line under the caption,
// so the caption and the program arrive as one paragraph. Fencing that
// paragraph as it came would put the caption inside the fence, and a caption
// inside a fence is a line of text: papers tags never writes an attribute
// block over it, nothing in the corpus can link to Algorithm 4, and audit
// rule C06 reports a listing with no anchor. Cutting it leaves the caption
// where the tagger will find it and fences the program under it.
//
// Only the caption line comes off, not the Input, Output and Method lines
// the older journals set under it. Those are part of the algorithm as the
// page displayed it, and they are inside the block the paper drew. The
// caption is different because it is the one line with a job outside the
// listing to do.
func uncaptioned(ps []Paragraph) []Paragraph {
	out := make([]Paragraph, 0, len(ps))
	for _, p := range ps {
		lines := strings.Split(p.Text, "\n")
		if len(lines) < 2 || !code.Caption(lines[0]) || !listing(strings.Join(lines[1:], "\n")) {
			out = append(out, p)
			continue
		}
		out = append(out,
			Paragraph{Text: strings.TrimSpace(lines[0]), Page: p.Page, Pages: 1},
			Paragraph{Text: strings.Join(lines[1:], "\n"), Page: p.Page, Pages: p.Pages},
		)
	}
	return out
}

// emphasis is a phrase between a matched pair of asterisks or underscores,
// which is how the reader writes the italics a page sets a theorem statement
// in. The phrase has to hold a letter, so that a line of arithmetic with two
// multiplications on it is not read as a phrase in italics.
var emphasis = regexp.MustCompile(`(\*|_)[^*_\n]*\p{L}[^*_\n]*(\*|_)`)

func emphasised(line string) bool { return emphasis.MatchString(line) }

func fenced(ps []Paragraph) []Paragraph {
	out := make([]Paragraph, 0, len(ps))
	for i := 0; i < len(ps); {
		if !listing(ps[i].Text) {
			out = append(out, ps[i])
			i++
			continue
		}
		j := i
		for j < len(ps) && listing(ps[j].Text) {
			j++
		}
		body := ps[i].Text
		for _, p := range ps[i+1 : j] {
			if open(body) > 0 {
				body += "\n" + p.Text
				continue
			}
			body += "\n\n" + p.Text
		}
		last := ps[j-1]
		out = append(out, Paragraph{
			Text:  mark(body) + "\n" + body + "\n" + mark(body),
			Page:  ps[i].Page,
			Pages: last.Page + last.Pages - ps[i].Page,
		})
		i = j
	}
	return out
}

// listing says whether a paragraph is program text that is not already in a
// fence.
//
// Fencing is a stronger claim than refusing to join, so it wants stronger
// evidence, and program on its own is satisfied by a run of lines that end
// in a semicolon. The clauses of a definition are written that way. Rabin
// and Scott number theirs (i) to (iv), end three of the four with a
// semicolon and never write a keyword or an assignment, and both of the
// paragraphs that shape went into the corpus inside a ```text fence, which
// is a page of mathematics rendered as a wall of monospace. So a paragraph
// wants one of the marks that has no reading in English at all before it is
// fenced, which is what code.Statement is for.
//
// A pipe table is not program text either, however much a run of rules
// looks like one line by line. Table 1 of Hoare's paper is fourteen rows of
// a formal proof, every row has an assignment in it, and the whole of it was
// fenced on the strength of those assignments.
func listing(s string) bool {
	if opens(strings.TrimSpace(s)) != "" || markdown.IsTable(s) {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		if code.Statement(line) {
			return program(s)
		}
	}
	return false
}

// open is how many braces the text has left unclosed, which says whether a
// break in it falls inside a block. Braces and not brackets or parentheses,
// because a brace is what a block is written with in every language a paper
// in this corpus prints, and a stray parenthesis in a comment is far commoner
// than a stray brace.
func open(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case '{':
			n++
		case '}':
			n--
		}
	}
	return n
}

// mark is a backtick fence long enough to hold the text, which is three
// unless the listing has a run of three or more backticks of its own in it.
func mark(s string) string {
	most, n := 0, 0
	for _, r := range s {
		if r == '`' {
			n++
			if n > most {
				most = n
			}
			continue
		}
		n = 0
	}
	if most < 3 {
		return "```"
	}
	return strings.Repeat("`", most+1)
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
