package book

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// Options are the few things about a book that are the operator's choice
// rather than the corpus's.
//
// There are not many on purpose. Everything else about how a paper is set
// comes out of the corpus, so that two people who build the same book get the
// same book.
type Options struct {
	// Font is the main text face, as fontspec takes it: a file name that
	// tectonic's bundle carries, or a family name installed on the machine.
	// Empty means Latin Modern out of the bundle, which is the reproducible
	// answer and the default.
	Font string
	// CJKFont is the family the Chinese and the Japanese are set in. It has
	// to be a font on the machine, because no TeX bundle ships a CJK face,
	// and a book in those two languages cannot be built without one. Empty
	// means look for one.
	CJKFont string
	Paper   string
	Size    string
	// Contents puts a table of contents in. Off for a short paper, where it
	// is a page listing the nine sections that follow it.
	Contents bool
}

// Sections is how many sections a paper needs before a table of contents
// earns its page. Eight: below that the contents and the paper's first page
// are the same information twice.
const Sections = 8

func (o Options) paper() string {
	if o.Paper == "" {
		return "a4paper"
	}
	return o.Paper
}

func (o Options) size() string {
	if o.Size == "" {
		return "11pt"
	}
	return o.Size
}

// Document writes the whole LaTeX source for a book.
//
// The renderer comes back filled in, because what it could not do is the
// interesting half of the result: a figure with no picture, a footnote marker
// with no definition, a table too wide for the page. The caller reports them
// and the document is still built, because a book with one wide table is
// worth having and a build that refused it is not.
func Document(b *Book, o Options) (string, *Renderer, error) {
	r := &Renderer{Book: b}
	if (b.Lang == corpus.ZH || b.Lang == corpus.JA) && o.CJKFont == "" {
		return "", r, fmt.Errorf("a book in %s needs a CJK font and none was found: pass one with -cjk-font", b.Lang.Name())
	}
	var w strings.Builder
	preamble(&w, b, o)
	w.WriteString("\n\\begin{document}\n\n")
	titlePage(&w, b, r)
	if o.Contents && len(b.Sections) >= Sections {
		w.WriteString("\\tableofcontents\n\\clearpage\n\n")
	}
	for _, s := range b.Sections {
		section(&w, r, s)
	}
	bibliography(&w, b, r)
	colophon(&w, b)
	w.WriteString("\\end{document}\n")
	return w.String(), r, nil
}

func preamble(w *strings.Builder, b *Book, o Options) {
	fmt.Fprintf(w, "%% %s in %s, set by papers book out of the corpus.\n", b.ID, b.Lang.Name())
	w.WriteString("% Do not edit: the corpus is the source and this file is rebuilt from it.\n\n")
	fmt.Fprintf(w, "\\documentclass[%s,%s]{article}\n", o.size(), o.paper())
	fmt.Fprintf(w, "\\usepackage[%s,margin=25mm]{geometry}\n", o.paper())
	// amsmath before fontspec, and fontspec with no-math, so that the text
	// face is the one asked for and the mathematics stays Computer Modern.
	// Every span in the corpus was written by a model that was shown a page
	// of TeX, so it writes the TeX that Computer Modern sets.
	w.WriteString("\\usepackage{amsmath}\n\\usepackage{amssymb}\n\\usepackage{mathtools}\n")
	w.WriteString("\\usepackage[no-math]{fontspec}\n")
	// fvextra and not fancyvrb, though fvextra loads fancyvrb itself. A
	// listing in a paper is set in a column narrower than the line it was
	// written on, and breaklines and breakanywhere are what keep it on the
	// page. They are fvextra's options and fancyvrb answers "breaklines
	// undefined", which is an error the first listing in the corpus found.
	w.WriteString("\\usepackage{graphicx}\n\\usepackage{fvextra}\n\\usepackage{booktabs}\n")
	// array is for the p column of a wide table, which needs \raggedright in
	// front of the column specification and \arraybackslash after it.
	w.WriteString("\\usepackage{array}\n")
	w.WriteString("\\usepackage{microtype}\n")
	w.WriteString("\\usepackage[hidelinks,unicode]{hyperref}\n\n")

	if o.Font != "" {
		fmt.Fprintf(w, "\\setmainfont{%s}\n", o.Font)
	} else {
		// By file name rather than by family. tectonic carries Latin Modern in
		// its bundle and never installs it, so a family name is a font the
		// machine has to have and a file name is one the build always finds.
		// Latin Modern has the full Vietnamese set, which is the reason it is
		// the default here rather than any other bundled face.
		w.WriteString("\\setmainfont{lmroman10-regular.otf}[\n")
		w.WriteString("  ItalicFont=lmroman10-italic.otf,\n")
		w.WriteString("  BoldFont=lmroman10-bold.otf,\n")
		w.WriteString("  BoldItalicFont=lmroman10-bolditalic.otf,\n")
		w.WriteString("]\n")
		w.WriteString("\\setmonofont{lmmono10-regular.otf}[Scale=MatchLowercase]\n")
	}
	if o.CJKFont != "" {
		w.WriteString("\\usepackage{xeCJK}\n")
		fmt.Fprintf(w, "\\setCJKmainfont{%s}\n", o.CJKFont)
		// The Chinese and the Japanese wrap between characters and not between
		// words, and they take no extra space at a line break.
		w.WriteString("\\XeTeXlinebreaklocale \"" + string(b.Lang) + "\"\n")
		w.WriteString("\\XeTeXlinebreakskip = 0pt plus 1pt\n")
	}
	w.WriteString("\n")

	words := Words(b.Lang)
	fmt.Fprintf(w, "\\renewcommand{\\abstractname}{%s}\n", words.Abstract)
	fmt.Fprintf(w, "\\renewcommand{\\contentsname}{%s}\n", words.Contents)
	fmt.Fprintf(w, "\\renewcommand{\\tablename}{%s}\n", words.Table)
	name := b.FigureName()
	if name == "" {
		name = words.Figure
	}
	fmt.Fprintf(w, "\\renewcommand{\\figurename}{%s}\n\n", escape(name))

	// A bibliography entry is a hanging paragraph with its printed number as
	// the anchor a citation in the prose jumps to. It is done by hand rather
	// than with thebibliography because the numbers are the paper's, printed
	// on the page it was read off, and LaTeX renumbering them would break
	// every citation in the corpus that refers to them.
	w.WriteString("\\newcommand{\\bibentry}[2]{%\n")
	w.WriteString("  \\noindent\\hangindent=1.8em\\hangafter=1\n")
	w.WriteString("  \\makebox[1.8em][l]{\\hypertarget{bib-#1}{[#1]}}#2\\par\\smallskip}\n")
	w.WriteString("\\newcommand{\\bibplain}[1]{%\n")
	w.WriteString("  \\noindent\\hangindent=1.8em\\hangafter=1 #1\\par\\smallskip}\n")
	w.WriteString("\\setlength{\\parskip}{0.55em}\n\\setlength{\\parindent}{0pt}\n")
	fmt.Fprintf(w, "\\hypersetup{pdftitle={%s},pdfauthor={%s}}\n", escape(b.Title), escape(strings.Join(b.Authors, ", ")))
}

func titlePage(w *strings.Builder, b *Book, r *Renderer) {
	w.WriteString("\\begin{center}\n")
	if b.TitleAs != "" {
		fmt.Fprintf(w, "{\\LARGE\\bfseries %s\\par}\n", r.Inline(b.TitleAs))
		fmt.Fprintf(w, "\\vspace{0.5em}\n{\\large %s\\par}\n", r.Inline(b.Title))
	} else {
		fmt.Fprintf(w, "{\\LARGE\\bfseries %s\\par}\n", r.Inline(b.Title))
	}
	if len(b.Authors) > 0 {
		fmt.Fprintf(w, "\\vspace{1em}\n{\\large %s\\par}\n", escape(strings.Join(b.Authors, ", ")))
	}
	for _, line := range b.Masthead {
		fmt.Fprintf(w, "\\vspace{0.4em}\n{\\small %s\\par}\n", r.Inline(line))
	}
	if s := imprint(b); s != "" {
		fmt.Fprintf(w, "\\vspace{0.6em}\n{\\small %s\\par}\n", escape(s))
	}
	w.WriteString("\\end{center}\n\n")
	if strings.TrimSpace(b.Abstract) != "" {
		w.WriteString("\\begin{abstract}\n" + r.LaTeX(b.Abstract) + "\n\\end{abstract}\n\n")
	}
}

// imprint is the one line that says where the paper was published and where
// these pages were read from.
func imprint(b *Book) string {
	var parts []string
	switch {
	case b.Venue != "" && b.Year != 0:
		parts = append(parts, fmt.Sprintf("%s %d", b.Venue, b.Year))
	case b.Venue != "":
		parts = append(parts, b.Venue)
	case b.Year != 0:
		parts = append(parts, fmt.Sprint(b.Year))
	}
	if b.Source != "" {
		parts = append(parts, b.Source)
	}
	return strings.Join(parts, "  \u00b7  ")
}

func section(w *strings.Builder, r *Renderer, s Section) {
	title := r.Inline(s.Title)
	head := title
	if s.Numbered() {
		head = escape(s.Number) + "\\quad " + title
	}
	// Starred, with the contents line added by hand, because the number in the
	// heading is the paper's own. LaTeX counting the sections itself would
	// number an appendix 10 where the paper calls it A, and every cross
	// reference in the prose says A.
	fmt.Fprintf(w, "\\section*{%s}\n", head)
	fmt.Fprintf(w, "\\label{%s}\n", s.Anchor)
	fmt.Fprintf(w, "\\addcontentsline{toc}{section}{%s}\n", Contents(head, plainHead(s)))
	fmt.Fprintf(w, "\\markright{%s}\n\n", title)
	w.WriteString(r.LaTeX(s.Body) + "\n\n")
}

// plainHead is one section heading with its number, as plain text.
func plainHead(s Section) string {
	if s.Numbered() {
		return s.Number + " " + bookmark(s.Title)
	}
	return bookmark(s.Title)
}

var bibNumber = regexp.MustCompile(`^\[([0-9]+)\]\s*`)

func bibliography(w *strings.Builder, b *Book, r *Renderer) {
	if len(b.Bibliography) == 0 {
		return
	}
	words := Words(b.Lang)
	fmt.Fprintf(w, "\\section*{%s}\n", escape(words.References))
	fmt.Fprintf(w, "\\addcontentsline{toc}{section}{%s}\n", escape(words.References))
	w.WriteString("\\begingroup\\small\\setlength{\\parskip}{0pt}\n")
	for _, e := range b.Bibliography {
		if m := bibNumber.FindStringSubmatch(e); m != nil {
			fmt.Fprintf(w, "\\bibentry{%s}{%s}\n", m[1], r.Inline(strings.TrimSpace(e[len(m[0]):])))
			continue
		}
		fmt.Fprintf(w, "\\bibplain{%s}\n", r.Inline(e))
	}
	w.WriteString("\\endgroup\n\n")
}

// colophon says what made the book.
//
// Every one of these is already in the front matter of every file, and a
// reader holding a PDF has no front matter. A translation that does not say
// which model wrote it and against which glossary is a translation nobody can
// check, and this project's whole claim is that its output can be checked.
func colophon(w *strings.Builder, b *Book) {
	lines := b.Colophon()
	if len(lines) == 0 {
		return
	}
	words := Words(b.Lang)
	w.WriteString("\\clearpage\n")
	fmt.Fprintf(w, "\\section*{%s}\n", escape(words.Colophon))
	w.WriteString("\\begingroup\\small\n")
	for _, l := range lines {
		fmt.Fprintf(w, "\\noindent %s\\par\n", escape(l))
	}
	w.WriteString("\\endgroup\n\n")
}

// Colophon is the provenance of the book, one fact per line, in English in
// every language because it is about the machinery and not about the paper.
func (b *Book) Colophon() []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	add("This is %s from the papers corpus, set in %s by papers book.", b.ID, b.Lang.Name())
	var f corpus.Front
	if len(b.Sections) > 0 {
		f = b.Sections[0].Front
	}
	// The book's own source and not the first section's. They are the same
	// in a corpus written by the toolchain, and the book's is the one off
	// the front matter file, which is the file a person would have
	// corrected.
	src := b.Source
	if src == "" {
		src = f.Source
	}
	if src != "" {
		add("The pages were read from %s.", src)
	}
	if f.Extraction != "" {
		how := f.Extraction
		if f.ExtractionModel != "" {
			how += " with " + f.ExtractionModel
		}
		add("Extraction: %s.", how)
	}
	if f.TranslationModel != "" {
		add("Translation: %s, run %s.", f.TranslationModel, f.TranslationRun)
	}
	if f.GlossaryVersion != 0 {
		add("Glossary version %d, terms %s.", f.GlossaryVersion, short(f.GlossaryTermsSHA256))
	}
	if f.PromptSHA256 != "" {
		add("Prompt %s.", short(f.PromptSHA256))
	}
	if len(b.Figures) > 0 {
		add("%d figures, cropped from the pages and listed in manifests/figures.yaml.", len(b.Figures))
	}
	if b.Lang.Translated() {
		add("The text is a machine translation of a machine reading, and neither has been checked by a person.")
	} else {
		add("The text is a machine reading of the pages and has not been checked by a person.")
	}
	return out
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
