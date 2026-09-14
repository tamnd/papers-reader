package extract

import (
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/mathtex"
)

// Money escapes the dollar signs that are currency and not mathematics.
//
// A reader transcribes what is printed, and what is printed on page 1 of the
// Unix paper is "$40,000". The page has one dollar sign on it, so it opens a
// math span that never closes, acceptance rule A2 refuses the page, and the
// reader is asked again at 400 and then at 600 dpi. It reads the price
// correctly all three times, because the price is correct, and the paper
// stops there with its first page missing.
//
// The native path has the same problem and answers it with a bigger hammer:
// pdftotext produces no TeX at all, so every dollar it hands over is escaped
// on sight. A vision reader does produce TeX, and escaping its dollars would
// turn every formula on the page into prose, so this has to tell the two
// apart.
//
// The test is arithmetic and is done a line at a time. An inline span opens
// and closes on one line, which is what the prompt asks for and what the rest
// of this package already assumes, so a prose line with an even number of
// dollars on it is a line whose dollars pair up and nothing here touches it.
// A line with an odd number has one dollar too many, and if one of them looks
// like a price then that is the one. Escaping stops the moment the count is
// even again, so a line carrying both a price and a formula keeps the
// formula.
//
// Nothing is guessed when there is nothing to go on. A line with an odd
// number of dollars and no price on it is a page that really did leave a
// formula open, and A2 should see it and ask again.
//
// Two prices on one line count as even and would otherwise slip through,
// with the sentence between them read as one long formula. That one is
// caught by reading it: a span that opens on a price and holds two ordinary
// words is a sentence, and neither of its delimiters was a delimiter.
//
// A last pass looks across lines rather than within one, for the dollar a
// paper uses as a footnote mark rather than as a price. See unstretched.
func Money(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	for i, line := range lines {
		if code[i] {
			continue
		}
		if dollars(line)%2 == 1 {
			lines[i] = unmoney(line)
			continue
		}
		lines[i] = unsentence(line)
	}
	return unstretched(strings.Join(lines, "\n"))
}

// unstretched escapes a pair of single dollars with a line break between
// them.
//
// The other dollar that is not a delimiter is the one a paper uses as a
// footnote mark. The TraceMonkey author line runs "Mohammad R. Haghighat$,
// Blake Kaplan*, ..." and gives the affiliation four lines down as "Intel
// Corporation$", using the dollar the way the line beside it uses a star, a
// hash and a plus. Those two pair up, the page has an even number of dollars
// so the counting above says nothing, and what they enclose is half the
// authors of the paper and three email addresses. Rule A4 hands it to KaTeX,
// KaTeX says what anybody would, and the page is refused at all three
// resolutions.
//
// An inline span does not cross a line. A paragraph is one line in what the
// prompt asks for and the rest of this package already assumes it: the
// rewriter that turns TeX's own delimiters into this corpus's pairs them
// only within a line, for the same reason. A display may cross as many lines
// as it likes and is left alone.
func unstretched(s string) string {
	for {
		open, shut, ok := stretched(s)
		if !ok {
			return s
		}
		s = s[:shut] + `\` + s[shut:]
		s = s[:open] + `\` + s[open:]
	}
}

// stretched is the first inline pair with a line break inside it.
//
// The fenced blocks are blanked out rather than cut, which leaves every byte
// where it was, so an offset found here is an offset in what was passed in.
func stretched(s string) (open, shut int, ok bool) {
	at := marksIn(mathtex.BlankFences(s))
	for i := 0; i+1 < len(at); i += 2 {
		a, b := at[i], at[i+1]
		if a.double || b.double {
			continue
		}
		if strings.Contains(s[a.at+1:b.at], "\n") {
			return a.at, b.at, true
		}
	}
	return 0, 0, false
}

// A mark is one math delimiter: where it is and whether it is a display.
type mark struct {
	at     int
	double bool
}

// marksIn is the delimiters of a body, in order, with the escaped dollars
// left out.
func marksIn(s string) []mark {
	var out []mark
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '$':
			if i+1 < len(s) && s[i+1] == '$' {
				out = append(out, mark{at: i, double: true})
				i++
				continue
			}
			out = append(out, mark{at: i})
		}
	}
	return out
}

// price is a dollar sign in front of an amount.
//
// The character in front of it has to be one a price can follow, so the
// second dollar of "$x$" and the "y$3" a broken subscript leaves behind are
// not prices. What comes after is a figure with the separators a figure is
// written with, and the "$2^n$" of a complexity bound is excluded by the
// caret rather than by the digit.
var price = regexp.MustCompile(`(?:^|[^\\$0-9A-Za-z])(\$[0-9]+(?:[.,][0-9]+)*)`)

// unmoney escapes the prices on one line until its dollars pair up.
func unmoney(line string) string {
	for dollars(line)%2 == 1 {
		m := price.FindStringSubmatchIndex(line)
		if m == nil {
			return line
		}
		at := m[2]
		// A price followed by a dollar is "$5$", which is a formula whose
		// content happens to be a number.
		if end := m[3]; end < len(line) && line[end] == '$' {
			return line
		}
		line = line[:at] + `\` + line[at:]
	}
	return line
}

// unsentence escapes a pair of dollars that opens on a price and encloses a
// sentence.
//
// The pair is the point. Escaping one of the two would leave the line odd
// and refuse the page for an unclosed formula, which is the failure this
// file exists to stop, so both go or neither does.
func unsentence(line string) string {
	at := marks(line)
	for i := 0; i+1 < len(at); i += 2 {
		open, shut := at[i], at[i+1]
		if !startsPrice(line, open) || !sentence(line[open+1:shut]) {
			continue
		}
		// The later one first, so the earlier offset is still the earlier
		// offset when it is used.
		line = line[:shut] + `\` + line[shut:]
		line = line[:open] + `\` + line[open:]
		return unsentence(line)
	}
	return line
}

// marks is where the unescaped dollars on a line are.
func marks(line string) []int {
	var out []int
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '$':
			out = append(out, i)
		}
	}
	return out
}

// startsPrice says whether the dollar at this offset has an amount after it.
func startsPrice(line string, at int) bool {
	m := price.FindStringSubmatchIndex(line[max(0, at-1):])
	return m != nil && m[2] == at-max(0, at-1)
}

// twoWords is two ordinary words with a space between them.
//
// Mathematics has variables and operators in it and hardly ever two English
// words in a row. Three letters, because a span full of \mathrm{kg} and set
// names should still read as mathematics, and \operatorname and its
// relatives are excluded by the backslash in front of them.
var twoWords = regexp.MustCompile(`(^|[^\\A-Za-z])[A-Za-z]{3,} [A-Za-z]{3,}`)

func sentence(s string) bool { return twoWords.MatchString(s) }

// dollars counts the delimiters on a line, which is every dollar sign the
// paper did not escape.
func dollars(line string) int { return len(marks(line)) }
