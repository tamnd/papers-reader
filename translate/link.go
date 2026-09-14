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
