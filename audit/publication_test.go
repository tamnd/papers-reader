package audit

import (
	"strings"
	"testing"
)

// covered is a glossary that covers both translated languages the corpus
// offers, so nothing is a draft unless a test says otherwise.
const covered = `version: 2
terms:
  - en: attention
    vi: chú ý
    zh: 注意力
    ja: 注意
  - en: model
    vi: mô hình
    zh: 模型
    ja: モデル
`

// halfCovered leaves Japanese at half the terms, which is under the floor.
const halfCovered = `version: 2
terms:
  - en: attention
    vi: chú ý
    zh: 注意力
    ja: 注意
  - en: model
    vi: mô hình
    zh: 模型
`

func TestP05PassesOnACorpusThatEmitsCleanly(t *testing.T) {
	res := result(t, Run(build(t, map[string]string{"manifests/glossary.yaml": covered}), true), "P05")
	if res.Failed() {
		t.Errorf("P05 failed on a corpus with nothing wrong with it: %v", res.Findings)
	}
}

// The two papers in the test manifest have no sources record, which makes
// them unknown, which is a real access class and a valid one to emit. A
// rule that could not build an empty corpus would never run on a new one.
func TestP05RunsOnACorpusWithNothingInIt(t *testing.T) {
	res := result(t, Run(build(t, nil), true), "P05")
	if res.NotRun {
		t.Error("P05 stood down on a corpus it could perfectly well build")
	}
	if res.Failed() {
		t.Errorf("P05 failed on an empty corpus: %v", res.Findings)
	}
}

// The identifier pattern in the schema is the sharpest thing in it, so a
// manifest entry that is not one is the way to prove the rule is actually
// validating rather than reporting success from a compile it never ran.
func TestP05FindsACorpusThatWouldNotRender(t *testing.T) {
	in := build(t, map[string]string{"manifests/papers.yaml": `papers:
  - id: Vaswani_2017
    title: Attention Is All You Need
    authors: [Ashish Vaswani]
    year: 2017
    field: ai-ml
    status: listed
`})
	res := result(t, Run(in, true), "P05")
	if !res.Failed() {
		t.Fatal("P05 passed a build the app could not key off")
	}
	joined := strings.Join(messages(res), " ")
	if !strings.Contains(joined, "/papers/0/id") {
		t.Errorf("the finding does not say where it is: %v", res.Findings)
	}
	// Both documents are built from the same manifest, so a bad identifier
	// is wrong in both of them and the rule reports it twice rather than
	// stopping at the first.
	if !strings.Contains(joined, "index.json") || !strings.Contains(joined, "graph.json") {
		t.Errorf("only one document was validated: %v", res.Findings)
	}
}

func TestP04PassesWhenEveryLanguageIsCovered(t *testing.T) {
	res := result(t, Run(build(t, map[string]string{"manifests/glossary.yaml": covered}), false), "P04")
	if res.Failed() {
		t.Errorf("P04 failed with a glossary that covers everything: %v", res.Findings)
	}
}

// The emitter marks a language under the floor as a draft, so with the
// emitter working the rule passes. What it is guarding is the two drifting
// apart: if the emitter ever stops marking one, this is what says so.
func TestP04AgreesWithTheEmitterAboutWhatIsADraft(t *testing.T) {
	res := result(t, Run(build(t, map[string]string{"manifests/glossary.yaml": halfCovered}), false), "P04")
	if res.Failed() {
		t.Errorf("P04 failed on a language the emitter does mark a draft: %v", res.Findings)
	}
}

// A corpus with no glossary has nothing to be under a floor, which is a
// corpus before anybody has started rather than a language in trouble.
func TestP04StandsDownWithNoGlossary(t *testing.T) {
	if !result(t, Run(build(t, nil), false), "P04").NotRun {
		t.Error("P04 claimed a result on a corpus with no glossary")
	}
}

// messages is a result's findings as plain strings, for a test that wants
// to look for a phrase across all of them.
func messages(res Result) []string {
	out := make([]string, 0, len(res.Findings))
	for _, f := range res.Findings {
		out = append(out, f.String())
	}
	return out
}

// page writes one content file with a body in it, for the three rules that
// are about what a built page refers to.
func page(paper, number, text string) string {
	return "---\npaper: " + paper + "\ntitle: Attention Is All You Need\nkind: section\nlang: en\n" +
		"section: \"" + number + "\"\nsection_title: A section\n---\n\n" + text
}

// The three fault rules stand down on a corpus with no content, because a
// build with no pages in it is a corpus nobody has extracted yet and not a
// corpus with nothing wrong with it.
func TestTheFaultRulesStandDownWithNoPages(t *testing.T) {
	rep := Run(build(t, nil), true)
	for _, id := range []string{"P01", "P02", "P03"} {
		if !result(t, rep, id).NotRun {
			t.Errorf("%s claimed a result on a corpus with no pages", id)
		}
	}
}

func TestTheFaultRulesPassOnAPageThatIsAllThere(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/01_method.md": page("vaswani-2017-attention", "1",
			"The rule is\n\n$$E = \\tfrac{1}{2}\\sum_j (y_j - d_j)^2$$\n\nand it holds [^1].\n\n[^1]: Under the usual conditions.\n"),
	})
	rep := Run(in, true)
	for _, id := range []string{"P01", "P02", "P03"} {
		if res := result(t, rep, id); res.Failed() {
			t.Errorf("%s failed on a page with nothing wrong with it: %v", id, res.Findings)
		}
	}
}

// P01 is the formulas and the markup. A formula KaTeX will not take is not
// a failed build, because a paper with one unreadable formula is worth
// reading, but it is a hard audit failure, because a corpus whose point is
// that the mathematics came through intact cannot ship it as a warning.
func TestP01FindsAFormulaThatWillNotRender(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/01_method.md": page("vaswani-2017-attention", "1",
			"$$\\notacommand{x}$$\n"),
	})
	res := result(t, Run(in, true), "P01")
	if !res.Failed() {
		t.Fatal("P01 passed a formula KaTeX refuses")
	}
	if !strings.Contains(strings.Join(messages(res), " "), "KaTeX refused") {
		t.Errorf("the finding does not say what happened: %v", res.Findings)
	}
	if res.Findings[0].File != "p/vaswani-2017-attention/en.json" {
		t.Errorf("the finding names %q", res.Findings[0].File)
	}
}

// P02 is the links. A footnote marker with no definition anywhere in the
// paper is set as a bare number rather than as a link to nowhere, and this
// is the rule that says the paper is missing a footnote.
func TestP02FindsALinkWithNothingAtTheOtherEnd(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/01_method.md": page("vaswani-2017-attention", "1",
			"The result holds [^4], as shown in [[nobody-1999-nothing]].\n"),
	})
	res := result(t, Run(in, true), "P02")
	if !res.Failed() {
		t.Fatal("P02 passed a page full of links to nowhere")
	}
	joined := strings.Join(messages(res), " ")
	if !strings.Contains(joined, "footnote marker 4") || !strings.Contains(joined, "nobody-1999-nothing") {
		t.Errorf("the findings are %v", res.Findings)
	}
	// The same page's formulas and figures are fine, so the other two rules
	// have nothing to say. Three rules over one list is only worth it if
	// each one reports its own pile.
	if result(t, Run(in, true), "P01").Failed() {
		t.Error("P01 reported a broken link")
	}
}

// P03 is the pictures a page actually shows. A caption the figures pass has
// not reached yet degrades to a paragraph and is rule F09's business, so
// what this rule is about is the other case: the manifest says the picture
// exists and it is not there, which is the one thing that would render as a
// broken image in a browser.
func TestP03PassesACaptionTheFiguresPassHasNotReached(t *testing.T) {
	const caption = "Figure 1: The architecture.\n{#vaswani-2017-attention-fig-1 .figure tag=00a1}\n"
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/01_method.md": page("vaswani-2017-attention", "1", caption),
	})
	if res := result(t, Run(in, true), "P03"); res.Failed() {
		t.Errorf("P03 failed a paper part way through the figures pass: %v", res.Findings)
	}
}

func TestP03FindsAFigureThatIsNotInTheBuild(t *testing.T) {
	const caption = "Figure 1: The architecture.\n{#vaswani-2017-attention-fig-1 .figure tag=00a1}\n"
	listed := build(t, map[string]string{
		"content/en/vaswani-2017-attention/01_method.md": page("vaswani-2017-attention", "1", caption),
		"manifests/figures.yaml": `figures:
  - paper: vaswani-2017-attention
    figure: fig-1
    number: "1"
    page: 3
    bbox: [10, 10, 200, 120]
    caption: The architecture.
    sha256: 0000000000000000000000000000000000000000000000000000000000000000
    method: crop
    page_fraction: 0.2
    width: 760
    height: 480
    bytes: 40960
`,
	})
	res := result(t, Run(listed, true), "P03")
	if !res.Failed() {
		t.Fatal("P03 passed a figure the manifest lists and nobody rendered")
	}
	if !strings.Contains(strings.Join(messages(res), " "), "is not in the corpus") {
		t.Errorf("the findings are %v", res.Findings)
	}
}

// All three are hard, because a page that refers to something that is not
// there renders as a gap in a paper.
func TestTheFaultRulesAreHard(t *testing.T) {
	for _, r := range Rules() {
		switch r.ID {
		case "P01", "P02", "P03":
			if !r.Hard {
				t.Errorf("%s is soft", r.ID)
			}
		}
	}
}
