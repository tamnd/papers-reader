package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

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
func writing(t *testing.T, c *corpus.Corpus, l corpus.Lang, name string) {
	t.Helper()
	wrote(t, filepath.Join(c.Content(l, "a-1970-paper"), name), "Translated.\n")
}

func TestAPaperIsNotPushedUntilEveryFileOfItIsWritten(t *testing.T) {
	// This is the MapReduce case. Two of twelve sections went into the
	// corpus in Vietnamese and the other ten did not, and the paper sat
	// there looking finished until audit rules T04 and T06 said otherwise.
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	writing(t, c, corpus.VI, "00_front.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("one file of three is %d files ready to push, want 0", s.pending())
	}
	if p := s.take(); p != nil {
		t.Errorf("one file of three would stage %v, want nothing", p)
	}

	writing(t, c, corpus.VI, "01_first.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("two files of three is %d files ready to push, want 0", s.pending())
	}

	writing(t, c, corpus.VI, "02_second.md")
	s.done("a-1970-paper", true)
	if s.pending() != 3 {
		t.Errorf("the whole paper is %d files ready to push, want 3", s.pending())
	}
}

func TestAWholePaperStagesItsOwnDirectoriesAndNothingElse(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI, corpus.JA})
	for _, l := range []corpus.Lang{corpus.VI, corpus.JA} {
		for _, name := range []string{"00_front.md", "01_first.md", "02_second.md"} {
			writing(t, c, l, name)
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

func TestAPaperWithAFileThatFailedIsHeldBack(t *testing.T) {
	c, s := shipped(t, []corpus.Lang{corpus.VI})
	writing(t, c, corpus.VI, "00_front.md")
	s.done("a-1970-paper", true)
	writing(t, c, corpus.VI, "01_first.md")
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
	writing(t, c, corpus.VI, "00_front.md")
	s.done("a-1970-paper", true)
	if s.pending() != 0 {
		t.Errorf("a paper with one of three sections translated is ready to push")
	}
}
