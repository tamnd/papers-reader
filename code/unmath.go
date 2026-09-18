package code

import (
	"regexp"
	"strings"
)

// Unmath takes the mathematics back out of a listing.
//
// A reader that has just read three pages of a paper full of formulas keeps
// reading the next thing that way, and the next thing is a program. Floyd's
// ALGOL is the clearest case in the corpus. The comment clause of Algorithm
// 97 is a paragraph of English about the parameters, the page sets the
// parameter names in italics the way ALGOL 60 listings did, and the reader
// wrote every one of them as `$b$`, `$c$`, `$op$`. Tarjan's algorithms are
// the same and so is Needham's pseudocode, which came back as
// `if $(X := \text{decrypt}(Y, Key1)) = \text{nonsense}$`.
//
// Program text is transcribed verbatim, so this is wrong twice over. The
// page printed `b` and the file says `$b$`, and everything downstream that
// counts delimiters reads the listing as mathematics. Rule M13 reported one
// hundred and thirty three spans across four papers, and the reading app
// would set a variable name in a serif italic in the middle of a monospaced
// listing.
//
// Only a span that is plain text once the markup is off comes out. A span
// with a backslash, a script or a brace left in it after the tables below
// have run is real mathematics, which has no business in a listing either,
// but it is not this function's to flatten. A sum written as the word sum is
// a lie about the page where a variable name written as a variable name is
// not, so those are left for M13 to go on reporting, which is what a rule is
// for.
//
// The fences this looks at are the ones marked text and the ones with no tag
// on them. Every named language is left alone, because a dollar is a sigil
// in a good few of them and taking it out would delete a character the page
// printed. McCabe's FORTRAN is the case, `FORMAT(...$)` and an alternate
// return written `CALL READB(...,$990,$990)`, and Label has already tagged
// that fence fortran by the time this runs.
func Unmath(body string) string {
	blocks, _ := Blocks(body)
	lines := strings.Split(body, "\n")
	touched := false
	for _, b := range blocks {
		if b.End == 0 || Sigil(b.Lang) {
			continue
		}
		for i := b.Line; i < b.End-1; i++ {
			if plain := untexted(unmathLine(lines[i])); plain != lines[i] {
				lines[i], touched = plain, true
			}
		}
	}
	if !touched {
		return body
	}
	return strings.Join(lines, "\n")
}

// Sigil says a dollar in a listing of this language is a character the
// program is written with, and not a delimiter somebody left there.
//
// Two things need the answer and they have to agree. Unmath will not touch a
// listing of one of these languages, and audit rule M13, which reports a
// dollar inside a fence, has nothing to report about one either. A rule that
// asked about a language the repair had already decided to leave alone would
// report something nobody can act on, which is where the three findings
// against McCabe's FORTRAN came from: `FORMAT(DOMOLKI STRUCTURE FILE NAME?
// $)` is a format descriptor and `CALL READB(...,$990,$990)` is a pair of
// alternate return labels, and both are exactly what the page printed.
//
// A fence with no tag on it is not one of these. papers split runs Label
// before either caller, so by then an untagged fence is one Sniff had no
// answer for, and the honest reading of no answer is that the dollars in it
// are not the language's own.
func Sigil(lang string) bool { return sigilLang[strings.ToLower(lang)] }

// sigilLang is the languages this corpus prints that write a dollar. The
// older papers are the reason for most of it: FORTRAN puts one in a FORMAT
// descriptor and in front of an alternate return label, BASIC ends a string
// variable with one, and the assembler listings use it for hexadecimal.
var sigilLang = map[string]bool{
	"asm": true, "awk": true, "bash": true, "basic": true, "fortran": true,
	"makefile": true, "perl": true, "php": true, "sh": true, "tcl": true,
	"tex": true,
}

// texSymbol is a TeX control sequence with the character that stands for it
// in a listing. Everything on it is a character the page printed and the
// reader spelled in TeX, and nothing on it is an operator that only exists
// set as mathematics.
//
// The order matters. A Replacer takes the first pattern that matches at a
// position rather than the longest, so `\leftarrow` has to come before `\le`
// or the arrow comes out as `≤ftarrow`.
var texSymbol = strings.NewReplacer(
	`\leftarrow`, "←", `\rightarrow`, "→", `\Rightarrow`, "⇒", `\gets`, "←",
	`\leq`, "≤", `\geq`, "≥", `\neq`, "≠", `\le`, "≤", `\ge`, "≥", `\ne`, "≠",
	`\times`, "×", `\div`, "÷", `\pm`, "±",
	`\land`, "∧", `\lor`, "∨", `\neg`, "¬",
	`\in`, "∈", `\subset`, "⊂", `\supset`, "⊃",
	`\cup`, "∪", `\cap`, "∩", `\emptyset`, "∅",
	`\cdots`, "...", `\ldots`, "...", `\cdot`, "·",
	`\alpha`, "α", `\beta`, "β", `\sigma`, "σ", `\lambda`, "λ",
	`\Gamma`, "Γ", `\Sigma`, "Σ", `\Delta`, "Δ", `\Lambda`, "Λ",
	`\{`, "{", `\}`, "}",
)

// texSpace is the TeX spacing commands, which stand for a space and are only
// ever taken out inside a math span. Outside one they are left alone: a
// backslash and a comma is a spacing command in TeX and could be almost
// anything in a program, and there is no need to guess.
var texSpace = strings.NewReplacer(
	`\ `, " ", `\,`, " ", `\;`, " ", `\quad`, "  ", `\qquad`, "    ",
)

// untexted spells the TeX a listing wrote outside its math spans.
//
// Unmath takes the dollars off a span that turns out to be plain text, and
// that only reaches what somebody put dollars round. The Aho and Corasick
// algorithms are written half and half: `for $i \leftarrow 1$ until $k$ do`
// has the arrow inside a span and `state \leftarrow 0` on the next line has
// it bare, both on the same page and both meaning the same thing. Spelling
// one and not the other leaves a listing with two arrows in it, and the same
// listing had `g(state, a_j)` bare on one line under `g(state, $a_j$)` on
// another.
//
// What is inside a span is left for unmathLine, which has already run by the
// time this does. A span that kept its dollars is mathematics, and `\leq`
// inside mathematics is how it is written and not something to replace. A
// line with an odd number of dollars on it has a span running off the end,
// so there is no telling which text is inside one and the line is left as it
// came.
func untexted(line string) string {
	if !strings.ContainsAny(line, `\_^`) {
		return line
	}
	if strings.Count(line, "$")%2 != 0 {
		return line
	}
	var b strings.Builder
	last := 0
	for _, m := range dollarSpan.FindAllStringIndex(line, -1) {
		b.WriteString(outside(line[last:m[0]]))
		b.WriteString(line[m[0]:m[1]])
		last = m[1]
	}
	b.WriteString(outside(line[last:]))
	return b.String()
}

func outside(s string) string {
	return looseScript.ReplaceAllStringFunc(texSymbol.Replace(s), func(m string) string {
		set := subscript
		if m[1] == '^' {
			set = superscript
		}
		r := m[2:]
		if spelled := set.Replace(r); spelled != r {
			return m[:1] + spelled
		}
		return m
	})
}

// looseScript is a one character script outside a math span, and is much
// narrower than script is inside one. Outside a span an underscore is as
// likely to be part of a name as a subscript, and `max_value` spelled as a
// subscript would come out `maxᵥalue`, so the whole of the name, the
// underscore and the one character after it, has to stand alone as a word.
// That is true of Aho and Corasick's `a_j)` and `a_1 ` and false of every
// snake case identifier in the corpus.
var looseScript = regexp.MustCompile(`\b([A-Za-z])([_^])([0-9A-Za-z])\b`)

// script is a subscript or a superscript of one character, which is how a
// paper prints an indexed name in a listing that has no way of typing one.
// Tarjan's stack holds the edge `(u_1, u_2)` and Floyd's convergence test is
// over a subinterval of length `(b-a)/2^n`.
var script = regexp.MustCompile(`([_^])([0-9A-Za-z+=()-])`)

// subscript and superscript are the Unicode characters that set a script
// without mathematics. They are gappy, and deliberately not filled in with
// anything else: there is no subscript b or z in Unicode, and writing `zb`
// for `z_b` would read as two letters and be a worse answer than leaving the
// span as it came. A span this table cannot spell keeps its dollars and M13
// goes on reporting it.
var (
	subscript = strings.NewReplacer(
		"0", "₀", "1", "₁", "2", "₂", "3", "₃", "4", "₄",
		"5", "₅", "6", "₆", "7", "₇", "8", "₈", "9", "₉",
		"+", "₊", "-", "₋", "=", "₌", "(", "₍", ")", "₎",
		"a", "ₐ", "e", "ₑ", "h", "ₕ", "i", "ᵢ", "j", "ⱼ",
		"k", "ₖ", "l", "ₗ", "m", "ₘ", "n", "ₙ", "o", "ₒ",
		"p", "ₚ", "r", "ᵣ", "s", "ₛ", "t", "ₜ", "u", "ᵤ",
		"v", "ᵥ", "x", "ₓ",
	)
	superscript = strings.NewReplacer(
		"0", "⁰", "1", "¹", "2", "²", "3", "³", "4", "⁴",
		"5", "⁵", "6", "⁶", "7", "⁷", "8", "⁸", "9", "⁹",
		"+", "⁺", "-", "⁻", "=", "⁼", "(", "⁽", ")", "⁾",
		"a", "ᵃ", "b", "ᵇ", "c", "ᶜ", "d", "ᵈ", "e", "ᵉ",
		"f", "ᶠ", "g", "ᵍ", "h", "ʰ", "i", "ⁱ", "j", "ʲ",
		"k", "ᵏ", "l", "ˡ", "m", "ᵐ", "n", "ⁿ", "o", "ᵒ",
		"p", "ᵖ", "r", "ʳ", "s", "ˢ", "t", "ᵗ", "u", "ᵘ",
		"v", "ᵛ", "w", "ʷ", "x", "ˣ", "y", "ʸ", "z", "ᶻ",
	)
)

// unscript spells the one character scripts of a span as the page set them.
// A character the table has no letter for is left as it came, script marker
// and all, which is what makes the span fail the plain text test below and
// keep its dollars.
func unscript(s string) string {
	return script.ReplaceAllStringFunc(s, func(m string) string {
		r := m[1:]
		set := subscript
		if m[0] == '^' {
			set = superscript
		}
		if spelled := set.Replace(r); spelled != r {
			return spelled
		}
		return m
	})
}

// texWrap is a TeX command that sets its argument in some face or other. In
// a listing the face is the monospace of the listing and the argument is the
// text, so the command comes off and the argument stays.
var texWrap = regexp.MustCompile(`\\(?:text|textit|textbf|texttt|mathrm|mathit|mathbf|mathcal|mathfrak|mathbb|operatorname)\{([^{}]*)\}`)

// dollarSpan is a math span on one line. A display has no place in a listing
// and is left for M13 to report.
var dollarSpan = regexp.MustCompile(`\$[^$\n]+\$`)

func unmathLine(line string) string {
	// A line with an odd number of dollars on it has a span that runs off
	// the end, and pairing the ones that are there would pair the wrong two.
	if strings.Count(line, "$")%2 != 0 {
		return line
	}
	return dollarSpan.ReplaceAllStringFunc(line, func(span string) string {
		plain := unscript(texSpace.Replace(texSymbol.Replace(span[1 : len(span)-1])))
		for {
			next := texWrap.ReplaceAllString(plain, "$1")
			if next == plain {
				break
			}
			plain = next
		}
		if strings.ContainsAny(plain, `\^_{}`) {
			return span
		}
		return plain
	})
}
