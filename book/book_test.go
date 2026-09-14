package book

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// The fixtures are typeset here and not taken from the corpus. Nothing in
// this file is a sentence of anybody's paper: the corpus is other people's
// work under other people's licences, and a test that carried a paragraph of
// it would put that paragraph in a public repository under ours.

// paper is the id every fixture uses. It is a real shape of id, invented
// people, because an id of the wrong shape hides the anchor mistakes.
const paper = "fermat-1637-margin"

// front is the front matter file of the fixture paper.
const front = `---
paper: fermat-1637-margin
title: A Remark in the Margin
authors:
  - Pierre de Fermat
year: 1637
venue: Arithmetica
field: theory
section: ""
section_title: Front Matter
kind: front
lang: en
source: arxiv:1637.0001
---

A Remark in the Margin

**Pierre de Fermat**

Faculty of Mathematics, Toulouse[^1]

We show that the margin is too narrow to hold the proof, and that this is the
case for every exponent above two. The argument is short and the margin is
not, which is the whole of the difficulty and the reason this note is as brief
as it is.

[^1]: Written in the margin of a copy of Arithmetica.
`

// body is a section file with one of everything the renderers handle.
const body = `---
paper: fermat-1637-margin
title: A Remark in the Margin
authors:
  - Pierre de Fermat
year: 1637
venue: Arithmetica
field: theory
section: "1"
section_title: The Margin
tag: 00A1
kind: section
lang: en
---

The exponent $n$ is at least three, and the margin holds $w$ characters. Every
author since [1] has said so, and so has [[euler-1770-algebra]].

$$a^n + b^n \ne c^n \tag{1}$$
{#fermat-1637-margin-eq-1 .equation tag=00B1}

Figure 1: the margin, to scale. {#fermat-1637-margin-fig-1 .figure tag=00C1}

### 1.1 The Narrow Case {#fermat-1637-margin-s1-1 .subsection tag=00D1}

- a margin of one line
- a margin of two lines

| Exponent | Width |
| --- | --- |
| 3 | narrow |
| 4 | narrower |

` + "```" + `python
def margin(n):
    return 1 / n
` + "```" + `

A note on the width.[^2]

[^2]: The width was measured in characters and not in millimetres.
`

// references is the references file.
const references = `---
paper: fermat-1637-margin
title: A Remark in the Margin
authors:
  - Pierre de Fermat
year: 1637
field: theory
kind: references
lang: en
---

[1] Diophantus of Alexandria. Arithmetica. Alexandria, 250.

[2] L. Euler. Vollstandige Anleitung zur Algebra. Saint Petersburg, 1770.
`

// manifest is manifests/figures.yaml with the one figure the body refers to.
const manifest = `figures:
  - paper: fermat-1637-margin
    figure: f01
    number: "1"
    page: 1
    bbox: [72, 400, 540, 600]
    caption: "Figure 1: the margin, to scale."
    sha256: "` + sha + `"
    method: vector
    page_fraction: 0.2
    width: 1200
    height: 800
    bytes: 4096
`

// bib is manifests/refs/<id>.yaml, with the second entry resolved to a paper
// of the corpus so that the corpus citation in the body has a number.
const bib = `paper: fermat-1637-margin
style: numbered
entries:
  - key: "1"
    raw: Diophantus of Alexandria. Arithmetica. Alexandria, 250.
    resolves_to: ""
  - key: "2"
    raw: L. Euler. Vollstandige Anleitung zur Algebra. Saint Petersburg, 1770.
    resolves_to: euler-1770-algebra
`

const sha = "0000000000000000000000000000000000000000000000000000000000000000"

const papers = `papers:
  - id: fermat-1637-margin
    title: A Remark in the Margin
    authors: [Pierre de Fermat]
    year: 1637
    venue: Arithmetica
    field: theory
    number: 1
`

// testBook loads the fixture paper as a book.
func testBook(t *testing.T) *Book {
	t.Helper()
	b, err := Load(testCorpus(t), paper, corpus.EN)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// testCorpus writes the fixture corpus to a temporary directory.
func testCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/papers.yaml", papers)
	write("manifests/figures.yaml", manifest)
	write("manifests/refs/"+paper+".yaml", bib)
	write("content/en/"+paper+"/00_front.md", front)
	write("content/en/"+paper+"/01_margin.md", body)
	write("content/en/"+paper+"/09_references.md", references)

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Gray{Y: 200}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	write("figures/"+paper+"/f01.png", buf.String())

	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadReadsThePaperAsThreeKindsOfFile(t *testing.T) {
	b := testBook(t)

	if b.Title != "A Remark in the Margin" {
		t.Errorf("the title is %q", b.Title)
	}
	if len(b.Sections) != 1 {
		t.Fatalf("the book has %d sections and the fixture has one", len(b.Sections))
	}
	if s := b.Sections[0]; s.Number != "1" || s.Title != "The Margin" {
		t.Errorf("the section is %q %q", s.Number, s.Title)
	}
	if b.Sections[0].Anchor != paper+"-s1" {
		t.Errorf("the section anchor is %q", b.Sections[0].Anchor)
	}
	if len(b.Bibliography) != 2 {
		t.Errorf("the bibliography is %d entries and the fixture has two: %v", len(b.Bibliography), b.Bibliography)
	}
	if len(b.Figures) != 1 {
		t.Errorf("the book found %d figures and the manifest has one", len(b.Figures))
	}
	if _, ok := b.Figures[paper+"-fig-1"]; !ok {
		t.Errorf("the figure is not under the anchor the body refers to: %v", b.Figures)
	}
}

// The abstract is the longest paragraph of the front matter file, and the
// lines above it are the masthead. Neither is marked in the corpus, because
// the page it came off did not mark them either.
func TestTheAbstractIsFoundAndTheMastheadIsWhatIsAboveIt(t *testing.T) {
	b := testBook(t)

	if !strings.HasPrefix(b.Abstract, "We show that the margin") {
		t.Errorf("the abstract is %q", b.Abstract)
	}
	if len(b.Masthead) != 1 || !strings.HasPrefix(b.Masthead[0], "Faculty of Mathematics") {
		t.Errorf("the masthead is %v and the title and the author line are not part of it", b.Masthead)
	}
	if b.TitleAs != "" {
		t.Errorf("the English book carries a translated title %q", b.TitleAs)
	}
}

// The title as the translator wrote it, which is the first paragraph of a
// translated front matter file and is nowhere else in the corpus.
func TestATranslatedBookCarriesTheTitleTheTranslatorWrote(t *testing.T) {
	const titled = "Một Nhận Xét Bên Lề"
	for _, c := range []struct {
		name    string
		printed string
		want    string
	}{
		{"a title the translator translated", titled, titled},
		{"a title the translator left alone", "A Remark in the Margin", ""},
		{"a title with the spacing tidied", "  " + titled + "  ", titled},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := viBook(t, c.printed)
			if b.TitleAs != c.want {
				t.Errorf("the translated title is %q, want %q", b.TitleAs, c.want)
			}
			if b.Title != "A Remark in the Margin" {
				t.Errorf("the English title is %q and the front matter is the same in every language", b.Title)
			}
			if len(b.Masthead) != 1 {
				t.Errorf("the masthead is %v and the title is not part of it", b.Masthead)
			}
		})
	}
}

// viBook writes a Vietnamese tree beside the English one and loads it. The
// front matter is what the translator writes: the same bibliographic facts,
// lang set to the language, and the body in Vietnamese.
func viBook(t *testing.T, printed string) *Book {
	t.Helper()
	c := testCorpus(t)
	page := strings.Replace(front, "lang: en", "lang: vi", 1)
	page = strings.Replace(page, "\nA Remark in the Margin\n", "\n"+printed+"\n", 1)
	page = strings.Replace(page, "Faculty of Mathematics, Toulouse", "Khoa Toán, Toulouse", 1)
	page = strings.Replace(page, "We show that the margin is too narrow to hold the proof, and that this is the\ncase for every exponent above two.",
		"Chúng tôi chỉ ra rằng lề quá hẹp để chứa chứng minh, và điều đó đúng với mọi\nsố mũ lớn hơn hai.", 1)
	dir := filepath.Join(c.Root, "content", "vi", paper)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00_front.md"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(c, paper, corpus.VI)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A marker on the front page regularly has its definition in a later file,
// because the page break fell between them. The book is the first thing that
// sees both, so it is where they are gathered.
func TestFootnotesAreGatheredFromEveryFile(t *testing.T) {
	b := testBook(t)

	if len(b.Notes) != 2 {
		t.Fatalf("the book has %d footnotes and the fixture has two: %v", len(b.Notes), b.Notes)
	}
	if !strings.HasPrefix(b.Notes["1"], "Written in the margin") {
		t.Errorf("note 1 is %q", b.Notes["1"])
	}
	for _, s := range b.Sections {
		if strings.Contains(s.Body, "[^2]:") {
			t.Error("the definition is still in the body, so it will set as a paragraph of its own")
		}
	}
}

func TestACorpusCitationIsTheNumberThePaperPrinted(t *testing.T) {
	b := testBook(t)

	n, ok := b.Cite("euler-1770-algebra")
	if !ok || n != "2" {
		t.Errorf("Cite said %q %v and the bibliography resolves that paper to entry 2", n, ok)
	}
	if _, ok := b.Cite("diophantus-0250-arithmetica"); ok {
		t.Error("Cite found a number for a paper this bibliography does not resolve to")
	}
}

func TestAPaperWithNoContentInALanguageIsNotABook(t *testing.T) {
	if _, err := Load(testCorpus(t), paper, corpus.JA); !os.IsNotExist(err) {
		t.Errorf("Load said %v and there is no Japanese, which the caller reads as nothing to set", err)
	}
}
