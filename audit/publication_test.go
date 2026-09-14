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
