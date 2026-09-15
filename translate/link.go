package translate

import (
	"regexp"
	"strings"
	"unicode"
)

// link is an inline Markdown link or image.
//
// The same shape audit rule T12 looks for, and the two have to stay the
// same: what is taken out here is exactly what would be reported there, and
// a body that leaves this package clean has to leave the audit clean too.
var link = regexp.MustCompile(`!?\[[^\]\n]*\]\([^)\n]*\)`)

// Links lists the links in a body, ignoring the ones inside a formula or a
// listing, where a pair of brackets followed by a pair of parentheses is
// usually neither a link nor anything to do with us.
func Links(body string) []string {
	spans := Protect(body)
	var out []string
	for _, m := range link.FindAllStringIndex(body, -1) {
		if inside(spans, runeIndex(body, m[0])) {
			continue
		}
		out = append(out, body[m[0]:m[1]])
	}
	return out
}

// Unlink puts back the web addresses a model turned into links.
//
// The corpus has no links in it, and a paper that prints an address as prose
// comes back from a translator as a link to itself often enough that
// refusing the answer and asking again just spends another question on the
// same mistake. All three translations of the GAN introduction did it to the
// same footnote, the one giving the address of the code, and all three did
// it the same way: the address, wrapped in brackets, pointing at itself.
//
// Only that shape is undone, and only when the source did not have it. The
// text and the target have to be the same string, so nothing is decided
// about which half a reader was meant to see, and nothing is lost: the
// address that is left is the address the paper printed. A link whose text
// says something the target does not is a model writing prose that was not
// on the page, and Verify refuses that rather than guessing at it.
func Unlink(source, answer string) string {
	ms := link.FindAllStringIndex(answer, -1)
	if len(ms) == 0 {
		return answer
	}
	spans := Protect(answer)
	var b strings.Builder
	at := 0
	for _, m := range ms {
		if inside(spans, runeIndex(answer, m[0])) {
			continue
		}
		whole := answer[m[0]:m[1]]
		if strings.HasPrefix(whole, "!") || strings.Contains(source, whole) {
			continue
		}
		text, target, ok := halves(whole)
		if !ok || text == "" || text != target {
			continue
		}
		b.WriteString(answer[at:m[0]])
		b.WriteString(text)
		at = m[1]
	}
	if at == 0 {
		return answer
	}
	b.WriteString(answer[at:])
	return b.String()
}

// Unescape takes the Markdown escapes out of a web address a model wrote
// back with them in.
//
// The other half of the same tic Unlink undoes. A translator that has been
// told the answer is Markdown sees punctuation in an address and escapes it,
// so "http://www.cse.wustl.edu/~jain" comes back as
// "http://www.cse.wustl.edu/\~jain" and
// "https://doi.org/10.1016/0022-0000(78)90014-4" comes back with the
// parentheses escaped. Neither escape is needed and neither renders: a
// tilde is not Markdown and a parenthesis outside a link is not either. The
// address a reader ends up with is the same one, which is exactly why this
// is a repair rather than a refusal.
//
// It is here because refusing does not work on it. Both papers above were
// asked three times and escaped the same address every time, and the run
// gave up on each of them with every other paragraph already translated.
// The refusal that stopped them is also confusing to read, because the URL
// pattern stops at a parenthesis: the answer's span is reported as
// "https://doi.org/10.1016/0022-0000\" and the source's as
// "https://doi.org/10.1016/0022-0000", which are two addresses that differ
// by one character neither of them ends with.
//
// The address is taken to the end of its whitespace delimited token rather
// than to the end of the match, because the match is what stops short. What
// is put back is the token with a backslash dropped from in front of each
// piece of ASCII punctuation, and it is only put back when that string is
// in the source and the escaped one is not. So an address the paper itself
// wrote with a backslash in it is left alone, and so is an answer that has
// invented an address, which Verify still refuses.
func Unescape(source, answer string) string {
	if !strings.Contains(answer, `\`) {
		return answer
	}
	rs := []rune(answer)
	var b strings.Builder
	at := 0
	for _, s := range Protect(answer) {
		if s.Kind != URL || s.Start < at {
			continue
		}
		end := token(rs, s.End)
		whole := string(rs[s.Start:end])
		plain := escape.ReplaceAllString(whole, "$1")
		if plain == whole || strings.Contains(source, whole) || !strings.Contains(source, plain) {
			continue
		}
		b.WriteString(string(rs[at:s.Start]))
		b.WriteString(plain)
		at = end
	}
	if at == 0 {
		return answer
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// UnescapeMath takes the Markdown escapes out of a formula a model wrote
// back with them in.
//
// The same tic as the one Unescape undoes in an address, in the one place
// where it costs the most. A translator that has been told the answer is
// Markdown sees an underscore in "$\mathrm{BERT}_{\mathrm{BASE}}$" and
// escapes it, and the answer comes back with "\_" where the source has
// "_". Every other word of the paragraph is translated and the paragraph is
// thrown away over one backslash.
//
// It does not cure by asking again. The BERT paper was asked three times
// for the same section on three passes of the run and escaped the same
// subscript every time, and the run gave up on the file each time.
//
// An underscore and a star, and nothing else. Those are the two characters
// Markdown reads inside a word, so they are the two a model escapes out of
// habit. The rest of what a backslash can escape in TeX is meant: "\{" and
// "\%" are how a formula prints a brace and a per cent sign.
//
// The repair is only made when the unescaped formula is one the source has
// and the escaped one is not, which is the same guard Unescape uses. So a
// paper that itself prints a literal underscore inside a formula is left
// alone, and a formula the answer invented is still refused.
func UnescapeMath(source, answer string) string {
	if !strings.Contains(answer, `\`) {
		return answer
	}
	rs := []rune(answer)
	var b strings.Builder
	at := 0
	for _, s := range Protect(answer) {
		if s.Kind != Math || s.Start < at {
			continue
		}
		whole := string(rs[s.Start:s.End])
		if strings.Contains(source, whole) {
			continue
		}
		plain := ""
		for _, c := range unescaped(whole) {
			if strings.Contains(source, c) {
				plain = c
				break
			}
		}
		if plain == "" {
			continue
		}
		b.WriteString(string(rs[at:s.Start]))
		b.WriteString(plain)
		at = s.End
	}
	if at == 0 {
		return answer
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// mathEscape is a backslash in front of one of the two characters Markdown
// reads inside a word, which are the two a translator escapes inside a
// formula that does not need escaping.
var mathEscape = regexp.MustCompile(`\\([_*])`)

// unescaped is the formulas a model might have meant by this one, in the
// order they are worth trying.
//
// There are two tics and they come together. One is the backslash in front
// of an underscore. The other is the backslash in front of a backslash,
// which is how Markdown writes a literal one: the ResNet paper's
// "$s \in \{200, 400\}$" came back with every brace doubled. That one is
// worse than it looks, because a doubled backslash is a line break in TeX
// rather than nothing, so the formula does not merely fail the comparison,
// it renders wrongly if it ever gets through.
//
// A formula with a real line break in it is left alone by the guard in the
// caller. Collapsing the pair gives something the source does not have, so
// nothing matches and nothing is put back.
func unescaped(whole string) []string {
	var out []string
	if s := mathEscape.ReplaceAllString(whole, "$1"); s != whole {
		out = append(out, s)
	}
	if s := strings.ReplaceAll(whole, `\\`, `\`); s != whole {
		out = append(out, s)
		if t := mathEscape.ReplaceAllString(s, "$1"); t != s {
			out = append(out, t)
		}
	}
	return out
}

// Repair is the answer with the tics taken out of it that asking again does
// not cure: a web address turned into a link to itself, an address written
// back with Markdown escapes in it, and a formula written back the same
// way. Every one of them leaves the reader the text the paper printed,
// which is why each is a repair and not a relaxation of the check.
func Repair(source, answer string) string {
	return UnescapeMath(source, Unescape(source, Unlink(source, answer)))
}

// escape is a backslash in front of a piece of ASCII punctuation, which is
// everything CommonMark lets a backslash escape and nothing else. A
// backslash in front of a letter is a TeX command and is not touched.
var escape = regexp.MustCompile(`\\([[:punct:]])`)

// token is the end of the whitespace delimited word that starts at from.
func token(rs []rune, from int) int {
	for from < len(rs) && !unicode.IsSpace(rs[from]) {
		from++
	}
	return from
}

// halves cuts a link into what it shows and where it points.
func halves(s string) (text, target string, ok bool) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	cut := strings.Index(s, "](")
	if cut < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[1:cut]), strings.TrimSpace(s[cut+2 : len(s)-1]), true
}

// added is a link in the answer that was not in the passage, or the empty
// string when there is none.
//
// Counted rather than merely looked for, because a passage that already has
// a link in it, which the bibliography of one paper does, should still fail
// when a model adds a second one.
func added(source, answer string) string {
	was := map[string]int{}
	for _, l := range Links(source) {
		was[l]++
	}
	for _, l := range Links(answer) {
		if was[l] > 0 {
			was[l]--
			continue
		}
		return l
	}
	return ""
}

func inside(spans []Span, at int) bool {
	for _, s := range spans {
		if at >= s.Start && at < s.End {
			return true
		}
	}
	return false
}
