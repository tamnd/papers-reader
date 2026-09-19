package extract

import (
	"strings"

	"github.com/tamnd/papers-reader/mathtex"
)

// Undisplay writes back as prose a paragraph the reader set as a display
// formula.
//
// A sentence broken across a page arrives on the next page with no capital
// at the front of it and a lower case letter after a bare symbol, and a
// reader looking at that opens a display. Razborov's page 20 begins in the
// middle of the definition of a formal complexity measure, so the top of the
// page is a formula that runs "\mu(g) \text{ for all } f, g \in F_n" and
// then four sentences of English, every one of them inside its own \text.
// Ford and Fulkerson's page 4 begins in the middle of the proof of the
// theorem and does the same thing.
//
// The display never closes, because there is nothing on the page to close
// it: the sentence ends and the next paragraph starts. Rule M11 is what
// reports it, and it is right to, because a `\[` with no `\]` under it is a
// page KaTeX will not set at all.
//
// An unclosed display is the whole of the test, which is what keeps this off
// the mathematics. A paper writes `\text` inside a real display all the time,
// in the where clause of a definition and in the conditions on a case split,
// and every one of those closes. What does not close is broken, and a broken
// display whose content is mostly words is a paragraph.
//
// Mostly is a quarter here, and the number is that low because there is
// nothing for it to separate. The corpus has four unclosed displays in 1876,
// and by the share of their runes that stand inside a \text they measure 0,
// 0, 50 and 64 per cent. The two zeros are Chiu's plot of the fairness index
// and the ALGOL listing on Knuth's page 7, both of them mathematics or
// program text with not one word in either, and the other two are Ford and
// Razborov. So the line has the whole of the middle to sit in and where
// exactly it sits changes nothing.
//
// A block under the line is left as it came and stays M11's to report,
// because an unclosed display that is really mathematics has no answer this
// function can give: a closer could go anywhere.
func Undisplay(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	// A page in dollars writes the same two characters at both ends of a
	// display, so which one a `$$` is has to be tracked rather than read off
	// the line. Without that every closing delimiter opened a display of its
	// own and the paragraph under it was read as the body of it.
	inside := false
	for i := 0; i < len(lines); i++ {
		if code[i] {
			continue
		}
		if inside {
			if closesHere(lines[i]) {
				inside = false
			}
			continue
		}
		if !bareOpener(lines[i]) {
			continue
		}
		inside = true
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" && !code[end] {
			end++
		}
		if end == i+1 || hasCloser(lines[i+1:end]) {
			continue
		}
		// The paragraph ended with the display still open, and a blank line
		// ends it whatever the delimiters say.
		inside = false
		body := strings.Join(lines[i+1:end], "\n")
		if !mostlyProse(body) {
			continue
		}
		out := append([]string{}, lines[:i]...)
		out = append(out, unmathed(body))
		out = append(out, lines[end:]...)
		lines, code = out, codeLines(out)
	}
	return strings.Join(lines, "\n")
}

// bareOpener says a line is nothing but the start of a display.
func bareOpener(line string) bool {
	line = strings.TrimSpace(line)
	return line == `\[` || line == "$$"
}

// closesHere says a line ends the display that is open above it.
func closesHere(line string) bool {
	return strings.Contains(line, `\]`) || strings.TrimSpace(line) == "$$"
}

// hasCloser says whether the body of a display has its closing delimiter in
// it, which is the ordinary case and the one this file leaves alone.
func hasCloser(body []string) bool {
	for _, line := range body {
		if closesHere(line) {
			return true
		}
	}
	return false
}

// proseShare is how much of a display has to be words before it is read as a
// paragraph. See Undisplay for the measurements behind it.
const proseShare = 0.25

// mostlyProse says whether the runs of \text in a span cover almost all of
// it. The delimiters and the braces of the runs themselves are counted
// against the prose, which is the conservative way round: a block that
// crosses the line does so on the strength of the words alone.
func mostlyProse(span string) bool {
	n := len([]rune(span))
	if n == 0 {
		return false
	}
	words := 0
	for _, r := range mathtex.TextRuns(span) {
		words += len([]rune(r.Text))
	}
	return float64(words)/float64(n) >= proseShare
}

// unmathed is the paragraph a display of prose was hiding: the words as they
// stand, and whatever is left between them set as inline mathematics.
//
// Punctuation at the end of a formula is moved out of it, because a full
// stop set in a math font at the end of a sentence is a full stop in the
// wrong font. The space either side of a run is kept as the run had it,
// since that is the space the sentence needs.
func unmathed(span string) string {
	rs := []rune(span)
	var b strings.Builder
	at := 0
	for _, r := range mathtex.TextRuns(span) {
		// The run is bounded by the text and not by the macro that sets it,
		// so the macro and its two braces have to come off here.
		b.WriteString(between(string(rs[at : r.Start-len([]rune(r.Macro))-1])))
		b.WriteString(r.Text)
		at = r.End + 1
	}
	b.WriteString(between(string(rs[at:])))
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}

// between is the mathematics that stands between two runs of prose, in the
// corpus's delimiters, and is the gap itself when there is nothing in it.
func between(gap string) string {
	inner := strings.TrimSpace(gap)
	if inner == "" {
		return gap
	}
	lead := gap[:strings.Index(gap, inner)]
	tail := gap[len(lead)+len(inner):]
	stops := strings.TrimRight(inner, ".,;:")
	if stops == "" {
		return gap
	}
	return lead + "$" + stops + "$" + inner[len(stops):] + tail
}
