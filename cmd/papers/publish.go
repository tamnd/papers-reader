package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/publish"
)

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "publish these papers only, comma separated")
	base := fs.String("base", "main", "the branch the pull request is opened against")
	branch := fs.String("branch", "", "the branch to push to, or empty for one named after the time")
	note := fs.String("note", "", "the opening sentence of the pull request body")
	hold := fs.Bool("no-merge", false, "open the pull request and leave it open")
	dry := fs.Bool("dry-run", false, "print the title and the body and push nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers publish [flags]

Pushes what the toolchain has written to the corpus repository as a pull
request, and merges it.

This is what makes a long run autonomous. A translation run over the whole
corpus is a day and a half of questions put to three hosts, and without this
the result of all of it sits in one working tree on one machine until a
person remembers to look. Run it by hand, or let papers translate -publish
call it every so many files.

It never checks anything out. The paths are staged, committed on the branch
that is already there, pushed to a branch of their own, merged, and the
local branch is moved forward to what the remote then has. A run writing
into the same working tree at the same time is not disturbed and its files
are picked up by the next batch.

--id publishes one paper and leaves the rest of the working tree where it
is, which is what a pipeline that finishes a paper before it starts the
next one wants. The content of the named papers is staged and the other
roots go whole, exactly as a translation batch does, so the manifests and
the reports that were rewritten along the way travel with the paper they
describe.

A paper a hard audit rule refuses is not published. The rule and the file
are printed, nothing is staged, and the paper stays in the working tree for
the run to fix and try again. A finding about something other than the named
papers is printed and holds nothing, because a rule about the whole corpus
is not a reason to keep one paper out of it.

No PDF and no EPUB is ever staged, whatever .gitignore says.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	opening := *note
	if opening == "" {
		opening = "This is the pipeline pushing out what it has written so far."
	}
	name := *branch
	if name == "" {
		name = publish.Branch("", 0)
	}
	ctx := context.Background()
	var paths []string
	if *ids != "" {
		chosen, err := papersNamed(c, *ids)
		if err != nil {
			return err
		}
		if err := clear(c, chosen); err != nil {
			return err
		}
		paths = paperPaths(c, chosen)
	} else {
		var err error
		if paths, err = ready(ctx, c); err != nil {
			return err
		}
	}
	res, err := push(ctx, c, *base, name, opening, paths, !*hold, *dry)
	if err != nil {
		if errors.Is(err, publish.ErrNothing) {
			fmt.Println("nothing has been written that is not already in the corpus")
			return nil
		}
		return err
	}
	if res == nil {
		return nil
	}
	fmt.Printf("%d files in %s\n", res.Files, res.URL)
	return nil
}

// push is one batch, and is what papers translate calls as it goes.
//
// It reads the changes twice, once to write the message and once inside
// publish.Do to stage them. Two reads because the message has to be written
// before the commit and the commit has to be the thing the message
// describes, and because a run that wrote another file in between those two
// moments has written another file that belongs in the same batch. The
// count in the message can therefore be one or two short of the commit. That
// is the right way round: a pull request that claims less than it holds is a
// pull request somebody reads carefully.
//
// paths narrows the batch to a list of directories, which is how a
// translation run pushes the papers it has finished and leaves the one it is
// halfway through where it is. Empty is every root.
func push(ctx context.Context, c *corpus.Corpus, base, branch, opening string, paths []string, merge, dry bool) (*publish.Result, error) {
	if len(paths) == 0 {
		paths = publish.Roots
	}
	changes, err := publish.Changed(ctx, publish.Exec, c.Root, paths)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, publish.ErrNothing
	}
	if err := publish.Guard(changes); err != nil {
		return nil, err
	}
	title, body := publish.Describe(changes, opening)
	if dry {
		fmt.Printf("%s\n\n%s", title, body)
		fmt.Printf("\ndry run, nothing committed and nothing pushed\n")
		return nil, nil
	}
	return publish.Do(ctx, &publish.Batch{
		Dir:    c.Root,
		Base:   base,
		Branch: branch,
		Paths:  paths,
		Title:  title,
		Body:   body,
		Merge:  merge,
		Logf:   func(format string, args ...any) { fmt.Printf("    "+format+"\n", args...) },
	})
}

// papersNamed is the identifiers of a comma separated list, checked against
// the manifest so that a typo is an error here rather than a batch that
// quietly stages nothing.
func papersNamed(c *corpus.Corpus, ids string) ([]string, error) {
	manifest, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	chosen, err := choosePapers(manifest, ids, "")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(chosen))
	for _, p := range chosen {
		out = append(out, p.ID)
	}
	return out, nil
}

// paperPaths is what a batch stages for one paper, which is the content
// directories of that paper in every language and then the other roots
// whole.
//
// The same shape a translation batch uses, for the same reason. Content is
// narrowed because the working tree usually holds another paper that is half
// written and has no business in this batch. The manifests, the reports, the
// figures and the tags are not written per paper in any way a path can
// express: the figures manifest is one file over the whole corpus, and a
// pipeline that runs one paper at a time has nothing in them but that
// paper's work anyway.
//
// Only directories that exist, because git add is given these verbatim and
// errors on a pathspec that matches nothing.
func paperPaths(c *corpus.Corpus, ids []string) []string {
	var paths []string
	for _, id := range ids {
		for _, l := range corpus.Langs {
			dir := c.Content(l, id)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			if rel, err := filepath.Rel(c.Root, dir); err == nil {
				paths = append(paths, rel)
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return append(paths, otherRoots(c)...)
}

// otherRoots is the roots that are not content. They travel with every batch
// whatever it holds under content, because none of them is written per paper
// in a way a path can express: the figures manifest is one file over the
// whole corpus.
//
// Only directories that exist, because git add is given these verbatim and
// errors on a pathspec that matches nothing.
func otherRoots(c *corpus.Corpus) []string {
	var paths []string
	for _, r := range publish.Roots {
		if r == "content" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c.Root, r)); err == nil {
			paths = append(paths, r)
		}
	}
	return paths
}

// ready is what a publish that named no papers stages: the papers with
// something to push that the hard audit does not refuse, and then the roots
// that are not a paper.
//
// A publish with no --id used to stage every root whole and never ran the
// audit at all. The gate was on --id only, so a run could put one paper at a
// time through it or send the whole tree straight past it, and the whole
// tree is what a pipeline reaches for when it has a day of work to push. The
// help has said all along that a paper a hard rule refuses is not published,
// and for a batch of fourteen papers with twelve of them refused that was
// simply untrue.
//
// A refused paper is dropped and the rest of the batch goes. That is the
// choice a translation batch makes too, for the same reason: a rule that
// fires on one paper is no reason to hold the other eighty, and the refused
// one stays in the working tree for the next pass to fix and try again.
//
// The papers come off the working tree and not the manifest, because a paper
// with nothing written is a paper there is no reason to audit and a finding
// about one would hold back a batch it was never in.
func ready(ctx context.Context, c *corpus.Corpus) ([]string, error) {
	changes, err := publish.Changed(ctx, publish.Exec, c.Root, []string{"content"})
	if err != nil {
		return nil, err
	}
	ids := changedPapers(changes)
	if len(ids) == 0 {
		// Nothing under content to gate, so the batch is the manifests and
		// the reports on their own and it goes as it always did.
		return publish.Roots, nil
	}
	found, err := hardFindings(c)()
	if err != nil {
		return nil, err
	}
	keep := cleared(ids, found)
	if len(keep) == 0 {
		return otherRoots(c), nil
	}
	return paperPaths(c, keep), nil
}

// changedPapers is the papers a list of changes touches, sorted and without
// repeats.
//
// A path under content is content/<lang>/<id>/<file> and anything shorter is
// not a paper: the language directory itself arrives as one line when git is
// listing an untracked tree, and a directory is not something to audit.
func changedPapers(changes []publish.Change) []string {
	seen := map[string]bool{}
	var ids []string
	for _, ch := range changes {
		parts := strings.Split(filepath.ToSlash(ch.Path), "/")
		if len(parts) < 4 || parts[0] != "content" {
			continue
		}
		if id := parts[2]; !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// cleared is the papers of a batch that no hard finding names. What it drops
// it says so, with the rule and the file, because a paper that quietly does
// not appear in a pull request is a paper nobody goes looking for.
//
// A finding that names none of them is printed and holds nothing. Those are
// the rules about the repository rather than about a paper, and there is no
// honest paper to blame one on. If one is failing then CI says so on the
// pull request, which is the right place for a thing that is true of the
// whole corpus.
func cleared(ids []string, found []audit.Finding) []string {
	bad := map[string]string{}
	for _, f := range found {
		id := blamed(f.File, ids)
		if id == "" {
			fmt.Printf("%s\n", f)
			continue
		}
		if _, was := bad[id]; !was {
			bad[id] = f.String()
		}
	}
	var keep []string
	for _, id := range ids {
		if why, no := bad[id]; no {
			fmt.Printf("%s is held back, a hard audit rule refuses it: %s\n", id, why)
			continue
		}
		keep = append(keep, id)
	}
	return keep
}

// clear runs the hard audit and refuses the batch when a rule names one of
// the papers in it.
//
// The same gate a translation batch keeps, moved to where a run that is
// building the English can reach it. A paper that goes out red is a paper
// somebody has to go and fix in a repository that is already showing a
// failing check, and the pipeline that pushed it carries on regardless.
// Held here, it stays in the working tree, the next pass of the run sees it
// and can do the work again.
func clear(c *corpus.Corpus, ids []string) error {
	found, err := hardFindings(c)()
	if err != nil {
		return err
	}
	var bad []audit.Finding
	for _, f := range found {
		if blamed(f.File, ids) != "" {
			bad = append(bad, f)
			continue
		}
		fmt.Printf("%s\n", f)
	}
	if len(bad) == 0 {
		return nil
	}
	for _, f := range bad {
		fmt.Printf("%s\n", f)
	}
	return fmt.Errorf("%d hard findings name these papers, so nothing is published", len(bad))
}
