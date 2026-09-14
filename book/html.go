package book

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/mathtex"
	"github.com/tamnd/papers-reader/tags"
)

// A Page turns the Markdown of the corpus into the XHTML an EPUB holds.
//
// It is the same shape as Renderer and deliberately so: the same block loop,
// the same hold and release around the escaping, the same order. Two
// renderers rather than one with a flag, because the two differ in every
// single case of the switch and a renderer full of "if latex" is a renderer
// where a fix goes into one branch and not the other.
//
// What it does not share is the mathematics. LaTeX gets the TeX back
// unchanged, because LaTeX is what the TeX was written for. A page gets what
// KaTeX makes of it, which is the same markup the reading app will show, so
// the EPUB and the web page agree on what a formula looks like by
// construction.
type Page struct {
	Book  *Book
	KaTeX *katex.Renderer
	// Refused is every span KaTeX would not read, in the order they came up.
	// The span is set as its TeX in a code face rather than dropped, because a
	// formula somebody can still read is better than a gap, and this is how
	// the caller finds out there was one.
	Refused  []string
	Missing  []string
	Orphans  []string
	Unlinked []string
	Notes    int

	held []string
}

// XHTML writes one Markdown body as the body of a page.
func (p *Page) XHTML(body string) string {
	var out []string
	for _, b := range markdown.Blocks(body) {
		if s := p.block(b); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

func (p *Page) block(b string) string {
	text, attr, labelled := markdown.TakeAttr(b)
	id := ""
	if labelled {
		id = fmt.Sprintf(" id=%q", attr.Anchor)
	}
	switch {
	case strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~"):
		return "<pre" + id + "><code>" + html.EscapeString(markdown.Fenced(text)) + "</code></pre>"
	case markdown.IsHeading(text):
		depth, title, _ := markdown.Heading(text)
		tag := "h3"
		if depth > 3 {
			tag = "h4"
		}
		return fmt.Sprintf("<%s%s>%s</%s>", tag, id, p.Inline(title), tag)
	case markdown.IsTable(text):
		return p.table(text, id)
	case markdown.IsDisplay(text):
		return p.display(text, id)
	case labelled && markdown.Has(attr.Classes, "figure"):
		return p.figure(text, attr)
	case markdown.IsList(text):
		return p.list(text, id)
	}
	return "<p" + id + ">" + p.Inline(text) + "</p>"
}

func (p *Page) display(text, id string) string {
	tex := strings.TrimSpace(text)
	tex = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(tex, "$$"), "$$"))
	return `<div class="equation"` + id + ">" + p.math(tex, true) + "</div>"
}

func (p *Page) figure(text string, attr tags.Attr) string {
	f, ok := p.Book.Figures[attr.Anchor]
	if !ok {
		p.Missing = append(p.Missing, attr.Anchor)
		return fmt.Sprintf("<p id=%q>%s</p>", attr.Anchor, p.Inline(text))
	}
	return fmt.Sprintf("<figure id=%q>\n<img src=%q alt=%q/>\n<figcaption>%s</figcaption>\n</figure>",
		attr.Anchor, "img/"+f.Name(), html.EscapeString(plainCaption(f.Caption)), p.Inline(text))
}

// plainCaption is the caption with the markup taken out, for the alt text. It
// is the caption and not a description of the picture, which is the honest
// thing to put there: nobody has described these images and inventing a
// description from the file name would be worse than repeating the caption.
func plainCaption(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	return strings.TrimSpace(s)
}

func (p *Page) list(text, id string) string {
	tag := "ul"
	if markdown.Ordinal.MatchString(strings.Split(text, "\n")[0]) {
		tag = "ol"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s%s>\n", tag, id)
	for _, l := range strings.Split(text, "\n") {
		item := markdown.Bullet.ReplaceAllString(l, "")
		item = markdown.Ordinal.ReplaceAllString(item, "")
		fmt.Fprintf(&b, "<li>%s</li>\n", p.Inline(item))
	}
	fmt.Fprintf(&b, "</%s>", tag)
	return b.String()
}

func (p *Page) table(text, id string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "<table%s>\n", id)
	for i, l := range lines {
		if i == 1 {
			continue
		}
		cell, row := "td", "tr"
		if i == 0 {
			cell = "th"
		}
		fmt.Fprintf(&b, "<%s>", row)
		for _, c := range markdown.Cells(l) {
			fmt.Fprintf(&b, "<%s>%s</%s>", cell, p.Inline(c), cell)
		}
		fmt.Fprintf(&b, "</%s>\n", row)
	}
	b.WriteString("</table>")
	return b.String()
}

// Inline renders one run of text. Same order as the LaTeX side: hold what
// must not be escaped, escape, put it back.
func (p *Page) Inline(text string) string {
	p.held = p.held[:0]
	s := p.hold(text)
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "  \n", "<br/>\n")
	return p.release(s)
}

func (p *Page) hold(s string) string {
	s = p.holdMath(s)
	s = p.holdCode(s)
	s = p.holdNotes(s)
	s = p.holdPapers(s)
	s = p.holdCites(s)
	s = p.holdEmphasis(s)
	return s
}

func (p *Page) mark(markup string) string {
	p.held = append(p.held, markup)
	return "\x00" + strconv.Itoa(len(p.held)-1) + "\x00"
}

func (p *Page) release(s string) string {
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

// math renders one span, and falls back to the TeX in a code face when KaTeX
// refuses it.
//
// Extraction already renders every span it writes, which is audit rule M04,
// so a refusal here is a span that got into the corpus before that rule did
// or one a translation broke. Either way the book is still built and the
// caller is told, because a paper with one unreadable formula is worth
// reading and a build that refused it is not.
func (p *Page) math(tex string, display bool) string {
	out, err := p.KaTeX.Render(tex, display)
	if err != nil {
		p.Refused = append(p.Refused, oneLine(tex)+": "+err.Error())
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

func (p *Page) holdMath(s string) string {
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

func (p *Page) holdCode(s string) string {
	return markdown.Code.ReplaceAllStringFunc(s, func(m string) string {
		return p.mark("<code>" + html.EscapeString(strings.Trim(m, "`")) + "</code>")
	})
}

// holdNotes turns a marker into an EPUB 3 footnote reference.
//
// epub:type="noteref" and a matching "footnote" is what a reader needs to
// show the note in a popup rather than sending somebody to the end of the
// chapter and back. The note itself is written at the foot of the same page
// by Chapter, so a reader that does not support popups still works.
func (p *Page) holdNotes(s string) string {
	s = markdown.LooseNote.ReplaceAllString(s, "$1")
	return markdown.Note.ReplaceAllStringFunc(s, func(m string) string {
		key := markdown.Note.FindStringSubmatch(m)[1]
		if _, ok := p.Book.Notes[key]; !ok {
			p.Orphans = append(p.Orphans, key)
			return p.mark("<sup>" + html.EscapeString(key) + "</sup>")
		}
		p.Notes++
		return p.mark(fmt.Sprintf(
			`<a class="noteref" epub:type="noteref" href="#fn-%s"><sup>%s</sup></a>`,
			html.EscapeString(key), html.EscapeString(key)))
	})
}

func (p *Page) holdPapers(s string) string {
	return markdown.PaperCite.ReplaceAllStringFunc(s, func(m string) string {
		id := markdown.PaperCite.FindStringSubmatch(m)[1]
		n, ok := p.Book.Cite(id)
		if !ok {
			p.Unlinked = append(p.Unlinked, id)
			return p.mark(`<span class="paper">` + html.EscapeString(id) + `</span>`)
		}
		// The identifier stays on the element even though the number is what
		// is shown, because a reading app that has the whole corpus can follow
		// it to the other paper and an EPUB reader will ignore it.
		return p.mark(fmt.Sprintf(`[<a class="cite paper" href="refs.xhtml#bib-%s" data-paper="%s">%s</a>]`,
			n, html.EscapeString(id), n))
	})
}

func (p *Page) holdCites(s string) string {
	if len(p.Book.Bibliography) == 0 {
		return s
	}
	return markdown.NumCite.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[1 : len(m)-1]
		var b strings.Builder
		b.WriteString("[")
		at := 0
		for _, loc := range markdown.Digits.FindAllStringIndex(inner, -1) {
			b.WriteString(html.EscapeString(inner[at:loc[0]]))
			n := inner[loc[0]:loc[1]]
			fmt.Fprintf(&b, `<a class="cite" href="refs.xhtml#bib-%s">%s</a>`, n, n)
			at = loc[1]
		}
		b.WriteString(html.EscapeString(inner[at:]))
		b.WriteString("]")
		return p.mark(b.String())
	})
}

func (p *Page) holdEmphasis(s string) string {
	s = markdown.Strong.ReplaceAllStringFunc(s, func(m string) string {
		return p.mark("<strong>") + markdown.Strong.FindStringSubmatch(m)[1] + p.mark("</strong>")
	})
	return markdown.Emph.ReplaceAllStringFunc(s, func(m string) string {
		q := markdown.Emph.FindStringSubmatch(m)
		return q[1] + p.mark("<em>") + q[2] + p.mark("</em>")
	})
}

// Footnotes writes the notes used on one page, at the foot of it.
//
// Only the ones used, and in the order they were used, because a chapter that
// carried every note in the book would carry the front page's affiliations at
// the end of section seven.
func (p *Page) Footnotes(used []string) string {
	if len(used) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="notes" epub:type="footnotes">` + "\n")
	for _, key := range used {
		text, ok := p.Book.Notes[key]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<aside id="fn-%s" epub:type="footnote"><p><sup>%s</sup> %s</p></aside>`+"\n",
			html.EscapeString(key), html.EscapeString(key), p.Inline(text))
	}
	b.WriteString("</section>")
	return b.String()
}

var usedNote = regexp.MustCompile(`\[\^([^\]\s]+)\]`)

// Used lists the footnote markers in a body, in order, once each.
func Used(body string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range usedNote.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}
