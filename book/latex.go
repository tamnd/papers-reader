package book

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/mathtex"
	"github.com/tamnd/papers-reader/tags"
)

// A Renderer turns the Markdown of the corpus into LaTeX, and says what it
// could not do.
//
// The order of operations is the whole of it and it is not negotiable. The
// mathematics comes out first, because inside a math span every character
// LaTeX cares about already means what it says and escaping it would destroy
// it. Then the Markdown that becomes a command comes out, because the command
// it becomes is full of braces and backslashes that the escaping would eat.
// Then what is left is prose, and prose is escaped. Then the commands go back,
// then the mathematics goes back. Doing any two of those in the other order
// produces a document that compiles and is wrong, which is the worst of the
// three outcomes.
type Renderer struct {
	Book *Book
	// Missing is every figure the text refers to that has no picture in the
	// manifest, by anchor. A caption with no figure is set as a paragraph
	// rather than dropped, and this is how the caller finds out.
	Missing []string
	// Notes is how many footnotes were set, and Orphans is every marker whose
	// definition is nowhere in the book. An orphan is set as the marker it
	// was, because a silent deletion of a reference is worse than a visible
	// one.
	Notes   int
	Orphans []string
	// Wide is every table with more columns than the page can hold at the
	// body size. They are set small rather than refused, and the caller says
	// so.
	Wide []string
	// Unlinked is every corpus citation with no entry in this paper's
	// bibliography, by identifier.
	Unlinked []string

	held []string
}

// LaTeX writes one Markdown body as LaTeX. It is the block loop; everything
// below it is one kind of block.
func (r *Renderer) LaTeX(body string) string {
	var out []string
	for _, b := range markdown.Blocks(body) {
		if s := r.block(b); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n\n")
}

func (r *Renderer) block(b string) string {
	text, attr, labelled := markdown.TakeAttr(b)
	switch {
	case strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~"):
		return r.code(text)
	case markdown.IsHeading(text):
		return r.heading(text, attr, labelled)
	case markdown.IsTable(text):
		return r.table(text, attr, labelled)
	case markdown.IsDisplay(text):
		return r.display(text, attr, labelled)
	case labelled && markdown.Has(attr.Classes, "figure"):
		return r.figure(text, attr)
	case markdown.IsList(text):
		return r.list(text, attr, labelled)
	}
	return r.paragraph(text, attr, labelled)
}

// paragraph is prose, with a label under it when the block was anchored.
func (r *Renderer) paragraph(text string, attr tags.Attr, labelled bool) string {
	s := r.Inline(text)
	if labelled {
		s = label(attr) + s
	}
	return s
}

// heading sets a heading inside a section file. The file's own heading is not
// here: the splitter lifted it into the front matter, and the document sets it
// as the \section. So the shallowest heading a body can hold is one below
// that, whatever number of hashes it was written with.
func (r *Renderer) heading(text string, attr tags.Attr, labelled bool) string {
	depth, title, _ := markdown.Heading(text)
	cmd := "subsection"
	if depth > 3 {
		cmd = "subsubsection"
	}
	s := fmt.Sprintf("\\%s*{%s}", cmd, r.Inline(title))
	if labelled {
		s += "\n" + strings.TrimSpace(label(attr))
	}
	// A heading that is starred is not in the table of contents by itself.
	// Numbering is the paper's and not LaTeX's, so the heading text already
	// carries its number and \addcontentsline is what puts it in the contents.
	return s + fmt.Sprintf("\n\\addcontentsline{toc}{%s}{%s}", cmd, Contents(r.Inline(title), title))
}

// code sets a fenced block verbatim.
//
// Verbatim and not listings. The corpus holds pseudocode and shell
// transcripts, not programs, and nothing in it is worth the risk of a
// highlighter deciding that an underscore in a variable name opens a
// subscript. breaklines is on because a transcript line off a two column paper
// is regularly wider than an A4 text block and an overfull line in the margin
// is the commonest ugly thing in a generated document.
func (r *Renderer) code(text string) string {
	lines := strings.Split(text, "\n")
	body := lines
	if len(lines) > 1 {
		body = lines[1:]
	}
	if n := len(body); n > 0 && (strings.HasPrefix(strings.TrimSpace(body[n-1]), "```") ||
		strings.HasPrefix(strings.TrimSpace(body[n-1]), "~~~")) {
		body = body[:n-1]
	}
	return "\\begin{Verbatim}[breaklines=true,breakanywhere=true,fontsize=\\small,xleftmargin=1em]\n" +
		strings.Join(body, "\n") + "\n\\end{Verbatim}"
}

// display sets a displayed equation.
//
// equation when the paper numbered it and equation* when it did not, which is
// what \tag says. An unnumbered display inside a numbered environment gets a
// number LaTeX invented, and a number the paper never printed is worse than no
// number: every cross reference in the prose is to the paper's numbering.
func (r *Renderer) display(text string, attr tags.Attr, labelled bool) string {
	tex := strings.TrimSpace(text)
	tex = strings.TrimSuffix(strings.TrimPrefix(tex, "$$"), "$$")
	env := "equation*"
	if markdown.Tagged(tex) {
		env = "equation"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\\begin{%s}\n%s\n", env, strings.TrimSpace(tex))
	if labelled {
		fmt.Fprintf(&b, "\\label{%s}\n", attr.Anchor)
	}
	fmt.Fprintf(&b, "\\end{%s}", env)
	return b.String()
}

// figure sets a caption paragraph as a figure, with the picture above it.
//
// The corpus writes a figure as a caption with an attribute block on it and
// nothing else, because the picture is a file beside the content rather than
// an image tag in it. Here is where the two are put back together.
//
// The number and the word in front of it are cut off the caption and handed
// to LaTeX instead, so that the figure is a real float with a real counter and
// a \ref to it resolves. The word is whatever the paper printed, which in a
// translation is the translated word, and Document uses the first one it sees
// as \figurename for the whole book. That is why the word is taken from the
// text rather than from a table of four languages here: the corpus already
// knows it and a table would be a second place for it to be wrong.
func (r *Renderer) figure(text string, attr tags.Attr) string {
	f, ok := r.Book.Figures[attr.Anchor]
	if !ok {
		r.Missing = append(r.Missing, attr.Anchor)
		return label(attr) + r.Inline(text)
	}
	caption := text
	if m := markdown.CaptionPrefix.FindStringSubmatch(text); m != nil && m[2] == f.Number {
		caption = text[len(m[0]):]
	}
	n, err := strconv.Atoi(strings.SplitN(f.Number, ".", 2)[0])
	var b strings.Builder
	b.WriteString("\\begin{figure}[htbp]\n\\centering\n")
	if err == nil {
		fmt.Fprintf(&b, "\\setcounter{figure}{%d}\n", n-1)
	}
	fmt.Fprintf(&b, "\\includegraphics[width=\\linewidth,height=0.42\\textheight,keepaspectratio]{%s}\n",
		r.Book.Dir+"/"+f.Name())
	fmt.Fprintf(&b, "\\caption{%s}\n\\label{%s}\n\\end{figure}", r.Inline(caption), attr.Anchor)
	return b.String()
}

func (r *Renderer) list(text string, attr tags.Attr, labelled bool) string {
	env := "itemize"
	if markdown.Ordinal.MatchString(strings.Split(text, "\n")[0]) {
		env = "enumerate"
	}
	var b strings.Builder
	if labelled {
		b.WriteString(label(attr))
	}
	fmt.Fprintf(&b, "\\begin{%s}\n", env)
	for _, l := range strings.Split(text, "\n") {
		item := markdown.Bullet.ReplaceAllString(l, "")
		item = markdown.Ordinal.ReplaceAllString(item, "")
		fmt.Fprintf(&b, "\\item %s\n", r.Inline(item))
	}
	fmt.Fprintf(&b, "\\end{%s}", env)
	return b.String()
}

// wide is the column count above which a table is set small. Twelve columns of
// prose across an A4 text block is about two centimetres a column, which is
// four characters. Six is where it stops being comfortable and eight is where
// it stops fitting, so eight is the line.
const wide = 8

func (r *Renderer) table(text string, attr tags.Attr, labelled bool) string {
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0, len(lines))
	for i, l := range lines {
		if i == 1 {
			continue
		}
		rows = append(rows, markdown.Cells(l))
	}
	n := 0
	for _, row := range rows {
		n = max(n, len(row))
	}
	if n == 0 {
		return r.paragraph(text, attr, labelled)
	}
	var b strings.Builder
	if labelled {
		b.WriteString(label(attr))
	}
	small := n > wide
	if small {
		r.Wide = append(r.Wide, firstCells(rows))
		b.WriteString("{\\footnotesize\n")
	}
	b.WriteString("\\noindent\\begin{tabular}{" + columns(n) + "}\n\\toprule\n")
	for i, row := range rows {
		for j := 0; j < n; j++ {
			if j > 0 {
				b.WriteString(" & ")
			}
			if j < len(row) {
				b.WriteString(r.Inline(row[j]))
			}
		}
		b.WriteString(" \\\\\n")
		if i == 0 {
			b.WriteString("\\midrule\n")
		}
	}
	b.WriteString("\\bottomrule\n\\end{tabular}")
	if small {
		b.WriteString("\n}")
	}
	return b.String()
}

// columns is the column specification for a table of n columns.
//
// Up to three columns the cells are set on their natural width, which is what
// l does and what looks right for a table of numbers. Above that they are
// paragraphs of an equal share of the text block, because a table of five
// columns of prose set on natural widths is eleven hundred points wider than
// the page, which is what the first build of this produced. Ragged right, as
// a narrow column always should be: justifying four words across two
// centimetres puts a centimetre between two of them.
func columns(n int) string {
	if n <= 3 {
		return strings.Repeat("l", n)
	}
	one := fmt.Sprintf(">{\\raggedright\\arraybackslash}p{\\dimexpr(\\linewidth-%d\\tabcolsep)/%d\\relax}", 2*n, n)
	return strings.Repeat(one, n)
}

func firstCells(rows [][]string) string {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "a table"
	}
	return strings.Join(rows[0], " | ")
}

// label writes an anchor as a LaTeX label, on its own line above whatever it
// anchors, so that a \ref to it lands on the right page.
func label(a tags.Attr) string { return "\\label{" + a.Anchor + "}%\n" }

// Inline renders one run of text: the mathematics, the emphasis, the
// citations, the footnotes, and the escaping of everything else.
func (r *Renderer) Inline(text string) string {
	r.held = r.held[:0]
	s := r.hold(text)
	s = escape(s)
	s = strings.ReplaceAll(s, "  \n", " \\\\\n")
	return r.release(s)
}

// hold replaces everything that must not be escaped with a marker.
//
// The order inside it matters as much as the order outside it. Mathematics
// first, so that a dollar span holding a bracket is not read as a citation.
// Code next, for the same reason. Then the things made of brackets, longest
// pattern first, so that [^1] is a footnote before [1] is a citation. Then
// emphasis, which is made of stars and cannot be confused with any of them.
func (r *Renderer) hold(s string) string {
	s = r.holdMath(s)
	s = r.holdCode(s)
	s = r.holdNotes(s)
	s = r.holdPapers(s)
	s = r.holdCites(s)
	s = r.holdEmphasis(s)
	return s
}

// mark is the marker a held span leaves behind. NUL because no corpus file has
// one, the escaper does not touch it, and it cannot be produced by any of the
// substitutions, so a marker can never be made by accident out of text.
func (r *Renderer) mark(latex string) string {
	r.held = append(r.held, latex)
	return "\x00" + strconv.Itoa(len(r.held)-1) + "\x00"
}

var marker = regexp.MustCompile("\x00([0-9]+)\x00")

// release puts the held spans back, repeatedly, because a span can hold a
// marker: a footnote holds its own text, which has its own mathematics in it.
func (r *Renderer) release(s string) string {
	for range 8 {
		if !strings.Contains(s, "\x00") {
			break
		}
		s = marker.ReplaceAllStringFunc(s, func(m string) string {
			n, err := strconv.Atoi(strings.Trim(m, "\x00"))
			if err != nil || n >= len(r.held) {
				return ""
			}
			return r.held[n]
		})
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// textMacros are the text mode commands that turn up inside a math span in
// this corpus, wrapped so that LaTeX will set them.
//
// There is one and it is \LaTeX. The acknowledgments of the GAN paper thank
// somebody for LaTeX typesetting, the page prints the logo, and the reader
// wrote $\LaTeX$ because the logo is not a word. LaTeX stops dead on it with
// "You can't use \spacefactor in math mode" and takes the whole book with it.
//
// This is a repair and repairs are usually the corpus's business rather than
// the setting's. This one is not: the span is correct, KaTeX renders it,
// nothing else in the toolchain minds it, and the only thing that objects is
// LaTeX's own mode system. The fix belongs where the objection is.
var textMacros = strings.NewReplacer(
	`\LaTeX`, `\text{\LaTeX}`,
	`\TeX`, `\text{\TeX}`,
	`\BibTeX`, `\text{\BibTeX}`,
)

func (r *Renderer) holdMath(s string) string {
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
		tex := textMacros.Replace(sp.Text)
		if sp.Display {
			b.WriteString(r.mark("\\[" + tex + "\\]"))
		} else {
			b.WriteString(r.mark("$" + tex + "$"))
		}
		at = sp.End + delim
	}
	b.WriteString(string(rs[at:]))
	return b.String()
}

func (r *Renderer) holdCode(s string) string {
	return markdown.Code.ReplaceAllStringFunc(s, func(m string) string {
		return r.mark("\\texttt{" + escape(strings.Trim(m, "`")) + "}")
	})
}

func (r *Renderer) holdNotes(s string) string {
	s = markdown.LooseNote.ReplaceAllString(s, "$1")
	return markdown.Note.ReplaceAllStringFunc(s, func(m string) string {
		key := markdown.Note.FindStringSubmatch(m)[1]
		text, ok := r.Book.Notes[key]
		if !ok {
			r.Orphans = append(r.Orphans, key)
			return r.mark("\\textsuperscript{" + escape(key) + "}")
		}
		r.Notes++
		// The note text goes back through hold rather than through Inline,
		// because Inline resets the held list and this call is inside one.
		return r.mark("\\footnote{" + escape(r.hold(text)) + "}")
	})
}

// holdPapers sets a link to another paper of the corpus.
//
// In a book there is nothing to link to, so it sets the short form the
// document's own list of related papers is keyed by. It stays a visible
// reference rather than becoming nothing, because the corpus put it there on
// purpose: [[shannon-1948-mathematical]] in the prose of a later paper is the
// one piece of navigation the corpus adds that the paper did not have.
// holdPapers sets a citation of another paper of the corpus.
//
// As the number this paper printed, wherever the bibliography knows it, so
// that the citation reads the way the page it came from read and jumps to the
// same entry as every other citation. Where it does not, the identifier is
// printed in small capitals and recorded on Unlinked, because a paper that
// cites a corpus paper its own bibliography does not list is a refs manifest
// that needs another look, not something to hide.
func (r *Renderer) holdPapers(s string) string {
	return markdown.PaperCite.ReplaceAllStringFunc(s, func(m string) string {
		id := markdown.PaperCite.FindStringSubmatch(m)[1]
		n, ok := r.Book.Cite(id)
		if !ok {
			r.Unlinked = append(r.Unlinked, id)
			return r.mark("\\textsc{" + escape(id) + "}")
		}
		return r.mark("[\\hyperlink{bib-" + n + "}{" + n + "}]")
	})
}

// holdCites turns [12] and [3, 7-9] into links to the bibliography.
//
// Only the digits inside are linked, one by one, so that [3, 7] is two links
// and the comma between them is punctuation. A range is left as printed and
// its endpoints linked, because expanding 7-9 into three links would put text
// on the page the paper did not print.
func (r *Renderer) holdCites(s string) string {
	if len(r.Book.Bibliography) == 0 {
		return s
	}
	return markdown.ReplaceCites(s, func(inner string) string {
		var b strings.Builder
		b.WriteString("[")
		at := 0
		for _, loc := range markdown.Digits.FindAllStringIndex(inner, -1) {
			b.WriteString(escape(inner[at:loc[0]]))
			n := inner[loc[0]:loc[1]]
			fmt.Fprintf(&b, "\\hyperlink{bib-%s}{%s}", n, n)
			at = loc[1]
		}
		b.WriteString(escape(inner[at:]))
		b.WriteString("]")
		return r.mark(b.String())
	})
}

func (r *Renderer) holdEmphasis(s string) string {
	s = markdown.Strong.ReplaceAllStringFunc(s, func(m string) string {
		inner := markdown.Strong.FindStringSubmatch(m)[1]
		return r.mark("\\textbf{") + inner + r.mark("}")
	})
	return markdown.Emph.ReplaceAllStringFunc(s, func(m string) string {
		p := markdown.Emph.FindStringSubmatch(m)
		return p[1] + r.mark("\\emph{") + p[2] + r.mark("}")
	})
}

// specials is every character LaTeX reads as something other than itself. The
// backslash is first and has to be, because every replacement below writes
// one.
var specials = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`{`, `\{`,
	`}`, `\}`,
	`$`, `\$`,
	`&`, `\&`,
	`%`, `\%`,
	`#`, `\#`,
	`_`, `\_`,
	`~`, `\textasciitilde{}`,
	`^`, `\textasciicircum{}`,
)

func escape(s string) string { return specials.Replace(s) }

// FigureName is the word the paper prints in front of a figure number, which
// in a translation is the translated word. It is read off the first caption
// in the body, and is empty when the paper has no captioned figure.
//
// Off the body and not off figures.yaml, which is where it used to be read
// and where it is wrong. The manifest holds one caption per figure, taken
// off the English page, so every translated book got \figurename{Figure} and
// the Japanese one printed "Figure 2:" over a Japanese caption that said 図
// 2. The body is the only place the translated word exists.
//
// The first one wins and the rest are not looked at. A paper prints the same
// word over all of its figures, and a caption that disagrees with the others
// is a reading mistake rather than a second convention.
func (b *Book) FigureName() string {
	for _, s := range b.Sections {
		for _, line := range strings.Split(s.Body, "\n") {
			attrs := tags.ParseAttrs(line)
			if len(attrs) == 0 {
				continue
			}
			if _, ok := b.Figures[attrs[0].Anchor]; !ok {
				continue
			}
			if m := markdown.CaptionPrefix.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// Contents is one heading as \addcontentsline wants it.
//
// hyperref writes the contents line into the PDF outline as well as onto the
// contents page, and an outline entry is a string of Unicode with no TeX in
// it. Hand it a \quad or a formula and it warns once per heading and then
// prints something nobody meant, which is what the first build of this did
// eight times over. \texorpdfstring is hyperref's own answer: the first
// argument is set on the page, the second goes in the outline.
func Contents(set, source string) string {
	return "\\texorpdfstring{" + set + "}{" + bookmark(source) + "}"
}

// bookmark is a heading with everything a PDF outline cannot hold taken out.
//
// The mathematics goes rather than being transliterated, because there is no
// honest plain text for a formula and a bookmark reading "p g = p data" is
// worse than one that stops short. A heading that was nothing but a formula
// would come back empty, so that case falls back to the source with the
// markup pulled off, which at least names the symbols.
func bookmark(s string) string {
	plain := strings.TrimSpace(strings.Join(strings.Fields(unmark(mathtex.Strip(s))), " "))
	if plain != "" {
		return plain
	}
	return strings.TrimSpace(strings.Join(strings.Fields(unmark(s)), " "))
}

// unmark takes the Markdown and the TeX punctuation off a run of text.
func unmark(s string) string {
	s = markdown.PaperCite.ReplaceAllString(s, "$1")
	s = markdown.Note.ReplaceAllString(s, "")
	return plainly.Replace(s)
}

var plainly = strings.NewReplacer("**", "", "*", "", "`", "", "\\", "", "{", "", "}", "", "$", "")
