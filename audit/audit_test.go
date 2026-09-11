package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

const papersYAML = `papers:
  - id: vaswani-2017-attention
    title: Attention Is All You Need
    authors: [Ashish Vaswani]
    year: 2017
    field: ai-ml
    status: listed
  - id: codd-1970-relational
    title: A Relational Model of Data for Large Shared Data Banks
    authors: [E. F. Codd]
    year: 1970
    field: databases
    status: listed
`

// build writes a corpus and loads it. Files are given as paths relative to
// the corpus root, so a test reads as a description of what is on disk.
func build(t *testing.T, files map[string]string) *Input {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{
		"manifests/papers.yaml":      papersYAML,
		"manifests/collections.yaml": "collections: []\n",
		"manifests/sources.yaml":     "sources: []\n",
	}
	for k, v := range files {
		all[k] = v
	}
	for path, body := range all {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	in, err := Load(c)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// result finds one rule's result in a report.
func result(t *testing.T, rep *Report, id string) Result {
	t.Helper()
	for _, r := range rep.Results {
		if r.Rule.ID == id {
			return r
		}
	}
	t.Fatalf("rule %s did not run at all", id)
	return Result{}
}

func TestRulesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Rules() {
		if seen[r.ID] {
			t.Errorf("rule %s is listed twice", r.ID)
		}
		seen[r.ID] = true
		if r.Group().Title() == "" {
			t.Errorf("rule %s is in group %q, which has no title", r.ID, r.Group())
		}
		if r.What == "" {
			t.Errorf("rule %s does not say what it checks", r.ID)
		}
		if !strings.HasSuffix(r.What, ".") {
			t.Errorf("rule %s is not written as a sentence: %q", r.ID, r.What)
		}
		if r.Check == nil {
			t.Errorf("rule %s has nothing to run", r.ID)
		}
	}
}

// A rule with nothing to look at reports "not run", which is neither a pass
// nor a failure. This is the distinction the whole report is built on: a
// corpus where every rule is skipped must not read as a clean corpus.
func TestAFreshCorpusRunsAlmostNothing(t *testing.T) {
	rep := Run(build(t, nil), false)
	if rep.HardFailures() != 0 {
		t.Errorf("a fresh corpus failed %d hard rules", rep.HardFailures())
	}
	if len(rep.Errors()) != 0 {
		t.Errorf("rules broke: %v", rep.Errors())
	}
	for _, id := range []string{"S02", "S04", "S06", "G01", "G02", "G03"} {
		if !result(t, rep, id).NotRun {
			t.Errorf("%s claims to have run on a corpus with nothing in it", id)
		}
	}
	if !strings.Contains(rep.Summary(), "not run") {
		t.Errorf("the summary hides the skipped rules: %s", rep.Summary())
	}
}

// S01 is the rule that makes not having looked the same as not publishing.
func TestS01RefusesContentWithoutAnAccessClass(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/00_front.md": "---\npaper: vaswani-2017-attention\nkind: front\nlang: en\n---\n\ntext\n",
	})
	res := result(t, Run(in, true), "S01")
	if !res.Failed() {
		t.Fatal("a paper with no access class was allowed to have content")
	}
	if !strings.Contains(res.Findings[0].File, "content/en/vaswani-2017-attention") {
		t.Errorf("the finding does not name the file: %+v", res.Findings[0])
	}
}

func TestS01PassesOnceThePaperIsOpen(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml": `sources:
  - id: vaswani-2017-attention
    access: open
    licence: arXiv non-exclusive licence to distribute
    url: https://arxiv.org/pdf/1706.03762v7
`,
		"content/en/vaswani-2017-attention/00_front.md": "---\npaper: vaswani-2017-attention\nkind: front\nlang: en\n---\n\ntext\n",
	})
	rep := Run(in, true)
	if res := result(t, rep, "S01"); res.Failed() {
		t.Errorf("an open paper was refused its content: %v", res.Findings)
	}
	if res := result(t, rep, "S06"); res.Failed() {
		t.Errorf("a licence was recorded and S06 still failed: %v", res.Findings)
	}
}

// A restricted paper gets the bibliographic record and a short abstract. No
// body text, no figures, whatever else is lying around.
func TestS02HoldsTheLineOnRestricted(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml": `sources:
  - id: codd-1970-relational
    access: restricted
    licence: all rights reserved
    landing: https://dl.acm.org/doi/10.1145/362384.362685
`,
		"content/en/codd-1970-relational/00_front.md":   "---\npaper: codd-1970-relational\nkind: front\nlang: en\n---\n\nabstract\n",
		"content/en/codd-1970-relational/01_section.md": "---\npaper: codd-1970-relational\nkind: section\nlang: en\n---\n\nbody\n",
		"content/vi/codd-1970-relational/01_section.md": "---\npaper: codd-1970-relational\nkind: section\nlang: vi\n---\n\nbody\n",
		"figures/codd-1970-relational/f01.png":          "not really a png",
	})
	res := result(t, Run(in, true), "S02")
	if len(res.Findings) != 3 {
		t.Fatalf("S02 found %d things, want 3: %v", len(res.Findings), res.Findings)
	}
	var figures int
	for _, f := range res.Findings {
		if strings.HasPrefix(f.File, "figures/") {
			figures++
		}
		if strings.HasSuffix(f.File, "00_front.md") {
			t.Error("S02 objected to the one file a restricted paper is allowed")
		}
	}
	if figures != 1 {
		t.Error("S02 let a figure of a restricted paper through")
	}
}

// S03 asks git what it is holding rather than reading .gitignore, because
// one `git add -f` is the difference between a corpus and a mirror.
func TestS03(t *testing.T) {
	in := build(t, nil)

	in.Tracked = nil
	if res := result(t, Run(in, true), "S03"); !res.NotRun {
		t.Error("S03 claimed a result outside a git checkout")
	}

	in.Tracked = []string{"manifests/papers.yaml", "README.md"}
	if res := result(t, Run(in, true), "S03"); res.Failed() || res.NotRun {
		t.Error("S03 objected to a clean index")
	}

	in.Tracked = []string{"pdf/vaswani-2017-attention.pdf", "docs/handout.PDF", "README.md"}
	res := result(t, Run(in, true), "S03")
	if len(res.Findings) != 2 {
		t.Errorf("S03 found %d PDFs, want 2: %v", len(res.Findings), res.Findings)
	}
}

func TestS04(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml": `sources:
  - id: vaswani-2017-attention
    access: open
    licence: arXiv non-exclusive licence to distribute
`,
	})
	res := result(t, Run(in, true), "S04")
	if len(res.Findings) != 1 {
		t.Fatalf("S04 found %d gaps, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "codd-1970-relational") {
		t.Errorf("S04 named the wrong paper: %s", res.Findings[0].Message)
	}
}

// An open paper with a URL and no licence is a paper somebody downloaded and
// nobody checked.
func TestS06(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml": `sources:
  - id: vaswani-2017-attention
    access: open
    url: https://arxiv.org/pdf/1706.03762v7
  - id: codd-1970-relational
    access: restricted
`,
	})
	res := result(t, Run(in, true), "S06")
	if len(res.Findings) != 1 {
		t.Fatalf("S06 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "vaswani-2017-attention") {
		t.Errorf("S06 named the wrong paper: %s", res.Findings[0].Message)
	}
}

func TestTagRules(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		rule  string
		fails bool
	}{
		{"a register that is fine", "0001,vaswani-2017-attention-s1\n0002,vaswani-2017-attention-s2\n", "G01", false},
		{"a tag that is not four hex", "00G1,vaswani-2017-attention-s1\n", "G01", true},
		{"a line that is not tag,anchor", "0001 vaswani-2017-attention-s1\n", "G01", true},
		{"the same tag twice", "0001,vaswani-2017-attention-s1\n0001,vaswani-2017-attention-s2\n", "G02", true},
		{"the same anchor twice", "0001,vaswani-2017-attention-s1\n0002,vaswani-2017-attention-s1\n", "G03", true},
	}
	for _, tc := range cases {
		in := build(t, map[string]string{"tags/tags": tc.file})
		res := result(t, Run(in, true), tc.rule)
		if res.Failed() != tc.fails {
			t.Errorf("%s: %s failed=%v, want %v (%v)", tc.name, tc.rule, res.Failed(), tc.fails, res.Findings)
		}
	}
}

func TestHardOnlySkipsTheSoftRules(t *testing.T) {
	in := build(t, nil)
	full := Run(in, false)
	hard := Run(in, true)
	if len(hard.Results) > len(full.Results) {
		t.Error("the hard run ran more rules than the full run")
	}
	for _, res := range hard.Results {
		if !res.Rule.Hard {
			t.Errorf("%s is soft and ran under --hard", res.Rule.ID)
		}
	}
}

func TestMarkdownSaysWhatHappened(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/00_front.md": "---\npaper: vaswani-2017-attention\nkind: front\nlang: en\n---\n\ntext\n",
	})
	md := Run(in, false).Markdown()
	for _, want := range []string{"# Audit", "## S Sources and licensing", "**S01**", "not run", "vaswani-2017-attention"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not contain %q", want)
		}
	}
}

func TestFindingString(t *testing.T) {
	cases := map[string]Finding{
		"S03: pdf/x.pdf: a PDF":   {Rule: "S03", File: "pdf/x.pdf", Message: "a PDF"},
		"T01: a.md:12: a heading": {Rule: "T01", File: "a.md", Line: 12, Message: "a heading"},
		"S04: no record anywhere": {Rule: "S04", Message: "no record anywhere"},
	}
	for want, f := range cases {
		if got := f.String(); got != want {
			t.Errorf("String is %q, want %q", got, want)
		}
	}
}
