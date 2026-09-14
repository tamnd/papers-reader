package emit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// body is one section file with a body written into it, so that a test can
// say what the Markdown is and nothing else.
func body(paper string, l corpus.Lang, number, title, text string) string {
	return "---\npaper: " + paper + "\ntitle: A paper\nkind: section\nlang: " + string(l) +
		"\nsection: \"" + number + "\"\nsection_title: " + title +
		"\nextraction: native\nextraction_model: pdftotext 25.09\n---\n\n" + text
}

// The figures manifest the page tests crop against. One figure, so a caption
// with its anchor becomes a picture and a caption with any other anchor does
// not.
const figuresYAML = `figures:
  - paper: rumelhart-1986-backprop
    figure: fig-1
    number: "1"
    page: 3
    bbox: [10, 10, 200, 120]
    caption: A network of units.
    sha256: 0000000000000000000000000000000000000000000000000000000000000000
    method: crop
    page_fraction: 0.2
    width: 760
    height: 480
    bytes: 40960
`

// pagesOf builds the pages of a corpus and hands back both them and the
// faults, because most of these tests are about one or the other and the
// call is four lines.
func pagesOf(t *testing.T, c *corpus.Corpus) ([]*Page, []Fault) {
	t.Helper()
	pages, faults, err := BuildPages(c)
	if err != nil {
		t.Fatal(err)
	}
	return pages, faults
}

func pageOf(t *testing.T, pages []*Page, id string, l corpus.Lang) *Page {
	t.Helper()
	for _, p := range pages {
		if p.ID == id && p.Lang == l {
			return p
		}
	}
	t.Fatalf("there is no %s page of %s", l, id)
	return nil
}

func TestAPageCarriesOneFilePerSectionAndTheFrontMatterApart(t *testing.T) {
	pages, _ := pagesOf(t, whole(t))
	if len(pages) != 3 {
		t.Fatalf("%d pages, want 3", len(pages))
	}
	// Sorted by paper and then by language, which is what makes two runs
	// over one corpus write the same list.
	if pages[0].ID != "hochreiter-1997-lstm" || pages[1].Lang != corpus.EN || pages[2].Lang != corpus.VI {
		t.Errorf("the pages came back in the order %v", []string{
			pages[0].ID + "/" + string(pages[0].Lang),
			pages[1].ID + "/" + string(pages[1].Lang),
			pages[2].ID + "/" + string(pages[2].Lang),
		})
	}
	p := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN)
	if len(p.Sections) != 1 {
		t.Fatalf("%d sections, want 1: the front matter is not one", len(p.Sections))
	}
	if p.Front.Title == "" || len(p.Front.Blocks) == 0 {
		t.Errorf("the front page is empty: %+v", p.Front)
	}
	if p.Sections[0].Anchor != "rumelhart-1986-backprop-s1" {
		t.Errorf("the section anchor is %q", p.Sections[0].Anchor)
	}
	if p.SourceLang != "" {
		t.Errorf("the English page says it was translated from %q", p.SourceLang)
	}
	if vi := pageOf(t, pages, "rumelhart-1986-backprop", corpus.VI); vi.SourceLang != corpus.EN {
		t.Errorf("the Vietnamese page says it was translated from %q", vi.SourceLang)
	}
}

// The bibliography comes from the refs manifest, which is the parsed form
// and the only one that carries the resolution.
func TestThePageCarriesTheParsedBibliography(t *testing.T) {
	pages, _ := pagesOf(t, whole(t))
	p := pageOf(t, pages, "hochreiter-1997-lstm", corpus.EN)
	if len(p.Refs) != 2 {
		t.Fatalf("%d references, want 2", len(p.Refs))
	}
	if p.Refs[0].ResolvesTo != "rumelhart-1986-backprop" || p.Refs[1].ResolvesTo != "" {
		t.Errorf("the resolution did not come through: %+v", p.Refs)
	}
	// A paper nobody has parsed the bibliography of gets an empty list and
	// not a missing field, because the app reads the same shape either way.
	back := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN)
	if back.Refs == nil || len(back.Refs) != 0 {
		t.Errorf("a paper with no refs manifest has %v", back.Refs)
	}
}

// The block index is the alignment key and everything else in the reading
// app rests on it, so the thing to check is that two languages of one
// section come back with the same kinds in the same positions.
func TestEveryLanguageCutsASectionTheSameWay(t *testing.T) {
	const en = "# Method\n\nThe rule is\n\n$$E = \\tfrac{1}{2}\\sum_j (y_j - d_j)^2$$\n\n" +
		"- one\n- two\n\nAnd then some prose.\n"
	const vi = "# Phương pháp\n\nQuy tắc là\n\n$$E = \\tfrac{1}{2}\\sum_j (y_j - d_j)^2$$\n\n" +
		"- một\n- hai\n\nRồi một ít văn xuôi.\n"
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 1),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method", en),
		"content/vi/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Học biểu diễn", corpus.VI, 0, 1),
		"content/vi/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.VI, "1", "Phương pháp", vi),
	})
	pages, faults := pagesOf(t, c)
	if len(faults) != 0 {
		t.Fatalf("faults on a clean corpus: %+v", faults)
	}
	kinds := func(l corpus.Lang) []string {
		var out []string
		for _, b := range pageOf(t, pages, "rumelhart-1986-backprop", l).Sections[0].Blocks {
			out = append(out, b.Kind)
		}
		return out
	}
	want := "heading p math list p"
	if got := strings.Join(kinds(corpus.EN), " "); got != want {
		t.Errorf("the English cuts as %q, want %q", got, want)
	}
	if got := strings.Join(kinds(corpus.VI), " "); got != want {
		t.Errorf("the Vietnamese cuts as %q, want %q", got, want)
	}
	for i, b := range pageOf(t, pages, "rumelhart-1986-backprop", corpus.VI).Sections[0].Blocks {
		if b.I != i {
			t.Errorf("block %d is numbered %d", i, b.I)
		}
	}
}

// Mathematics is rendered at build time, and the TeX is carried beside it so
// a reader can copy the formula out and the search can index it.
func TestADisplayedEquationIsRenderedAndKeptAsTeX(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 1),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method", "$$x^2 + y^2 = z^2 \\tag{3}$$\n"),
	})
	pages, _ := pagesOf(t, c)
	b := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN).Sections[0].Blocks[0]
	if b.Kind != "math" {
		t.Fatalf("the block is a %s", b.Kind)
	}
	if !strings.Contains(b.TeX, "x^2") {
		t.Errorf("the TeX is %q", b.TeX)
	}
	if !strings.Contains(b.HTML, "katex") {
		t.Errorf("KaTeX did not render it: %q", b.HTML)
	}
	// The number is the one the paper printed and never one this toolchain
	// invented, because every cross reference in the prose is to the
	// paper's own numbering.
	if b.Number != "3" {
		t.Errorf("the equation is numbered %q, want 3", b.Number)
	}
}

// A formula KaTeX will not take is a fault and not a failure. The page is
// still built, with the TeX in a code face where the formula would be, and
// rule P01 reports it.
func TestAFormulaKaTeXRefusesIsAFaultAndThePageIsStillBuilt(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 1),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method", "$$\\notacommand{x}$$\n"),
	})
	pages, faults := pagesOf(t, c)
	b := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN).Sections[0].Blocks[0]
	if !strings.Contains(b.HTML, "<code") {
		t.Errorf("the fallback is %q", b.HTML)
	}
	if len(faults) != 1 || faults[0].Kind != FaultMath {
		t.Fatalf("the faults are %+v", faults)
	}
	if faults[0].Page != "p/rumelhart-1986-backprop/en.json" {
		t.Errorf("the fault is filed under %q", faults[0].Page)
	}
}

// A caption whose picture the manifest knows becomes a figure with its
// pixels on it, which is what the app needs before the image has loaded if
// the page is not to jump under the reader.
func TestAFigureCarriesItsSourceAndItsSize(t *testing.T) {
	const text = "Figure 1: A network of units.\n{#rumelhart-1986-backprop-fig-1 .figure tag=00a1}\n"
	c := corpusOf(t, map[string]string{
		"manifests/figures.yaml": figuresYAML,
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 1, 0),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method", text),
	})
	pages, faults := pagesOf(t, c)
	b := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN).Sections[0].Blocks[0]
	if b.Kind != "figure" {
		t.Fatalf("the block is a %s", b.Kind)
	}
	if b.Src != "figures/rumelhart-1986-backprop/fig-1.png" {
		t.Errorf("the source is %q", b.Src)
	}
	if b.W != 760 || b.H != 480 {
		t.Errorf("the size is %d by %d", b.W, b.H)
	}
	if b.Number != "1" || !strings.Contains(b.CaptionHTML, "network of units") {
		t.Errorf("the caption is %q numbered %q", b.CaptionHTML, b.Number)
	}
	// The caption's own prefix is cut off, because the app prints the number
	// from the field and printing it twice is what the first build did.
	if strings.Contains(b.CaptionHTML, "Figure 1") {
		t.Errorf("the caption still has its prefix: %q", b.CaptionHTML)
	}
	if len(faults) != 0 {
		t.Errorf("faults on a figure the manifest holds: %+v", faults)
	}
}

// A caption whose picture nobody rendered degrades to a paragraph rather
// than disappearing, because a block that vanished would move the index of
// every block after it and break the side by side view of a paper that is
// otherwise fine. It is not a fault of the page: a paper part way through
// the figures pass is what rule F09 is about.
func TestAFigureWithNoPictureDegradesToProseAndKeepsItsIndex(t *testing.T) {
	const text = "First.\n\nFigure 9: A picture nobody cropped.\n{#rumelhart-1986-backprop-fig-9 .figure tag=00b2}\n\nLast.\n"
	c := corpusOf(t, map[string]string{
		"manifests/figures.yaml": figuresYAML,
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 1, 0),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method", text),
	})
	pages, faults := pagesOf(t, c)
	blocks := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN).Sections[0].Blocks
	if len(blocks) != 3 {
		t.Fatalf("%d blocks, want 3", len(blocks))
	}
	if blocks[1].Kind != "p" || blocks[2].I != 2 {
		t.Errorf("the blocks are %+v", blocks)
	}
	if !strings.Contains(blocks[1].HTML, "nobody cropped") {
		t.Errorf("the caption did not survive: %q", blocks[1].HTML)
	}
	if len(faults) != 0 {
		t.Errorf("a paper part way through the figures pass reported %+v", faults)
	}
}

// Three outcomes for a citation of another paper, and they are different
// things: the bibliography has it, the corpus has it, or nobody has it.
func TestACitationOfAnotherPaperLinksWhereverItCan(t *testing.T) {
	const text = "As shown in [[rumelhart-1986-backprop]] and in [[nobody-1999-nothing]].\n"
	c := corpusOf(t, map[string]string{
		"content/en/hochreiter-1997-lstm/00_front.md": front(
			"hochreiter-1997-lstm", "Long Short-Term Memory", corpus.EN, 0, 0),
		"content/en/hochreiter-1997-lstm/01_method.md": body(
			"hochreiter-1997-lstm", corpus.EN, "1", "Method", text),
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 0),
		"manifests/refs/hochreiter-1997-lstm.yaml": `paper: hochreiter-1997-lstm
style: numeric
entries:
  - key: "1"
    raw: Rumelhart, Hinton and Williams. Learning representations. Nature, 1986.
    resolves_to: rumelhart-1986-backprop
`,
	})
	pages, faults := pagesOf(t, c)
	html := pageOf(t, pages, "hochreiter-1997-lstm", corpus.EN).Sections[0].Blocks[0].HTML
	if !strings.Contains(html, `href="#ref-1"`) {
		t.Errorf("a paper the bibliography lists did not link to the entry: %q", html)
	}
	if strings.Contains(html, `href="/p/nobody-1999-nothing"`) {
		t.Errorf("a paper the corpus does not have was linked: %q", html)
	}
	if len(faults) != 1 || faults[0].Kind != FaultPaper {
		t.Fatalf("the faults are %+v", faults)
	}
}

// A numeric citation is only checked where somebody has parsed the
// bibliography. A paper waiting for papers refs is not a paper full of
// citations pointing at nothing.
func TestNumericCitationsAreOnlyCheckedAgainstABibliographyThatExists(t *testing.T) {
	const text = "Earlier work [1, 4] said otherwise.\n"
	with := corpusOf(t, map[string]string{
		"content/en/hochreiter-1997-lstm/00_front.md": front(
			"hochreiter-1997-lstm", "Long Short-Term Memory", corpus.EN, 0, 0),
		"content/en/hochreiter-1997-lstm/01_method.md": body(
			"hochreiter-1997-lstm", corpus.EN, "1", "Method", text),
		"manifests/refs/hochreiter-1997-lstm.yaml": `paper: hochreiter-1997-lstm
style: numeric
entries:
  - key: "1"
    raw: Somebody. A paper. 1980.
`,
	})
	pages, faults := pagesOf(t, with)
	html := pageOf(t, pages, "hochreiter-1997-lstm", corpus.EN).Sections[0].Blocks[0].HTML
	if !strings.Contains(html, `href="#ref-1"`) {
		t.Errorf("entry 1 did not link: %q", html)
	}
	if len(faults) != 1 || faults[0].Kind != FaultCitation {
		t.Fatalf("the faults are %+v", faults)
	}

	without := corpusOf(t, map[string]string{
		"content/en/hochreiter-1997-lstm/00_front.md": front(
			"hochreiter-1997-lstm", "Long Short-Term Memory", corpus.EN, 0, 0),
		"content/en/hochreiter-1997-lstm/01_method.md": body(
			"hochreiter-1997-lstm", corpus.EN, "1", "Method", text),
	})
	pages, faults = pagesOf(t, without)
	html = pageOf(t, pages, "hochreiter-1997-lstm", corpus.EN).Sections[0].Blocks[0].HTML
	if strings.Contains(html, "<a") {
		t.Errorf("a citation was linked with no bibliography to link it to: %q", html)
	}
	if len(faults) != 0 {
		t.Errorf("a paper waiting for papers refs reported %+v", faults)
	}
}

// A footnote defined in a later section still resolves, which is why the
// page is read in two passes.
func TestAFootnoteDefinedLaterInThePaperStillResolves(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 0),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method",
			"The units are linear [^2] and the loss is squared [^7].\n"),
		"content/en/rumelhart-1986-backprop/02_results.md": body(
			"rumelhart-1986-backprop", corpus.EN, "2", "Results",
			"Some prose.\n\n[^2]: Except at the output.\n"),
	})
	pages, faults := pagesOf(t, c)
	p := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN)
	html := p.Sections[0].Blocks[0].HTML
	if !strings.Contains(html, `href="#fn-2"`) {
		t.Errorf("a footnote defined two sections later did not link: %q", html)
	}
	if strings.Contains(html, `href="#fn-7"`) {
		t.Errorf("a footnote defined nowhere was linked: %q", html)
	}
	if len(p.Notes) != 1 || p.Notes[0].Key != "2" {
		t.Errorf("the notes are %+v", p.Notes)
	}
	if len(faults) != 1 || faults[0].Kind != FaultNote {
		t.Fatalf("the faults are %+v", faults)
	}
	// The definition is lifted out of the body rather than left in it, so it
	// is not also a paragraph of the section it happened to fall in.
	for _, b := range p.Sections[1].Blocks {
		if strings.Contains(b.HTML, "Except at the output") {
			t.Errorf("the definition is still in the body: %q", b.HTML)
		}
	}
}

// The colophon is the whole paper's answer, so a field the sections disagree
// about is empty rather than the first or the commonest of them.
func TestTheColophonIsEmptyWhereTheSectionsDisagree(t *testing.T) {
	one := body("rumelhart-1986-backprop", corpus.EN, "1", "Method", "Some prose.\n")
	two := strings.Replace(
		body("rumelhart-1986-backprop", corpus.EN, "2", "Results", "More prose.\n"),
		"extraction: native", "extraction: vision", 1)
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations", corpus.EN, 0, 0),
		"content/en/rumelhart-1986-backprop/01_method.md":  one,
		"content/en/rumelhart-1986-backprop/02_results.md": two,
	})
	pages, _ := pagesOf(t, c)
	p := pageOf(t, pages, "rumelhart-1986-backprop", corpus.EN)
	if p.Provenance.Extraction != "" {
		t.Errorf("the colophon says the paper was extracted %q", p.Provenance.Extraction)
	}
	// The tool is the same in both, so it survives the disagreement about
	// the path. One field disagreeing does not empty the rest.
	if p.Provenance.ExtractionTool != "pdftotext 25.09" {
		t.Errorf("the tool is %q", p.Provenance.ExtractionTool)
	}
}

func TestLevelIsCountedOffTheNumberThePaperPrinted(t *testing.T) {
	for _, c := range []struct {
		number string
		want   int
	}{{"", 2}, {"3", 2}, {"3.2", 3}, {"3.2.1", 4}, {"A", 2}} {
		if got := level(c.number); got != c.want {
			t.Errorf("section %q is level %d, want %d", c.number, got, c.want)
		}
	}
}

// Numeric footnotes sort as numbers and the rest as words, because papers
// use both and a few use daggers.
func TestNotesSortNumericallyFirst(t *testing.T) {
	got := noteOrder(map[string]string{"10": "", "2": "", "a": "", "*": ""})
	want := []string{"2", "10", "*", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the notes come out %v, want %v", got, want)
	}
}

func TestAPageIsWrittenWhereTheAppLooksForIt(t *testing.T) {
	if got := PagePath("rumelhart-1986-backprop", corpus.VI); got != "p/rumelhart-1986-backprop/vi.json" {
		t.Errorf("the path is %q", got)
	}
}
