package main

import (
	"os"
	"path/filepath"
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
