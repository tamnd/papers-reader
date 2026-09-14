package code

import (
	"regexp"
	"strings"
)

// Langs is the tags a fence in this corpus may carry.
//
// A list and not a pattern, because the point of rule C02 is that the tag is
// one the renderer and the translator both recognise, and a pattern would
// accept `algo`, `lang-c` and `C++ (1998)` alike. Anything genuinely missing
// from it is one line to add, and the rule failing is how anybody finds out
// that it is missing.
//
// `text` is on it and is not a language. It is what the prompt asks for when
// a listing is in no language anybody names, and it is also what a table that
// a pipe table cannot carry is written in, so it is the most common tag in
// the corpus and the one that means "do not colour this, do not reflow it,
// and do not translate a word of it".
//
// It lives here rather than in the audit because two things need it: the
// rule that says a tag is not on the list, and the pass that puts a tag on a
// fence that came back without one. Two lists would drift and the drift would
// show up as a rule failing on a tag the toolchain itself had just written.
var Langs = map[string]bool{
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

// Sniff names the language a listing is written in, and names it `text` when
// it is not sure, which is most of the time.
//
// This is not language detection and it is not trying to be. It is a short
// list of signatures that only one language has, so that a fence a reader
// handed back without a tag gets the right one where the answer is not in
// doubt and gets `text` where it is. The cost of the two mistakes is not the
// same: `text` on a C++ listing loses the colouring, and `cpp` on a page of
// pseudocode colours the wrong words and tells the reading app the listing
// is something it is not.
//
// The MapReduce paper is a fair sample of what comes through here. Five
// bare fences: one is the C++ program in the appendix and has an #include
// and a class with a public base on it, and the other four are the map and
// reduce pseudocode, a pair of type signatures written with arrows, a
// counter fragment and a table of job statistics. One of the five has an
// answer and four of them are `text`, which is the ratio to design for.
func Sniff(text string) string {
	for _, s := range signatures {
		if s.pattern.MatchString(text) {
			return s.lang
		}
	}
	return "text"
}

// signatures is in order, and the order matters where two could match. C++
// before C, because every C++ program is full of C.
var signatures = []struct {
	lang    string
	pattern *regexp.Regexp
}{
	// A class with a base, a namespace qualifier, a template, or the
	// standard library's own headers. None of these is C.
	{"cpp", regexp.MustCompile(`(?m)^\s*(?:class|struct)\s+\w+\s*:\s*(?:public|private|protected)\s|(?m)^\s*template\s*<|\bstd::|#include\s*<(?:iostream|string|vector|map>)`)},
	// A preprocessor line, or a declaration with a brace, next to nothing
	// that says C++.
	{"c", regexp.MustCompile(`(?m)^\s*#include\s*[<"]|(?m)^\s*(?:static\s+)?(?:void|int|char|double|float|struct\s+\w+)\s+\*?\w+\s*\([^)]*\)\s*\{`)},
	{"python", regexp.MustCompile(`(?m)^\s*(?:def|class)\s+\w+\s*\(?[^)]*\)?\s*:\s*$|(?m)^\s*(?:from\s+\w+\s+)?import\s+\w`)},
	{"go", regexp.MustCompile(`(?m)^\s*func\s+(?:\(\s*\w+\s+\*?\w+\s*\)\s*)?\w+\s*\(|(?m)^\s*package\s+\w+\s*$`)},
	{"java", regexp.MustCompile(`(?m)^\s*(?:public|private|protected)\s+(?:static\s+)?(?:final\s+)?(?:class|interface|void|int|String)\b|(?m)^\s*import\s+java\.`)},
	{"sql", regexp.MustCompile(`(?i)\bselect\b[\s\S]{0,400}?\bfrom\b[\s\S]{0,400}?\b(?:where|group by|order by|join)\b`)},
	{"xml", regexp.MustCompile(`(?m)^\s*<\?xml\b|(?m)^\s*<!DOCTYPE\s`)},
}

// Label puts a tag on every closed fence of a body that came back without
// one, and leaves every other byte of the body alone.
//
// A reader that transcribes a listing without tagging the fence is the
// commonest C02 finding by a wide margin, and re-reading the page does not
// reliably fix it, because the tag is a thing the prompt asks for rather
// than a thing on the page. So it is repaired here instead of being asked
// for again.
//
// An unclosed fence is left as it is. C01 is the rule for that and it is a
// worse problem than a missing tag: writing a tag onto a fence that never
// closed would make a broken listing look tidy.
func Label(body string) string {
	blocks, _ := Blocks(body)
	lines := strings.Split(body, "\n")
	for _, b := range blocks {
		if b.Lang != "" || !b.Closed() {
			continue
		}
		at := b.Line - 1
		if at < 0 || at >= len(lines) {
			continue
		}
		lines[at] = strings.TrimRight(lines[at], " \t") + Sniff(b.Text)
	}
	return strings.Join(lines, "\n")
}
