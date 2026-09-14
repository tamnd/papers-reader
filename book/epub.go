package book

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"html"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/katex"
)

// EPUB writes one book as an EPUB 3 file.
//
// EPUB 3 and not 2, because the mathematics is the point. KaTeX writes MathML
// alongside its own markup, EPUB 3 readers understand both, and an EPUB 2
// reader would get a picture of a formula or nothing. The stylesheet and the
// fonts KaTeX needs go in the file, so the book reads the same on a device
// that has never been online.
//
// Everything is written from the Book, so an EPUB and a PDF of the same paper
// are the same document set two ways rather than two documents that have to
// be kept in step.
func EPUB(b *Book, out string) (*Pages, error) {
	kr, err := katex.New()
	if err != nil {
		return nil, err
	}
	p := &Page{Book: b, KaTeX: kr}
	w, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	z := zip.NewWriter(w)

	// The mimetype entry has to be first and stored rather than deflated. It
	// is how a reader identifies the file without unpacking it, and it is the
	// one rule of the format that is about bytes on disk rather than about
	// markup.
	// CreateRaw and not Create, because Create with Store still writes a data
	// descriptor and a reader that sniffs the first bytes of the file expects
	// the sizes in the header. Raw means the sizes and the checksum are the
	// caller's to fill in, which is what the header below does.
	head := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	head.UncompressedSize64 = uint64(len(mimetype))
	head.CompressedSize64 = head.UncompressedSize64
	head.CRC32 = crc32.ChecksumIEEE([]byte(mimetype))
	m, err := z.CreateRaw(head)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(m, mimetype); err != nil {
		return nil, err
	}

	files := []struct{ name, body string }{
		{"META-INF/container.xml", container},
		{"OEBPS/style.css", stylesheet},
	}
	chapters := chapters(b, p)
	for _, c := range chapters {
		files = append(files, struct{ name, body string }{"OEBPS/" + c.file, c.xhtml})
	}
	files = append(files,
		struct{ name, body string }{"OEBPS/nav.xhtml", nav(b, chapters)},
		struct{ name, body string }{"OEBPS/content.opf", opf(b, chapters)},
	)
	for _, f := range files {
		if err := add(z, f.name, []byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := addKaTeX(z); err != nil {
		return nil, err
	}
	if err := addFigures(z, b); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return &Pages{
		Chapters: len(chapters),
		Refused:  p.Refused,
		Missing:  p.Missing,
		Orphans:  p.Orphans,
		Unlinked: p.Unlinked,
		Notes:    p.Notes,
	}, nil
}

// Pages is what came of writing an EPUB, which is the same list of complaints
// the LaTeX side keeps and for the same reason.
type Pages struct {
	Chapters int
	Refused  []string
	Missing  []string
	Orphans  []string
	Unlinked []string
	Notes    int
}

// Clean says nobody has to look at the result.
func (p *Pages) Clean() bool {
	return len(p.Refused) == 0 && len(p.Missing) == 0 && len(p.Orphans) == 0 && len(p.Unlinked) == 0
}

type chapter struct {
	file  string
	title string
	xhtml string
	// nav says whether the chapter is in the table of contents. The title
	// page is not: a contents page whose first entry is the page the reader
	// just came from is a wasted line.
	nav bool
}

func chapters(b *Book, p *Page) []chapter {
	words := Words(b.Lang)
	out := []chapter{{file: "title.xhtml", title: b.Title, xhtml: wrap(b, b.Title, titleBody(b, p))}}
	for i, s := range b.Sections {
		head := s.Title
		if s.Numbered() {
			head = s.Number + ". " + s.Title
		}
		var body strings.Builder
		fmt.Fprintf(&body, "<h2 id=%q>%s</h2>\n", s.Anchor, html.EscapeString(head))
		body.WriteString(p.XHTML(s.Body))
		if notes := p.Footnotes(Used(s.Body)); notes != "" {
			body.WriteString("\n" + notes)
		}
		out = append(out, chapter{
			file:  fmt.Sprintf("s%03d.xhtml", i+1),
			title: head,
			xhtml: wrap(b, head, body.String()),
			nav:   true,
		})
	}
	if len(b.Bibliography) > 0 {
		out = append(out, chapter{
			file: "refs.xhtml", title: words.References, nav: true,
			xhtml: wrap(b, words.References, bibBody(b, p, words.References)),
		})
	}
	out = append(out, chapter{
		file: "colophon.xhtml", title: words.Colophon, nav: true,
		xhtml: wrap(b, words.Colophon, colophonBody(b, words.Colophon)),
	})
	return out
}

func titleBody(b *Book, p *Page) string {
	var w strings.Builder
	w.WriteString(`<section class="titlepage" epub:type="titlepage">` + "\n")
	if b.TitleAs != "" {
		fmt.Fprintf(&w, "<h1>%s</h1>\n", p.Inline(b.TitleAs))
		fmt.Fprintf(&w, `<p class="title-en">%s</p>`+"\n", p.Inline(b.Title))
	} else {
		fmt.Fprintf(&w, "<h1>%s</h1>\n", p.Inline(b.Title))
	}
	if len(b.Authors) > 0 {
		fmt.Fprintf(&w, `<p class="authors">%s</p>`+"\n", html.EscapeString(strings.Join(b.Authors, ", ")))
	}
	for _, line := range b.Masthead {
		fmt.Fprintf(&w, `<p class="masthead">%s</p>`+"\n", p.Inline(line))
	}
	if s := imprint(b); s != "" {
		fmt.Fprintf(&w, `<p class="imprint">%s</p>`+"\n", html.EscapeString(s))
	}
	w.WriteString("</section>\n")
	if strings.TrimSpace(b.Abstract) != "" {
		fmt.Fprintf(&w, `<section class="abstract" epub:type="abstract">`+"\n<h2>%s</h2>\n%s\n",
			html.EscapeString(Words(b.Lang).Abstract), p.XHTML(b.Abstract))
		if notes := p.Footnotes(Used(b.Abstract + strings.Join(b.Masthead, "\n") + b.Title)); notes != "" {
			w.WriteString(notes + "\n")
		}
		w.WriteString("</section>\n")
	}
	return w.String()
}

func bibBody(b *Book, p *Page, title string) string {
	var w strings.Builder
	fmt.Fprintf(&w, `<section epub:type="bibliography">`+"\n<h2>%s</h2>\n<div class=\"refs\">\n", html.EscapeString(title))
	for _, e := range b.Bibliography {
		if m := bibNumber.FindStringSubmatch(e); m != nil {
			fmt.Fprintf(&w, `<p class="ref" id="bib-%s"><span class="num">[%s]</span> %s</p>`+"\n",
				m[1], m[1], p.Inline(strings.TrimSpace(e[len(m[0]):])))
			continue
		}
		fmt.Fprintf(&w, `<p class="ref">%s</p>`+"\n", p.Inline(e))
	}
	w.WriteString("</div>\n</section>\n")
	return w.String()
}

func colophonBody(b *Book, title string) string {
	var w strings.Builder
	fmt.Fprintf(&w, `<section epub:type="colophon">`+"\n<h2>%s</h2>\n", html.EscapeString(title))
	for _, l := range b.Colophon() {
		fmt.Fprintf(&w, "<p>%s</p>\n", html.EscapeString(l))
	}
	w.WriteString("</section>\n")
	return w.String()
}

// wrap puts one chapter body in an XHTML document.
//
// xml:lang and lang both, because readers disagree about which they honour
// and the language is what decides whether a Japanese reader gets Japanese
// glyphs or the Chinese ones a font would otherwise pick for the same code
// points. It is the one attribute in this file that changes what is on the
// page.
func wrap(b *Book, title, body string) string {
	return xmlHeader + fmt.Sprintf(
		`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang=%q lang=%q>
<head>
<meta charset="utf-8"/>
<title>%s</title>
<link rel="stylesheet" type="text/css" href="katex.css"/>
<link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
%s</body>
</html>
`, b.Lang, b.Lang, html.EscapeString(title), body)
}

func nav(b *Book, cs []chapter) string {
	var w strings.Builder
	w.WriteString(`<nav epub:type="toc" id="toc">` + "\n<h1>" + html.EscapeString(Words(b.Lang).Contents) + "</h1>\n<ol>\n")
	for _, c := range cs {
		if !c.nav {
			continue
		}
		fmt.Fprintf(&w, "<li><a href=%q>%s</a></li>\n", c.file, html.EscapeString(c.title))
	}
	w.WriteString("</ol>\n</nav>\n")
	return wrap(b, Words(b.Lang).Contents, w.String())
}

func opf(b *Book, cs []chapter) string {
	var w strings.Builder
	w.WriteString(xmlHeader)
	w.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="pub-id" xml:lang="en">` + "\n")
	w.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	fmt.Fprintf(&w, "<dc:identifier id=\"pub-id\">urn:uuid:%s</dc:identifier>\n", uuid(b))
	fmt.Fprintf(&w, "<dc:title>%s</dc:title>\n", html.EscapeString(b.Title))
	fmt.Fprintf(&w, "<dc:language>%s</dc:language>\n", b.Lang)
	for _, a := range b.Authors {
		fmt.Fprintf(&w, "<dc:creator>%s</dc:creator>\n", html.EscapeString(a))
	}
	if b.Source != "" {
		fmt.Fprintf(&w, "<dc:source>%s</dc:source>\n", html.EscapeString(b.Source))
	}
	// dcterms:modified has to be there and has to be to the second in UTC. It
	// is the build time and not a corpus fact, which is why it is the one
	// thing in the file that makes two builds of the same book differ.
	fmt.Fprintf(&w, "<meta property=\"dcterms:modified\">%s</meta>\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	w.WriteString("</metadata>\n<manifest>\n")
	w.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	w.WriteString(`<item id="style" href="style.css" media-type="text/css"/>` + "\n")
	w.WriteString(`<item id="katexcss" href="katex.css" media-type="text/css"/>` + "\n")
	for i, name := range katexFonts() {
		fmt.Fprintf(&w, `<item id="font%d" href="fonts/%s" media-type="font/woff2"/>`+"\n", i, name)
	}
	for _, f := range sortedFigures(b) {
		fmt.Fprintf(&w, `<item id="%s" href="img/%s" media-type="image/png"/>`+"\n", f.ID, f.Name())
	}
	for i, c := range cs {
		props := ""
		if strings.Contains(c.xhtml, "<math") {
			props = ` properties="mathml"`
		}
		fmt.Fprintf(&w, `<item id="c%03d" href=%q media-type="application/xhtml+xml"%s/>`+"\n", i, c.file, props)
	}
	w.WriteString("</manifest>\n<spine>\n")
	for i := range cs {
		fmt.Fprintf(&w, `<itemref idref="c%03d"/>`+"\n", i)
	}
	w.WriteString("</spine>\n</package>\n")
	return w.String()
}

// uuid is the book's identifier, derived from what the book is rather than
// drawn at random, so that rebuilding a book that has not changed produces
// the same identifier and a reader treats it as the same book rather than as
// a second copy.
func uuid(b *Book) string {
	h := sha256.Sum256([]byte("papers/" + b.ID + "/" + string(b.Lang)))
	s := hex.EncodeToString(h[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

// sortedFigures is the figures of the paper in file name order, which is
// figure order. The map is keyed by anchor and a map has no order, and the
// manifest and the spine both have to list the same pictures the same way
// twice.
func sortedFigures(b *Book) []figures.Figure {
	out := make([]figures.Figure, 0, len(b.Figures))
	for _, f := range b.Figures {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func addFigures(z *zip.Writer, b *Book) error {
	for _, f := range sortedFigures(b) {
		data, err := os.ReadFile(filepath.Join(b.Dir, f.Name()))
		if err != nil {
			if os.IsNotExist(err) {
				// The manifest is the record and the file may simply not be
				// in this working tree. The page already links to it, which
				// is the right markup; the reader shows a gap.
				continue
			}
			return err
		}
		if err := add(z, "OEBPS/img/"+f.Name(), data); err != nil {
			return err
		}
	}
	return nil
}

// addKaTeX copies the stylesheet and the fonts into the book.
//
// All of them, without looking at which the document uses. Working that out
// would mean parsing the rendered markup for font families, the whole set is
// three hundred kilobytes, and a book that is missing the one font a single
// formula needed is a book with a blank square in it.
func addKaTeX(z *zip.Writer) error {
	assets := katex.Assets()
	css, err := fs.ReadFile(assets, "katex.min.css")
	if err != nil {
		return err
	}
	// The stylesheet asks for fonts/KaTeX_Main-Regular.woff2 and so on, which
	// is where they go, so the paths need no rewriting.
	if err := add(z, "OEBPS/katex.css", css); err != nil {
		return err
	}
	for _, name := range katexFonts() {
		data, err := fs.ReadFile(assets, path.Join("fonts", name))
		if err != nil {
			return err
		}
		if err := add(z, "OEBPS/fonts/"+name, data); err != nil {
			return err
		}
	}
	return nil
}

func katexFonts() []string {
	entries, err := fs.ReadDir(katex.Assets(), "fonts")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".woff2") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func add(z *zip.Writer, name string, body []byte) error {
	w, err := z.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

const (
	mimetype  = "application/epub+zip"
	xmlHeader = `<?xml version="1.0" encoding="utf-8"?>` + "\n"

	container = xmlHeader + `<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`
)

// stylesheet is deliberately short.
//
// An EPUB reader is somebody else's typesetter and the reader has set their
// own body size, their own margins and often their own face. What is here is
// the handful of things the corpus needs that no reader would guess: that a
// displayed equation is centred and may scroll rather than overflow, that a
// figure is centred with its caption small under it, that a listing does not
// wrap, and that a bibliography entry hangs.
const stylesheet = `body { line-height: 1.5; }
h1 { font-size: 1.5em; line-height: 1.3; }
h2 { font-size: 1.25em; }
h3, h4 { font-size: 1.05em; }
p { margin: 0.6em 0; text-align: justify; }
.titlepage { text-align: center; }
.titlepage h1 { margin-bottom: 0.6em; }
.title-en { font-size: 1.15em; margin-top: 0; }
.authors { font-size: 1.05em; }
.masthead, .imprint { font-size: 0.85em; color: #444; }
.abstract { margin: 1.5em 1em; font-size: 0.95em; }
.equation { margin: 1em 0; text-align: center; overflow-x: auto; }
figure { margin: 1.2em 0; text-align: center; page-break-inside: avoid; }
figure img { max-width: 100%; height: auto; }
figcaption { font-size: 0.85em; text-align: left; margin-top: 0.4em; }
pre { font-size: 0.8em; overflow-x: auto; white-space: pre; padding: 0.5em; background: #f6f6f6; }
code { font-size: 0.9em; }
code.tex { background: #fee; }
table { border-collapse: collapse; margin: 1em auto; font-size: 0.9em; }
th, td { border-bottom: 1px solid #ccc; padding: 0.25em 0.6em; text-align: left; }
th { border-bottom: 2px solid #666; }
.refs .ref { padding-left: 2.2em; text-indent: -2.2em; text-align: left; margin: 0.4em 0; }
.refs .num { font-variant-numeric: tabular-nums; }
.cite, .noteref { text-decoration: none; }
.paper { font-variant: small-caps; }
.notes { margin-top: 2em; border-top: 1px solid #ccc; font-size: 0.85em; }
`
