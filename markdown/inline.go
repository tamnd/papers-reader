package markdown

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The patterns of the inline markup the corpus writes.
//
// They are exported and the renderers are not built on top of a shared
// renderer, because each of the three writes different markup for the same
// span and they share only the question of where the span is. A pattern here
// is read by all three, so a citation that one of them stopped recognising
// would be a citation none of them recognised, which is the kind of fault
// somebody notices.
var (
	// Code is a span in backticks, any number of them.
	Code = regexp.MustCompile("`+[^`]+`+")

	// Note is a footnote marker.
	Note = regexp.MustCompile(`\[\^([^\]\s]+)\]`)

	// LooseNote is a space in front of a footnote marker. The corpus has
	// them, off pages where the marker was set raised and the reader put a
	// space where the baseline dropped. A footnote hangs on the word before
	// it with no space, in every one of the four languages, so the space
	// goes.
	LooseNote = regexp.MustCompile(`[ \t]+(\[\^[^\]\s]+\])`)

	// PaperCite is a citation of another paper of this corpus, written as
	// the identifier in double brackets. It is the one piece of navigation
	// the corpus adds that the paper did not have.
	PaperCite = regexp.MustCompile(`\[\[([a-z][a-z0-9]*-[0-9]{4}-[a-z0-9]+)\]\]`)

	// NumCite is a numeric citation, single or a list or a range.
	//
	// Three digits at most, which is the cap the refs package already puts
	// on the citations it rewrites. A bibliography label is a small number
	// and the longest reference list in the corpus is under two hundred
	// entries, so a four digit number in brackets is the year of an author
	// and year citation and not a label. The Paxos paper writes "the
	// explanation of the algorithm for computer scientists by Lampson
	// [1996]", its bibliography has no entry 1996 and never will, and
	// without the cap that page carried six links to nothing and rule P02
	// held the paper out of the corpus for all six.
	//
	// The pattern is not the whole of the question. What is around the
	// brackets decides it too, and that part is in ReplaceCites, which is
	// how everything here should be read.
	NumCite = regexp.MustCompile(`\[([0-9]{1,3}(?:\s*[,\x{2013}-]\s*[0-9]{1,3})*)\]`)

	// Digits picks the numbers out of one of those, so that [3, 7] links
	// twice and the comma between stays punctuation.
	Digits = regexp.MustCompile(`[0-9]+`)

	Strong = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	Emph   = regexp.MustCompile(`(^|[^*])\*([^*]+)\*`)
)

// ReplaceCites rewrites the numeric citations of s, passing each one to f
// without its brackets and putting back whatever f returns in place of the
// whole citation, brackets and all. A bracket NumCite matches that is not a
// citation is left exactly as it stands and f never sees it.
//
// Two things rule one out, and both of them are outside the brackets or
// inside the numbers rather than in the shape NumCite describes.
//
// A bracket with a name against it is an array index. The X100 paper has a
// page of loop bodies written F(A[0]),G(A[0]), F(A[1]),G(A[1]) in text the
// reader left unfenced, and every one of them was being linked to a
// reference list that has no such entry. So a letter, a digit or a closing
// bracket in front of the opening one says this belongs to what precedes
// it. Go has no lookbehind, hence the offset.
//
// A zero anywhere in the group rules out the group. No reference list
// numbers an entry zero, and what gets written that way is the unit
// interval: LSTM draws its inputs from "the interval [0, 1]" in the prose
// of two of its sections rather than inside a math span. The whole group
// goes and not just the zero, because the other half of [0, 1] is the other
// end of the interval and not a reference to entry 1 either.
func ReplaceCites(s string, f func(inner string) string) string {
	var b strings.Builder
	at := 0
	for _, loc := range NumCite.FindAllStringIndex(s, -1) {
		inner := s[loc[0]+1 : loc[1]-1]
		if !cites(s, loc[0], inner) {
			continue
		}
		b.WriteString(s[at:loc[0]])
		b.WriteString(f(inner))
		at = loc[1]
	}
	if at == 0 {
		return s
	}
	b.WriteString(s[at:])
	return b.String()
}

// Cites lists the numeric citations of s without their brackets, in the
// order they appear. It reads s exactly as ReplaceCites does, so a bracket
// the renderers will not link is not one anything else counts either.
func Cites(s string) []string {
	var out []string
	for _, loc := range NumCite.FindAllStringIndex(s, -1) {
		if inner := s[loc[0]+1 : loc[1]-1]; cites(s, loc[0], inner) {
			out = append(out, inner)
		}
	}
	return out
}

// cites says whether the bracketed numbers at this offset are a citation.
// See ReplaceCites for the two things that say they are not.
func cites(s string, at int, inner string) bool {
	if at > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:at])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ']' || r == ')' {
			return false
		}
	}
	for _, n := range Digits.FindAllString(inner, -1) {
		if strings.Trim(n, "0") == "" {
			return false
		}
	}
	return true
}
