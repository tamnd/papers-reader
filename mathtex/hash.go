package mathtex

import "strings"

// Hash escapes a number sign that the page prints as a character.
//
// Codd names the columns of his example relations manager#, serial# and
// part#, and the number sign is part of the name in the same way the letters
// are. Page 10 of that paper carries
//
//	$\Delta_t(manager#) \subset \Delta_t(serial#)$
//
// which is what the page says, transcribed correctly, and which KaTeX will not
// read. In TeX a number sign is the macro parameter character, so it is markup
// and not a character, and a reader meets it where an argument should be and
// stops: "Expected 'EOF', got '#'". The page was asked for three times at three
// resolutions and came back the same way all three times, correctly, because
// there is nothing wrong with it.
//
// So the repair belongs here and not in the asking. \# is how TeX sets the
// character, it renders as the number sign the page prints, and it is the only
// reading available: this corpus defines no macros. Nothing in it has a \def or
// a \newcommand in it, and a paper that is not about TeX has no reason to grow
// one, so a number sign inside a math span is the character every time.
//
// The exception is written down anyway, because the cost of being wrong about
// it is a definition silently turned into nonsense. A span that defines a macro
// is left exactly as it stands, since that is the one place where the number
// sign really is markup and \# would break it.
//
// It works inside the math spans only. A number sign in prose is a heading
// marker in Markdown, an anchor in a link, and the first character of the
// attribute block this corpus hangs on every caption, and all three of those
// are on pages that also carry mathematics.
func Hash(body string) (string, int) {
	spans, _ := Split(body)
	rs := []rune(body)
	var b strings.Builder
	n, at := 0, 0
	for _, s := range spans {
		b.WriteString(string(rs[at:s.Start]))
		at = s.End
		fixed, count := hashSpan(string(rs[s.Start:s.End]))
		b.WriteString(fixed)
		n += count
	}
	b.WriteString(string(rs[at:]))
	return b.String(), n
}

// defines are the commands that give the number sign its other meaning.
var defines = []string{`\def`, `\newcommand`, `\renewcommand`, `\providecommand`}

// hashSpan escapes the bare number signs in one span.
//
// Bare means not already escaped, and the count of the backslashes in front of
// it is what says which it is. \# is the character and needs nothing done to
// it, and \\# is a line break followed by a bare number sign, because the pair
// of backslashes is itself the escape. So an even run leaves the sign bare and
// an odd one has already taken it.
func hashSpan(s string) (string, int) {
	if !strings.Contains(s, "#") {
		return s, 0
	}
	for _, d := range defines {
		if strings.Contains(s, d) {
			return s, 0
		}
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	n, slashes := 0, 0
	for _, r := range s {
		switch {
		case r == '#' && slashes%2 == 0:
			b.WriteString(`\#`)
			n++
		default:
			b.WriteRune(r)
		}
		if r == '\\' {
			slashes++
		} else {
			slashes = 0
		}
	}
	return b.String(), n
}
