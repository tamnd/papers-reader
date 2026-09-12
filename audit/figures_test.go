package audit

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/figures"
	"gopkg.in/yaml.v3"
)

// picture is a committed figure: a plain grey rectangle of a given size,
// encoded the way the renderer encodes one.
func picture(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Gray{Y: 200}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// noise is a figure that will not compress, which is how a test makes a file
// that is genuinely over the byte cap rather than one that claims to be.
func noise(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(1))
	for i := range img.Pix {
		img.Pix[i] = byte(r.Intn(256))
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// record is an entry in manifests/figures.yaml that passes every rule, so
// that a test can break the one thing it is about.
func record(paper, id string) figures.Figure {
	return figures.Figure{
		Paper: paper, ID: id, Number: strings.TrimPrefix(id, "f0"), Page: 3,
		Box: figures.Box{72, 400, 540, 600}, Caption: "Figure 1: a diagram",
		SHA256: strings.Repeat("ab", 32), Method: figures.Vector,
		Fraction: 0.2, Width: 1200, Height: 800, Bytes: 90 << 10,
	}
}

// manifest is the YAML for a set of entries, marshalled rather than written
// out by hand so that a change to the file format cannot leave the tests
// checking a shape the toolchain no longer writes.
func manifest(t *testing.T, figs ...figures.Figure) string {
	t.Helper()
	b, err := yaml.Marshal(figures.Manifest{Figures: figs})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// corpusOf is build with a figures manifest in it.
func corpusOf(t *testing.T, figs []figures.Figure, files map[string]string) *Input {
	t.Helper()
	all := map[string]string{"manifests/figures.yaml": manifest(t, figs...)}
	for k, v := range files {
		all[k] = v
	}
	return build(t, all)
}

// Nothing extracted yet is not a clean corpus. Every rule in the group has
// to say so rather than pass.
func TestTheFigureRulesStandDownOnACorpusWithNoFigures(t *testing.T) {
	rep := Run(build(t, nil), false)
	for _, id := range []string{"F01", "F02", "F03", "F04", "F05", "F06", "F07", "F08", "F09"} {
		if !result(t, rep, id).NotRun {
			t.Errorf("%s claimed a result on a corpus with no figures", id)
		}
	}
}

func TestACorrectPaperPassesTheWholeGroup(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
		"content/en/vaswani-2017-attention/02_section.md": front("vaswani-2017-attention", "section",
			"As Figure 1 shows, it works.\n\n![Figure 1: a diagram](../../../figures/vaswani-2017-attention/f01.png)"),
	})
	in.Tracked = []string{"figures/vaswani-2017-attention/f01.png"}
	for _, res := range Run(in, false).Results {
		if res.Rule.Group() == GroupFigures && res.Failed() {
			t.Errorf("%s failed on a paper that is correct: %v", res.Rule.ID, res.Findings)
		}
	}
}

// The commonest way to break F01 is to commit the manifest and forget the
// PNG, which leaves a reader with a broken image and the manifest insisting
// the figure is there.
func TestF01FindsAnEntryWithNoFile(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, nil)
	res := result(t, Run(in, true), "F01")
	if len(res.Findings) != 1 {
		t.Fatalf("F01 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "f01") {
		t.Errorf("the finding does not name the figure: %v", res.Findings[0])
	}
}

func TestF01FindsALinkToAFigureThatIsNotThere(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
		"content/en/vaswani-2017-attention/02_section.md": front("vaswani-2017-attention", "section",
			"first line\n\n![Figure 2](../../../figures/vaswani-2017-attention/f02.png)"),
	})
	res := result(t, Run(in, true), "F01")
	if len(res.Findings) != 1 {
		t.Fatalf("F01 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if res.Findings[0].Line != 9 {
		t.Errorf("the finding is on line %d, want 9", res.Findings[0].Line)
	}
}

// A link out to the web is somebody else's problem and is left alone.
func TestF01LeavesLinksThatLeaveTheCorpusAlone(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
		"content/en/vaswani-2017-attention/02_section.md": front("vaswani-2017-attention", "section",
			"![a badge](https://example.org/badge.png)"),
	})
	if res := result(t, Run(in, true), "F01"); res.Failed() {
		t.Errorf("F01 objected to a link off the site: %v", res.Findings)
	}
}

// A hundred pixels on a side is the floor, because below it the thing is an
// inline glyph, a rule or a logo rather than a diagram.
func TestF02FindsAGlyph(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 64, 64),
	})
	res := result(t, Run(in, true), "F02")
	if len(res.Findings) != 1 {
		t.Fatalf("F02 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "64x64") {
		t.Errorf("the finding does not say how big it is: %v", res.Findings[0])
	}
}

// The size is measured from the file and not read out of the manifest,
// because the number in the manifest was written by the run that wrote the
// file and the two agreeing proves nothing.
func TestF02MeasuresTheFileAndNotTheManifest(t *testing.T) {
	entry := record("vaswani-2017-attention", "f01")
	entry.Width, entry.Height = 1200, 800
	in := corpusOf(t, []figures.Figure{entry}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 40, 40),
	})
	if res := result(t, Run(in, true), "F02"); !res.Failed() {
		t.Fatal("F02 believed the manifest over the file")
	}
}

func TestF02FindsAFileThatIsNotAPNG(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": "GIF89a and the rest of it",
	})
	res := result(t, Run(in, true), "F02")
	if !res.Failed() {
		t.Fatal("a file that is not a PNG was accepted as a figure")
	}
}

func TestF03FindsAFileOverTheCap(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": noise(t, 800, 800),
	})
	res := result(t, Run(in, true), "F03")
	if len(res.Findings) != 1 {
		t.Fatalf("F03 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "KB") {
		t.Errorf("the finding does not say how big it is: %v", res.Findings[0])
	}
}

// A figure that was rendered and never added is a figure the site will not
// have, and the person who rendered it is the last one who will notice.
func TestF04FindsAFigureGitIsNotHolding(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})

	in.Tracked = nil
	if res := result(t, Run(in, true), "F04"); !res.NotRun {
		t.Error("F04 claimed a result outside a git checkout")
	}

	in.Tracked = []string{"manifests/figures.yaml"}
	res := result(t, Run(in, true), "F04")
	if len(res.Findings) != 1 {
		t.Fatalf("F04 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}

	in.Tracked = []string{"figures/vaswani-2017-attention/f01.png"}
	if res := result(t, Run(in, true), "F04"); res.Failed() {
		t.Errorf("F04 objected to a figure git is holding: %v", res.Findings)
	}
}

// The same diagram printed twice in one paper is one figure. Committing it
// twice gives a reader two names for one picture and translates its caption
// twice.
func TestF05FindsTheSameFigureTwiceInOnePaper(t *testing.T) {
	same := picture(t, 600, 400)
	in := corpusOf(t, []figures.Figure{
		record("vaswani-2017-attention", "f01"),
		record("vaswani-2017-attention", "f02"),
	}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": same,
		"figures/vaswani-2017-attention/f02.png": same,
	})
	res := result(t, Run(in, true), "F05")
	if len(res.Findings) != 1 {
		t.Fatalf("F05 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "f01.png") {
		t.Errorf("the finding does not say which one it duplicates: %v", res.Findings[0])
	}
}

// Two papers reprinting the same diagram are two papers, and each of them
// keeps it.
func TestF05LeavesTheSameFigureInTwoPapersAlone(t *testing.T) {
	same := picture(t, 600, 400)
	in := corpusOf(t, []figures.Figure{
		record("vaswani-2017-attention", "f01"),
		record("codd-1970-relational", "f01"),
	}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": same,
		"figures/codd-1970-relational/f01.png":   same,
	})
	if res := result(t, Run(in, true), "F05"); res.Failed() {
		t.Errorf("F05 objected to two papers carrying the same diagram: %v", res.Findings)
	}
}

// F06 is the line between a corpus and a mirror. A cropped diagram is a
// figure and a page image is a scan of somebody else's paper, and to git
// those two look identical.
func TestF06RefusesAPageImage(t *testing.T) {
	entry := record("vaswani-2017-attention", "f01")
	entry.Fraction = 0.92
	in := corpusOf(t, []figures.Figure{entry}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})
	res := result(t, Run(in, true), "F06")
	if len(res.Findings) != 1 {
		t.Fatalf("F06 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "92%") {
		t.Errorf("the finding does not say how much of the page it covers: %v", res.Findings[0])
	}
}

// The rule cannot be satisfied by leaving the field out. A figure with no
// fraction recorded is a figure nothing says is not a page image.
func TestF06RefusesAFigureWithNoFractionAtAll(t *testing.T) {
	entry := record("vaswani-2017-attention", "f01")
	entry.Fraction = 0
	in := corpusOf(t, []figures.Figure{entry}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})
	if res := result(t, Run(in, true), "F06"); !res.Failed() {
		t.Fatal("a figure with no page_fraction passed F06")
	}
}

func TestF06TakesAFigureExactlyAtTheCap(t *testing.T) {
	entry := record("vaswani-2017-attention", "f01")
	entry.Fraction = figures.MaxFraction
	in := corpusOf(t, []figures.Figure{entry}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})
	if res := result(t, Run(in, true), "F06"); res.Failed() {
		t.Errorf("a figure exactly at the cap was refused: %v", res.Findings)
	}
}

// A figure with no entry has no caption, so the reading app has nothing to
// put under it and the translators have nothing to translate.
func TestF07FindsAFileWithNoEntry(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
		"figures/vaswani-2017-attention/f02.png": picture(t, 500, 400),
	})
	res := result(t, Run(in, true), "F07")
	if len(res.Findings) != 1 {
		t.Fatalf("F07 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.HasSuffix(res.Findings[0].File, "f02.png") {
		t.Errorf("F07 named the wrong file: %v", res.Findings[0])
	}
}

func TestF07FindsAnEntryWithNoCaption(t *testing.T) {
	entry := record("vaswani-2017-attention", "f01")
	entry.Caption = "  "
	in := corpusOf(t, []figures.Figure{entry}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})
	res := result(t, Run(in, true), "F07")
	if len(res.Findings) != 1 {
		t.Fatalf("F07 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "caption") {
		t.Errorf("the finding does not say what is missing: %v", res.Findings[0])
	}
}

// A restricted paper is one the corpus may describe and may not reproduce,
// and a diagram is the part of a paper its publisher is most protective of.
func TestF08RefusesEveryFigureOfARestrictedPaper(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("codd-1970-relational", "f01")}, map[string]string{
		"manifests/sources.yaml": `sources:
  - id: codd-1970-relational
    access: restricted
    licence: all rights reserved
`,
		"figures/codd-1970-relational/f01.png": picture(t, 600, 400),
	})
	res := result(t, Run(in, true), "F08")
	if len(res.Findings) != 2 {
		t.Fatalf("F08 found %d things, want the file and the entry: %v", len(res.Findings), res.Findings)
	}
}

func TestF08StandsDownWhenNoPaperIsRestricted(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
	})
	if res := result(t, Run(in, true), "F08"); !res.NotRun {
		t.Errorf("F08 claimed a result with no restricted paper in the corpus: %v", res.Findings)
	}
}

// Soft, because it cannot tell a figure the detector dropped from a sentence
// citing another paper's figure. Its job is to say so and let a person look.
func TestF09NoticesAFigureThePaperTalksAboutAndDoesNotHave(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png": picture(t, 600, 400),
		"content/en/vaswani-2017-attention/02_section.md": front("vaswani-2017-attention", "section",
			"Figure 1 shows the model and Fig. 4 shows the results."),
	})
	res := result(t, Run(in, false), "F09")
	if len(res.Findings) != 1 {
		t.Fatalf("F09 found %d things, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "Figure 4") {
		t.Errorf("F09 named the wrong figure: %v", res.Findings[0])
	}
}

func TestF09IsSoft(t *testing.T) {
	for _, r := range Rules() {
		if r.ID == "F09" && r.Hard {
			t.Fatal("F09 is hard, and it cannot tell its two causes apart")
		}
	}
}

// The translations are the same prose in another language and mention the
// same figures. Reading them would report every missing figure once per
// language, which is the same finding four times.
func TestF09ReadsTheEnglishOnly(t *testing.T) {
	in := corpusOf(t, []figures.Figure{record("vaswani-2017-attention", "f01")}, map[string]string{
		"figures/vaswani-2017-attention/f01.png":          picture(t, 600, 400),
		"content/vi/vaswani-2017-attention/02_section.md": "---\npaper: vaswani-2017-attention\nkind: section\nlang: vi\n---\n\nFigure 4 is mentioned here too.\n",
	})
	if res := result(t, Run(in, false), "F09"); res.Failed() {
		t.Errorf("F09 read a translation: %v", res.Findings)
	}
}
