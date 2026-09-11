package mathtex

import (
	"regexp"
	"strings"
)

// Negation puts back the stroke that negates a relation sign.
//
// "x is not in A" is set as an epsilon with a stroke through it. A text layer
// with no glyph for the struck sign hands back the sign and the stroke as two
// characters, and the stroke arrives as an ordinary solidus. It falls on
// whichever side the layer met first, and both forms turned up in the
// Eléments:
//
//	if $0\in /S$ and
//	pour $\lambda  /\in$ Sp($u$)
//
// This is worth a repair of its own rather than a line in the errata, for the
// reason that makes it the worst class of fault a corpus can carry. It is silent
// and it inverts the sentence. $0 \in S$ is good TeX, it renders, it reads as
// ordinary mathematics, and it says the opposite of what the author wrote. A
// reader has no way to know. Every other fault these commands repair shows up
// on the page as damage.
//
// What makes it safe to do mechanically is that nothing divides by a relation
// sign. A solidus in mathematics is a quotient and $a/b$, $\mathbf{Z}/n$ and
// $G/H$ are on every other page of the library, but the operand here is \in or
// \subset, which is not a denominator and cannot be made into one. So the
// pattern is not "a solidus near a relation", it is "a solidus whose other
// operand is a relation sign", and that has no second reading.
//
// The three signs are the three anybody has seen struck through, and no more.
// \supset and \subseteq would take the same repair and neither has turned up
// struck, so a rule for them would be a rule no page has ever tested. \neq is
// left out for a different reason: the text layer already writes that one as
// \not=, which is correct, and = is a denominator often enough that reading a
// solidus beside it as a stroke would be a guess.
//
// It works inside the math spans only. A solidus in prose is a solidus, and a
// paper writes and/or and I/O in its own sentences.
func Negation(body string) (string, int) {
	spans, _ := Split(body)
	rs := []rune(body)
	var b strings.Builder
	n, at := 0, 0
	for _, s := range spans {
		b.WriteString(string(rs[at:s.Start]))
		at = s.End
		fixed, count := negateSpan(string(rs[s.Start:s.End]))
		b.WriteString(fixed)
		n += count
	}
	b.WriteString(string(rs[at:]))
	return b.String(), n
}

// Strokes is the same reading with nothing given back, for the audit. It hands
// over each span that carries a stroke the repair would take, and the sign it
// found there, so a rule can say which relation the page has inverted.
func Strokes(body string) ([]Span, []string) {
	spans, _ := Split(body)
	var out []Span
	var signs []string
	for _, s := range spans {
		for _, m := range strokeRE.FindAllStringSubmatch(s.Text, -1) {
			sign := m[1]
			if sign == "" {
				sign = m[2]
			}
			if _, ok := negated[sign]; ok {
				out = append(out, s)
				signs = append(signs, sign)
			}
		}
	}
	return out, signs
}

// negated is what each struck sign becomes.
var negated = map[string]string{
	`\in`:     `\notin`,
	`\subset`: `\not\subset`,
	`\equiv`:  `\not\equiv`,
}

// strokeRE is a solidus on either side of one of those signs.
//
// The signs are named in the pattern rather than matched as any macro and
// looked up after, because the two alternatives compete for the same solidus:
// in "\lambda  /\in" a pattern that took any macro would match "\lambda  /"
// first, decline it as not a relation, and leave the \in behind with the
// solidus already eaten.
//
// The word boundary keeps \int out. \in followed by a letter opens another
// macro and is not the relation.
var strokeRE = regexp.MustCompile(`(\\(?:in|subset|equiv))\b\s*/|/\s*(\\(?:in|subset|equiv))\b`)

// negateSpan repairs one span. Only the solidus and the sign are rewritten and
// the spacing around them is left as it stands, so the diff on a page is the
// two characters that were wrong and nothing else.
//
// The one place a space has to be put back is where the stroke was doing the
// work of ending the control word. In "\in /S" the space after \in ends the
// word and the S belongs to the formula, but write \notin in its place and the
// two run together into \notinS, which is no macro at all. A control word ends
// at the first character that is not a letter, so a letter is the only thing
// that needs holding off.
func negateSpan(s string) (string, int) {
	ms := strokeRE.FindAllStringSubmatchIndex(s, -1)
	if ms == nil {
		return s, 0
	}
	var b strings.Builder
	n, at := 0, 0
	for _, m := range ms {
		sign := ""
		switch {
		case m[2] >= 0:
			sign = s[m[2]:m[3]]
		case m[4] >= 0:
			sign = s[m[4]:m[5]]
		}
		with, ok := negated[sign]
		if !ok {
			continue
		}
		b.WriteString(s[at:m[0]])
		b.WriteString(with)
		if m[1] < len(s) && isLetter(s[m[1]]) {
			b.WriteByte(' ')
		}
		at, n = m[1], n+1
	}
	b.WriteString(s[at:])
	return b.String(), n
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
