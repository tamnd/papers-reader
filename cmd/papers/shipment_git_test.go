package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
)

// The gate asks whether the corpus is sound after the batch, so a figure the
// run has just rendered counts as held. Before this the Paxos paper was
// refused for its own two figures, seconds before the commit that carried
// them.
func TestTheGateCountsWhatTheBatchIsAboutToCommitAsTracked(t *testing.T) {
	c := &corpus.Corpus{Root: gitRepo(t)}
	writeAt(t, filepath.Join(c.Root, "figures", "amdahl-1967-law", "f01.png"), "not really a png")

	in := &audit.Input{Tracked: []string{"manifests/papers.yaml"}}
	if err := pending(c, in); err != nil {
		t.Fatal(err)
	}
	if want := "figures/amdahl-1967-law/f01.png"; !slices.Contains(in.Tracked, want) {
		t.Errorf("tracked is %v and does not hold %s", in.Tracked, want)
	}
}

// Outside a checkout there is nothing to ask, and a list of nothing is not an
// answer. The two rules that read the index stand down on a nil list and they
// have to keep standing down.
func TestTheGateLeavesTheTrackedListAloneOutsideACheckout(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	in := &audit.Input{}
	if err := pending(c, in); err != nil {
		t.Fatal(err)
	}
	if in.Tracked != nil {
		t.Errorf("tracked is %v outside a git checkout", in.Tracked)
	}
}

// gitRepo is an empty repository with one commit in it, because git status
// says nothing useful about a repository with no HEAD.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "README.md"), "a corpus")
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"add", "README.md"},
		{"commit", "--quiet", "-m", "first"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func writeAt(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
