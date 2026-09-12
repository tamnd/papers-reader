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
// Order is part of the answer and not a convenience. The comparison is
// positional: two bodies whose spans are the same set but in a different
// order are two different documents, and a translator that moved a formula
// from one sentence to the next has changed what the paper says.
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
// Nothing comes back for an answer that is right, and the first difference is
// enough to throw the answer away: the caller asks again rather than trying
// to repair it. A translation with a quietly renamed variable is worse than
// no translation, because nothing downstream will ever catch it.
func Compare(source, answer string) []Difference {
	want, got := Protect(source), Protect(answer)
	var out []Difference
	for i := range want {
		if i >= len(got) {
			out = append(out, Difference{At: i + 1, Want: want[i],
				Why: fmt.Sprintf("the source has %s %q here and the answer has run out of spans", want[i].Kind, short(want[i].Text))})
			break
		}
		if want[i].Kind != got[i].Kind || !same(want[i], got[i]) {
			out = append(out, Difference{At: i + 1, Want: want[i], Got: got[i]})
		}
	}
	if len(got) > len(want) {
		at := len(want)
		out = append(out, Difference{At: at + 1, Got: got[at],
			Why: fmt.Sprintf("the answer has %d spans and the source has %d, the first extra being %s %q",
				len(got), len(want), got[at].Kind, short(got[at].Text))})
	}
	return out
}

// same compares two spans of the same kind.
//
// Byte for byte, with one exception. Inside mathematics a word set with
// \text{...} is prose put in a formula because TeX has no other way of
// writing a word in one, and "$(\text{not } A) \text{ or } B$" has two words
// in it that become Vietnamese. So the argument of a \text is masked before
// the comparison, unless the name inside it is one of the upright names that
// is not prose.
func same(want, got Span) bool {
	if want.Kind != Math {
		return want.Text == got.Text
	}
	return maskText(want.Text) == maskText(got.Text)
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
