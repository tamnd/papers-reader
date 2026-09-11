package mathtex

import (
	"fmt"
	"regexp"
	"strings"
)

// The two things papers need that the Eléments did not.
//
// A book numbers its displays inside a chapter and refers to them by that
// number in the same chapter. A paper numbers them for the whole paper and
// then refers to them constantly, because a paper is an argument and the
// argument is carried by the equations. So the number has to come out of the
// mathematics, and the references to it have to become links.

// numberAtEnd is an equation number the text layer swept into the display.
//
// The layer has no idea the number is set at the right margin rather than in
// the formula, so it hands back the row it read, which is the formula and then
// the number. Three spellings turn up and all three are here: a bare
// parenthesised number, one held off the formula by a spacing command, and
// \tag, which is what a source that was LaTeX to begin with writes.
//
// The number itself is deliberately narrow. A paper numbers equations 1, 2, 3
// or 3.1, 3.2 or 12a, 12b, and nothing else, so the pattern is digits with at
// most a dotted or hyphenated part and at most one trailing letter. Widening it
// to anything in brackets would take the (n) out of $f(n)$ on any display that
// happens to end in a function call, which is most of them.
var numberAtEnd = regexp.MustCompile(
	`(?:\\(?:q?quad|h?space\{[^{}]*\}|,|;|:|!)\s*)*` +
		`(?:\\tag\{\s*\(?\s*(` + eqNumber + `)\s*\)?\s*\}|\(\s*(` + eqNumber + `)\s*\))\s*$`)

// eqNumber is how a paper writes an equation number.
const eqNumber = `[0-9]+(?:[.\-][0-9]+)*[a-z]?`

// TakeNumber separates the equation number from the mathematics of a display.
//
// It returns the mathematics without the number and the number as the paper
// prints it, or the display unchanged and an empty string. The number goes in
// the attribute block that follows the display and never in the TeX: \tag{3}
// inside the mathematics would make the span differ between the English and
// its translations, and L01 would then refuse the translation for altering a
// formula that nobody altered.
//
// A display that is nothing but a number is left alone. That is not an equation
// the layer mangled, it is a page number or a stray the extraction should be
// reporting rather than quietly eating.
func TakeNumber(tex string) (string, string) {
	m := numberAtEnd.FindStringSubmatchIndex(tex)
	if m == nil {
		return tex, ""
	}
	number := submatch(tex, m, 1)
	if number == "" {
		number = submatch(tex, m, 2)
	}
	rest := strings.TrimRight(tex[:m[0]], " \t\n\r")
	if strings.TrimSpace(rest) == "" {
		return tex, ""
	}
	// The trailing whitespace of the display is put back, because a display is
	// written with its delimiters on their own lines and taking the newline
	// with the number would weld the closing $$ to the last row of a matrix.
	tail := tex[len(strings.TrimRight(tex, " \t\n\r")):]
	return rest + tail, number
}

// submatch is one group of a FindStringSubmatchIndex result, empty when the
// group did not take part.
func submatch(s string, m []int, n int) string {
	if 2*n+1 >= len(m) || m[2*n] < 0 {
		return ""
	}
	return s[m[2*n]:m[2*n+1]]
}

// A Numbering is how one paper writes a reference to one of its own equations.
//
// It is learned per paper and never assumed, because the shapes are not
// compatible with each other. A paper that writes "by (3)" and a paper that
// writes "by Eq. 3" both mean the same thing, and a pattern wide enough for
// both turns every parenthesised number in the first into a candidate link and
// every section number in the second into one too.
//
// The zero Numbering links nothing, which is the right behaviour for a paper
// that numbers no equations.
type Numbering struct {
	// known is the numbers the paper actually prints on a display. A reference
	// to a number that is not in here is not a reference to an equation, and
	// this is the whole defence against turning "(1)" in a list of conditions
	// into a link to nowhere.
	known map[string]bool
	// forms is the spellings this paper was seen to use, longest first so that
	// "Equation 3" is not read as the bare "3".
	forms []form
}

// A form is one spelling of a reference, as a pattern with the number in group
// one.
type form struct {
	name string
	re   *regexp.Regexp
}

// candidates are every spelling worth looking for, in the order they are
// tried. Each is anchored on a word boundary so that "Eq. 3" is not found
// inside "Seq. 3", and the bare parenthesised form comes last because it is
// the one that overlaps with ordinary prose.
//
// The closing bracket is optional but its whitespace is not: "Eq. (1)" and
// "Eq. 1" both match and the space after the number belongs to the sentence
// either way, so the link is written round the reference and not round the
// reference and the gap after it.
var candidates = []form{
	{"equation", regexp.MustCompile(`\b[Ee]quations?\.?\s*\(?\s*(` + eqNumber + `)(?:\s*\))?`)},
	{"eqn", regexp.MustCompile(`\b[Ee]qns?\.?\s*\(?\s*(` + eqNumber + `)(?:\s*\))?`)},
	{"eq", regexp.MustCompile(`\b[Ee]qs?\.?\s*\(?\s*(` + eqNumber + `)(?:\s*\))?`)},
	{"formula", regexp.MustCompile(`\b[Ff]ormulas?\s*\(?\s*(` + eqNumber + `)(?:\s*\))?`)},
	{"paren", regexp.MustCompile(`\(\s*(` + eqNumber + `)\s*\)`)},
}

// LearnNumbering works out how a paper refers to its equations by looking at
// what it does with the numbers it is known to have printed.
//
// numbers is what TakeNumber pulled off the displays of the whole paper. A
// spelling counts as this paper's when the prose uses it on at least one of
// those numbers, so a paper that writes "Eq. 3" once and "(3)" forty times
// gets both and a paper that writes neither gets nothing and links nothing.
//
// The body it learns from is the whole paper and not one section, because a
// paper numbers an equation in section 3 and refers to it in section 6, and a
// per-section pattern would be learned from a section that has no references
// in it at all.
func LearnNumbering(body string, numbers []string) *Numbering {
	n := &Numbering{known: make(map[string]bool, len(numbers))}
	for _, number := range numbers {
		n.known[number] = true
	}
	if len(n.known) == 0 {
		return n
	}
	prose := BlankDisplays(Strip(body))
	for _, c := range candidates {
		for _, m := range c.re.FindAllStringSubmatch(prose, -1) {
			if n.known[m[1]] {
				n.forms = append(n.forms, c)
				break
			}
		}
	}
	return n
}

// Numbers is the equation numbers this paper prints, which is what an audit
// needs to say that a reference points at an equation that exists.
func (n *Numbering) Numbers() map[string]bool {
	out := make(map[string]bool, len(n.known))
	for k := range n.known {
		out[k] = true
	}
	return out
}

// Forms names the spellings this paper was found to use. It is for the report,
// so that a paper whose references did not link can be told from a paper that
// has none.
func (n *Numbering) Forms() []string {
	var out []string
	for _, f := range n.forms {
		out = append(out, f.name)
	}
	return out
}

// Link rewrites this paper's references to its own equations as Markdown links.
//
// href is asked for the target of one number and returns an empty string for a
// number it will not link, which is how a caller refuses an equation whose
// anchor has not been assigned yet. It is called once per reference rather than
// once per number, so a caller that counts calls is counting references.
//
// Three things are left alone, and each of them is a way this could go wrong:
//
//   - Mathematics. A reference inside a formula is part of the formula, and
//     rewriting it would change a math span, which L01 refuses.
//   - Fenced code. A line number in a listing is not an equation reference.
//   - A bare parenthesised number that starts a line. That is a list label, as
//     in a paper that sets out its conditions as (1), (2), (3), and linking it
//     points the definition of a condition at a formula.
//
// It returns the new body and how many references it linked.
func (n *Numbering) Link(body string, href func(number string) string) (string, int) {
	if len(n.forms) == 0 {
		return body, 0
	}
	count := 0
	for _, f := range n.forms {
		var linked int
		body, linked = n.linkOne(body, f, href)
		count += linked
	}
	return body, count
}

// linkOne rewrites the references of one spelling, left to right.
//
// The protected regions are found again for each spelling rather than once for
// all of them. A link written by an earlier spelling moves every offset after
// it, and a set of regions carried across the passes would be pointing at the
// wrong bytes by the second one. Finding them again is a walk of the body per
// spelling and there are five spellings.
func (n *Numbering) linkOne(body string, f form, href func(string) string) (string, int) {
	safe := regions(body)
	var b strings.Builder
	at, count := 0, 0
	for _, m := range f.re.FindAllStringSubmatchIndex(body, -1) {
		if m[0] < at || covered(safe, m[0]) {
			continue
		}
		number := body[m[2]:m[3]]
		if !n.known[number] {
			continue
		}
		if f.name == "paren" && startsLine(body, m[0]) {
			continue
		}
		target := href(number)
		if target == "" {
			continue
		}
		b.WriteString(body[at:m[0]])
		fmt.Fprintf(&b, "[%s](%s)", body[m[0]:m[1]], target)
		at = m[1]
		count++
	}
	if count == 0 {
		return body, 0
	}
	b.WriteString(body[at:])
	return b.String(), count
}

// A region is a stretch of the body that is not prose, in byte offsets.
type region struct{ from, to int }

// regions is the mathematics and the code of a body, which is everything Link
// must not touch. They are returned in order and they do not overlap, because
// mathtex.Split already refuses to open a span inside a fence when the caller
// has blanked the fences, and a fence inside a math span is M13's finding
// rather than this function's problem.
func regions(body string) []region {
	out := fences(body)
	rs := []rune(body)
	// Split counts in runes and the rest of this counts in bytes, so the span
	// bounds are converted once here rather than at every comparison.
	offset := make([]int, len(rs)+1)
	at := 0
	for i, r := range rs {
		offset[i] = at
		at += len(string(r))
	}
	offset[len(rs)] = at
	spans, _ := Split(body)
	for _, s := range spans {
		out = append(out, region{offset[s.Start], offset[s.End]})
	}
	return out
}

// fences is every fenced code block in a body, in byte offsets.
//
// It is a scanner and not a pattern because the closing fence has to be at
// least as long as the opening one, which is CommonMark's rule and is what
// lets a listing that contains three backticks be fenced in four, and RE2 has
// no back reference to say it with. An opening fence that is never closed runs
// to the end of the body, which is the reading that keeps a half written page
// from having its listing treated as prose.
func fences(body string) []region {
	var out []region
	at, open, from := 0, "", 0
	for _, line := range strings.SplitAfter(body, "\n") {
		mark := fenceMark(line)
		switch {
		case open == "" && mark != "":
			open, from = mark, at
		case open != "" && mark != "" && mark[0] == open[0] && len(mark) >= len(open):
			out = append(out, region{from, at + len(line)})
			open = ""
		}
		at += len(line)
	}
	if open != "" {
		out = append(out, region{from, len(body)})
	}
	return out
}

// fenceMark is the run of backticks or tildes a line opens or closes a fence
// with, empty when the line is not a fence. Up to three spaces of indentation
// are allowed, which is CommonMark's rule, and four make it an indented code
// block rather than a fence.
func fenceMark(line string) string {
	i := 0
	for i < len(line) && i < 4 && line[i] == ' ' {
		i++
	}
	if i == 4 || i >= len(line) {
		return ""
	}
	c := line[i]
	if c != '`' && c != '~' {
		return ""
	}
	j := i
	for j < len(line) && line[j] == c {
		j++
	}
	if j-i < 3 {
		return ""
	}
	return line[i:j]
}

// covered says whether an offset falls inside a protected region.
func covered(rs []region, at int) bool {
	for _, r := range rs {
		if at >= r.from && at < r.to {
			return true
		}
	}
	return false
}

// startsLine says whether an offset is the first thing on its line, ignoring
// the indentation. A list label is written at the start of a line and a
// reference to an equation is written in the middle of a sentence.
func startsLine(body string, at int) bool {
	for i := at - 1; i >= 0; i-- {
		switch body[i] {
		case '\n':
			return true
		case ' ', '\t':
			continue
		default:
			return false
		}
	}
	return true
}
