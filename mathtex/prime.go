package mathtex

import (
	"regexp"
	"strings"
)

// primeThenPower is a base carrying a prime, then a power on the same base.
//
// TeX reads a prime as a superscript. It is not a character that happens to sit
// high, it is \sp{\prime}, which is why $E'^*$ is not "E prime star" but two
// superscripts on one atom, and why TeX stops on it with "Double superscript".
// The Eléments were written that way 593 times over 199 files, and every one of
// them was a volume that would not build past that line.
//
// The four groups are the base, the primes, the subscript if the base carries
// one, and nothing else. The base is an atom in TeX's sense: a control
// sequence, a braced group, or a single character. A subscript is allowed
// between the primes and the power because bodies write that too,
// $x'_\beta^{m'(\beta)}$, and the reading is the same.
//
// A base that is already braced does not match, because $\{x'\}^*$ has no
// prime next to its power. That is the form this rewrites to, so running it
// twice changes nothing the second time.
var primeThenPower = regexp.MustCompile(
	`(\\[a-zA-Z]+|\{[^{}]*\}|[^\s{}\^_$\\])('+)((?:_(?:\\[a-zA-Z]+|\{[^{}]*\}|[^\s{}\^_$\\]))?)\^`)

// Prime braces a primed base so that the power after it is the only power.
//
// It returns the body and how many it braced. Only mathematics is touched: an
// apostrophe in a sentence is an apostrophe, and English prose is full of
// them.
func Prime(body string) (string, int) {
	spans, _ := Split(body)
	rs := []rune(body)
	var b strings.Builder
	n, at := 0, 0
	for _, s := range spans {
		b.WriteString(string(rs[at:s.Start]))
		at = s.End
		fixed, count := primeSpan(string(rs[s.Start:s.End]))
		b.WriteString(fixed)
		n += count
	}
	b.WriteString(string(rs[at:]))
	return b.String(), n
}

// Powers is the same reading with nothing given back, for the audit.
func Powers(body string) []Span {
	spans, _ := Split(body)
	var out []Span
	for _, s := range spans {
		if primeThenPower.MatchString(s.Text) {
			out = append(out, s)
		}
	}
	return out
}

// primeSpan rewrites one span.
//
// The loop is there because a span can carry more than one, and because the
// replacement of the first moves the offsets of the rest. ReplaceAll would do
// it in one pass, but a base that is itself the tail of a longer base would
// then be brace matched wrongly, and a loop that rescans from the start of the
// changed text cannot make that mistake.
func primeSpan(s string) (string, int) {
	n := 0
	for {
		m := primeThenPower.FindStringSubmatchIndex(s)
		if m == nil {
			return s, n
		}
		base := s[m[2]:m[3]]
		primes := s[m[4]:m[5]]
		sub := s[m[6]:m[7]]
		s = s[:m[0]] + "{" + base + primes + sub + "}^" + s[m[1]:]
		n++
	}
}
