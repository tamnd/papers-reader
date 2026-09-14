package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/emit"
	"github.com/tamnd/papers-reader/schema"
)

func runEmit(args []string) error {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	out := fs.String("out", "site", "the directory to write the JSON into")
	check := fs.Bool("check", false, "validate the build and write nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers emit [flags]

Builds the JSON the reading app consumes: the catalogue, the citation graph
and a page per paper per language, derived from the corpus and written as
static files, with the figures those pages show copied in beside them.

Everything it writes is derived, so the output directory can be deleted and
built again from a checkout at any time. It is not committed to the corpus.
The app repository builds it in the same job that deploys the site, which is
what makes a deployed site always a build of a committed corpus rather than
of somebody's working tree.

Every document is held to schema/site.schema.json before it is written, so a
build that would not render is a build that does not happen. That is the
same check as audit rule P05, run here as well because an emit is usually
run on its own and a person who has just changed the shape should find out
from the command they ran.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := corpus.Open(*root)
	if err != nil {
		return err
	}
	site, err := emit.Build(c)
	if err != nil {
		return err
	}
	files, err := site.Files()
	if err != nil {
		return err
	}

	names := emit.SortedNames(files)
	bad := 0
	for _, name := range names {
		why, err := schema.Validate(name, files[name])
		if err != nil {
			return err
		}
		for _, line := range why {
			fmt.Fprintf(os.Stderr, "%s: %s\n", name, line)
			bad++
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d places where the build does not match schema/site.schema.json", bad)
	}

	// The faults are printed and do not fail the command. A page that
	// refers to a figure nobody rendered is a page the app can still show,
	// with a paragraph where the picture would be, and refusing to build the
	// site over it would mean a corpus with one bad reference has no site at
	// all. Rules P01, P02 and P03 are where that becomes a failure, on the
	// audit, which is the thing that runs before a release.
	for _, f := range site.Faults {
		fmt.Fprintf(os.Stderr, "%s: %s: %s\n", f.Page, f.Kind, f.What)
	}

	if *check {
		fmt.Printf("%s, and it validates\n", site.Summary())
		return nil
	}
	if err := site.Write(*out); err != nil {
		return err
	}
	fmt.Println(site.Summary())
	return nil
}
