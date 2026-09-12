package main

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
)

// page is a page of the right rough length for a paper of ordinary pages. The
// text is nonsense on purpose: what is being tested is a length and a count,
// and a fixture cut from somebody's paper would be their words in our
// repository for no reason at all.
func page(n int) string {
	return strings.Repeat("the quick brown fox jumps over the lazy dog. ", n)
}

func store(t *testing.T, pages map[int]string) extract.Store {
	t.Helper()
	s := extract.Store{Dir: t.TempDir()}
	for n, text := range pages {
		if err := s.Write(n, text); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// The regression. Re-reading one page of a paper used to seed the checker
// from the run's own page range, which for a run of one page is the page
// being re-read and therefore nothing. Rule A5 stood down for want of a
// sample and an answer that stopped after one sentence was accepted over a
// page that was correct.
func TestSeedingOneRereadPageUsesTheWholePaper(t *testing.T) {
	pages := map[int]string{}
	for n := 1; n <= 9; n++ {
		pages[n] = page(60)
	}
	s := store(t, pages)

	checker := &extract.Checker{Model: true}
	seed(s, map[int]bool{7: true}, checker, nil)
	if got := checker.Pages(); got != 8 {
		t.Fatalf("seeded %d pages, want the 8 that are not being re-read", got)
	}
	if faults := checker.Check(7, "Given our assumption, the probability drops exponentially as the"); len(faults) == 0 {
		t.Error("a page that stopped after one sentence was accepted")
	}
}

// The page being re-read is left out, because its old text is what the new
// text is replacing and a page should not set the standard it is judged
// against. A paper of nine pages where eight are truncated is a paper with a
// problem, and the one good page is the one that should be reported.
func TestSeedingLeavesOutThePageBeingRead(t *testing.T) {
	pages := map[int]string{}
	for n := 1; n <= 9; n++ {
		pages[n] = page(60)
	}
	pages[7] = "a page that came back short"
	s := store(t, pages)

	checker := &extract.Checker{Model: true}
	seed(s, map[int]bool{7: true}, checker, nil)
	if got := checker.Pages(); got != 8 {
		t.Fatalf("seeded %d pages, want 8", got)
	}
	if faults := checker.Check(7, page(60)); len(faults) > 0 {
		t.Errorf("a page of the usual length was refused: %v", faults)
	}
}

// A paper with nothing extracted yet has nothing to seed from, and the
// checker has to come back empty rather than come back wrong.
func TestSeedingAnEmptyStoreIsQuiet(t *testing.T) {
	checker := &extract.Checker{Model: true}
	seed(extract.Store{Dir: t.TempDir() + "/never-written"}, nil, checker, nil)
	if got := checker.Pages(); got != 0 {
		t.Errorf("seeded %d pages from an empty store, want 0", got)
	}
}

// Pages outside the range the run was given still count. This is the same
// bug as the first test seen from the other side: a run over pages 8 and 9
// of a nine page paper learns what the paper's pages look like from the
// seven it is not touching.
func TestSeedingCountsPagesOutsideTheRange(t *testing.T) {
	pages := map[int]string{}
	for n := 1; n <= 9; n++ {
		pages[n] = page(60)
	}
	s := store(t, pages)

	checker := &extract.Checker{Model: true}
	seed(s, map[int]bool{8: true, 9: true}, checker, nil)
	if got := checker.Pages(); got != 7 {
		t.Fatalf("seeded %d pages, want 7", got)
	}
}

// retidyPages exists so that a page already on disk can pick up a step the
// tidier learned after it was read, without going back to a model. The whole
// case for it is that re-reading a page that is already right can come back
// wrong, so these tests are about writing as little as possible.
func TestRetidyRewritesOnlyThePagesThatChange(t *testing.T) {
	root := t.TempDir()
	c := &corpus.Corpus{Root: root}
	s := extract.Store{Dir: c.Work("paper-1999-example", "pages")}
	html := "<table>\n<tr><th>Term</th><th>Value</th></tr>\n<tr><td>a</td><td>1</td></tr>\n</table>"
	clean := "| Term | Value |\n| --- | --- |\n| a | 1 |"
	for n, text := range map[int]string{1: strings.TrimSpace(page(40)), 2: html, 3: strings.TrimSpace(page(40))} {
		if err := s.Write(n, text); err != nil {
			t.Fatal(err)
		}
	}
	papers := []corpus.Paper{{ID: "paper-1999-example"}}
	if err := retidyPages(c, papers, 0, 0, false, false); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read(2)
	if err != nil {
		t.Fatal(err)
	}
	if got != clean+"\n" {
		t.Errorf("page 2 is:\n%s\nwant:\n%s", got, clean)
	}
	if got, err := s.Read(1); err != nil || got != strings.TrimSpace(page(40))+"\n" {
		t.Errorf("page 1 was rewritten: %q", got)
	}
}

// Every step in Tidy is a translation between two spellings, so a second run
// over a page that is already in the second spelling writes nothing. If this
// ever fails, a step has been added that is a repair rather than a
// translation, and --retidy is no longer safe to run over a whole corpus.
func TestRetidyIsIdempotent(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	s := extract.Store{Dir: c.Work("paper-1999-example", "pages")}
	body := "| Term | Value |\n| --- | --- |\n| $d_k$ | 64 |\n\n" +
		"```c\nint main(void) { return 0; }\n```\n\nFigure.\n\n" + strings.TrimSpace(page(20))
	if err := s.Write(1, body); err != nil {
		t.Fatal(err)
	}
	papers := []corpus.Paper{{ID: "paper-1999-example"}}
	for i := 0; i < 2; i++ {
		if err := retidyPages(c, papers, 0, 0, false, false); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Read(1)
	if err != nil {
		t.Fatal(err)
	}
	if got != body+"\n" {
		t.Errorf("the page changed:\n%s\nwant:\n%s", got, body)
	}
}

func TestRetidyDryRunWritesNothing(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	s := extract.Store{Dir: c.Work("paper-1999-example", "pages")}
	html := "<table>\n<tr><th>a</th></tr>\n<tr><td>1</td></tr>\n</table>"
	if err := s.Write(1, html); err != nil {
		t.Fatal(err)
	}
	papers := []corpus.Paper{{ID: "paper-1999-example"}}
	if err := retidyPages(c, papers, 0, 0, true, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Read(1); got != html+"\n" {
		t.Errorf("a dry run wrote the page: %q", got)
	}
}

// A paper with nothing extracted yet is skipped and not an error, because
// --retidy over a whole corpus runs across papers at every stage.
func TestRetidyOverAPaperWithNoPagesIsQuiet(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	papers := []corpus.Paper{{ID: "paper-1999-example"}}
	if err := retidyPages(c, papers, 0, 0, false, false); err != nil {
		t.Fatalf("a paper with no pages returned %v", err)
	}
}
