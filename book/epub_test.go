package book

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/katex"
)

// page renders the fixture body as XHTML, with KaTeX, and gives back the page
// so a test can ask what it could not do.
func page(t *testing.T) (string, *Page) {
	t.Helper()
	b := testBook(t)
	r, err := katex.New()
	if err != nil {
		t.Skipf("KaTeX is not available here: %v", err)
	}
	p := &Page{Book: b, KaTeX: r}
	return p.XHTML(b.Sections[0].Body), p
}

func TestTheMathematicsIsRenderedAndNotPassedThrough(t *testing.T) {
	xhtml, p := page(t)

	if len(p.Refused) != 0 {
		t.Errorf("KaTeX refused %v and every span of the fixture is ordinary", p.Refused)
	}
	if !strings.Contains(xhtml, "<math") {
		t.Error("the page has no MathML, so a reader with no fonts has nothing to show")
	}
	if strings.Contains(xhtml, `$n$`) {
		t.Error("a span went through as TeX")
	}
	if !strings.Contains(xhtml, `class="equation"`) {
		t.Error("the display is not a block of its own")
	}
}

// A span KaTeX cannot read is set as its own TeX in a code face. A formula
// somebody can still read is better than a gap, and the caller is told.
func TestASpanKaTeXRefusesIsShownAsTeX(t *testing.T) {
	b := testBook(t)
	r, err := katex.New()
	if err != nil {
		t.Skipf("KaTeX is not available here: %v", err)
	}
	p := &Page{Book: b, KaTeX: r}

	s := p.Inline(`the value $\nosuchmacro{x}$ is fixed`)
	if !strings.Contains(s, `class="tex"`) {
		t.Errorf("the refused span was not shown: %s", s)
	}
	if len(p.Refused) != 1 {
		t.Errorf("the page refused %v and one span is bad", p.Refused)
	}
}

func TestTheXHTMLIsTheSameDocumentAsTheLaTeX(t *testing.T) {
	xhtml, p := page(t)

	for _, want := range []string{
		`id="fermat-1637-margin-eq-1"`,
		`id="fermat-1637-margin-fig-1"`,
		`<img src="img/f01.png"`,
		`href="refs.xhtml#bib-1"`,
		`data-paper="euler-1770-algebra"`,
		"<ul>",
		"<table>",
		"<pre",
		`epub:type="noteref"`,
	} {
		if !strings.Contains(xhtml, want) {
			t.Errorf("the page has no %q:\n%s", want, xhtml)
		}
	}
	if len(p.Missing) != 0 || len(p.Orphans) != 0 || len(p.Unlinked) != 0 {
		t.Errorf("the page is missing %v, orphaned %v and could not link %v", p.Missing, p.Orphans, p.Unlinked)
	}
}

// Only the notes a page used, in the order it used them. A chapter that
// carried every note in the book would carry the front page's affiliations at
// the end of the last section.
func TestAChapterCarriesItsOwnNotes(t *testing.T) {
	b := testBook(t)
	used := Used(b.Sections[0].Body)

	if len(used) != 1 || used[0] != "2" {
		t.Fatalf("the section uses %v and it has one marker", used)
	}
	r, err := katex.New()
	if err != nil {
		t.Skipf("KaTeX is not available here: %v", err)
	}
	p := &Page{Book: b, KaTeX: r}
	notes := p.Footnotes(used)
	if !strings.Contains(notes, `id="fn-2"`) || !strings.Contains(notes, `epub:type="footnote"`) {
		t.Errorf("the note is not where the marker points:\n%s", notes)
	}
	if strings.Contains(notes, "Written in the margin") {
		t.Error("the front page's note was set at the foot of the section")
	}
}

// The one rule of the format that is about bytes on disk and not about
// markup: mimetype first, stored, with its size in the header where a reader
// that sniffs the first bytes of the file will find it.
func TestTheMimetypeEntryIsFirstAndStored(t *testing.T) {
	path := build(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	first := z.File[0]
	if first.Name != "mimetype" {
		t.Fatalf("the first entry is %q", first.Name)
	}
	if first.Method != zip.Store {
		t.Error("the mimetype entry is deflated")
	}
	rc, err := first.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "application/epub+zip" {
		t.Errorf("the mimetype entry is %q, and it was empty once because CreateRaw fills in no sizes", b)
	}
}

// Every file in the container is in the manifest and every file in the
// manifest is in the container. A reader that cannot find a file named in the
// manifest refuses the whole book.
func TestTheManifestAndTheContainerAgree(t *testing.T) {
	z, err := zip.OpenReader(build(t))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	inZip := map[string]bool{}
	for _, f := range z.File {
		if name, ok := strings.CutPrefix(f.Name, "OEBPS/"); ok {
			inZip[name] = true
		}
	}
	var pkg struct {
		Manifest struct {
			Items []struct {
				Href       string `xml:"href,attr"`
				ID         string `xml:"id,attr"`
				Properties string `xml:"properties,attr"`
			} `xml:"item"`
		} `xml:"manifest"`
		Spine struct {
			Refs []struct {
				IDRef string `xml:"idref,attr"`
			} `xml:"itemref"`
		} `xml:"spine"`
	}
	if err := xml.Unmarshal(read(t, &z.Reader, "OEBPS/content.opf"), &pkg); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, item := range pkg.Manifest.Items {
		if !inZip[item.Href] {
			t.Errorf("the manifest names %q and the container has no such file", item.Href)
		}
		ids[item.ID] = true
		delete(inZip, item.Href)
	}
	// The package document is the one file that is not in its own manifest.
	delete(inZip, "content.opf")
	for name := range inZip {
		t.Errorf("%q is in the container and not in the manifest", name)
	}
	if len(pkg.Spine.Refs) == 0 {
		t.Fatal("the spine is empty, so the book has no reading order")
	}
	for _, ref := range pkg.Spine.Refs {
		if !ids[ref.IDRef] {
			t.Errorf("the spine names %q and the manifest does not", ref.IDRef)
		}
	}
}

// A chapter with MathML in it has to say so in the manifest, or a reader is
// entitled to refuse to render it.
func TestAChapterWithMathematicsSaysSo(t *testing.T) {
	z, err := zip.OpenReader(build(t))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	opf := string(read(t, &z.Reader, "OEBPS/content.opf"))
	for _, f := range z.File {
		name, ok := strings.CutPrefix(f.Name, "OEBPS/")
		if !ok || !strings.HasSuffix(name, ".xhtml") {
			continue
		}
		if !strings.Contains(string(read(t, &z.Reader, f.Name)), "<math") {
			continue
		}
		line := ""
		for _, l := range strings.Split(opf, "\n") {
			if strings.Contains(l, `href="`+name+`"`) {
				line = l
			}
		}
		if !strings.Contains(line, `properties="mathml"`) {
			t.Errorf("%s has mathematics and its manifest entry is %q", name, strings.TrimSpace(line))
		}
	}
}

// Every chapter is well formed XML. XHTML is not HTML and a reader does not
// forgive an unclosed tag: it stops.
func TestEveryChapterIsWellFormedXML(t *testing.T) {
	z, err := zip.OpenReader(build(t))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	seen := 0
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".xhtml") && !strings.HasSuffix(f.Name, ".opf") && !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		seen++
		d := xml.NewDecoder(strings.NewReader(string(read(t, &z.Reader, f.Name))))
		d.Strict = true
		for {
			_, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s is not well formed: %v", f.Name, err)
				break
			}
		}
	}
	if seen < 4 {
		t.Errorf("only %d files were checked, so the book is not the shape this test thinks", seen)
	}
}

func TestTheFiguresAndTheFontsAreCarried(t *testing.T) {
	z, err := zip.OpenReader(build(t))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()

	fonts, images := 0, 0
	for _, f := range z.File {
		switch {
		case strings.HasSuffix(f.Name, ".woff2"):
			fonts++
		case strings.HasPrefix(f.Name, "OEBPS/img/"):
			images++
		}
	}
	if images != 1 {
		t.Errorf("the book carries %d pictures and the paper has one figure", images)
	}
	// An EPUB is read on a machine that has never heard of KaTeX, so the
	// faces it sets a formula in travel with it.
	if fonts == 0 {
		t.Error("the book carries no KaTeX fonts")
	}
}

// build writes the fixture book as an EPUB and gives back the path.
func build(t *testing.T) string {
	t.Helper()
	if _, err := katex.New(); err != nil {
		t.Skipf("KaTeX is not available here: %v", err)
	}
	out := filepath.Join(t.TempDir(), "book.epub")
	p, err := EPUB(testBook(t), out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Chapters == 0 {
		t.Fatal("the book has no chapters")
	}
	return out
}

func read(t *testing.T, z *zip.Reader, name string) []byte {
	t.Helper()
	f, err := z.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
