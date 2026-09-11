package mathtex

import "strings"

// A TextRun is prose set inside the mathematics.
//
// TeX has no way of writing a word in a formula except by handing it to a macro
// that sets it upright, and papers write a great many words in formulae:
// operator names, table headings, the where clause of a definition, and the
// conditions on a case split.
//
//	$(\text{not } A) \text{ or } B$
//
// 526 spans of the Eléments held a run like it, all but nine of them opened by
// \text. That is prose by every test but the one that matters to a splitter: it
// stands between dollar signs, so everything that copies the mathematics through
// untouched copies those words through untouched as well.
//
// Which is why this is here rather than in the one rule that wanted it. The
// mathematics is compared byte for byte in three places, in the translator that
// refuses an answer, in the L rules that read the corpus back, and in the
// chunker that splits a section into questions, and all three have to agree on
// what part of a span is a word and what part is a formula.
type TextRun struct {
	// Macro is the one that opened the run, with its backslash on: \text or
	// \textit. Two runs that differ only in their macro are two different
	// pieces of typesetting and this says which.
	Macro string
	// Text is what stands between the braces, as it stands.
	Text string
	// Start and End are where Text sits in the span it was read from, counted
	// in runes, so that a caller can put a different word back in its place.
	Start, End int
}

// textMacros are the macros that set prose inside mathematics, longest first so
// that \textit is not read as \text with an it after it.
//
// The Eléments used two of them, \text 526 times and \textit 9 times, and the
// others are here because they set the same thing and a source that used one
// would otherwise have its words read as formulae without anything saying so.
var textMacros = []string{"textnormal", "textit", "textrm", "textbf", "textsf", "texttt", "text", "mbox"}

// TextRuns is every run of prose inside one math span, in the order they are
// written.
//
// The span is the text between the delimiters, which is what Split hands back,
// and not a whole body. Runs are read at any depth: a word inside a script
// inside a fraction is still a word.
func TextRuns(span string) []TextRun {
	rs := []rune(span)
	var out []TextRun
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\\' {
			continue
		}
		name := macroAt(rs[i+1:])
		if name == "" {
			i++ // the escape takes the character after it, whatever it is
			continue
		}
		open := i + 1 + len([]rune(name))
		if open >= len(rs) || rs[open] != '{' {
			continue
		}
		end := closingBrace(rs, open)
		if end < 0 {
			// A run whose brace never closes is a span that will not set at
			// all, which is M02's business and not this function's. Left
			// unread it is copied through with the rest of the formula.
			continue
		}
		out = append(out, TextRun{Macro: `\` + name,
			Text: string(rs[open+1 : end]), Start: open + 1, End: end})
		i = end
	}
	return out
}

// MaskText is the span with the words taken out of it: every run of prose
// replaced by one placeholder, and the macro and braces around it left standing.
//
// It is what compares two spans that are meant to be the same mathematics in
// two languages. The words inside a \text are the one part of a formula a
// translation is allowed to change, and they are also the part a model is most
// likely to change by accident, so the comparison has to see the run without
// seeing what is in it.
func MaskText(span string) string {
	runs := TextRuns(span)
	if len(runs) == 0 {
		return span
	}
	rs := []rune(span)
	var b strings.Builder
	at := 0
	for _, r := range runs {
		b.WriteString(string(rs[at:r.Start]))
		b.WriteString(placeholder)
		at = r.End
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// placeholder stands where the words were. It is a character no printing sets
// and no keyboard writes, so a span that carries one already cannot be told
// from a span that was masked, and none does.
const placeholder = "\x00"

// macroAt reads the name of a text macro at the start of rs, or "" if what
// stands there is not one of them.
func macroAt(rs []rune) string {
	for _, name := range textMacros {
		n := []rune(name)
		if len(rs) < len(n) {
			continue
		}
		if string(rs[:len(n)]) != name {
			continue
		}
		// \textrm must not be read as \text with an rm after it, and the list
		// is ordered longest first so it is not. What is left to refuse is a
		// longer name this list has never heard of, \textcolor say, whose
		// argument is not the prose.
		if len(rs) > len(n) && isTeXLetter(rs[len(n)]) {
			continue
		}
		return name
	}
	return ""
}

// closingBrace is where the brace opened at position at closes, or -1 if it
// never does. Braces nest and a backslash escapes the character after it.
func closingBrace(rs []rune, at int) int {
	depth := 0
	for i := at; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}
