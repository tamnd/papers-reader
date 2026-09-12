package audit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/mathtex"
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

// codeLangs is the tags a fence in this corpus may carry.
//
// A list and not a pattern, because the point of the rule is that the tag is
// one the renderer and the translator both recognise, and a pattern would
// accept `algo`, `lang-c` and `C++ (1998)` alike. Anything genuinely missing
// from it is one line to add, and the rule failing is how anybody finds out
// that it is missing.
//
// `text` is on it and is not a language. It is what the prompt asks for when a
// listing is in no language anybody names, and it is also what a table that a
// pipe table cannot carry is written in, so it is the most common tag in the
// corpus and the one that means "do not colour this, do not reflow it, and do
// not translate a word of it".
var codeLangs = map[string]bool{
	"abnf": true, "algol": true, "apl": true, "asm": true, "awk": true,
	"basic": true, "bash": true, "bnf": true, "c": true, "clu": true,
	"cobol": true, "cpp": true, "csharp": true, "css": true, "diff": true,
	"ebnf": true, "erlang": true, "forth": true, "fortran": true, "go": true,
	"haskell": true, "html": true, "java": true, "javascript": true,
	"json": true, "lisp": true, "lua": true, "makefile": true, "matlab": true,
	"ml": true, "modula": true, "ocaml": true, "pascal": true, "perl": true,
	"pl1": true, "postscript": true, "prolog": true, "python": true,
	"r": true, "ruby": true, "rust": true, "scala": true, "scheme": true,
	"sh": true, "simula": true, "smalltalk": true, "snobol": true,
	"sql": true, "swift": true, "tcl": true, "tex": true, "text": true,
	"verilog": true, "vhdl": true, "xml": true, "yaml": true,
}

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
			case !codeLangs[b.Lang]:
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
func ruleC04(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C04", func(f *File) []Finding {
		var out []Finding
		spans, unclosed := mathtex.Split(f.Body)
		if unclosed != nil {
			spans = append(spans, *unclosed)
		}
		for _, s := range spans {
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

// statement matches a line that English prose does not write and a program
// does. Every one of these was picked for being unambiguous rather than for
// being common: a rule that guessed from indentation would report every
// quotation in the corpus.
var statement = regexp.MustCompile(`^\s*(?:` +
	// A brace on a line of its own, which is C, Java, Go and every
	// descendant of them, and is not a sentence.
	`[{}]\s*;?` +
	// A preprocessor directive. The hash is not a heading because a heading
	// has a space after it.
	`|#(?:include|define|ifdef|ifndef|endif|pragma)\b` +
	// A declaration or a keyword at the head of a line, in the languages the
	// papers in this corpus print. BEGIN and END are ALGOL, which is most of
	// what the older papers print and reads as ordinary prose in lower case,
	// so they are matched in capitals only.
	`|(?:BEGIN|END)\b` +
	`|(?:int|double|float|char|void|struct|union|typedef|static|const|unsigned|long|short|bool)\s+\w+\s*[;(=]` +
	`|(?:if|while|for|switch)\s*\(` +
	`|(?:def|func|function|procedure|class)\s+\w+\s*\(` +
	`)`)

// semicolon matches a line that ends in a semicolon, which in prose happens
// where a sentence is joined to the next and in a program happens on almost
// every line.
var semicolon = regexp.MustCompile(`[^\s;];\s*$`)

// minLooseRun is how many lines in a row have to look like a program before
// this rule says anything, and minLooseMarks is how many of them have to carry
// a mark.
//
// Three and two. One line is a citation with a brace in it, two is a
// coincidence, and three in a row of which two are statements is a listing
// that lost its fence. The pair of thresholds is the same shape as acceptance
// rule A5 and for the same reason: either one alone reports things that are
// fine.
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
		for i := 0; i < len(lines); i++ {
			if skip[i+1] || strings.TrimSpace(lines[i]) == "" {
				continue
			}
			end, marks := i, 0
			for end < len(lines) && !skip[end+1] && strings.TrimSpace(lines[end]) != "" {
				if statement.MatchString(lines[end]) || semicolon.MatchString(lines[end]) {
					marks++
				}
				end++
			}
			if end-i >= minLooseRun && marks >= minLooseMarks {
				out = append(out, Finding{
					Rule: "C08", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("%d lines here read as program text and are not in a fence", end-i),
				})
			}
			i = end
		}
		return out
	})
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
// Soft, because a run of spaces is a strong hint and not a proof, and because
// a stray double space somebody typed is not worth failing a build over.
func ruleC09(in *Input) ([]Finding, error) {
	return eachCodeFile(in, "C09", func(f *File) []Finding {
		var out []Finding
		lines := strings.Split(f.Body, "\n")
		skip := protectedLines(f.Body, len(lines))
		for i := 0; i < len(lines); i++ {
			if skip[i+1] || strings.Contains(lines[i], "|") || !aligned.MatchString(lines[i]) {
				continue
			}
			end := i
			for end < len(lines) && !skip[end+1] && !strings.Contains(lines[end], "|") && aligned.MatchString(lines[end]) {
				end++
			}
			out = append(out, Finding{
				Rule: "C09", File: f.Path, Line: i + 1,
				Message: fmt.Sprintf("%d lines here are lined up with spaces that Markdown will collapse, so the columns are lost", end-i),
			})
			i = end
		}
		return out
	})
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

// codeRules is the C group.
//
// C06 and C07 are not here. C06 wants a numbered listing to carry an
// attribute block with a `.code` class, and the corpus has no numbered
// listings yet because nothing writes the attribute blocks. C07 compares the
// fenced regions of a translation with those of its source byte for byte, and
// there are no translations. Both arrive with the milestone that produces the
// files they read, which is how every other group in this audit has grown.
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
			ID:    "C08",
			What:  "no run of lines reads as program text outside a fence.",
			Check: ruleC08,
		},
		{
			ID:    "C09",
			What:  "no run of lines is lined up with spaces Markdown will collapse.",
			Check: ruleC09,
		},
	}
}
