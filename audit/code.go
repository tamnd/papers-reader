package audit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/mathtex"
	"github.com/tamnd/papers-reader/tags"
)

// The C group is about program text, which this corpus carries more of than a
// mathematics corpus does: a paper on a language prints its grammar, a paper
// on an algorithm prints the algorithm, and a paper from 1968 prints ALGOL
// with the indentation carrying the block structure.
//
// A listing is the one kind of content in the corpus that is not prose and not
// mathematics, and it is the one kind that has to survive byte for byte. Every
// other stage is allowed to normalise: the mathematics is rewritten into one
// dialect of TeX, the number sets into one font, the paragraphs unwrapped. A
// listing is not, because a space in a listing is part of the program and a
// published listing with a bug in it is a published listing with a bug in it.

// eachCodeFile runs a check over every content file that parsed.
//
// The group stands down on a corpus with nothing committed rather than on a
// corpus with no fences in it. A corpus of papers with no listing in any of
// them is possible and is a pass, where a corpus with no content is a corpus
// that has not been extracted and is not.
func eachCodeFile(in *Input, rule string, check func(*File) []Finding) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		out = append(out, check(f)...)
	}
	return out, nil
}

// ruleC01 is the cheapest rule in the group and it fails the loudest. An
// unclosed fence swallows the rest of the file: everything after it renders
// as program text in a monospaced box, the translator is told not to touch a
// word of it, and the reader of the corpus gets half a paper in grey.
func ruleC01(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C01", func(f *File) []Finding {
		_, unclosed := code.Blocks(f.Body)
		if unclosed == nil {
			return nil
		}
		return []Finding{{
			Rule: "C01", File: f.Path, Line: unclosed.Line,
			Message: fmt.Sprintf("a fence of %s was opened here and never closed", unclosed.Mark),
		}}
	})
}

// ruleC02 wants a tag on every fence, from a list.
//
// The tag is not decoration. The translator reads it to know that the block is
// protected, the reading app reads it to colour the listing, and a person
// reading the corpus reads it to know what language they are looking at. A
// bare fence is also what a model writes when it is wrapping an answer rather
// than transcribing a listing, so an untagged fence is as often a packaging
// mistake as a missing tag.
func ruleC02(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C02", func(f *File) []Finding {
		var out []Finding
		blocks, _ := code.Blocks(f.Body)
		for _, b := range blocks {
			switch {
			case b.Lang == "":
				out = append(out, Finding{
					Rule: "C02", File: f.Path, Line: b.Line,
					Message: "the fence carries no language tag, and every fence carries one, `text` where the listing is in no language anybody names",
				})
			case !code.Langs[b.Lang]:
				out = append(out, Finding{
					Rule: "C02", File: f.Path, Line: b.Line,
					Message: fmt.Sprintf("the fence is tagged %s, which is not a tag this corpus uses", b.Lang),
				})
			}
		}
		return out
	})
}

// ruleC03 is about a fence inside a fence.
//
// Two fences of the same character cannot nest in Markdown: the second one
// closes the first. So what this rule can see is a longer run of the same
// character inside a block, or a run of the other character, and both mean the
// same thing, which is that somebody wrapped an answer that already had a
// listing in it. The symptom in the file is a listing whose first line is
// ```` ```c ```` and a paragraph of prose in the middle of the program.
//
// The exception is a listing that is deliberately showing Markdown, which the
// corpus writes as a `text` or `markdown` fence, and which a paper about
// document formats has every right to print. Those are left alone, and the way
// to write one is to say so in the tag.
func ruleC03(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C03", func(f *File) []Finding {
		var out []Finding
		blocks, _ := code.Blocks(f.Body)
		for _, b := range blocks {
			if b.Lang == "text" || b.Lang == "markdown" || b.Lang == "md" {
				continue
			}
			for i, line := range strings.Split(b.Text, "\n") {
				if !opensFence(line) {
					continue
				}
				out = append(out, Finding{
					Rule: "C03", File: f.Path, Line: b.Line + 1 + i,
					Message: fmt.Sprintf("a fence opens inside the %s fence on line %d, so the two did not nest", b.Mark, b.Line),
				})
				break
			}
		}
		return out
	})
}

// fenceLine is a line that would open or close a fence: up to three spaces and
// then three or more backticks or tildes.
var fenceLine = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")

func opensFence(line string) bool { return fenceLine.MatchString(line) }

// ruleC04 is about a fence that opens inside a math span.
//
// It is the code half of the same accident rule M01 catches from the other
// side. An unclosed dollar swallows the fence, or a fence swallows the
// dollars, and which of the two happened depends on which delimiter came
// first. Both rules fire on the same file and they are kept apart because the
// repair is different: M01 is a missing dollar and this is a fence in the
// wrong place.
//
// A span that opens inside a fence is not this rule's finding. It is M13's,
// and the two say different things about the same file: this one says the
// mathematics of the page ran into a listing, and M13 says a dollar in a
// listing was read as mathematics when the listing meant the character. The
// FORTRAN in McCabe's paper is the second kind twice over, in `FORMAT(...$)`
// and in `CALL READB(...,$990,$990)`, and reporting it here as well sent
// whoever read the audit to the prose, where there is nothing wrong.
func ruleC04(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C04", func(f *File) []Finding {
		var out []Finding
		fenced := codeRanges(f.Body)
		spans, unclosed := mathtex.Split(f.Body)
		if unclosed != nil {
			spans = append(spans, *unclosed)
		}
		for _, s := range spans {
			if inRanges(fenced, s.Line) {
				continue
			}
			if !fenceLine.MatchString(strings.TrimLeft(s.Text, "\n")) && !strings.Contains(s.Text, "\n```") && !strings.Contains(s.Text, "\n~~~") {
				continue
			}
			out = append(out, Finding{
				Rule: "C04", File: f.Path, Line: s.Line,
				Message: "a fence opens inside the mathematics that starts here",
			})
		}
		return out
	})
}

// maxCodeLines is how long a listing may be.
//
// A hundred and twenty lines is two and a half printed pages of program, which
// no paper in this corpus prints and which is what a fence that swallowed the
// prose looks like from the outside. The rule is soft because a paper that
// prints its whole implementation as an appendix is a real thing and the
// answer to it is a person looking, not a build that fails.
const maxCodeLines = 120

func ruleC05(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C05", func(f *File) []Finding {
		var out []Finding
		blocks, _ := code.Blocks(f.Body)
		for _, b := range blocks {
			if n := b.Lines(); n > maxCodeLines {
				out = append(out, Finding{
					Rule: "C05", File: f.Path, Line: b.Line,
					Message: fmt.Sprintf("the listing runs to %d lines, and a fence that swallowed the prose looks exactly like this", n),
				})
			}
		}
		return out
	})
}

// minLooseRun is how many lines in a row have to look like a program before
// this rule says anything, and minLooseMarks is how many of them have to carry
// a mark.
//
// Three and two. One line is a citation with a brace in it, two is a
// coincidence, and three in a row of which two are statements is a listing
// that lost its fence. The pair of thresholds is the same shape as acceptance
// rule A5 and for the same reason: either one alone reports things that are
// fine.
//
// What counts as a mark is code.Mark, because the assembler reads the same
// marks when it decides whether a paragraph is prose, and two ideas about
// what a line of a program looks like would be two answers about one page.
//
// One of the marks has to be a statement, though, and not a semicolon at the
// end of a line. A semicolon there is how a program is written and also how
// verse is punctuated, and the GPT-3 paper prints nine lines of a generated
// poem with one at the end of two of them: over the whole corpus that was
// the only thing this rule found that was not a listing. A brace, a
// directive, a comment or a declaration has no reading in English, so
// asking for one of those is asking for the evidence that cannot be
// punctuation. The bitcoin C program has all four.
const (
	minLooseRun   = 3
	minLooseMarks = 2
)

// ruleC08 is not in the original list and it is the rule this group was
// missing.
//
// C01 to C07 are all about a fence that exists. None of them has anything to
// say about a listing that never got one, and that is the failure that
// actually happened: the reader transcribed the C program on page 7 of the
// bitcoin paper correctly, line by line and space by space, and wrote it as
// prose. Every fence rule passed because there was no fence. What the reader
// of the corpus gets is a paragraph in a proportional font with `#include
// <math.h>` in it, and the `<math.h>` is eaten by the renderer as a tag.
//
// The masthead of a front page is skipped, which is the same exception rule
// C09 makes and for the same reason. The TraceMonkey paper has sixteen
// authors at four institutions and prints each institution's addresses as a
// brace list, {gal,brendan,shaver}@mozilla.com, which opens on a brace and
// so reads as a block of C. Four of those with an affiliation line between
// each pair is an eight line listing as far as this rule can tell.
//
// The prompt asks for the fence in as many words and the reader ignored it,
// which is the same thing that happens with the math delimiters, so this is
// not something a better prompt fixes. It is soft because the marks below are
// evidence and not proof, and because the repair is a person putting a fence
// where it belongs rather than a program guessing that a paragraph is a
// program.
func ruleC08(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C08", func(f *File) []Finding {
		var out []Finding
		lines := strings.Split(f.Body, "\n")
		skip := protectedLines(f.Body, len(lines))
		for i := mastheadLines(f); i < len(lines); i++ {
			if skip[i+1] || strings.TrimSpace(lines[i]) == "" {
				continue
			}
			end, run, marks, statements := i, 0, 0, 0
			for end < len(lines) && !skip[end+1] {
				if strings.TrimSpace(lines[end]) == "" {
					if !continues(lines, skip, end, marks) {
						break
					}
					end++
					continue
				}
				if code.Mark(lines[end]) {
					marks++
				}
				if code.Statement(lines[end]) {
					statements++
				}
				run++
				end++
			}
			if run >= minLooseRun && marks >= minLooseMarks && statements > 0 {
				out = append(out, Finding{
					Rule: "C08", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("%d lines here read as program text and are not in a fence", run),
				})
			}
			i = end
		}
		return out
	})
}

// continues reports whether the blank line at at is inside a listing rather than
// at the end of one, which it is when a marked line stands on each side of it.
//
// A reader that is given a program and asked for Markdown sometimes writes
// every line of it as its own paragraph. Floyd's Algorithm 97 came back that
// way, ten lines of ALGOL 60 with a blank line between each pair, and read as
// runs of one line it was ten runs of one line and no listing at all. It went
// out unfenced, and the translator then left the program lines in English
// because they are not prose, which is how a rule about fences turned into
// three findings from the rule about untranslated paragraphs.
//
// One blank line and no more, and the line above it marked. Two blank lines
// is a gap between two things and stops the run either way.
//
// The line below is allowed to carry the mark instead, but only once the run
// has one of its own. That second case is for the unmarked lines a listing
// has inside it: an ALGOL block opens on a bare begin and closes on a bare
// end, and wanting the line above every blank to be marked cut Floyd's ten
// lines into a run of one, a run of one and a run of three, which pointed
// the reader at the middle of the listing. Letting the line below carry it
// from the start would be worse, because the sentence introducing a listing
// sits one blank line above the listing and would be read as the first line
// of it.
func continues(lines []string, skip []bool, at, marks int) bool {
	next := at + 1
	if at == 0 || next >= len(lines) || skip[next+1] || strings.TrimSpace(lines[next]) == "" {
		return false
	}
	if code.Mark(lines[at-1]) {
		return true
	}
	return marks > 0 && code.Mark(lines[next])
}

// aligned matches a run of three or more spaces between two things that are
// not spaces, which is a column of a printed table and is not a sentence.
var aligned = regexp.MustCompile(`\S {3,}\S`)

// ruleC09 is the other half of C08 and catches the thing that comes with it.
//
// A paper that prints a program usually prints the numbers it produced, laid
// out in columns and lined up with spaces. Markdown collapses a run of spaces
// to one, so the columns the page printed come out as a single ragged
// paragraph and the reader of the corpus cannot tell which number belongs to
// which heading. The prompt asks for a pipe table where the cells form a grid
// and a `text` fence where they do not, and neither of those loses the
// alignment.
//
// A line containing a pipe is left alone, because a pipe table is padded with
// exactly this kind of white space and is the right answer rather than the
// wrong one. So is a line inside a fence or inside mathematics, where the
// spaces are already safe.
//
// One line on its own is not a table, because a column needs rows. That is
// the whole of what this rule used to get wrong: seven of its eight findings
// over the corpus were a single line, and every one of them was prose with a
// wide gap in it rather than data in columns. Three are a GPT-3 figure
// caption where the typesetter set the number in bold and the reader kept
// the gap after it, two are a run-in heading with its paragraph on the same
// line, and collapsing the gap in any of them costs a reader nothing.
//
// The masthead of a front page is skipped too, which is the same exception
// rule L07 makes and for the same reason. A byline is names in three columns
// and an affiliation line is institutions in six, set that way because the
// page is two columns wide and not because the paper is presenting a table.
// The Borg byline and the P4 affiliations were the last two findings and are
// both that.
//
// Soft, because a run of spaces is a strong hint and not a proof, and because
// a stray double space somebody typed is not worth failing a build over.
func ruleC09(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C09", func(f *File) []Finding {
		var out []Finding
		lines := strings.Split(f.Body, "\n")
		skip := protectedLines(f.Body, len(lines))
		from := mastheadLines(f)
		for i := from; i < len(lines); i++ {
			if skip[i+1] || strings.Contains(lines[i], "|") || !aligned.MatchString(lines[i]) {
				continue
			}
			end := i
			for end < len(lines) && !skip[end+1] && !strings.Contains(lines[end], "|") && aligned.MatchString(lines[end]) {
				end++
			}
			if end-i > 1 {
				out = append(out, Finding{
					Rule: "C09", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("%d lines here are lined up with spaces that Markdown will collapse, so the columns are lost", end-i),
				})
			}
			i = end
		}
		return out
	})
}

// mastheadLines is how many lines of a file the masthead takes up, or none
// for a file that is not a front page. See masthead, which does the same
// thing in blocks for the rules that work in blocks.
func mastheadLines(f *File) int {
	if f.Front.Kind != "front" {
		return 0
	}
	blocks := blocksOf(f.Body)
	at := masthead(blocks)
	if at == 0 || at > len(blocks) {
		return 0
	}
	// The end of the last block of the masthead, found in the body rather
	// than counted from the blocks, because Blocks drops the blank lines
	// between them and this needs the line number the file has.
	last := blocks[at-1]
	cut := strings.Index(f.Body, last)
	if cut < 0 {
		return 0
	}
	return strings.Count(f.Body[:cut]+last, "\n") + 1
}

// protectedLines is the lines of a body that C08 and C09 do not look at:
// everything inside a fence, and everything inside a math span.
//
// Indexed from one so that a caller holding a line number can use it without
// arithmetic. The math side is done by counting newlines up to the span rather
// than by asking mathtex for an end line, because a span carries the line it
// opened on and the offsets of its text, which is enough.
func protectedLines(body string, n int) []bool {
	out := code.Inside(body)
	for len(out) < n+1 {
		out = append(out, false)
	}
	spans, unclosed := mathtex.Split(body)
	if unclosed != nil {
		spans = append(spans, *unclosed)
	}
	for _, s := range spans {
		end := s.Line + strings.Count(s.Text, "\n")
		for i := s.Line; i <= end+1 && i < len(out); i++ {
			out[i] = true
		}
	}
	return out
}

// ruleC06 wants a numbered listing to carry its attribute block.
//
// A paper that numbers a listing refers to it: "the loop of Algorithm 2".
// The reading app turns that into a link and the book into a page reference,
// and both of them need the anchor, which is what the attribute block
// carries. A figure and a table get one because papers tags writes one over
// every caption it recognises, and Algorithm and Listing are in its table of
// captions for exactly this reason.
//
// The caption is what is checked rather than the fence, because the caption
// is where the number is and where papers tags puts the block. A caption
// whose block has some other class is a finding too: the class is what the
// reader filters on and a listing filed under .figure is a listing the
// figure gallery will try to draw.
//
// Hard. A cross reference to an anchor that is not there is a dead link in
// every format the corpus is published in, and this is the one rule that
// sees it before the link is made.
func ruleC06(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C06", func(f *File) []Finding {
		var out []Finding
		lines := strings.Split(f.Body, "\n")
		inside := code.Inside(f.Body)
		for i, line := range lines {
			if i+1 < len(inside) && inside[i+1] {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if !code.Caption(trimmed) {
				continue
			}
			attrs := tags.ParseAttrs(line)
			if len(attrs) == 0 {
				out = append(out, Finding{
					Rule: "C06", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("the listing captioned %q carries no attribute block, so nothing can refer to it", shorten(trimmed)),
				})
				continue
			}
			if !classed(attrs[0], "code") {
				out = append(out, Finding{
					Rule: "C06", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("the listing captioned %q is filed under .%s and not .code", shorten(trimmed), strings.Join(attrs[0].Classes, " .")),
				})
			}
		}
		return out
	})
}

// classed says whether an attribute block carries a class.
func classed(a tags.Attr, want string) bool {
	for _, c := range a.Classes {
		if c == want {
			return true
		}
	}
	return false
}

// ruleC07 compares the fenced regions of a translation with those of its
// English, byte for byte and including the spaces.
//
// This is what makes code a protected kind, and it looks like a second copy
// of L18. It is not, and the difference is the point. L18 asks the
// translator's own Protect for the spans on both sides, so a bug in Protect
// is invisible to it: the same wrong answer is compared with itself. This
// reads the fences with the code package, which is an independent parser
// written for the other rules of this group, and it compares the whole
// region rather than the span, so an opening line whose language tag changed
// is caught here and not there.
//
// Byte for byte means the trailing spaces too. A published listing is a
// published listing, a translator that tidied the right hand edge of an
// ALGOL program has changed the program, and nothing downstream will ever
// tell you, because trailing space is the one difference a diff viewer
// hides.
func ruleC07(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		want, _ := code.Blocks(p.en.Body)
		got, _ := code.Blocks(p.tr.Body)
		if len(want) != len(got) {
			return []Finding{{
				Rule: "C07", File: p.tr.Path,
				Message: fmt.Sprintf("the English has %d listings and this has %d", len(want), len(got)),
			}}
		}
		var out []Finding
		for i := range want {
			switch {
			case want[i].Lang != got[i].Lang:
				out = append(out, Finding{
					Rule: "C07", File: p.tr.Path, Line: got[i].Line,
					Message: fmt.Sprintf("listing %d is tagged %q and the English tags it %q", i+1, got[i].Lang, want[i].Lang),
				})
			case want[i].Text != got[i].Text:
				out = append(out, Finding{
					Rule: "C07", File: p.tr.Path, Line: got[i].Line,
					Message: fmt.Sprintf("listing %d is not the English listing byte for byte", i+1),
				})
			}
		}
		return out
	})
}

// ruleC10 is about a run of backticks with prose after it on the same line.
//
// CommonMark says a closing fence carries nothing after the backticks, so a
// line like "``` token, e.g.," does not close anything: it is a line of the
// listing, and the fence it should have closed runs on until the next bare
// run of backticks, swallowing whatever prose is between them.
//
// It is a reader transcribing an inline code span as a display block. The
// BERT appendix has a sentence about replacing a word with the [MASK] token,
// and it came back as a fenced block holding [MASK], the words "token, e.g.,"
// on the closing line, and another fenced block. Nothing else in the audit
// saw it: the block is tagged text, which is the one tag C03 exempts, and
// the tag on the opening line is a real tag, so C02 is happy too.
//
// It is worth a rule of its own because of what it does further down. The
// translator holds a fenced block out of the question and compares it with
// the answer byte for byte, so a block with a sentence inside it is a
// sentence that may not be translated, and three attempts at the file were
// refused for translating it before anybody looked at the English.
//
// A run with a single word after it is a fence with a language tag and is
// C02's business, not this rule's. What this rule wants is the punctuation
// and the spaces of a sentence.
func ruleC10(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C10", func(f *File) []Finding {
		var out []Finding
		for i, line := range strings.Split(f.Body, "\n") {
			if !prosePastFence(line) {
				continue
			}
			out = append(out, Finding{
				Rule: "C10", File: f.Path, Line: i + 1,
				Message: "a run of backticks here is followed by prose rather than a language tag, so it closes nothing and the fence runs on",
			})
		}
		return out
	})
}

// pastFence is a run of backticks or tildes and whatever follows it on the
// line.
var pastFence = regexp.MustCompile("^ {0,3}(?:`{3,}|~{3,})(.*)$")

// prosePastFence reports whether a line is a fence run with prose after it
// rather than a language tag.
//
// The test is on the first word alone, because what comes after a real tag
// is a listing's attribute block and that is rule C06's business. A word
// with a comma or a full stop in it is not a tag anybody writes, and a word
// that could be one is left to C02, which knows the list of tags and will
// say so if it is not on it.
func prosePastFence(line string) bool {
	m := pastFence.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	rest := strings.TrimSpace(m[1])
	if rest == "" {
		return false
	}
	return !tagWord.MatchString(strings.Fields(rest)[0])
}

// tagWord is what a language tag may be spelled with: a letter, and then
// the letters, digits and the handful of marks that the names of languages
// have in them, c++ and c# and objective-c among them.
var tagWord = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+#._-]*$`)

// codeRules is the C group.
func codeRules() []Rule {
	return []Rule{
		{
			ID: "C01", Hard: true,
			What:  "every fence is closed.",
			Check: ruleC01,
		},
		{
			ID: "C02", Hard: true,
			What:  "every fence carries a language tag from the known list.",
			Check: ruleC02,
		},
		{
			ID: "C03", Hard: true,
			What:  "no fence is nested inside another.",
			Check: ruleC03,
		},
		{
			ID: "C04", Hard: true,
			What:  "no fence opens inside a math span.",
			Check: ruleC04,
		},
		{
			ID:    "C05",
			What:  "no listing runs past 120 lines.",
			Check: ruleC05,
		},
		{
			ID: "C06", Hard: true,
			What:  "a numbered listing carries an attribute block with a .code class.",
			Check: ruleC06,
		},
		{
			ID: "C07", Hard: true,
			What:  "the fenced regions of a translation are its English ones, byte for byte.",
			Check: ruleC07,
		},
		{
			ID:    "C08",
			What:  "no run of lines reads as program text outside a fence.",
			Check: ruleC08,
		},
		{
			ID:    "C09",
			What:  "no run of lines is lined up with spaces Markdown will collapse.",
			Check: ruleC09,
		},
		{
			ID: "C10", Hard: true,
			What:  "no run of backticks has a sentence after it on the same line.",
			Check: ruleC10,
		},
	}
}
