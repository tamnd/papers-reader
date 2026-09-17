package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/publish"
)

// One paper goes out and the paper being written two lanes over stays where
// it is. This is the whole reason the flag exists.
func TestPublishingOnePaperStagesThatPaperAndNotTheOther(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	for _, dir := range []string{
		c.Content(corpus.EN, "amdahl-1967-law"),
		c.Content(corpus.VI, "amdahl-1967-law"),
		c.Content(corpus.EN, "lamport-1998-paxos"),
		c.Manifests(),
		c.Reports(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := paperPaths(c, []string{"amdahl-1967-law"})
	want := []string{
		filepath.Join("content", "en", "amdahl-1967-law"),
		filepath.Join("content", "vi", "amdahl-1967-law"),
		"manifests",
		"reports",
	}
	if !slices.Equal(got, want) {
		t.Errorf("staged %v, want %v", got, want)
	}
}

// A language the paper has nothing in is not a pathspec, because git add
// errors on one that matches nothing and that would stop the batch.
func TestPublishingOnePaperNamesNoDirectoryThatIsNotThere(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	if err := os.MkdirAll(c.Content(corpus.EN, "amdahl-1967-law"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range paperPaths(c, []string{"amdahl-1967-law"}) {
		if _, err := os.Stat(filepath.Join(c.Root, p)); err != nil {
			t.Errorf("%s is staged and is not there", p)
		}
	}
}

// A paper with nothing written of it at all stages nothing, rather than
// staging the manifests on their own and calling it a paper.
func TestPublishingAPaperWithNoContentStagesNothing(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	if err := os.MkdirAll(c.Manifests(), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := paperPaths(c, []string{"amdahl-1967-law"}); got != nil {
		t.Errorf("staged %v for a paper that has nothing", got)
	}
}

// A batch with no --id ran no audit at all, so the pipeline could send the
// whole tree past a gate that only --id ever reached. Fourteen papers were
// staged with twelve of them refused.
func TestABatchWithNoIdDropsTheRefusedPapersAndKeepsTheRest(t *testing.T) {
	ids := []string{"amdahl-1967-law", "lamport-1998-paxos", "shannon-1948-communication"}
	found := []audit.Finding{
		{Rule: "T10", File: "content/en/lamport-1998-paxos/03_ballots.md", Message: "a page number on a line of its own"},
		{Rule: "L13", File: "content/vi/lamport-1998-paxos/03_ballots.md", Message: "Cyrillic in the Vietnamese"},
		{Rule: "S09", File: "manifests/sources.yaml", Message: "a rule about the whole corpus"},
	}
	want := []string{"amdahl-1967-law", "shannon-1948-communication"}
	if got := cleared(ids, found); !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

// Every paper refused is not every paper held: the manifests and the reports
// still have to travel, or the figure the run rendered stays on the machine
// that rendered it.
func TestABatchWithEveryPaperRefusedStillHasTheOtherRoots(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	for _, dir := range []string{c.Manifests(), c.Reports()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	found := []audit.Finding{{Rule: "T10", File: "content/en/lamport-1998-paxos/03_ballots.md", Message: "a page number"}}
	if got := cleared([]string{"lamport-1998-paxos"}, found); len(got) != 0 {
		t.Fatalf("kept %v", got)
	}
	if got := otherRoots(c); !slices.Equal(got, []string{"manifests", "reports"}) {
		t.Errorf("the other roots are %v", got)
	}
}

// The papers of a batch come off the paths git reports, and the language
// directory itself is one of those paths when the tree is untracked.
func TestThePapersOfABatchAreReadOffTheChangedPaths(t *testing.T) {
	changes := []publish.Change{
		{Path: "content/vi/shannon-1948-communication/00_front.md"},
		{Path: "content/en/amdahl-1967-law/01_introduction.md"},
		{Path: "content/en/amdahl-1967-law/02_the_law.md"},
		{Path: "content/ja"},
		{Path: "manifests/figures.yaml"},
	}
	want := []string{"amdahl-1967-law", "shannon-1948-communication"}
	if got := changedPapers(changes); !slices.Equal(got, want) {
		t.Errorf("the batch is %v, want %v", got, want)
	}
}
