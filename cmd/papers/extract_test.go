package main

import (
	"errors"
	"sort"
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

// The regression judge is written against. A recheck reads every page twice,
// and if both reads counted, A5's sample would have every page in it twice.
// That leaves the mean where it was and halves the variance the rule works
// from, so the rule that is supposed to notice a page at a third of the usual
// length stops noticing anything.
func TestRecheckingDoesNotBluntTheLengthRule(t *testing.T) {
	pages := map[int]string{}
	for n := 1; n <= 9; n++ {
		pages[n] = page(60)
	}
	pages[7] = page(4)
	var ids []int
	for n := range pages {
		ids = append(ids, n)
	}
	sort.Ints(ids)

	checker := &extract.Checker{Model: true}
	faults := judge(checker, ids, pages)
	if len(faults[7]) == 0 {
		t.Error("the short page was not reported")
	}
	for _, n := range ids {
		if n != 7 && len(faults[n]) > 0 {
			t.Errorf("page %d was reported: %v", n, faults[n])
		}
	}
	// Eight and not nine, because the seeding pass refuses the short page too
	// and Check only counts a page it accepted. A5's sample is the pages that
	// look like pages, which is what it should be measured against.
	if got := checker.Pages(); got != len(ids)-1 {
		t.Errorf("the checker counted %d pages over a paper of %d with one short one", got, len(ids))
	}
}

// A paper of one page has no sample for A5 and has to come back quiet rather
// than come back with every page refused.
func TestJudgingAPaperOfOnePageIsQuiet(t *testing.T) {
	checker := &extract.Checker{Model: true}
	if faults := judge(checker, []int{1}, map[int]string{1: page(60)}); len(faults) > 0 {
		t.Errorf("a paper of one page was refused: %v", faults)
	}
}

func TestATextLayerIsReadOncePerPage(t *testing.T) {
	var reads int
	layer := keepLayer(func(page int) (string, error) {
		reads++
		if page == 2 {
			return "", errors.New("pdftotext found nothing on this page")
		}
		return "the words this page was typeset from", nil
	})
	for range 3 {
		if body, ok := layer(1); !ok || body == "" {
			t.Errorf("page 1 came back %q %v, want its text layer", body, ok)
		}
		if _, ok := layer(2); ok {
			t.Error("a page pdftotext could not read came back as having a layer")
		}
	}
	if reads != 2 {
		t.Errorf("the layer was read %d times over two pages asked for three times each, want 2", reads)
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

// The window a restricted paper is read through is three pages long, and it
// starts where --pages says rather than always at the front of the file. No
// Silver Bullet is the UNC tech report: a cover sheet, a page holding one
// line of legal notice, and a second title page, with the abstract on page 4.
func TestARestrictedPaperIsReadThreePagesFromWhereItStarts(t *testing.T) {
	for _, c := range []struct{ first, last int }{
		{0, 3},
		{1, 3},
		{4, 6},
		{11, 13},
	} {
		if got := restrictedWindow(c.first); got != c.last {
			t.Errorf("reading a restricted paper from page %d stops at page %d, want %d", c.first, got, c.last)
		}
	}
}

func TestTheWindowNeverRunsOffTheEndOfThePaper(t *testing.T) {
	e := &extraction{source: &corpus.Source{Pages: 5}, first: 4, last: restrictedWindow(4)}
	if err := e.pageRange(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if e.first != 4 || e.last != 5 {
		t.Errorf("read pages %d to %d of a 5 page paper, want 4 to 5", e.first, e.last)
	}
}

func TestAPageRangeThatStartsAfterTheEndIsAnError(t *testing.T) {
	e := &extraction{source: &corpus.Source{Pages: 5}, first: 9, last: restrictedWindow(9)}
	if err := e.pageRange(t.Context(), ""); err == nil {
		t.Fatal("reading from page 9 of a 5 page paper was accepted")
	}
}

func TestNoPageRangeReadsTheWholePaper(t *testing.T) {
	e := &extraction{source: &corpus.Source{Pages: 14}}
	if err := e.pageRange(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if e.first != 1 || e.last != 14 {
		t.Errorf("read pages %d to %d, want 1 to 14", e.first, e.last)
	}
}
