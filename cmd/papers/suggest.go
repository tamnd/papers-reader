package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/papers-reader/report"
)

func runSuggest(args []string) error {
	fs := flag.NewFlagSet("suggest", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	min := fs.Int("min-citations", 3, "how many papers of the corpus must cite a work before it is listed")
	write := fs.Bool("write", false, "write reports/suggest.md as well as printing the list")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers suggest [flags]

Lists works the corpus does not hold that several of its papers cite, most
cited first, with the papers add line to run for each one that carries an
identifier.

It reads the parsed bibliographies in manifests/refs and nothing else, so
it only sees as far as papers refs has got, and it says how far that is.

A work on this list is a candidate and not a decision. What the list knows
is that the papers already here keep pointing at it, which is a fact worth
having. Whether the paper is the canonical version, whether it is worth
reading, and whether its licence allows any of what the pipeline would do
next are all things a person has to decide.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *min < 1 {
		return fmt.Errorf("a work has to be cited at least once to be suggested, not %d times", *min)
	}

	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	all, err := report.Suggest(c, *min)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Printf("nothing outside the corpus is cited by %d of its papers\n", *min)
		return nil
	}
	for _, s := range all {
		fmt.Printf("%2d  %s\n", len(s.By), shortened(s.Title, 70))
		if cmd := s.Command(); cmd != "" {
			fmt.Printf("    %s\n", cmd)
		}
		fmt.Printf("    cited by %s\n", strings.Join(s.By, ", "))
	}
	fmt.Printf("\n%d works cited by at least %d papers and not in the corpus\n", len(all), *min)

	if !*write {
		return nil
	}
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(c.Reports(), "suggest.md")
	if err := os.WriteFile(path, []byte(report.SuggestMarkdown(all, *min)), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path)
	return nil
}

// shortened is a title cut to fit a terminal line, with an ellipsis where it
// was cut so that a reader can see something is missing.
func shortened(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
