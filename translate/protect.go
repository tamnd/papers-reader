package translate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/mathtex"
)

// A Kind is what a protected span is.
type Kind string

const (
	// Math is a `$...$` or `$$...$$` span, delimiters included.
	Math Kind = "mathematics"
	// Code is a fenced block, both fences included.
	Code Kind = "code"
	// Inline is a backticked span in the middle of a sentence.
	//
	// The spec names four kinds and this is a fifth, because the corpus has
	// nine of them and every one would be ruined by a translation. They are
	// all in the BERT paper's account of the masked language model, where the
	// example sentence "my dog is hairy" is shown four times with a different
	// word masked out. The sentence is the input the paper is demonstrating,
	// not a sentence the paper is making, and a Vietnamese reader needs to
	// see the English the model saw.
	Inline Kind = "inline code"
	// Citation is a reference into the bibliography, either the number the
	// paper printed or the corpus identifier papers refs resolved it to.
	Citation Kind = "citation"
	// Attribute is the block papers tags writes on an anchored line, which
	// holds the anchor, the class and the permanent tag.
	Attribute Kind = "attribute"
	// URL is a bare web address in the prose.
	//
	// It is protected because it is the one piece of a paper that has to be
	// typed in, and because a translator that touches it does not mistype it,
	// it decorates it. All three translations of the GAN paper turned the
	// footnote "available at http://www.github.com/goodfeli/adversarial" into
	// a Markdown link with the address as both the text and the target, which
	// is markup the English does not have, in a corpus whose vocabulary has
	// no links in it.
	URL Kind = "url"
)

// A Span is one stretch of a body that a translation has to reproduce byte
// for byte.
//
// Text is the span as written, delimiters included, because the delimiters
// are as much a part of what must survive as what is between them: an answer
// that turns a display into an inline span has changed the document even
// though the mathematics is the same.
type Span struct {
	Kind Kind
	Text string
	// Start and End are where the span sits in the body, counted in runes,
	// the way mathtex counts. Nothing here needs them to put a span back, but
	// a span written twice on a line cannot be found again by searching for
	// its text, and a caller that wants to show a reader where the difference
	// is has to be able to say where.
	Start, End int
}

// Protect finds every protected span of a body, in the order they appear.
//
// The order is reported because a caller wants to show a reader where in a
// body a span sits, not because the comparison depends on it. Compare says
// what it does and does not make of the order, and why.
//
// A span inside another is not reported twice. A fence holds dollar signs
// that are not mathematics and citations that are not citations, and the
// fence is protected whole, so nothing inside one is looked at again.
func Protect(body string) []Span {
	var out []Span
	rs := []rune(body)
	taken := make([]bool, len(rs)+1)
	add := func(kind Kind, start, end int) {
		for i := start; i < end; i++ {
			if taken[i] {
				return
			}
		}
		for i := start; i < end; i++ {
			taken[i] = true
		}
		out = append(out, Span{Kind: kind, Text: string(rs[start:end]), Start: start, End: end})
	}

	// The fences first, so that everything they hold is already taken.
	for _, b := range fences(body) {
		add(Code, b.start, b.end)
	}
	// Then the inline code, which can hold anything at all, before the
	// mathematics reads a dollar sign out of one.
	for _, m := range inline.FindAllStringIndex(body, -1) {
		add(Inline, runeIndex(body, m[0]), runeIndex(body, m[1]))
	}
	// Then the mathematics, which can hold a bracket that reads as a
	// citation: $[0,1]$ is an interval.
	spans, _ := mathtex.Split(body)
	for _, s := range spans {
		start, end := delimited(rs, s)
		add(Math, start, end)
	}
	for _, m := range attribute.FindAllStringIndex(body, -1) {
		add(Attribute, runeIndex(body, m[0]), runeIndex(body, m[1]))
	}
	for _, m := range citation.FindAllStringIndex(body, -1) {
		add(Citation, runeIndex(body, m[0]), runeIndex(body, m[1]))
	}
	for _, m := range address.FindAllStringIndex(body, -1) {
		add(URL, runeIndex(body, m[0]), runeIndex(body, m[1]))
	}
	sortByStart(out)
	return out
}

// delimited widens a math span to take in its own dollar signs, which
// mathtex.Split reports without.
func delimited(rs []rune, s mathtex.Span) (start, end int) {
	n := 1
	if s.Display {
		n = 2
	}
	start, end = s.Start-n, s.End+n
	if start < 0 {
		start = 0
	}
	if end > len(rs) {
		end = len(rs)
	}
	return start, end
}

// number is one entry of a numeric citation, which is a number or a run of
// them written with a dash. Bitcoin's "[2-5]" is four references and not one.
const number = `[0-9]+(?:[-\x{2013}][0-9]+)?`

var (
	// attribute is the block papers tags writes: an anchor, then classes and
	// a tag in any order. It is matched before a citation because it holds no
	// brackets but does hold a hash, and matching it first is what keeps the
	// two sets of rules from having to know about each other.
	attribute = regexp.MustCompile(`\{#[A-Za-z0-9][-A-Za-z0-9_.]*(?:[ \t]+[^}\n]*)?\}`)

	// inline is a backticked span on one line. CommonMark lets one run over a
	// line break and no translator ever writes one that way, so the line is
	// the boundary and a stray backtick costs at most its own line. A fence
	// is found before this runs and is already taken, so the three backticks
	// that open one are never read as a span of their own.
	inline = regexp.MustCompile("``[^`\n]+``|`[^`\n]+`")

	// citation is a pointer at something else in the document: the corpus
	// identifier papers refs resolved a reference to, a footnote marker, or
	// the number the paper printed, alone or several in one pair of brackets
	// or as a range. All four are numbering that only means anything if it
	// survives, and a translation that renumbers a footnote has broken the
	// page as surely as one that renames a variable.
	//
	// A bracket that holds anything else is prose and is translated: "[sic]"
	// is a word, "[CLS]" is a token of the BERT paper's input and "[.5, .95]"
	// is the interval the COCO metric averages over.
	//
	// The trailing locator is for the one reference in the corpus written
	// "[16, Figure 3(e)]". It is protected whole, page word and all. The word
	// would read better translated, but a rule that has to decide where a
	// citation stops being a citation is a rule that will one day decide
	// wrong about the number, and there is one of these.
	citation = regexp.MustCompile(`\[\[[a-z0-9][-a-z0-9]*\]\]` +
		`|\[\^[A-Za-z0-9][-A-Za-z0-9_]*\]` +
		`|\[` + number + `(?:[,;][ \t]*` + number + `)*(?:,[ \t]*[A-Za-z][^]\n]*)?\]`)

	// address is a bare web address. It stops at whitespace and then gives
	// back the punctuation a sentence put after it, because a paper writes
	// "available at http://example.org/x." and the full stop is the
	// sentence's rather than the address's.
	address = regexp.MustCompile(`\b(?:https?://|www\.)[^\s<>()\[\]"]*[^\s<>()\[\]".,;:!?]`)
)

// A Difference is one way a translation's protected spans are not the
// source's. It names the position rather than only the text, because a paper
// writes the same formula twice and "$n$ is missing" is not something anybody
// can act on.
type Difference struct {
	// At is the position in the source's list of spans, counting from one.
	At   int
	Want Span
	Got  Span
	Why  string
}

func (d Difference) String() string {
	switch {
	case d.Why != "":
		return fmt.Sprintf("span %d: %s", d.At, d.Why)
	default:
		return fmt.Sprintf("span %d: the source has %s %q and the answer has %s %q",
			d.At, d.Want.Kind, short(d.Want.Text), d.Got.Kind, short(d.Got.Text))
	}
}

// Compare says how the protected spans of an answer differ from the source's.
//
// Paragraph by paragraph, and inside a paragraph as a bag rather than as a
// list. A paragraph of the answer has to hold the same spans as the same
// paragraph of the source, each one the same number of times, and it may
// hold them in any order.
//
// The order used to matter and it cost the corpus its first Chinese and its
// first Japanese. Both runs stopped dead on their third attempt at the third
// section of the GAN paper: "the source has mathematics $p_g$ and the answer
// has mathematics $\boldsymbol{x}$" for the Chinese, the same shape of thing
// for the Japanese. Neither answer was wrong. English writes "the
// generator's distribution $p_g$ over data $\boldsymbol{x}$" and Chinese
// writes the modifier in front of what it modifies, so the two formulas come
// out the other way round, and any translation into either language that did
// not swap them would be the broken one. A rule that refuses correct work
// three times and then gives up is not a strict rule, it is a wrong one.
//
// What is left is still the check worth having. A dropped formula, an added
// one, a renamed variable, a renumbered citation and a footnote marker that
// moved to another paragraph are all still caught, and those are what a
// model that has misread a passage actually does. What is no longer caught
// is two spans of one paragraph swapped with each other, and there is no way
// to catch that and keep Chinese: a citation moves with the clause it is
// attached to exactly as a formula does.
//
// Nothing comes back for an answer that is right, and one difference is
// enough to throw the answer away: the caller asks again rather than trying
// to repair it.
func Compare(source, answer string) []Difference {
	want, got := paragraphs(source), paragraphs(answer)
	if len(want) != len(got) {
		// The shapes do not line up, so there is no paragraph to compare
		// within and the whole passage is the bag. Verify reports the block
		// count itself and says it better than this could.
		return compare(Protect(source), Protect(answer), 0)
	}
	var out []Difference
	at := 0
	for i := range want {
		a, b := Protect(want[i]), Protect(got[i])
		out = append(out, compare(a, b, at)...)
		at += len(a)
	}
	return out
}

// paragraphs cuts a body into the texts of its blocks.
func paragraphs(body string) []string {
	rs := []rune(body)
	var out []string
	for _, b := range blocks(body) {
		out = append(out, string(rs[b.start:b.end]))
	}
	return out
}

// compare matches two paragraphs' spans up, and says what is left over.
//
// base is how many spans of the source came before this paragraph, so that
// the position in a difference counts through the whole passage and a reader
// of the message can find the span being complained about.
func compare(want, got []Span, base int) []Difference {
	used := make([]bool, len(got))
	var missing, extra []int
	for i := range want {
		found := false
		for j := range got {
			if !used[j] && want[i].Kind == got[j].Kind && same(want[i], got[j]) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			missing = append(missing, i)
		}
	}
	for j := range got {
		if !used[j] {
			extra = append(extra, j)
		}
	}

	var out []Difference
	// A span that went missing and one that turned up in its place are one
	// change and not two: a renamed variable is the commonest thing this
	// finds, and reporting it as a loss and a gain describes it twice and
	// names neither. They pair off in the order they were written, which for
	// the one span in a paragraph that a model got wrong is the right pair.
	for k, i := range missing {
		if k >= len(extra) {
			out = append(out, Difference{At: base + i + 1, Want: want[i],
				Why: fmt.Sprintf("the source has %s %q and the answer has nothing like it",
					want[i].Kind, short(want[i].Text))})
			continue
		}
		j := extra[k]
		out = append(out, Difference{At: base + i + 1, Want: want[i], Got: got[j]})
	}
	for k := len(missing); k < len(extra); k++ {
		j := extra[k]
		out = append(out, Difference{At: base + len(want) + 1, Got: got[j],
			Why: fmt.Sprintf("the answer has %s %q and the source has nothing like it",
				got[j].Kind, short(got[j].Text))})
	}
	return out
}

// same compares two spans of the same kind.
//
// Byte for byte, with two exceptions, both of them inside mathematics. A
// word set with \text{...} is prose put in a formula because TeX has no
// other way of writing a word in one, and "$(\text{not } A) \text{ or } B$"
// has two words in it that become Vietnamese. So the argument of a \text is
// masked before the comparison, unless the name inside it is one of the
// upright names that is not prose. And the whitespace of a formula is
// normalised, because TeX ignores it.
func same(want, got Span) bool {
	if want.Kind != Math {
		return want.Text == got.Text
	}
	return spacing(maskText(want.Text)) == spacing(maskText(got.Text))
}

// blanks is a run of whitespace.
var blanks = regexp.MustCompile(`\s+`)

// spacing normalises the whitespace of a formula.
//
// TeX ignores it: "$p_g$" and "$ p_g$" set the same thing, so a run that
// refuses the second is throwing away an answer that copied the formula
// correctly and then breathed on it. That is not a hypothetical. The first
// Japanese of the GAN paper died on it. Six answers in a row were refused
// because one of them had written "$ p_g$" for "$p_g$", and the sixth
// refusal gave up on the file and stopped the run, four files short.
//
// Runs of whitespace collapse to one space, and the space beside a
// delimiter goes. What does not go is the single space between a control
// word and what follows it: "\alpha x" and "\alphax" are not the same
// formula, and only one of them is a formula at all.
func spacing(s string) string {
	s = blanks.ReplaceAllString(s, " ")
	open := len(s) - len(strings.TrimLeft(s, "$"))
	shut := len(s) - len(strings.TrimRight(s, "$"))
	if open+shut >= len(s) {
		return s
	}
	return s[:open] + strings.TrimSpace(s[open:len(s)-shut]) + s[len(s)-shut:]
}

// textCommand is a TeX command whose argument is set upright, which is how a
// paper writes a word inside a formula.
var textCommand = regexp.MustCompile(`\\(?:text|textit|textbf|textrm|textnormal|mbox)\{`)

// maskText replaces the argument of every \text with a marker, so that two
// formulas that differ only in the words inside their \text are the same
// formula. The braces themselves stay, because moving one is a change to the
// mathematics and not to the prose inside it.
func maskText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		loc := textCommand.FindStringIndex(s[i:])
		if loc == nil {
			b.WriteString(s[i:])
			break
		}
		open := i + loc[1]
		b.WriteString(s[i:open])
		shut := closingBrace(s, open)
		if shut < 0 {
			b.WriteString(s[open:])
			break
		}
		arg := s[open:shut]
		// A \text whose argument holds mathematics of its own is compared
		// whole. mathtex reads \text{$\Gamma$ correspondence} as one span on
		// purpose, and masking the argument would let a translator rewrite the
		// \Gamma inside it with nothing to catch that.
		if strings.Contains(arg, "$") || Upright[strings.TrimSpace(arg)] {
			b.WriteString(arg)
		} else {
			// The spaces at the edges stay. A "\text{not }" carries the space
			// that separates it from the symbol after it, and a translation
			// that drops it runs two things together.
			b.WriteString(edgeSpace(arg, true) + "…" + edgeSpace(arg, false))
		}
		b.WriteString("}")
		i = shut + 1
	}
	return b.String()
}

// Upright is the arguments of a \text that are not prose and do not move.
//
// It is a list and not a rule because there is no rule. "\text{if}" is prose,
// "\text{iff}" is arguable and "\text{argmax}" is an operator that happens to
// be spelled with letters. What is here is what a paper sets upright because
// TeX would otherwise set it in italics and make it look like a product of
// variables.
var Upright = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range strings.Fields(`
		argmax argmin arg max min sup inf lim limsup liminf
		softmax sigmoid tanh relu ReLU GELU LSTM RNN CNN MLP
		diag rank trace tr det dim deg card Card supp Supp
		mod div gcd lcm lg ln log exp sin cos tan
		data model real fake true false
		Attention MultiHead FFN Concat Softmax Sigmoid
		E Var Cov KL JSD
		resp. cf. i.e. e.g. etc. s.t. w.r.t. iff
	`) {
		m[s] = true
	}
	return m
}()

func edgeSpace(s string, lead bool) string {
	if lead {
		return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	}
	return s[len(strings.TrimRight(s, " \t")):]
}

// closingBrace is the index of the brace that closes the group opened just
// before open, and -1 for a group that never closes. A brace the paper escaped
// is not a brace.
func closingBrace(s string, open int) int {
	depth := 1
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// fence is a fenced block with both of its fences, in rune offsets, which is
// not what code.Blocks reports: it reports the text between them in lines.
type fence struct{ start, end int }

func fences(body string) []fence {
	blocks, _ := code.Blocks(body)
	if len(blocks) == 0 {
		return nil
	}
	rs := []rune(body)
	// The offset of the first rune of each line, counting from one at index
	// one so that a Block's Line can be used without arithmetic.
	offsets := []int{0, 0}
	at := 0
	for _, line := range strings.Split(body, "\n") {
		at += len([]rune(line)) + 1
		offsets = append(offsets, at)
	}
	var out []fence
	for _, b := range blocks {
		last := b.End
		if last == 0 {
			// An unclosed fence runs to the end of the body, which is what
			// code.Blocks says and what every renderer does.
			last = len(offsets) - 2
		}
		if b.Line >= len(offsets) || last+1 >= len(offsets) {
			continue
		}
		end := offsets[last+1] - 1
		if end > len(rs) {
			end = len(rs)
		}
		// A closed fence already ends on its last backtick, because the
		// offset of the line after it is one past the newline. An unclosed
		// one runs to the end of the body and picks up the newline that ends
		// the file, which is not part of the listing.
		for end > offsets[b.Line] && rs[end-1] == '\n' {
			end--
		}
		out = append(out, fence{start: offsets[b.Line], end: end})
	}
	return out
}

// Prose is the body with every protected span taken out, which is the part
// of a paper that is written in a language.
//
// Each span leaves a newline behind rather than nothing, so that the words
// on either side of a formula do not run into one another and read as a
// phrase. This is what the glossary extractor counts, and counting the
// mathematics with it would fill the candidate list with variable names.
func Prose(body string) string {
	rs := []rune(body)
	var b strings.Builder
	at := 0
	for _, s := range Protect(body) {
		if s.Start > at {
			b.WriteString(string(rs[at:s.Start]))
		}
		b.WriteByte('\n')
		at = s.End
	}
	if at < len(rs) {
		b.WriteString(string(rs[at:]))
	}
	return b.String()
}

func runeIndex(s string, byteAt int) int { return len([]rune(s[:byteAt])) }

func sortByStart(spans []Span) {
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].Start < spans[j-1].Start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
}

// short cuts a span down to something that fits on a line of a report.
func short(s string) string {
	const most = 60
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) <= most {
		return s
	}
	return string([]rune(s)[:most]) + "…"
}
