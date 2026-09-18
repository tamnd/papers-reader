// Package publish pushes what the toolchain has written into the corpus
// repository, as a pull request that reads as though a person opened it.
//
// The pipeline runs for hours. A translation run over the whole corpus is
// five hundred questions put to three hosts and it finishes some time the
// next day, and until this package existed the result of all of it sat in
// one working tree on one machine, unpushed, where a lost disk or a killed
// screen session threw the lot away. So the run pushes as it goes: every so
// many files it opens a pull request for what it has finished, merges it,
// and carries on.
//
// The awkward part is that the run is still writing while the push happens.
// Publish never checks anything out and never touches the working tree. It
// stages the paths it was given, commits on the branch that is already
// there, pushes that commit to a branch on the remote, merges it, and then
// moves the local branch pointer forward to what the remote now has. The
// files the run wrote in the meantime are untracked or modified the whole
// way through and are still untracked or modified at the end, which is what
// the next batch picks up.
package publish

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Roots are the directories of the corpus repository a run may push.
//
// A list and not "everything that changed", because the repository has
// directories in it that are ignored rather than absent: the PDFs, the page
// rasters, the scratch space and the set books. An ignored file cannot be
// staged by accident, but the rule that keeps a PDF out of a public
// repository is worth more than one line of .gitignore, so the paths are
// named here as well and Guard refuses anything outside them.
var Roots = []string{"content", "manifests", "reports", "figures", "tags"}

// A Git runs one command in a directory and returns what it printed.
//
// A field rather than a direct call to os/exec so that the tests can watch
// the order of the commands without a repository, a remote or a network.
// Everything that can be got wrong here is in that order.
type Git func(ctx context.Context, dir, name string, args ...string) (string, error)

// Exec is the Git that actually runs the commands.
func Exec(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// A Batch is one push: what to take, what to call it and where to put it.
type Batch struct {
	// Dir is a checkout of the corpus repository.
	Dir string
	// Base is the branch the pull request is opened against, and the branch
	// the checkout is expected to be sitting on. Publish refuses to run from
	// anywhere else, because a run that pushed from a half-finished feature
	// branch would carry that branch's commits into the corpus.
	Base string
	// Branch is the branch the commit is pushed to. It is created by the
	// push and deleted by the merge, so it exists for about a minute.
	Branch string
	// Paths are the directories to stage, Roots when empty.
	Paths []string
	// Title and Body are the commit message and the pull request. Describe
	// writes both from the list of changes, which is what the run uses.
	Title, Body string
	// Merge says whether to merge the pull request as well as open it.
	Merge bool
	// Git runs the commands, Exec when empty.
	Git Git
	// Logf says what is being run, and may be nil.
	Logf func(string, ...any)
}

// A Result is what one push did.
type Result struct {
	Files  int
	Branch string
	// URL is the pull request, as gh printed it.
	URL string
	// Merged says whether the pull request went in.
	Merged bool
}

// ErrNothing is returned when there was nothing to push.
//
// Not an error at the call site that matters: a run publishing every twenty
// files reaches a batch with nothing in it whenever the twenty files were
// all copied bibliographies, and stopping a six hour run over that would be
// absurd.
var ErrNothing = errors.New("nothing has changed under the corpus directories")

// Do stages, commits, pushes, opens the pull request and merges it.
func Do(ctx context.Context, b *Batch) (*Result, error) {
	git := b.Git
	if git == nil {
		git = Exec
	}
	logf := b.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	base := b.Base
	if base == "" {
		base = "main"
	}
	roots := b.Paths
	if len(roots) == 0 {
		roots = Roots
	}

	on, err := git(ctx, b.Dir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	if on := strings.TrimSpace(on); on != base {
		return nil, fmt.Errorf("the checkout is on %s and a batch is pushed from %s", on, base)
	}

	changes, err := Changed(ctx, git, b.Dir, roots)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, ErrNothing
	}
	if err := Guard(changes); err != nil {
		return nil, err
	}

	add := append([]string{"add", "--"}, roots...)
	if _, err := git(ctx, b.Dir, "git", add...); err != nil {
		return nil, err
	}
	staged, err := git(ctx, b.Dir, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(staged) == "" {
		return nil, ErrNothing
	}

	logf("committing %d files as %s", len(changes), b.Branch)
	commit := append([]string{"commit", "-m", b.Title, "-m", b.Body, "--"}, roots...)
	if _, err := git(ctx, b.Dir, "git", commit...); err != nil {
		return nil, err
	}
	if _, err := git(ctx, b.Dir, "git", "push", "--quiet", "origin", "HEAD:refs/heads/"+b.Branch); err != nil {
		return nil, err
	}

	out, err := git(ctx, b.Dir, "gh", "pr", "create",
		"--base", base, "--head", b.Branch, "--title", b.Title, "--body", b.Body)
	if err != nil {
		return nil, err
	}
	res := &Result{Files: len(changes), Branch: b.Branch, URL: link(out)}
	if !b.Merge {
		return res, nil
	}

	if _, err := git(ctx, b.Dir, "gh", "pr", "merge", b.Branch, "--squash", "--admin", "--delete-branch"); err != nil {
		return res, err
	}
	res.Merged = true

	// The local branch is moved to what the remote now has. The working tree
	// keeps every file the run has written since the staging, which is the
	// next batch.
	//
	// A mixed reset rather than a soft one. Soft moves HEAD and leaves the
	// index alone, so the index went on holding the tree this batch had just
	// committed while HEAD moved to a main that other people had been pushing
	// to. Everything they had changed outside this run was then a staged
	// revert of their work, and the next batch's commit took the whole index
	// and carried it along. That is tamnd/papers#107: a run staged twelve
	// Vietnamese files and pushed a thirteenth change nobody asked for, moving
	// the CI toolchain pin back two releases, so the corpus was audited by a
	// version whose fixed rules it no longer had.
	//
	// The commit above names the roots for the same reason, so that a commit
	// is the roots whatever else is in the index. Guard is the check before
	// the fact and it reads git status under the roots, which is the right
	// scope for what the run has written and the wrong scope for what somebody
	// else already staged.
	if _, err := git(ctx, b.Dir, "git", "fetch", "--quiet", "origin", base); err != nil {
		return res, err
	}
	if _, err := git(ctx, b.Dir, "git", "reset", "--quiet", "--mixed", "origin/"+base); err != nil {
		return res, err
	}
	rest := append([]string{"checkout", "--quiet", "origin/" + base}, outside(roots)...)
	if _, err := git(ctx, b.Dir, "git", rest...); err != nil {
		return res, err
	}
	return res, nil
}

// outside is the pathspec for everything in the repository that is not one of
// the roots, as arguments to git checkout.
//
// The mixed reset above puts the index back to main and leaves the working
// tree alone, which is right for the roots because the files the run has
// written since the staging are the next batch. It is wrong for everything
// else: a file somebody changed on main stays on disk at the version this
// checkout last saw, reported as a local modification for ever after. So
// everything else is taken from main, which leaves the checkout as main plus
// the corpus files this run has written and nothing besides.
func outside(roots []string) []string {
	out := []string{"--", "."}
	for _, r := range roots {
		out = append(out, ":(exclude)"+r)
	}
	return out
}

// A Change is one path the run has written.
type Change struct {
	Path string
	// New says the file was not in the repository before. It is the
	// difference between a paper arriving and a paper being translated
	// again, which is the one thing a person reading the pull request title
	// wants to know.
	New bool
}

// Changed lists what has been written under the given directories.
func Changed(ctx context.Context, git Git, dir string, roots []string) ([]Change, error) {
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, roots...)
	out, err := git(ctx, dir, "git", args...)
	if err != nil {
		return nil, err
	}
	var changes []Change
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		state, name := line[:2], strings.TrimSpace(line[3:])
		// A rename prints "old -> new" and the new name is the one that is
		// in the tree afterwards.
		if _, to, ok := strings.Cut(name, " -> "); ok {
			name = to
		}
		name = strings.Trim(name, `"`)
		if name == "" {
			continue
		}
		changes = append(changes, Change{Path: name, New: strings.Contains(state, "?") || strings.Contains(state, "A")})
	}
	return changes, nil
}

// Guard refuses a batch that is about to commit something the corpus must
// never hold.
//
// The corpus repository is public and the rule is that no PDF and no EPUB
// ever goes into it, whatever the licence of the paper. Audit rule S03
// checks the same thing after the fact by reading git's index. This is the
// check before the fact, in the one place that runs git add without a person
// watching, and it is deliberately a separate check from .gitignore: an
// ignore file is one line away from not applying and a run that pushes every
// twenty minutes with nobody reading the diff is exactly where that would go
// unnoticed.
func Guard(changes []Change) error {
	for _, c := range changes {
		switch strings.ToLower(path.Ext(c.Path)) {
		case ".pdf", ".epub":
			return fmt.Errorf("%s is a whole paper in one file and the corpus never holds one", c.Path)
		}
		root, _, _ := strings.Cut(c.Path, "/")
		if !allowed(root) {
			return fmt.Errorf("%s is outside the directories a run may push", c.Path)
		}
	}
	return nil
}

func allowed(root string) bool {
	for _, r := range Roots {
		if r == root {
			return true
		}
	}
	return false
}

// Branch is the name for one batch's branch.
//
// The run identifier and the batch number, so that two runs going at once on
// two machines do not collide and so that a person looking at a merged pull
// request can find every other batch of the same run.
func Branch(run string, batch int) string {
	run = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return '-'
	}, run)
	if run == "" {
		run = time.Now().UTC().Format("20060102-150405")
	}
	return fmt.Sprintf("run-%s-%d", run, batch)
}

// link is the pull request URL out of what gh printed. gh prints the URL on
// a line of its own, after whatever it had to say about the branch.
func link(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return strings.TrimSpace(out)
}
