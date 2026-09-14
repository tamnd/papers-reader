package roundtrip

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/translate"
)

// pages is a small corpus: two papers at the head of the canon, one added
// later, three languages, a front file and some sections each.
func pages() []Page {
	var out []Page
	add := func(paper string, number int, names ...string) {
		for _, l := range []corpus.Lang{corpus.VI, corpus.ZH, corpus.JA} {
			for _, name := range names {
				kind := "section"
				switch {
				case strings.HasSuffix(name, "front.md"):
					kind = "front"
				case strings.HasSuffix(name, "references.md"):
					kind = "references"
				}
				out = append(out, Page{
					Paper: paper, Lang: l, Name: name, Kind: kind,
					Number: number, Model: "gpt-5",
				})
			}
		}
	}
	add("a-1970-paper", 1, "00_front.md", "01_one.md", "02_two.md", "09_references.md")
	add("b-1971-paper", 40, "00_front.md", "01_one.md", "02_two.md", "09_references.md")
	add("c-2020-paper", 0, "00_front.md", "01_one.md", "02_two.md", "09_references.md")
	return out
}

func TestEveryAbstractIsChecked(t *testing.T) {
	got := map[string]bool{}
	for _, s := range Pick(pages(), Default("seed")) {
		got[s.Path()] = true
	}
	for _, l := range []corpus.Lang{corpus.VI, corpus.ZH, corpus.JA} {
		for _, p := range []string{"a-1970-paper", "b-1971-paper", "c-2020-paper"} {
			path := fmt.Sprintf("content/%s/%s/00_front.md", l, p)
			if !got[path] {
				t.Errorf("%s is an abstract and was not checked", path)
			}
		}
	}
}

func TestAPaperAtTheHeadOfTheCanonIsCheckedWhole(t *testing.T) {
	n := 0
	for _, s := range Pick(pages(), Default("seed")) {
		if s.Paper == "a-1970-paper" {
			n++
		}
	}
	// Three languages, a front file and two sections each, and the
	// bibliography is not checked.
	if n != 9 {
		t.Errorf("paper 1 of the canon had %d pages checked and it has 9 worth checking", n)
	}
}

// A bibliography is copied rather than asked for, which is rule L14. There
// is nothing in it a back-translation could disagree with, and two asks a
// page is too much to spend finding that out.
func TestABibliographyIsNeverChecked(t *testing.T) {
	for _, s := range Pick(pages(), Policy{Top: 100, Rate: 100, Seed: "seed"}) {
		if strings.HasSuffix(s.Name, "references.md") {
			t.Errorf("%s is a bibliography and was checked anyway", s.Path())
		}
	}
}

// The sample has to be the same sample on Tuesday as it was on Monday, or a
// clean run after a prompt fix says nothing about whether the fix worked.
func TestTheSampleDoesNotMoveBetweenRuns(t *testing.T) {
	first, second := Pick(pages(), Default("seed")), Pick(pages(), Default("seed"))
	if len(first) != len(second) {
		t.Fatalf("two runs of one policy picked %d and %d pages", len(first), len(second))
	}
	for i := range first {
		if first[i].Path() != second[i].Path() {
			t.Fatalf("two runs of one policy picked %s and %s", first[i].Path(), second[i].Path())
		}
	}
}

// It does have to move when the prompt does. The sample is refreshed by
// changing the seed, and nothing else refreshes it.
func TestANewPromptDrawsANewSample(t *testing.T) {
	// A wide corpus, because a five per cent sample of twelve sections is
	// nearly always empty whatever the seed.
	var wide []Page
	for i := range 400 {
		wide = append(wide, Page{
			Paper: fmt.Sprintf("p-%03d", i), Lang: corpus.VI, Name: "01_one.md", Kind: "section",
		})
	}
	was := map[string]bool{}
	for _, s := range Pick(wide, Default("the old prompt")) {
		was[s.Path()] = true
	}
	now := 0
	for _, s := range Pick(wide, Default("the new prompt")) {
		if !was[s.Path()] {
			now++
		}
	}
	if now == 0 {
		t.Error("a changed prompt drew exactly the sample the old one did")
	}
}

// Five per cent has to be about five per cent. A draw that is uniform in
// theory and clumped in practice would check the same corner of the corpus
// every time.
func TestTheRateIsAboutTheRateAsked(t *testing.T) {
	var wide []Page
	for i := range 2000 {
		wide = append(wide, Page{
			Paper: fmt.Sprintf("p-%04d", i), Lang: corpus.VI, Name: "01_one.md", Kind: "section",
		})
	}
	got := len(Pick(wide, Default("seed")))
	if got < 80 || got > 120 {
		t.Errorf("a 5%% sample of 2000 pages picked %d, and 100 was asked for", got)
	}
}

func TestTheJudgeIsRead(t *testing.T) {
	for _, c := range []struct {
		name    string
		answer  string
		verdict Verdict
		count   int
	}{
		{
			name:    "the plain answer",
			answer:  "verdict: same",
			verdict: Same,
		},
		{
			name: "a verdict with differences under it",
			answer: "verdict: differs-materially\n" +
				"- the original says the bound holds for every input and the back-translation says it holds for some\n" +
				"- the original's second sentence has no counterpart",
			verdict: Material,
			count:   2,
		},
		{
			name:    "a judge that decided the verdict was prose",
			answer:  "**verdict:** differs-in-wording\n- approach became method",
			verdict: Wording,
			count:   1,
		},
		{
			name:    "a judge that opened with a sentence",
			answer:  "Comparing the two passages.\n\nverdict: same\n",
			verdict: Same,
		},
		{
			// Believing the list rather than the label is the conservative
			// reading, and it is the one that puts the page in front of a
			// person rather than filing it as clean.
			name:    "a judge contradicting itself",
			answer:  "verdict: same\n- the original says at least and the back-translation says more than",
			verdict: Wording,
			count:   1,
		},
		{
			// The case the first run over the corpus was full of: a page
			// labelled material under a list the judge itself had tagged as
			// wording, every line of it. The list is the evidence and the
			// label is not.
			name: "a material label over a list of wording",
			answer: "verdict: differs-materially\n" +
				"- wording: the original says dramatically and the back-translation says significantly\n" +
				"- wording: the original says extend and the back-translation says is an extension",
			verdict: Wording,
			count:   2,
		},
		{
			// One tagged line is enough, and the rest of the list being
			// wording does not soften it.
			name: "one material line among the wording",
			answer: "verdict: differs-in-wording\n" +
				"- material: the original says the optimum is a minimum and the back-translation says a maximum\n" +
				"- wording: the original says several and the back-translation says many",
			verdict: Material,
			count:   2,
		},
		{
			// A judge that ignored the tagging is a judge whose label is all
			// there is, and taking it is the conservative reading.
			name: "a list the judge did not tag",
			answer: "verdict: differs-materially\n" +
				"- the original says every input and the back-translation says some",
			verdict: Material,
			count:   1,
		},
		{
			// Half a tagged list is not a tagged list. One untagged line
			// could be the material one the judge forgot to mark.
			name: "a list the judge tagged halfway",
			answer: "verdict: differs-materially\n" +
				"- wording: the original says dramatically and the back-translation says significantly\n" +
				"- the original's second sentence has no counterpart",
			verdict: Material,
			count:   2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			v, differences, err := Parse(c.answer)
			if err != nil {
				t.Fatal(err)
			}
			if v != c.verdict {
				t.Errorf("read %s and the judge said %s", v, c.verdict)
			}
			if len(differences) != c.count {
				t.Errorf("read %d differences and there are %d: %v", len(differences), c.count, differences)
			}
		})
	}
}

// An answer with no verdict in it, or with two, is not a result. Writing
// "same" down and carrying on is how a check that stopped working keeps
// reporting that everything is fine.
func TestAJudgeThatDidNotJudgeIsAnError(t *testing.T) {
	for _, c := range []struct{ name, answer string }{
		{"no verdict at all", "The two passages are broadly similar."},
		{"a verdict that is not one of the three", "verdict: mostly fine"},
		{"two verdicts", "verdict: same\nverdict: differs-materially"},
		{"nothing at all", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if v, _, err := Parse(c.answer); err == nil {
				t.Errorf("read %q as a verdict of %s", c.answer, v)
			}
		})
	}
}

// ask is a fleet of one that answers with whatever it is told to.
func ask(answers ...string) (func(context.Context, string, string, llm.Request) (translate.Reply, error), *[]string) {
	var avoided []string
	i := 0
	return func(_ context.Context, _, avoid string, _ llm.Request) (translate.Reply, error) {
		avoided = append(avoided, avoid)
		if i >= len(answers) {
			return translate.Reply{}, fmt.Errorf("asked more times than there are answers")
		}
		a := answers[i]
		i++
		return translate.Reply{
			Response: llm.Response{Text: a, Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
			Model:    "a-different-model",
		}, nil
	}, &avoided
}

func sample() Sample {
	return Sample{Paper: "a-1970-paper", Lang: corpus.VI, Name: "01_one.md", Why: "a test", Model: "gpt-5"}
}

func TestACheckIsTwoAsksAndAVerdict(t *testing.T) {
	fn, _ := ask("The bound holds for every input.", "verdict: same")
	c := &Checker{Ask: fn}

	got, err := c.Run(context.Background(), sample(), translate.Paper{ID: "a-1970-paper"},
		"The bound holds for every input.\n", "Giới hạn đúng với mọi đầu vào.\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != Same {
		t.Errorf("the verdict is %s", got.Verdict)
	}
	if got.Back != "The bound holds for every input." {
		t.Errorf("the back-translation is %q", got.Back)
	}
	if got.Usage.InputTokens != 20 || got.Usage.OutputTokens != 10 {
		t.Errorf("two asks cost %d in and %d out", got.Usage.InputTokens, got.Usage.OutputTokens)
	}
}

// The model that wrote the translation is the one model whose opinion of it
// is worth least, so both asks say who to steer away from.
func TestBothAsksAreSteeredAwayFromTheModelThatTranslated(t *testing.T) {
	fn, avoided := ask("The bound holds.", "verdict: same")
	c := &Checker{Ask: fn}

	got, err := c.Run(context.Background(), sample(), translate.Paper{ID: "a-1970-paper"},
		"The bound holds.\n", "Giới hạn đúng.\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(*avoided) != 2 || (*avoided)[0] != "gpt-5" {
		t.Errorf("the asks were steered away from %v", *avoided)
	}
	if (*avoided)[1] != "a-different-model" {
		t.Errorf("the judge was steered away from %q, and it should be the model that did the back-translation", (*avoided)[1])
	}
	if got.SameModel {
		t.Error("the check says it was a model marking its own work and neither ask went to that model")
	}
}

// When the fleet has nothing else, the check runs anyway and says what it
// is worth. A self-check still catches a dropped paragraph.
func TestAModelMarkingItsOwnWorkIsRecorded(t *testing.T) {
	c := &Checker{Ask: func(_ context.Context, _, _ string, _ llm.Request) (translate.Reply, error) {
		return translate.Reply{
			Response: llm.Response{Text: "verdict: same"},
			Model:    "gpt-5",
		}, nil
	}}

	got, err := c.Run(context.Background(), sample(), translate.Paper{ID: "a-1970-paper"},
		"The bound holds.\n", "Giới hạn đúng.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !got.SameModel {
		t.Error("the same model did both halves and the check does not say so")
	}
}

func TestAnEmptyBackTranslationIsAnError(t *testing.T) {
	fn, _ := ask("   ", "verdict: same")
	c := &Checker{Ask: fn}

	if _, err := c.Run(context.Background(), sample(), translate.Paper{ID: "a-1970-paper"},
		"The bound holds.\n", "Giới hạn đúng.\n"); err == nil {
		t.Error("a page whose back-translation came back empty was judged anyway")
	}
}

func TestThePageIsInTheReportWithBothEnglishes(t *testing.T) {
	rep := &Report{
		Run:    "20260913T000000Z",
		Policy: Default("seed"),
		Checks: []Check{{
			Sample:      sample(),
			English:     "The bound holds for every input.",
			Back:        "The bound holds for some inputs.",
			Verdict:     Material,
			Differences: []string{"the original says every and the back-translation says some"},
			BackModel:   "gpt-6", JudgeModel: "gpt-6",
		}},
	}
	md := rep.Markdown()
	for _, want := range []string{
		"Differs materially",
		"content/vi/a-1970-paper/01_one.md",
		"> The bound holds for every input.",
		"> The bound holds for some inputs.",
		"the original says every and the back-translation says some",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not hold %q", want)
		}
	}
}

// The report is committed to a public repository and the hosts are not
// public. The models are, and they are the part worth naming.
func TestTheReportNamesNoHosts(t *testing.T) {
	rep := &Report{
		Run:    "20260913T000000Z",
		Policy: Default("seed"),
		Checks: []Check{{Sample: sample(), Verdict: Same, BackModel: "gpt-6", JudgeModel: "gpt-6"}},
		Failed: []string{"`content/vi/a-1970-paper/02_two.md` the fleet had nothing to answer with"},
	}
	md := rep.Markdown()
	for _, no := range []string{"server1", "server2", "server3", "/Users/", "/home/", "127.0.0.1", "http"} {
		if strings.Contains(md, no) {
			t.Errorf("the report holds %q, and it is committed", no)
		}
	}
}

// A page the fleet could not answer for is not a page that passed, and the
// count in the summary has to say so rather than quietly shrinking.
func TestAPageThatCouldNotBeCheckedIsInTheSummary(t *testing.T) {
	rep := &Report{Failed: []string{"`content/vi/a-1970-paper/01_one.md` the host went away"}}
	if !strings.Contains(rep.Summary(), "1 could not be checked") {
		t.Errorf("the summary is %q", rep.Summary())
	}
}
