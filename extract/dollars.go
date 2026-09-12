package extract

import "strings"

// Dollars rewrites TeX's own math delimiters as the dollars this corpus is
// written in.
//
// The prompt asks for `$...$` and `$$...$$` and most readers give them. A
// model trained on LaTeX source does not always: olmOCR 2 on the GPU reader
// answers in `\(...\)` and `\[...\]` no matter how the question is worded,
// because that is what its training data looks like. The mathematics is
// correct and the delimiters are a dialect.
//
// It matters more than a difference in punctuation because every later stage
// looks for dollars. mathtex splits spans on them, the acceptance rules count
// them, the audit's rule M14 reads a page with no span on it as a page whose
// formulas were flattened into the prose, and the reader renders them. A page
// in the other dialect passes through all of that looking like a page with no
// mathematics on it at all, which is the one failure this corpus is least able
// to see.
//
// This is a translation between two spellings of the same thing and not a
// repair: nothing here decides that a piece of prose is mathematics. What is
// already marked as mathematics keeps its marking and changes its delimiters,
// and anything the rules below are not sure about is left exactly as it came.
func Dollars(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	display(lines, code)
	for i, line := range lines {
		if code[i] {
			continue
		}
		lines[i] = inline(line)
	}
	return strings.Join(lines, "\n")
}

// codeLines marks the lines inside a fenced code block, including the fences
// themselves.
//
// Program text is transcribed verbatim, so a listing that happens to contain
// `\(` contains `\(` and not a formula. A page of a paper about regular
// expressions is the obvious one, and it is not hypothetical enough to leave
// to chance.
//
// An unclosed fence marks everything after it, which is the cautious reading:
// a page with a fence problem is a page rule A7 should see as it is, and
// rewriting the tail of it here would change what A7 is looking at.
func codeLines(lines []string) []bool {
	out := make([]bool, len(lines))
	open := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case open == "":
			for _, mark := range []string{"```", "~~~"} {
				if strings.HasPrefix(trimmed, mark) {
					open, out[i] = mark, true
					break
				}
			}
		default:
			out[i] = true
			if strings.HasPrefix(trimmed, open) {
				open = ""
			}
		}
	}
	return out
}

// display rewrites `\[` and `\]` in place, and only where they stand at the
// edge of a line.
//
// That condition is the whole of the care this function takes. An escaped
// square bracket is ordinary Markdown, written to stop a citation like `\[12\]`
// from being read as a link, and it sits in the middle of a line of prose. A
// display opens a line and closes one. Requiring the edge separates the two
// without either of them having to be recognised as mathematics, so a citation
// is never turned into a formula.
//
// An opener with no closer after it is left alone along with everything it
// would have opened, because half a rewrite is worse than none: it would put a
// single `$$` on a page and leave every dollar after it paired with the wrong
// one.
func display(lines []string, code []bool) {
	for i := 0; i < len(lines); i++ {
		if code[i] || !opensDisplay(lines[i]) {
			continue
		}
		// A display written on one line, `\[ x = 1 \]`, opens and closes in
		// the same place.
		if j := closesDisplay(lines[i]); j >= 0 && j > strings.Index(lines[i], `\[`) {
			lines[i] = replaceAt(lines[i], j, `\]`, "$$")
			lines[i] = replaceAt(lines[i], strings.Index(lines[i], `\[`), `\[`, "$$")
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if code[j] || opensDisplay(lines[j]) {
				break
			}
			if closesDisplay(lines[j]) >= 0 {
				end = j
				break
			}
		}
		if end < 0 {
			continue
		}
		lines[end] = replaceAt(lines[end], closesDisplay(lines[end]), `\]`, "$$")
		lines[i] = replaceAt(lines[i], strings.Index(lines[i], `\[`), `\[`, "$$")
		i = end
	}
}

// opensDisplay reports whether the line starts with `\[` and nothing but
// spaces before it.
func opensDisplay(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), `\[`)
}

// closesDisplay is where `\]` ends the line, and -1 for a line that does not
// end with one.
func closesDisplay(line string) int {
	trimmed := strings.TrimRight(line, " \t")
	if !strings.HasSuffix(trimmed, `\]`) {
		return -1
	}
	return len(trimmed) - len(`\]`)
}

// inline rewrites the `\(...\)` pairs on one line.
//
// Both halves have to be on the line. A paragraph is one line in what the
// prompt asks for, so an inline formula that has grown a line break in it is a
// page that went wrong somewhere else, and pairing across the break would tidy
// the evidence away.
//
// An opener with no closer after it leaves the rest of the line alone, for the
// same reason the display above does.
func inline(line string) string {
	var out strings.Builder
	rest := line
	for {
		open := strings.Index(rest, `\(`)
		if open < 0 {
			break
		}
		shut := strings.Index(rest[open+2:], `\)`)
		if shut < 0 {
			break
		}
		shut += open + 2
		inner := rest[open+2 : shut]
		// An empty pair is not a formula and writing `$$` for it would open a
		// display in the middle of a sentence.
		if strings.TrimSpace(inner) == "" {
			out.WriteString(rest[:shut+2])
			rest = rest[shut+2:]
			continue
		}
		out.WriteString(rest[:open])
		out.WriteString("$")
		out.WriteString(strings.TrimSpace(inner))
		out.WriteString("$")
		rest = rest[shut+2:]
	}
	out.WriteString(rest)
	return out.String()
}

// replaceAt swaps the delimiter at one offset and leaves every other
// occurrence on the line alone.
func replaceAt(line string, at int, old, new string) string {
	if at < 0 || at+len(old) > len(line) || line[at:at+len(old)] != old {
		return line
	}
	return line[:at] + new + line[at+len(old):]
}
