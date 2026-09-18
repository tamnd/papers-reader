package extract

import (
	"regexp"
	"strings"
)

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
	lines = standing(lines, codeLines(lines))
	code := codeLines(lines)
	for i, line := range lines {
		if code[i] {
			continue
		}
		lines[i] = inline(line)
	}
	return strings.Join(numbers(display(lines, code)), "\n")
}

// standing handles the `\(` that stands on a line of its own.
//
// inline below will not pair across a line break, and says why: a formula
// that has grown one in the middle of a sentence is a page that went wrong
// somewhere else, and pairing it up would tidy the evidence away. That
// reasoning holds and this is not the case it is about. An opener alone on
// its line is not a broken inline formula, it is a display written with the
// inline delimiters, and two papers do it: Lamport sets the invariants of
// the Paxos protocol as blocks of conjuncts with `\\` between them, and
// Bayer opens the line and puts the symbol and its closer on the next one to
// give a glossary of what goes into the timing formula.
//
// Which of the two it is, is the closing line. A closer alone on its own
// line as well is a display, and gets handed to display below as `\[` and
// `\]` so that one function stays the only place that knows what a display
// looks like. A closer with the rest of a sentence after it is the glossary,
// and the line break is only where the reader wrapped, so the lines are
// joined and inline pairs them on the line it makes.
//
// Everything else is left as it came, which is what sends it to the audit.
// Milner has three formulas that open with `\[` and close with `\)` and
// Hoare has one that closes with `\}`, and a mismatched pair is a misreading
// rather than a dialect: there is no way to know from here which of the two
// delimiters is the one the page actually printed.
func standing(lines []string, code []bool) []string {
	for i := 0; i < len(lines); i++ {
		if code[i] || strings.TrimSpace(lines[i]) != `\(` {
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if code[j] || strings.TrimSpace(lines[j]) == "" || strings.Contains(lines[j], `\(`) || opensDisplay(lines[j]) {
				break
			}
			if strings.Contains(lines[j], `\)`) {
				end = j
				break
			}
		}
		if end < 0 {
			continue
		}
		if strings.TrimSpace(lines[end]) == `\)` {
			lines[i], lines[end] = `\[`, `\]`
			i = end
			continue
		}
		joined := strings.Join(lines[i:end+1], " ")
		lines = append(lines[:i], append([]string{joined}, lines[end+1:]...)...)
		// The lines after this one have moved, and so has every fence in
		// them, so the map of what is code has to be drawn again.
		code = codeLines(lines)
	}
	return lines
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

// citation is what an escaped square bracket holds when it is not
// mathematics: a reference number, a list of them, or a range. Nothing else
// in a paper is written with brackets and no letters in between, and the
// reason a reader escapes them at all is to stop `[12]` from being read as a
// Markdown link.
var citation = regexp.MustCompile(`^[\s0-9,;–—-]*$`)

// number is an equation number printed after the closing delimiter, which is
// where a page prints it and where olmOCR leaves it: `\] (1)`. It is not part
// of the mathematics and it does not stop the line from being a closer.
var number = regexp.MustCompile(`^\s*\([0-9A-Za-z.\-]{1,8}\)\s*$`)

// display rewrites the `\[` and `\]` pairs, and leaves the escaped brackets
// of a citation alone.
//
// Telling the two apart is the whole of the care this function takes. An
// escaped square bracket is ordinary Markdown, written to stop a citation
// like `\[12\]` from being read as a link. The first version of this asked
// for the delimiters to stand at the edges of a line, on the grounds that a
// citation sits in the middle of one, and that was right about the citations
// and wrong about a third of the displays. olmOCR writes Hoare's axioms as
// `A5 \[(r-y) + y \times (1+q)\]`, with the label ahead of the formula and
// the pair in the middle of the line, and it writes an equation number after
// the closer, `\] (1)`, because that is where the page prints it. Thirty four
// delimiters in this corpus were left in the other dialect by the edge rule.
//
// So the edge rule is gone and the citations are recognised for what they
// are: a pair with nothing but digits and punctuation between it is a
// citation and is left exactly as it came.
//
// A pair that shares its line with anything else is written with one dollar
// rather than two. `\[` means display in TeX, but a display in the middle of
// a line of prose is not a display, and Hoare's table sets the axiom label
// and the formula side by side. Writing two dollars there would break the
// line in the reading app at a place the page does not break.
//
// A label ahead of an opener that runs over several lines is put on a line
// of its own, and the `$$` goes under it. `A9 $$` reads as an opener to
// anything that counts delimiters and as a paragraph of running text to the
// assembler, which joined it to the display under it and then broke the
// display in half with a blank line. The label and the delimiter want
// separate lines.
//
// An opener with no closer after it is left alone along with everything it
// would have opened, because half a rewrite is worse than none: it would put
// a single `$$` on a page and leave every dollar after it paired with the
// wrong one.
func display(lines []string, code []bool) []string {
	labelled := map[int]bool{}
	for i := 0; i < len(lines); i++ {
		if code[i] {
			continue
		}
		lines[i] = onOneLine(lines[i])
		at := openIndex(lines[i])
		if at < 0 {
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
		if label := strings.TrimRight(lines[i][:at], " \t"); label != "" {
			lines[i], labelled[i] = label, true
		} else {
			lines[i] = replaceAt(lines[i], at, `\[`, "$$")
		}
		i = end
	}
	if len(labelled) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines)+len(labelled))
	for i, line := range lines {
		out = append(out, line)
		if labelled[i] {
			out = append(out, "$$")
		}
	}
	return out
}

var (
	// oneLineNumber is a display written on one line with its equation
	// number after it: `$$x = y$$ (1)`.
	oneLineNumber = regexp.MustCompile(`^(\$\$.*\$\$)\s*\(([0-9A-Za-z.\-]{1,8})\)$`)
	// closerNumber is a closing delimiter with the number after it, which is
	// the shape a display over several lines ends in.
	closerNumber = regexp.MustCompile(`^\$\$\s*\(([0-9A-Za-z.\-]{1,8})\)$`)
)

// numbers moves an equation number printed after the closing delimiter
// inside the display, as the `\tag` the rest of the corpus is written in.
//
// A page prints the number in the margin beside the display and every reader
// transcribes it after the closer, `$$ (1)`. Left there it is not part of the
// mathematics, so KaTeX does not set it beside the equation, and it is not a
// paragraph either, so the line renders as a stray `(1)` under the display.
//
// Worse than either, it stops the line from being a bare `$$`. The two places
// that walk a body counting displays, tags.Scan and the audit, both read the
// line as more mathematics and then read everything after it as being inside
// the display, headings included. Sections 1.4 and 1.5 of the Chiu paper lost
// their attribute blocks that way and nothing reported it, because as far as
// the scanner could see there was no heading there to tag.
//
// `\tag{1}` is what the corpus already uses, in twenty one displays, and it
// is what KaTeX sets in the margin where the page had it.
func numbers(lines []string) []string {
	code := codeLines(lines)
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		switch {
		case code[i]:
		case oneLineNumber.MatchString(trimmed):
			m := oneLineNumber.FindStringSubmatch(trimmed)
			inner := strings.TrimRight(m[1][:len(m[1])-2], " \t")
			out = append(out, inner+` \tag{`+m[2]+`}$$`)
			continue
		case closerNumber.MatchString(trimmed):
			// A number never opens a display, so a line of this shape is
			// always the end of one.
			out = append(out, `\tag{`+closerNumber.FindStringSubmatch(trimmed)[1]+`}`, "$$")
			continue
		}
		out = append(out, line)
	}
	return out
}

// onOneLine rewrites the pairs that open and close on the same line.
func onOneLine(line string) string {
	var out strings.Builder
	rest := line
	for {
		open := strings.Index(rest, `\[`)
		if open < 0 {
			break
		}
		shut := strings.Index(rest[open+2:], `\]`)
		if shut < 0 {
			break
		}
		shut += open + 2
		inner := rest[open+2 : shut]
		if citation.MatchString(inner) {
			out.WriteString(rest[:shut+2])
			rest = rest[shut+2:]
			continue
		}
		mark := "$"
		if strings.TrimSpace(out.String()+rest[:open]) == "" && closesDisplay(rest[shut:]) == 0 {
			mark = "$$"
		}
		out.WriteString(rest[:open])
		out.WriteString(mark)
		out.WriteString(strings.TrimSpace(inner))
		out.WriteString(mark)
		rest = rest[shut+2:]
	}
	out.WriteString(rest)
	return out.String()
}

// openIndex is where the `\[` that opens a display over several lines
// begins, and -1 for a line that does not open one.
//
// Either edge will do. A delimiter at the end of the line is the usual
// shape, and it allows a label ahead of it because Hoare's axioms are
// written that way. A delimiter at the start with the first line of the
// formula after it is the other shape, and it is the one the edge rule was
// written for.
func openIndex(line string) int {
	trimmed := strings.TrimRight(line, " \t")
	if strings.HasSuffix(trimmed, `\[`) {
		return len(trimmed) - len(`\[`)
	}
	if at := strings.Index(line, `\[`); at >= 0 && strings.TrimSpace(line[:at]) == "" {
		return at
	}
	return -1
}

func opensDisplay(line string) bool { return openIndex(line) >= 0 }

// closesDisplay is where the `\]` that ends the line begins, and -1 for a
// line that does not end with one. An equation number after the delimiter is
// allowed, because that is where the page prints it, and numbers moves it
// inside the display afterwards.
func closesDisplay(line string) int {
	trimmed := strings.TrimRight(line, " \t")
	if at := strings.LastIndex(trimmed, `\]`); at >= 0 {
		if at+2 == len(trimmed) || number.MatchString(trimmed[at+2:]) {
			return at
		}
	}
	return -1
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
