package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
)

// shipped builds a corpus with one paper of three English sections in it and
// returns the shipment a run over it would keep.
func shipped(t *testing.T, langs []corpus.Lang) (*corpus.Corpus, *shipment) {
	t.Helper()
	root := t.TempDir()
	c := &corpus.Corpus{Root: root}
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		wrote(t, filepath.Join(c.Content(corpus.EN, "a-1970-paper"), name), "English.\n")
	}
	var jobs []job
	for _, l := range langs {
		for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
			jobs = append(jobs, job{lang: l, name: name, front: corpus.Front{Paper: "a-1970-paper"}})
		}
	}
	return c, newShipment(c, langs, jobs)
}

func wrote(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writing is what the run does to disk for one file, which is all the
// shipment reads.
func writing(t *testing.T, c *corpus.Corpus, l corpus.Lang, id, name string) {
	t.Helper()
	wrote(t, filepath.Join(c.Content(l, id), name), "Translated.\n")
}

func TestAPaperIsNotPushedUntilEveryFileOfItIsWritten(t *testing.T) {
	// This is the MapReduce case. Two of twelve sections went into the
	// corpus in Vietnamese and the other ten did not, and the paper sat
	// there looking finished until audit rules T04 and T06 said otherwise.
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	writing(t, c, corpus.VI, "a-1970-paper", "00_front.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("one file of three is %d files ready to push, want 0", s.pending())
	}
	if p := s.take(); p != nil {
		t.Errorf("one file of three would stage %v, want nothing", p)
	}

	writing(t, c, corpus.VI, "a-1970-paper", "01_first.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("two files of three is %d files ready to push, want 0", s.pending())
	}

	writing(t, c, corpus.VI, "a-1970-paper", "02_second.md")
	s.done("a-1970-paper", true)
	if s.pending() != 3 {
		t.Errorf("the whole paper is %d files ready to push, want 3", s.pending())
	}
}

func TestAWholePaperStagesItsOwnDirectoriesAndNothingElse(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI, corpus.JA})
	for _, l := range []corpus.Lang{corpus.VI, corpus.JA} {
		for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
			writing(t, c, l, "a-1970-paper", name)
			s.done("a-1970-paper", true)
		}
	}
	// A second paper the run is halfway through, which must not be caught
	// by the batch that the first one triggers.
	wrote(t, filepath.Join(c.Content(corpus.VI, "b-1980-paper"), "00_front.md"), "Half a paper.\n")
	for _, r := range []string{"manifests", "reports", "tags"} {
		wrote(t, filepath.Join(c.Root, r, "keep"), "")
	}

	got := s.take()
	want := []string{
		"content/vi/a-1970-paper",
		"content/ja/a-1970-paper",
		"manifests", "reports", "tags",
	}
	for _, w := range want {
		if !slices.Contains(got, filepath.FromSlash(w)) {
			t.Errorf("the batch does not stage %s: %v", w, got)
		}
	}
	for _, p := range got {
		if p == filepath.FromSlash("content/vi/b-1980-paper") || p == "content" {
			t.Errorf("the batch stages the half written paper: %v", got)
		}
	}
	// Emptied, so the next batch does not push the same paper twice.
	if s.pending() != 0 {
		t.Errorf("after a batch %d files are still waiting", s.pending())
	}
}

// A translation names the English file it answers and carries that file's
// hash, so a batch that stages the translation and not the English publishes
// files pointing at nothing. Rule L04 catches it after the fact. This is the
// gamma and ethernet case: twenty two Vietnamese files went in against an
// English directory holding one page of front matter.
func TestABatchStagesTheEnglishItsTranslationsAnswer(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		writing(t, c, corpus.VI, "a-1970-paper", name)
		s.done("a-1970-paper", true)
	}
	got := s.take()
	if !slices.Contains(got, filepath.FromSlash("content/en/a-1970-paper")) {
		t.Errorf("a Vietnamese batch does not stage the English it answers: %v", got)
	}
}

// English asked for once is English staged once, because git add is given
// these verbatim.
func TestABatchNamesTheEnglishOnceWhenItIsWritingIt(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.EN})
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		writing(t, c, corpus.EN, "a-1970-paper", name)
		s.done("a-1970-paper", true)
	}
	var n int
	for _, p := range s.take() {
		if p == filepath.FromSlash("content/en/a-1970-paper") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the batch stages the English %d times, want once", n)
	}
}

func TestAPaperWithAFileThatFailedIsHeldBack(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	writing(t, c, corpus.VI, "a-1970-paper", "00_front.md")
	s.done("a-1970-paper", true)
	writing(t, c, corpus.VI, "a-1970-paper", "01_first.md")
	s.done("a-1970-paper", true)
	// The third came back wrong six times and was given up on.
	s.done("a-1970-paper", false)

	if s.pending() != 0 {
		t.Errorf("a paper missing a section is %d files ready to push, want 0", s.pending())
	}
	if len(s.held) != 1 || s.held[0] != "a-1970-paper" {
		t.Errorf("the paper was not reported as held: %v", s.held)
	}
	// The two files that were written are still on disk, for the next run
	// to finish rather than to write again.
	if _, err := os.Stat(filepath.Join(c.Content(corpus.VI, "a-1970-paper"), "00_front.md")); err != nil {
		t.Errorf("holding the paper back threw away what was written: %v", err)
	}
}

func TestAPaperWhoseTranslationIsMissingAFileIsNotWhole(t *testing.T) {
	// The run is owed one file, writes it, and the paper is still short
	// because an earlier run left a hole in it. Counting what this run owed
	// would push it; reading the directory does not.
	root := t.TempDir()
	c := &corpus.Corpus{Root: root}
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		wrote(t, filepath.Join(c.Content(corpus.EN, "a-1970-paper"), name), "English.\n")
	}
	s := newShipment(c, []corpus.Lang{corpus.VI}, []job{{lang: corpus.VI, name: "00_front.md", front: corpus.Front{Paper: "a-1970-paper"}}})
	writing(t, c, corpus.VI, "a-1970-paper", "00_front.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("a paper with one of three sections translated is ready to push")
	}
}

// A paper a hard rule refuses stays in the working tree. Two batches went
// out red before this existed, tamnd/papers#38 and #39, and main was red
// until somebody noticed.
func TestAPaperTheAuditRefusesIsNotPublished(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	s.findings = func() ([]audit.Finding, error) {
		return []audit.Finding{{
			Rule:    "L07",
			File:    "content/vi/a-1970-paper/01_first.md",
			Message: "paragraph 3 is the English paragraph, word for word",
		}}, nil
	}
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		writing(t, c, corpus.VI, "a-1970-paper", name)
		s.done("a-1970-paper", true)
	}
	if got := s.take(); len(got) != 0 {
		t.Errorf("the batch staged %v and the paper is refused", got)
	}
	if !slices.Contains(s.held, "a-1970-paper") {
		t.Fatalf("the refused paper is not in the held list: %v", s.held)
	}
	if why := s.why["a-1970-paper"]; !strings.Contains(why, "L07") {
		t.Errorf("the run does not say which rule refused it: %q", why)
	}
	if s.pending() != 0 {
		t.Errorf("after the refusal %d files are still counted as waiting", s.pending())
	}
}

// One bad paper is no reason to hold the good ones. A batch of five where
// one is refused is a batch of four.
func TestOnlyTheRefusedPaperIsHeld(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		wrote(t, filepath.Join(c.Content(corpus.EN, "b-1980-paper"), name), "English.\n")
	}
	s.owed["b-1980-paper"] = 3
	s.findings = func() ([]audit.Finding, error) {
		return []audit.Finding{{Rule: "L10", File: "content/vi/b-1980-paper/00_front.md", Message: "a term stands in English"}}, nil
	}
	for _, id := range []string{"a-1970-paper", "b-1980-paper"} {
		for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
			writing(t, c, corpus.VI, id, name)
			s.done(id, true)
		}
	}
	got := s.take()
	if !slices.Contains(got, filepath.FromSlash("content/vi/a-1970-paper")) {
		t.Errorf("the good paper was held with the bad one: %v", got)
	}
	if slices.Contains(got, filepath.FromSlash("content/vi/b-1980-paper")) {
		t.Errorf("the refused paper went out: %v", got)
	}
}

// A finding about the repository rather than about a paper holds nothing.
// There is no honest paper to blame one on, and CI says it on the pull
// request, which is where a thing true of the whole corpus belongs.
func TestAFindingWithNoFileHoldsNothing(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	s.findings = func() ([]audit.Finding, error) {
		return []audit.Finding{{Rule: "S03", Message: "a pdf is in the index"}}, nil
	}
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		writing(t, c, corpus.VI, "a-1970-paper", name)
		s.done("a-1970-paper", true)
	}
	if got := s.take(); !slices.Contains(got, filepath.FromSlash("content/vi/a-1970-paper")) {
		t.Errorf("a finding about the repository held the paper back: %v", got)
	}
}

// An audit that cannot run holds the batch. A check that fails open is not
// a check, and the corpus is what it was reading.
func TestAnAuditThatCannotRunHoldsTheBatch(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	s.findings = func() ([]audit.Finding, error) { return nil, errors.New("sources.yaml does not parse") }
	for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
		writing(t, c, corpus.VI, "a-1970-paper", name)
		s.done("a-1970-paper", true)
	}
	if got := s.take(); len(got) != 0 {
		t.Errorf("the batch went out without being audited: %v", got)
	}
	if !slices.Contains(s.held, "a-1970-paper") {
		t.Errorf("the paper is not in the held list: %v", s.held)
	}
}

func TestBlamedFindsThePaperAnywhereInThePath(t *testing.T) {
	ready := []string{"a-1970-paper", "b-1980-paper"}
	for _, c := range []struct{ file, want string }{
		{"content/vi/a-1970-paper/00_front.md", "a-1970-paper"},
		{"figures/b-1980-paper/fig-1.png", "b-1980-paper"},
		{"manifests/a-1970-paper.yaml", ""},
		{"content/vi/c-1990-paper/00_front.md", ""},
		{"reports/coverage.md", ""},
		{"", ""},
	} {
		if got := blamed(c.file, ready); got != c.want {
			t.Errorf("blamed(%q) is %q, want %q", c.file, got, c.want)
		}
	}
}
