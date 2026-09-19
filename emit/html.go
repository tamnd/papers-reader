package emit

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/mathtex"
	"github.com/tamnd/papers-reader/tags"
)

// A pager turns one paper's Markdown into the blocks of one page.
//
// It is the third renderer over this corpus, after the LaTeX of the books
// and the XHTML of the EPUBs, and it is a third renderer rather than a flag
// on one of them for the same reason those two are separate: they differ in
// every case of the switch. What the three share is where a block begins and
// what an inline span is, and that is package markdown.
//
// Nothing a model wrote reaches the output unescaped. Every run of text goes
// through html.EscapeString, and the only markup in the result is markup
// this file wrote, plus what KaTeX made of a formula. That is what the
// allowlist in sanitise.go checks, and it checks it rather than trusting it
// because the claim is worth a test.
type pager struct {
	paper   string
	katex   *katex.Renderer
	figures map[string]figures.Figure
	// known is every paper identifier the corpus has, for the links the
	// corpus adds that the paper did not have.
	known map[string]bool
	// cites is the citation keys this paper's bibliography holds, and is nil
	// where nobody has parsed the bibliography. Nil and empty are not the
	// same: a paper whose references nobody has read cites nothing that can
	// be checked, and a paper whose references were read and hold no key 12
	// has a citation pointing at nothing.
	cites map[string]bool
	// byPaper is the citation key each corpus paper is listed under, so a
	// [[link]] in the prose can jump to the bibliography entry the reader
	// would otherwise have to find.
	byPaper map[string]string
	notes   map[string]string

	refused  map[string]bool
	orphans  map[string]bool
	unlinked map[string]bool
	dangling map[string]bool
	gaps     map[string]bool

	held []string
}

// blocks cuts a body into blocks and renders each one, numbering them as it
// goes. The number is the alignment key and it is the position in this
// slice, so nothing here may drop a block: a block that could not be
// rendered comes back as prose rather than as nothing.
func (p *pager) blocks(body string) []Block {
	out := []Block{}
	for _, b := range markdown.Blocks(body) {
		blk := p.block(b)
		blk.I = len(out)
		out = append(out, blk)
	}
	return out
}

func (p *pager) block(b string) Block {
	text, attr, labelled := markdown.TakeAttr(b)
	blk := Block{}
	if labelled {
		blk.Anchor, blk.Tag = attr.Anchor, string(attr.Tag)
	}
	blk.plain, blk.symbols = strip(text)
	switch {
	case markdown.IsFenced(text):
		blk.Kind, blk.Syntax, blk.Text = "code", markdown.Fence(text), markdown.Fenced(text)
		blk.plain, blk.symbols = "", blk.Text
	case markdown.IsHeading(text):
		depth, title, _ := markdown.Heading(text)
		blk.Kind, blk.Level, blk.HTML = "heading", depth, p.inline(title)
	case markdown.IsTable(text):
		blk.Kind, blk.HTML = "table", p.table(text)
	case markdown.IsDisplay(text):
		tex := markdown.TeX(text)
		blk.Kind, blk.TeX = "math", tex
		blk.Number, blk.HTML = markdown.Tag(tex), p.math(tex, true)
		blk.plain, blk.symbols = "", tex
	case labelled && markdown.Has(attr.Classes, "figure"):
		return p.figure(text, attr, blk)
	case markdown.IsList(text):
		blk.Kind, blk.HTML = "list", p.list(text)
	default:
		blk.Kind, blk.HTML = "p", p.inline(text)
	}
	return blk
}

// figure puts a caption back together with the picture it belongs to.
//
// The corpus writes a figure as a caption with an attribute block on it and
// nothing else, because the picture is a file beside the content rather than
// an image tag in it. A caption whose picture the manifest does not have
// comes back as the caption on its own, set as prose, so that the block is
// still there and the indices of everything after it are unmoved.
//
// That is not a fault of the page. A paper whose captions are tagged and
// whose pictures nobody has cropped yet is a paper part way through the
// figures pass, and rule F09 is the one that says so, once per paper rather
// than once per paper per language. Nothing broken reaches the reader: the
// caption is a paragraph and the paper reads as it would in a journal that
// printed the plates elsewhere.
func (p *pager) figure(text string, attr tags.Attr, blk Block) Block {
	f, ok := p.figures[attr.Anchor]
	if !ok {
		blk.Kind, blk.HTML = "p", p.inline(text)
		return blk
	}
	number, caption := markdown.CaptionNumber(text, f.Number)
	if number == "" {
		number = f.Number
	}
	blk.Kind, blk.Number = "figure", number
	blk.Src, blk.W, blk.H = FigureSrc(p.paper, f.Name()), f.Width, f.Height
	blk.CaptionHTML = p.inline(caption)
	return blk
}

func (p *pager) list(text string) string {
	tag := "ul"
	if markdown.Ordered(text) {
		tag = "ol"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>", tag)
	for _, item := range markdown.Items(text) {
		fmt.Fprintf(&b, "<li>%s</li>", p.inline(item))
	}
	fmt.Fprintf(&b, "</%s>", tag)
	return b.String()
}

func (p *pager) table(text string) string {
	head, body := markdown.Rows(text)
	var b strings.Builder
	b.WriteString("<table><thead><tr>")
	for _, c := range head {
		fmt.Fprintf(&b, "<th>%s</th>", p.inline(c))
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range body {
		b.WriteString("<tr>")
		for _, c := range row {
			fmt.Fprintf(&b, "<td>%s</td>", p.inline(c))
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
	return b.String()
}

// inline renders one run of text: hold what must not be escaped, escape,
// put it back. The same order as the other two renderers, and the order is
// the point. Escaping first would escape the markup this file is about to
// write, and holding after escaping would hold text that had already been
// mangled.
func (p *pager) inline(text string) string {
	p.held = p.held[:0]
	s := p.hold(text)
	s = html.EscapeString(s)
	return p.release(s)
}

func (p *pager) hold(s string) string {
	s = p.holdMath(s)
	s = p.holdCode(s)
	s = p.holdNotes(s)
	s = p.holdPapers(s)
	s = p.holdCites(s)
	s = p.holdEmphasis(s)
	return s
}

// mark is the marker a held span leaves behind. NUL because no corpus file
// has one, the escaper does not touch it, and it cannot be produced by any
// of the substitutions, so a marker can never be made by accident out of
// text.
func (p *pager) mark(markup string) string {
	p.held = append(p.held, markup)
	return "\x00" + strconv.Itoa(len(p.held)-1) + "\x00"
}

var marker = regexp.MustCompile("\x00([0-9]+)\x00")

// release puts the held spans back, repeatedly, because a span can hold a
// marker: a footnote holds its own text, which has its own mathematics in
// it.
func (p *pager) release(s string) string {
	for range 8 {
		if !strings.Contains(s, "\x00") {
			break
		}
		s = marker.ReplaceAllStringFunc(s, func(m string) string {
			n, err := strconv.Atoi(strings.Trim(m, "\x00"))
			if err != nil || n >= len(p.held) {
				return ""
			}
			return p.held[n]
		})
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// math renders one span at build time, and falls back to the TeX in a code
// face when KaTeX refuses it.
//
// At build time and not in the browser. KaTeX in the page would be three
// hundred kilobytes of JavaScript and a visible reflow on every paper,
// against markup that is already correct when the HTML arrives. The reader
// still needs the stylesheet and the fonts, which is a fraction of that and
// is cacheable across the whole site.
//
// Extraction already renders every span it writes, which is audit rule M04,
// so a refusal here is a span that got into the corpus before that rule did
// or one a translation broke. The page is still built and the fault is
// recorded, because a paper with one unreadable formula is worth reading and
// a build that refused it is not. Rule P01 is that list.
func (p *pager) math(tex string, display bool) string {
	out, err := p.katex.Render(tex, display)
	if err != nil {
		p.note(&p.refused, oneLine(tex)+": "+err.Error())
		d := "$"
		if display {
			d = "$$"
		}
		return `<code class="tex">` + html.EscapeString(d+tex+d) + `</code>`
	}
	return out
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func (p *pager) holdMath(s string) string {
	spans, unclosed := mathtex.Split(s)
	if unclosed != nil || len(spans) == 0 {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		delim := 1
		if sp.Display {
			delim = 2
		}
		b.WriteString(string(rs[at : sp.Start-delim]))
		b.WriteString(p.mark(p.math(sp.Text, sp.Display)))
		at = sp.End + delim
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

func (p *pager) holdCode(s string) string {
	return markdown.Code.ReplaceAllStringFunc(s, func(m string) string {
		return p.mark("<code>" + html.EscapeString(strings.Trim(m, "`")) + "</code>")
	})
}

// holdNotes turns a marker into a link to the note at the foot of the page.
//
// A marker with no definition anywhere in the paper is set as the bare
// number and recorded, because a superscript with nothing behind it is
// better than a link to nowhere. Rule P02 is that list.
func (p *pager) holdNotes(s string) string {
	s = markdown.LooseNote.ReplaceAllString(s, "$1")
	return markdown.Note.ReplaceAllStringFunc(s, func(m string) string {
		key := markdown.Note.FindStringSubmatch(m)[1]
		if _, ok := p.notes[key]; !ok {
			p.note(&p.orphans, key)
			return p.mark("<sup>" + html.EscapeString(key) + "</sup>")
		}
		return p.mark(fmt.Sprintf(`<sup><a class="noteref" href="#fn-%s">%s</a></sup>`,
			html.EscapeString(key), html.EscapeString(key)))
	})
}

// holdPapers sets a citation of another paper of this corpus.
//
// Three outcomes and they are different things. Where this paper's own
// bibliography lists it, the citation is set as the number the paper printed
// and links to that entry, so it reads the way the page it came from read.
// Where the bibliography does not list it but the corpus has the paper, it
// links straight to the other paper, which is the one piece of navigation
// the corpus adds that the paper did not have. Where the corpus does not
// have the paper at all, the identifier is set as plain text and recorded:
// the prose names a paper that is not here, and that is a fault in the
// content rather than something to hide.
func (p *pager) holdPapers(s string) string {
	return markdown.PaperCite.ReplaceAllStringFunc(s, func(m string) string {
		id := markdown.PaperCite.FindStringSubmatch(m)[1]
		if key, ok := p.byPaper[id]; ok {
			return p.mark(fmt.Sprintf(`[<a class="cite" href="#ref-%s" data-paper="%s">%s</a>]`,
				html.EscapeString(key), html.EscapeString(id), html.EscapeString(key)))
		}
		if p.known[id] {
			return p.mark(fmt.Sprintf(`<a class="paper" href="/p/%s">%s</a>`,
				html.EscapeString(id), html.EscapeString(id)))
		}
		p.note(&p.unlinked, id)
		return p.mark(`<span class="paper">` + html.EscapeString(id) + `</span>`)
	})
}

// holdCites turns [12] and [3, 7-9] into links to the bibliography.
//
// Only the digits inside are linked, one by one, so that [3, 7] is two links
// and the comma between them is punctuation. A range is left as printed and
// its endpoints linked, because expanding 7-9 into three links would put
// text on the page the paper did not print.
//
// Nothing is linked at all where nobody has parsed this paper's
// bibliography. That is not the same as a bibliography that was parsed and
// has no entry 12: the first is a paper waiting for papers refs and the
// second is a citation pointing at nothing, and only the second is a fault.
func (p *pager) holdCites(s string) string {
	if p.cites == nil {
		return s
	}
	return markdown.ReplaceCites(s, func(inner string) string {
		var b strings.Builder
		b.WriteString("[")
		at := 0
		for _, loc := range markdown.Digits.FindAllStringIndex(inner, -1) {
			b.WriteString(html.EscapeString(inner[at:loc[0]]))
			n := inner[loc[0]:loc[1]]
			if p.cites[n] {
				fmt.Fprintf(&b, `<a class="cite" href="#ref-%s">%s</a>`, n, n)
			} else {
				if !p.gaps[n] {
					p.note(&p.dangling, n)
				}
				b.WriteString(n)
			}
			at = loc[1]
		}
		b.WriteString(html.EscapeString(inner[at:]))
		b.WriteString("]")
		return p.mark(b.String())
	})
}

func (p *pager) holdEmphasis(s string) string {
	s = markdown.Strong.ReplaceAllStringFunc(s, func(m string) string {
		return p.mark("<strong>") + markdown.Strong.FindStringSubmatch(m)[1] + p.mark("</strong>")
	})
	return markdown.Emph.ReplaceAllStringFunc(s, func(m string) string {
		q := markdown.Emph.FindStringSubmatch(m)
		return q[1] + p.mark("<em>") + q[2] + p.mark("</em>")
	})
}

// note records a fault once however many times it comes up. A formula
// repeated in four sections is one broken formula to fix, and a findings
// list with it in four times is a list nobody reads to the end.
func (p *pager) note(into *map[string]bool, what string) {
	if *into == nil {
		*into = map[string]bool{}
	}
	(*into)[what] = true
}

// faults is everything this page refers to that is not there, in a fixed
// order so that two runs over one corpus report the same list.
func (p *pager) faults(page string) []Fault {
	var out []Fault
	for _, group := range []struct {
		kind string
		say  string
		set  map[string]bool
	}{
		{FaultMath, "KaTeX refused %s", p.refused},
		{FaultNote, "the footnote marker %s is defined nowhere in the paper", p.orphans},
		{FaultPaper, "the prose cites %s, which is not a paper of this corpus", p.unlinked},
		{FaultCitation, "the bibliography has no entry %s", p.dangling},
	} {
		keys := make([]string, 0, len(group.set))
		for k := range group.set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, Fault{Page: page, Kind: group.kind, What: fmt.Sprintf(group.say, k)})
		}
	}
	return out
}
