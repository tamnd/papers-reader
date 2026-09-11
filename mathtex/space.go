package mathtex

import "strings"

// SqueezeSpace is a math span with the whitespace TeX ignores taken out.
//
// In mathematics mode a space sets nothing. $M \cap N$ and $M\cap N$ are the
// same formula, a display broken over three lines is the same display as one
// written on a single line, and the indentation inside an aligned block is
// there for whoever reads the source. A comparison that reads any of that as a
// difference refuses correct work.
//
// It refuses it in the place it costs most. The translator holds an answer to
// the rule that the mathematics comes back exactly as it went out, and a model
// asked to translate a paragraph reflows the source of the formulae in it as a
// matter of course. Three of the answers on disk in bourbaki-solver differed
// from the English in nothing else: M \cap N against M\cap N, a display the
// model wrapped, and an aligned block it re-indented. Each of those is a
// question
// asked, five minutes waited, the answer thrown away and the question asked
// again, and asked again to the same end.
//
// The one space that stays is the one between two letters. It is what ends a
// control word: taking it out of \cap N runs the macro into the letter after
// it and makes \capN, which is not what anybody wrote. Everywhere else a run of
// whitespace goes, and a run of any length reads the same as a single space, so
// two spans that differ only in how they were laid out come out identical.
//
// This is not a rewriter. Nothing here goes back into the corpus; it is read by
// the comparison and thrown away, and the file keeps the spacing the model
// wrote.
func SqueezeSpace(span string) string {
	rs := []rune(span)
	var b strings.Builder
	b.Grow(len(span))
	var prev rune
	for i := 0; i < len(rs); i++ {
		if !isMathSpace(rs[i]) {
			b.WriteRune(rs[i])
			prev = rs[i]
			continue
		}
		j := i
		for j < len(rs) && isMathSpace(rs[j]) {
			j++
		}
		i = j - 1
		if j < len(rs) && isTeXLetter(prev) && isTeXLetter(rs[j]) {
			b.WriteRune(' ')
			prev = ' '
		}
	}
	return b.String()
}

// isMathSpace is the whitespace a TeX formula ignores. A non-breaking space is
// not in the list: it is a character the author put there to be set, and two
// spans that differ by one differ.
func isMathSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
