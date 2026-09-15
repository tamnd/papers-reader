package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
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
