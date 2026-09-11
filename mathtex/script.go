package mathtex

import "strings"

// A base in TeX takes one superscript and one subscript and no more. x^a^b is
// not a formula, it is an error, and TeX and KaTeX both refuse it by name:
// Double superscript. So a span that puts two of a mark against one base is a
// span that does not render, and finding them needs no judgement about what the
// page meant.
//
// They arrive because a text layer has no rows in it. TeX stacks a subscript
// and a superscript at the same place across the page, and pdftohtml hands the
// runs back in the order it finds them, left to right, so a stack comes back
// interleaved. The -1 of an inverse is set over the E of \theta_E, its minus
// starting a hair to the left of the E and its one ending a hair to the right,
// so the three runs arrive as minus, E, one and are written ^-_E^1. The same
// linearisation flattens a two by two matrix into ^X_0^0_I, and pulls the p of
// \sum_{i=0}^{p-1} out to the front as ^p_{i=0}^{-1}.
//
// Three faults, three repairs, one shape, and the shape is what can be tested
// for without opening the printed page.

// DoubleScripts is every run of scripts in a span that puts two of the same
// mark against one base, as each is written.
//
// The reading is TeX's. A chain is the scripts written one after another
// against a base: ^a_b is a chain of two and renders, ^a_b^c is a chain of
// three and does not. Braces end a chain the way they end it in TeX, so
// {x^a}^b is two chains of one and is not reported, which is the form somebody
// writes when they mean it.
//
// A prime is a superscript, which is the part that has to be said out loud
// because it is invisible in the source: \Gamma '_1^{\pi'_1} carries no two
// marks that a reader would count, and it is a double superscript all the same,
// because the prime is the first of them. Eleven of the Eléments' were that
// shape and every one was in one chapter.
func DoubleScripts(s string) []string {
	var out []string
	doubles([]rune(s), &out)
	return out
}

// doubles walks one brace level, recursing into the groups it meets.
func doubles(rs []rune, out *[]string) {
	for i := 0; i < len(rs); {
		switch rs[i] {
		case '\\':
			i = command(rs, i)
		case '{':
			end := group(rs, i)
			doubles(rs[i+1:end], out)
			i = end + 1
		case '^', '_', '\'':
			end, marks := chain(rs, i, out)
			if repeats(marks) {
				*out = append(*out, strings.TrimRight(string(rs[i:end]), " "))
			}
			i = end
		default:
			i++
		}
	}
}

// chain reads the scripts hanging off one base and returns where they end and
// the marks they were written with, a prime counting as the superscript it is.
func chain(rs []rune, i int, out *[]string) (int, []rune) {
	var marks []rune
	for i < len(rs) {
		switch rs[i] {
		case '^', '_':
			marks = append(marks, rs[i])
			i = arg(rs, i+1, out)
		case '\'':
			for i < len(rs) && rs[i] == '\'' {
				i++
			}
			marks = append(marks, '^')
			// TeX gathers a superscript written straight after a prime into
			// the same one, so f'^2 is one superscript and f' ^2 is two. The
			// space is the whole of the difference and KaTeX keeps it.
			if i < len(rs) && rs[i] == '^' {
				i = arg(rs, i+1, out)
			}
		default:
			return i, marks
		}
		for i < len(rs) && rs[i] == ' ' {
			i++
		}
	}
	return i, marks
}

// arg consumes what a mark takes and returns where it ends: a braced group, a
// command, or the single character TeX takes when there is neither.
func arg(rs []rune, i int, out *[]string) int {
	for i < len(rs) && rs[i] == ' ' {
		i++
	}
	if i >= len(rs) {
		return i
	}
	switch rs[i] {
	case '{':
		end := group(rs, i)
		doubles(rs[i+1:end], out)
		return end + 1
	case '\\':
		return command(rs, i)
	}
	return i + 1
}

// group is where the brace that closes the one at i sits, or the end of the
// span when nothing closes it. An unclosed brace is M04's finding and not this
// one, so it is read as reaching to the end rather than refused twice.
func group(rs []rune, i int) int {
	depth := 0
	for ; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			i = command(rs, i) - 1
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(rs)
}

// command is one rune past the backslash sequence at i. A run of letters is a
// command name and anything else is the one character it escapes, which is what
// keeps \^ and \_ from being read as marks.
func command(rs []rune, i int) int {
	i++
	if i < len(rs) && !isTeXLetter(rs[i]) {
		return i + 1
	}
	for i < len(rs) && isTeXLetter(rs[i]) {
		i++
	}
	return i
}

func repeats(marks []rune) bool {
	var sup, sub bool
	for _, m := range marks {
		seen := &sup
		if m == '_' {
			seen = &sub
		}
		if *seen {
			return true
		}
		*seen = true
	}
	return false
}
