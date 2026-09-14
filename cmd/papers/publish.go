package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/publish"
)

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
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
	res, err := push(context.Background(), c, *base, name, opening, !*hold, *dry)
	if err != nil {
		if errors.Is(err, publish.Nothing) {
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
func push(ctx context.Context, c *corpus.Corpus, base, branch, opening string, merge, dry bool) (*publish.Result, error) {
	changes, err := publish.Changed(ctx, publish.Exec, c.Root, publish.Roots)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, publish.Nothing
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
		Title:  title,
		Body:   body,
		Merge:  merge,
		Logf:   func(format string, args ...any) { fmt.Printf("    "+format+"\n", args...) },
	})
}
