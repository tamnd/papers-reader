package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fake is a Git that answers from a table and writes down what it was asked.
type fake struct {
	say  map[string]string
	err  map[string]error
	said []string
}

func (f *fake) git(_ context.Context, _, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.said = append(f.said, line)
	for prefix, err := range f.err {
		if strings.HasPrefix(line, prefix) {
			return "", err
		}
	}
	for prefix, out := range f.say {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func onMain(status string) *fake {
	return &fake{say: map[string]string{
		"git rev-parse":     "main\n",
		"git status":        status,
		"git diff --cached": "content/vi/a-1900-x/01_intro.md\n",
		"gh pr create":      "opening pull request\nhttps://github.com/tamnd/papers/pull/42\n",
	}}
}

func TestABatchIsCommittedPushedAndMerged(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n M manifests/glossary.yaml\n")
	res, err := Do(context.Background(), &Batch{
		Dir: "/tmp/papers", Branch: "run-abc-1", Title: "content: a-1900-x in Vietnamese",
		Body: "one file.\n", Merge: true, Git: f.git,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 2 {
		t.Errorf("Files = %d, want 2", res.Files)
	}
	if res.URL != "https://github.com/tamnd/papers/pull/42" {
		t.Errorf("URL = %q", res.URL)
	}
	if !res.Merged {
		t.Error("the batch was not merged")
	}
	want := []string{
		"git rev-parse --abbrev-ref HEAD",
		"git status --porcelain",
		"git fetch --quiet origin main",
		"git diff --name-only HEAD origin/main",
		"git add --",
		"git diff --cached --name-only",
		"git commit -m content: a-1900-x in Vietnamese",
		"git push --quiet origin HEAD:refs/heads/run-abc-1",
		"gh pr create --base main --head run-abc-1",
		"gh pr merge run-abc-1 --squash --admin --delete-branch",
		"git fetch --quiet origin main",
		"git reset --quiet --mixed origin/main",
		"git checkout --quiet origin/main --",
	}
	if len(f.said) != len(want) {
		t.Fatalf("ran %d commands, want %d:\n%s", len(f.said), len(want), strings.Join(f.said, "\n"))
	}
	for i, w := range want {
		if !strings.HasPrefix(f.said[i], w) {
			t.Errorf("command %d is %q, want it to start %q", i, f.said[i], w)
		}
	}
}

// The corpus directories are what the run is still writing into, so nothing
// may be checked out over them wholesale. Everything else in the checkout is
// taken back from main, because a run leaving other people's changes
// reverted in its tree is what put a two release old toolchain pin into
// tamnd/papers#107.
func TestNothingUnderTheCorpusIsCheckedOutWholesale(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Merge: true, Git: f.git}); err != nil {
		t.Fatal(err)
	}
	for _, line := range f.said {
		for _, bad := range []string{"switch", "stash", "clean", "reset --hard", "pull"} {
			if strings.Contains(line, bad) {
				t.Errorf("%q takes the run's files away from under it", line)
			}
		}
		if !strings.Contains(line, "checkout") {
			continue
		}
		for _, r := range Roots {
			if strings.Contains(line, " "+r) && !strings.Contains(line, ":(exclude)"+r) {
				t.Errorf("%q checks out over %s, which the run is writing", line, r)
			}
		}
	}
}

// A file somebody else pushed while the run was going is not in the run's
// checkout, and everything git has to say about it says the run deleted it.
// Staging that is how tamnd/papers#115 took thirteen figures of the Gamma
// paper back out an hour after tamnd/papers#113 put them in.
func TestWhatSomebodyElsePushedIsTakenAndNotReverted(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n M manifests/figures.yaml\n")
	f.say["git diff --name-only HEAD origin/main"] = strings.Join([]string{
		"figures/b-1990-y/f05.png",
		"manifests/figures.yaml",
		"content/en/c-1970-z/03_gone.md",
		"",
	}, "\n")
	f.say["git ls-tree"] = "figures/b-1990-y/f05.png\nmanifests/figures.yaml\n"
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Merge: true, Git: f.git}); err != nil {
		t.Fatal(err)
	}
	var took, dropped string
	for _, line := range f.said {
		switch {
		case strings.HasPrefix(line, "git checkout origin/main --"):
			took = line
		case strings.HasPrefix(line, "git rm"):
			dropped = line
		}
	}
	if !strings.Contains(took, "figures/b-1990-y/f05.png") {
		t.Errorf("the figure somebody else pushed was not taken: %q", took)
	}
	// The run is holding a new version of the manifest, so the run's version
	// is the one that goes in and main's is not pulled over it.
	if strings.Contains(took, "manifests/figures.yaml") {
		t.Errorf("main was checked out over a file the run has written: %q", took)
	}
	if !strings.Contains(dropped, "content/en/c-1970-z/03_gone.md") {
		t.Errorf("a file main no longer holds was kept, and the next batch puts it back: %q", dropped)
	}
}

// Nothing to take is the usual case, and it costs one command and no
// checkout at all.
func TestACheckoutThatIsUpToDateIsLeftAlone(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Merge: true, Git: f.git}); err != nil {
		t.Fatal(err)
	}
	for _, line := range f.said {
		if strings.HasPrefix(line, "git checkout origin/main --") || strings.HasPrefix(line, "git ls-tree") {
			t.Errorf("%q ran with nothing to take", line)
		}
	}
}

// The index is not the run's to commit. A soft reset after a merge left
// everything main had moved on to staged as a revert of it, and the next
// batch committed the index whole and carried that revert into the corpus.
func TestABatchCommitsTheCorpusDirectoriesAndNotWhateverElseIsStaged(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Merge: true, Git: f.git}); err != nil {
		t.Fatal(err)
	}
	var commit string
	for _, line := range f.said {
		if strings.HasPrefix(line, "git commit") {
			commit = line
		}
	}
	_, paths, found := strings.Cut(commit, " -- ")
	if !found {
		t.Fatalf("the commit names no paths and takes the whole index: %q", commit)
	}
	if paths != strings.Join(Roots, " ") {
		t.Errorf("the commit names %q, want the corpus directories", paths)
	}
}

func TestABatchIsOnlyPushedFromTheBaseBranch(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	f.say["git rev-parse"] = "translate-fix\n"
	_, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Git: f.git})
	if err == nil || !strings.Contains(err.Error(), "translate-fix") {
		t.Fatalf("err = %v, want it to name the branch the checkout is on", err)
	}
	if len(f.said) != 1 {
		t.Errorf("ran %d commands after refusing, want 1", len(f.said))
	}
}

func TestAnEmptyBatchIsNotAnError(t *testing.T) {
	f := onMain("")
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Git: f.git}); !errors.Is(err, ErrNothing) {
		t.Fatalf("err = %v, want ErrNothing", err)
	}
}

// git status can say a file changed and git add can stage nothing, because
// the change was to a file's mode or because it was written back to what it
// already was. Committing then fails, and an empty commit in the history of
// the corpus is worse than a batch that did not happen.
func TestABatchThatStagesNothingIsNotCommitted(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	f.say["git diff --cached"] = "\n"
	if _, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Git: f.git}); !errors.Is(err, ErrNothing) {
		t.Fatalf("err = %v, want ErrNothing", err)
	}
	for _, line := range f.said {
		if strings.HasPrefix(line, "git commit") {
			t.Error("an empty batch was committed")
		}
	}
}

func TestAPullRequestIsLeftOpenWhenItIsNotToBeMerged(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	res, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Git: f.git})
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged {
		t.Error("the batch was merged and Merge was not set")
	}
	for _, line := range f.said {
		if strings.HasPrefix(line, "gh pr merge") {
			t.Error("the pull request was merged anyway")
		}
	}
}

func TestChangedReadsWhatTheRunWrote(t *testing.T) {
	f := onMain(strings.Join([]string{
		"?? content/vi/a-1900-x/01_intro.md",
		" M content/en/a-1900-x/00_front.md",
		"A  manifests/glossary.yaml",
		"R  reports/old.md -> reports/usage.md",
		"",
	}, "\n"))
	got, err := Changed(context.Background(), f.git, "/tmp/papers", Roots)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{"content/vi/a-1900-x/01_intro.md", true},
		{"content/en/a-1900-x/00_front.md", false},
		{"manifests/glossary.yaml", true},
		{"reports/usage.md", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d changes, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("change %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The corpus is public and holds no whole paper in one file. This is the one
// place in the toolchain that runs git add with nobody watching.
func TestNoWholePaperIsEverStaged(t *testing.T) {
	for _, name := range []string{"content/en/a-1900-x/paper.pdf", "content/en/a-1900-x/PAPER.PDF", "content/vi/a-1900-x/book.epub"} {
		if err := Guard([]Change{{Path: name}}); err == nil {
			t.Errorf("Guard(%q) allowed it", name)
		}
	}
}

func TestOnlyTheCorpusDirectoriesArePushed(t *testing.T) {
	for _, name := range []string{"pdf/a-1900-x.pdf", "work/a-1900-x/pages/0001.txt", "images/a.png", ".github/workflows/ci.yml"} {
		if err := Guard([]Change{{Path: name}}); err == nil {
			t.Errorf("Guard(%q) allowed it", name)
		}
	}
	for _, name := range Roots {
		if err := Guard([]Change{{Path: name + "/x/y.md"}}); err != nil {
			t.Errorf("Guard(%q) refused it: %v", name, err)
		}
	}
}

func TestABatchStopsWhenSomethingIsAboutToBeCommittedThatShouldNotBe(t *testing.T) {
	f := onMain("?? pdf/a-1900-x.pdf\n")
	_, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Git: f.git})
	if err == nil {
		t.Fatal("the batch went ahead")
	}
	for _, line := range f.said {
		if strings.HasPrefix(line, "git add") {
			t.Error("it was staged anyway")
		}
	}
}

// A run that could not merge still has to leave the local branch somewhere a
// person can see, and has to say what happened rather than report success.
func TestAFailedMergeIsReported(t *testing.T) {
	f := onMain("?? content/vi/a-1900-x/01_intro.md\n")
	f.err = map[string]error{"gh pr merge": fmt.Errorf("required status check is pending")}
	res, err := Do(context.Background(), &Batch{Dir: "/tmp/papers", Branch: "b", Merge: true, Git: f.git})
	if err == nil {
		t.Fatal("the failure was swallowed")
	}
	if res == nil || res.URL == "" {
		t.Fatal("the pull request that was opened is not reported")
	}
	if res.Merged {
		t.Error("Merged is set on a merge that failed")
	}
}

func TestTwoRunsGetDifferentBranches(t *testing.T) {
	if a, b := Branch("20260914T101500Z", 3), Branch("20260914T101501Z", 3); a == b {
		t.Errorf("both runs got %q", a)
	}
	if a, b := Branch("r", 1), Branch("r", 2); a == b {
		t.Errorf("both batches got %q", a)
	}
	for _, run := range []string{"A/B", "2026-09-14T10:15:00Z", ""} {
		got := Branch(run, 1)
		if strings.ContainsAny(got, "/: ") {
			t.Errorf("Branch(%q) = %q, which is not a branch name", run, got)
		}
	}
}

func TestTheURLIsPulledOutOfWhatGhSaid(t *testing.T) {
	out := "Warning: 3 uncommitted changes\nCreating pull request for run-1 into main\n\nhttps://github.com/tamnd/papers/pull/7\n"
	if got := link(out); got != "https://github.com/tamnd/papers/pull/7" {
		t.Errorf("link = %q", got)
	}
}
