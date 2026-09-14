package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/ledger"
	"github.com/tamnd/papers-reader/report"
)

func TestAReportOfNothingDoesNotOverwriteAReportOfSomething(t *testing.T) {
	// The ledger is on the machine that did the work and the report is in
	// the corpus, so a checkout on a second machine has one and not the
	// other. Rebuilding there would look like a stage that had been
	// reverted.
	path := filepath.Join(t.TempDir(), "usage.md")
	night := report.BuildUsage([]ledger.Entry{{
		TS: time.Now().UTC(), Stage: "extract", Target: "codd-1970-relational p1",
		Model: "a model", OK: true, Usage: llm.Usage{InputTokens: 10},
	}}, report.UsageOptions{})
	if err := os.WriteFile(path, []byte(night.Markdown()), 0o644); err != nil {
		t.Fatal(err)
	}

	empty := report.BuildUsage(nil, report.UsageOptions{Stages: []string{"extract"}})
	if err := keep(path, empty, false); err == nil {
		t.Error("a report of nothing was written over a night of work")
	}
	if err := keep(path, empty, true); err != nil {
		t.Errorf("-force did not overwrite it: %v", err)
	}
	// A report of nothing over a report of nothing is not a loss, and a
	// first run has nothing to overwrite at all.
	if err := os.WriteFile(path, []byte(empty.Markdown()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := keep(path, empty, false); err != nil {
		t.Errorf("rewriting an empty report was refused: %v", err)
	}
	if err := keep(filepath.Join(t.TempDir(), "usage.md"), empty, false); err != nil {
		t.Errorf("the first run was refused: %v", err)
	}
	if err := keep(path, night, false); err != nil {
		t.Errorf("writing a night of work was refused: %v", err)
	}
}

func TestTheWindowIsADateOrAWindow(t *testing.T) {
	got, err := when("2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.Format(time.DateOnly) != "2026-09-01" {
		t.Errorf("-since 2026-09-01 is %v", got)
	}
	got, err = when("168h")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(got); d < 167*time.Hour || d > 169*time.Hour {
		t.Errorf("-since 168h is %v ago", d)
	}
	if got, err := when(""); err != nil || !got.IsZero() {
		t.Errorf("no window is %v %v, want the whole ledger", got, err)
	}
	if _, err := when("last tuesday"); err == nil {
		t.Error("last tuesday was accepted")
	}
}

// reportCorpus is tagsCorpus with the licence records the audit reads. The
// tags tests do not need them and this one does, because report all runs
// the whole audit and group S is about the licences.
func reportCorpus(t *testing.T, body string) string {
	t.Helper()
	root := tagsCorpus(t, body)
	const sources = `sources:
  - id: a-1970-paper
    access: open
    licence: CC-BY-4.0
    url: https://example.org/a.pdf
    text_layer: native
`
	if err := os.WriteFile(filepath.Join(root, "manifests", "sources.yaml"), []byte(sources), 0o644); err != nil {
		t.Fatal(err)
	}
	const collections = `collections:
  - id: canon-100
    title: The hundred
    description: The seed list, in its own numbering.
    order: number
    members: all-with-number
`
	if err := os.WriteFile(filepath.Join(root, "manifests", "collections.yaml"), []byte(collections), 0o644); err != nil {
		t.Fatal(err)
	}
	// The ledger is on the machine that did the work and this is not that
	// machine, so the usage report has nothing to build from. The other
	// three still have to be written, which is the case a second checkout
	// is in and the one worth proving.
	t.Setenv("LLM_LEDGER", filepath.Join(t.TempDir(), "ledger.jsonl"))
	return root
}

// The point of report all is that four commands become one, so the test is
// that all four files are there afterwards and that none of them is the
// stub a failed build would leave.
func TestReportAllWritesEveryReport(t *testing.T) {
	root := reportCorpus(t, section(""))
	if err := runReportAll([]string{"-corpus", root}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"coverage.md": "# Coverage",
		"graph.md":    "# The citation graph",
		"audit.md":    "rules",
		"usage.md":    "",
	} {
		b, err := os.ReadFile(filepath.Join(root, "reports", name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if want != "" && !strings.Contains(string(b), want) {
			t.Errorf("%s does not read like itself:\n%s", name, b)
		}
	}
}

// A corpus with a hard rule failing still gets its reports. A report that
// refused to be written while the corpus had a problem in it would be a
// report nobody could use to diagnose the problem.
func TestReportAllDoesNotFailOnAFinding(t *testing.T) {
	root := reportCorpus(t, section(""))
	// A link to a paper that is not in papers.yaml, which is rule R01.
	body := section("") + "\nAnd a paragraph that cites [[no-such-1970-paper]] and nothing else.\n"
	if err := os.WriteFile(filepath.Join(root, "content/en/a-1970-paper/01_first.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runReportAll([]string{"-corpus", root}); err != nil {
		t.Fatalf("a finding failed the report: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "reports", "audit.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "no-such-1970-paper") {
		t.Errorf("the audit report does not carry the finding:\n%s", b)
	}
}
