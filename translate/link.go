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
var link = regexp.MustCompile(`!?\[[^\]\n]*\]\((?:\\.|[^)\n\\])*\)`)

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
		if !ok || text == "" {
			continue
		}
		plain := selfLink(source, text, target)
		if plain == "" {
			continue
		}
		b.WriteString(answer[at:m[0]])
		b.WriteString(plain)
		at = m[1]
	}
	if at == 0 {
		return answer
	}
	b.WriteString(answer[at:])
	return b.String()
}

// selfLink is the address a link points at, when the link is that address
// and nothing more. It is the empty string for a link that says anything
// the target does not.
//
// The two halves are usually the same string, which is the plain shape of
// the tic. They also come back differing by a backslash: the AIMD paper's
// front page came back three times running as
// "[http://www.cse.wustl.edu/\~jain](http://www.cse.wustl.edu/~jain)",
// which is the linking tic and the escaping tic written on top of one
// another. Neither repair sees it on its own. Unlink wants the two halves
// to be equal and they are not, and Unescape never gets a turn because the
// answer is thrown away for having a link in it first.
//
// Taking the escapes off both halves says whether it is still the one
// address, and what goes back is whichever spelling the source has, so
// nothing is decided here about how the paper wrote it.
func selfLink(source, text, target string) string {
	if text == target {
		return text
	}
	if bare(text) != bare(target) {
		return ""
	}
	return pick(source, []string{target, text, bare(target)})
}

// bare is a string with its Markdown escapes taken off, as many rounds as
// it takes, for deciding whether two spellings are the same address.
func bare(s string) string {
	for i := 0; i < 3; i++ {
		next := escape.ReplaceAllString(s, "$1")
		if next == s {
			break
		}
		s = next
	}
	return s
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
		if s.Kind != URL && s.Kind != Email || s.Start < at {
			continue
		}
		end := token(rs, s.End)
		whole := string(rs[s.Start:end])
		if strings.Contains(source, whole) {
			continue
		}
		plain := pick(source, rounds(whole, escape))
		if plain == "" {
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
		plain := pick(source, unescaped(whole))
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

// letterMath is an inline formula that is one letter and nothing else.
// Those are the ones a translator drops the dollars from, because a single
// letter in the middle of a sentence reads as a word and not as mathematics.
// Anything longer comes back with its dollars on.
var letterMath = regexp.MustCompile(`^\$([A-Za-z])\$$`)

// Redollar puts the dollar signs back around a one letter formula the answer
// wrote bare.
//
// Shannon's appendix 6 has "and similarly when $q$ is varied. Hence the
// conditions for a minimum are" and the Vietnamese came back as "và tương tự
// khi q được biến thiên. Do đó, các điều kiện cho một cực tiểu là". The
// sentence is right and the letter is the letter the paper printed. What is
// missing is the two characters around it. Asking again does not cure it:
// the section came back the same way on every try of the run, and the paper
// is held back whole over them.
//
// Paragraph by paragraph, and only where the arithmetic comes out exactly.
// The English paragraph has to write the letter as mathematics and never as
// a bare letter, and the answer has to be short of that formula by the same
// number of bare copies of it as the answer has standing on their own
// outside every protected span. Both halves matter and neither is enough.
//
// A page is what showed that. Section 8 of Shannon came back with two bare
// q, one bare N and two bare H in its twenty third paragraph, two more bare
// N in its twenty seventh and one in its thirty first, and every one of them
// was a formula whose dollars had gone. Counted over the whole page instead
// of paragraph by paragraph, the same answer has four bare q against a
// shortfall of two, seven bare N against four and six bare H against two,
// because Shannon's prose also says "for each possible state i" and "the
// sequences of N symbols" in plain words. Whole page arithmetic never comes
// out and the repair never fires; paragraph arithmetic came out on the nose
// five times out of five.
//
// Standing on its own means with no letter, digit, dash, backslash, brace,
// underscore, caret, backtick, apostrophe or dollar sign either side of it.
// The dash is in that list for "$q$-ary", which is a compound word: wrapping
// its letter would be right and the guard is cheaper than being sure.
//
// A paragraph that dropped the formula and wrote no letter at all is still
// refused, and so is an answer whose paragraphs do not line up with the
// source's, which Verify has better words for.
func Redollar(source, answer string) string {
	if !strings.Contains(source, "$") {
		return answer
	}
	was, now := blocks(source), blocks(answer)
	if len(was) != len(now) {
		return answer
	}
	src, rs := []rune(source), []rune(answer)
	var b strings.Builder
	at := 0
	for i := range now {
		had := string(rs[now[i].start:now[i].end])
		fixed := redollarBlock(string(src[was[i].start:was[i].end]), had)
		if fixed == had {
			continue
		}
		b.WriteString(string(rs[at:now[i].start]))
		b.WriteString(fixed)
		at = now[i].end
	}
	if at == 0 {
		return answer
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// redollarBlock is one paragraph of the answer with its lost dollar signs
// put back.
func redollarBlock(source, answer string) string {
	letters, want := singles(Protect(source))
	if len(letters) == 0 {
		return answer
	}
	_, have := singles(Protect(answer))
	for _, l := range letters {
		n := want[l] - have[l]
		if n <= 0 || len(loose(source, l)) > 0 {
			continue
		}
		answer = redollar(answer, l, n)
	}
	return answer
}

// singles counts the one letter formulas of a body by their letter, and
// lists the letters in the order the body first writes them so that the
// repair does not depend on the order a map is walked in.
func singles(spans []Span) ([]rune, map[rune]int) {
	var letters []rune
	count := map[rune]int{}
	for _, s := range spans {
		if s.Kind != Math {
			continue
		}
		m := letterMath.FindStringSubmatch(s.Text)
		if m == nil {
			continue
		}
		l := rune(m[1][0])
		if count[l] == 0 {
			letters = append(letters, l)
		}
		count[l]++
	}
	return letters, count
}

// redollar wraps the bare copies of one letter, if there are exactly as many
// of them as the answer is short of.
func redollar(answer string, l rune, short int) string {
	at := loose(answer, l)
	if len(at) != short {
		return answer
	}
	rs := []rune(answer)
	var b strings.Builder
	last := 0
	for _, i := range at {
		b.WriteString(string(rs[last:i]))
		b.WriteString("$" + string(l) + "$")
		last = i + 1
	}
	b.WriteString(string(rs[last:]))
	return b.String()
}

// loose lists where a body has the letter standing on its own, outside every
// protected span.
func loose(body string, l rune) []int {
	spans := Protect(body)
	rs := []rune(body)
	var out []int
	for i, r := range rs {
		if r != l {
			continue
		}
		if i > 0 && joined(rs[i-1]) || i+1 < len(rs) && joined(rs[i+1]) {
			continue
		}
		if inside(spans, i) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// joined is a character that makes the letter next to it part of something
// larger: a word, a number, a formula, a listing, a TeX command, a subscript
// or a compound.
func joined(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("$`\\_^{}-'", r)
}

// Unrename puts back the name of something the paper names inside a formula
// and the answer translated.
//
// A \text that carries an index or an argument is a name and not prose, and
// Applied has the reasoning and the measurements for that. The prompt does
// not say so in as many words, and where a paper names something with an
// ordinary word the translator translates it. Goldwasser calls the jth of m
// pairs $\text{pair}_j$, and the Vietnamese came back with
// $(v_j)^2 w^{-1} \mod x \in \text{cặp}_j$. The same paragraph writes the
// same object as a bare pair$_j$ four more times, in prose, where nothing
// asked the translator to rename it and it did not, so the page would have
// called one thing two things. The section came back that way on every try
// of the run and the paper is held back whole over it.
//
// Paragraph by paragraph, and only where the pairing is not in doubt. A
// formula of the answer is put back when the source does not have it
// anywhere in the paragraph, exactly one formula of the source is the same
// but for the words inside its \text commands, and that one is itself
// nowhere in the answer. Two candidates and it is left alone, because
// picking between them would be guessing.
//
// What goes back is the source formula, character for character, so the
// reader gets what the paper printed and never something this invented. And
// an answer that passes the comparison is never touched at all, because the
// only formulas looked at are the ones Compare would report as additions.
func Unrename(source, answer string) string {
	if !textCommand.MatchString(answer) {
		return answer
	}
	was, now := blocks(source), blocks(answer)
	if len(was) != len(now) {
		return answer
	}
	src, rs := []rune(source), []rune(answer)
	var b strings.Builder
	at := 0
	for i := range now {
		had := string(rs[now[i].start:now[i].end])
		fixed := unrenameBlock(string(src[was[i].start:was[i].end]), had)
		if fixed == had {
			continue
		}
		b.WriteString(string(rs[at:now[i].start]))
		b.WriteString(fixed)
		at = now[i].end
	}
	if at == 0 {
		return answer
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// unrenameBlock is one paragraph of the answer with its renamed formulas put
// back.
func unrenameBlock(source, answer string) string {
	want, got := Protect(source), Protect(answer)
	rs := []rune(answer)
	var b strings.Builder
	at := 0
	for _, s := range got {
		if s.Kind != Math || holds(want, s) {
			continue
		}
		t := only(want, got, s)
		if t == "" {
			continue
		}
		b.WriteString(string(rs[at:s.Start]))
		b.WriteString(t)
		at = s.End
	}
	if at == 0 {
		return answer
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

// only is the one formula of the source that this formula of the answer can
// have been, and the empty string when there is no such formula or more than
// one of them. A formula the answer still has somewhere is not a candidate,
// because it came through and is not the one that was renamed.
func only(want, got []Span, s Span) string {
	out := ""
	for _, t := range want {
		if t.Kind != Math || holds(got, t) || !alike(t, s) {
			continue
		}
		if out != "" && out != t.Text {
			return ""
		}
		out = t.Text
	}
	return out
}

// Repair is the answer with the tics taken out of it that asking again does
// not cure: a web address turned into a link to itself, an address written
// back with Markdown escapes in it, a formula written back the same way, a
// name the paper prints inside a formula written back in another language,
// and a one letter formula written back without its dollar signs. Every one
// of them leaves the reader the text the paper printed, which is why each is
// a repair and not a relaxation of the check.
func Repair(source, answer string) string {
	return Redollar(source, Unrename(source, UnescapeMath(source, Unescape(source, Unlink(source, answer)))))
}

// escape is a backslash in front of a piece of ASCII punctuation, which is
// everything CommonMark lets a backslash escape and nothing else. A
// backslash in front of a letter is a TeX command and is not touched.
var escape = regexp.MustCompile(`\\([[:punct:]])`)

// pick is the first of the candidates the source has, or the empty string
// when it has none of them. A repair that cannot find what it is putting
// back does not happen, and Verify refuses the answer as it did before.
func pick(source string, candidates []string) string {
	for _, c := range candidates {
		if strings.Contains(source, c) {
			return c
		}
	}
	return ""
}

// rounds is the token with the escapes taken out of it once, then twice,
// then three times.
//
// Once is not always enough. A backslash is punctuation, so a model that
// escapes its own escape writes two of them, and taking one round off
// "\\~jain" leaves "\~jain", which is neither what the paper printed nor
// what the model meant. Both addresses that forced this were written that
// way by a run in September: the tilde in the AIMD paper's address and the
// parentheses in the Milner citation.
//
// Three rounds because two is the deepest anything has come back, and a
// bound is cheaper than reasoning about a pattern that rewrites its own
// output.
func rounds(whole string, re *regexp.Regexp) []string {
	var out []string
	s := whole
	for i := 0; i < 3; i++ {
		next := re.ReplaceAllString(s, "$1")
		if next == s {
			break
		}
		out = append(out, next)
		s = next
	}
	return out
}

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
