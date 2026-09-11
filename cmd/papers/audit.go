package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tamnd/papers-reader/audit"
)

func runAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	hard := fs.Bool("hard", false, "run only the hard rules, and exit non-zero if any of them fails")
	write := fs.Bool("write", false, "write reports/audit.md as well as printing")
	quiet := fs.Bool("quiet", false, "print the summary line only")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers audit [flags]

Checks the corpus against the numbered rules. A rule reports one of three
things: it passed, it failed, or it did not run because it had nothing to
look at. The last of those is not a pass and is printed as itself.

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
	in, err := audit.Load(c)
	if err != nil {
		return err
	}
	rep := audit.Run(in, *hard)

	if !*quiet {
		for _, f := range rep.Findings() {
			fmt.Println(f)
		}
	}
	for _, err := range rep.Errors() {
		fmt.Fprintf(os.Stderr, "papers: %v\n", err)
	}
	fmt.Println(rep.Summary())

	if *write {
		path := filepath.Join(c.Reports(), "audit.md")
		if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(rep.Markdown()), 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}

	if n := rep.HardFailures(); n > 0 {
		return fmt.Errorf("%d hard rules failed", n)
	}
	if len(rep.Errors()) > 0 {
		return fmt.Errorf("%d rules could not run", len(rep.Errors()))
	}
	return nil
}
