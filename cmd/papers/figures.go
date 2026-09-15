package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/poppler"
)

func runFigures(args []string) error {
	fs := flag.NewFlagSet("figures", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "crop these papers only, comma separated")
	field := fs.String("field", "", "crop one field only")
	all := fs.Bool("all", false, "crop every paper that has been fetched")
	pages := fs.String("pages", "", "look at these pages only, as N or N-M")
	dry := fs.Bool("dry-run", false, "find and render everything and write nothing")
	long := fs.Bool("v", false, "print every region, committed or not")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers figures [flags]

Crops the diagrams out of the pages and writes them to figures/<id>, and
records the page, the box, the caption, the hash and how each crop was made
in manifests/figures.yaml.

A paper that is cropped again replaces what it had: the PNGs it wrote last
time are removed first, and its entries in the manifest are rewritten. A
figure the detector no longer finds is a figure that should no longer be
there.

A figure is a cropped diagram. It is never a whole page:

    PNG, 8-bit, at least 100x100 px, at most 512 KB, at most 0.75 of
    the page it came from, deduplicated by SHA-256

The fraction is the rule that keeps this a corpus and not a mirror. A
cropped diagram is a figure; a page image committed because the extraction
was hard is a scan of somebody's copyrighted paper in a public repository,
and to git those two look identical. A region over the cap is refused here
and audit rule F06 refuses it again afterwards.

A restricted paper commits no figures at all, whatever their size, and a
paper whose licence has not been resolved commits nothing either.

None of this applies to a corpus whose manifests/policy.yaml says body.
That corpus publishes every paper in full, whatever its licence says, and
the decision to do so is recorded in that file rather than in this one.

Regions are found as the holes in a column of text, then paired with the
caption nearest them. A region with no caption is reported and not
committed: in practice it is a decorative rule, a logo or a display
equation far more often than it is a figure. A region whose caption says
Table is reported and not committed either, because a table belongs in the
corpus as a Markdown table and one that only exists as a picture is a hole
in the extraction.

This needs poppler. Run papers doctor to see whether it is installed.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to crop: --id, --field or --all")
	}
	first, last, err := pageRange(*pages)
	if err != nil {
		return err
	}

	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return err
	}
	recorded, err := c.LoadSources()
	if err != nil {
		return err
	}
	todo, err := choosePapers(manifest, *ids, *field)
	if err != nil {
		return err
	}

	// Read once and written once. The manifest covers the whole corpus, so
	// a run over the hundred that saved after every paper would rewrite the
	// same file a hundred times and would leave it half done if the run
	// were interrupted.
	catalogue, err := figures.Load(c.FiguresManifest())
	if err != nil {
		return err
	}

	ctx := context.Background()
	var written, skipped, papers int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		res, err := cropOne(ctx, c, p, rec, first, last, *dry)
		switch {
		case err != nil:
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		case res == nil:
			continue
		}
		papers++
		written += res.Written
		skipped += res.Skipped
		if !*dry {
			catalogue.Replace(p.ID, res.Figures)
		}
		if res.Written > 0 || res.Skipped > 0 || *long {
			fmt.Printf("  %-34s %d figures, %d regions refused\n", p.ID, res.Written, res.Skipped)
		}
		if *long || res.Written == 0 {
			for _, note := range res.Notes {
				fmt.Printf("    %s\n", note)
			}
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
		fmt.Printf("%d papers, %d figures found, %d regions refused\n", papers, written, skipped)
		return nil
	}
	if papers > 0 {
		if err := catalogue.Save(c.FiguresManifest()); err != nil {
			return err
		}
	}
	fmt.Printf("%d papers, %d figures written, %d regions refused\n", papers, written, skipped)
	return nil
}

// cropOne is one paper. A nil result with no error is a paper this command
// has nothing to do with, which is the usual case for a paper that has not
// been fetched.
func cropOne(ctx context.Context, c *corpus.Corpus, p corpus.Paper, rec *corpus.Source, first, last int, dry bool) (*figures.Result, error) {
	if rec == nil {
		return nil, fmt.Errorf("nothing is known about what may be published from it, so nothing is cropped")
	}
	// The corpus decides this and the budget does not. Publishing by licence,
	// a restricted paper publishes front matter and an abstract, and a diagram
	// out of it is the part of the paper its publisher is most protective of.
	// A corpus whose policy carries every paper in full crops every paper.
	if !c.PublishesFigures(rec.Access) {
		if rec.Access == corpus.AccessUnknown || rec.Access == "" {
			return nil, fmt.Errorf("nothing is known about what may be published from it, so nothing is cropped")
		}
		return nil, nil
	}
	file := c.PDF(p.ID)
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("not fetched yet")
	}
	if first == 0 {
		first = 1
	}
	if last == 0 {
		last = rec.Pages
	}
	if last == 0 {
		doc, err := poppler.Info(ctx, file)
		if err != nil {
			return nil, err
		}
		last = doc.Pages
	}

	pages, err := poppler.Layouts(ctx, file, first, last)
	if err != nil {
		return nil, err
	}
	if !dry {
		if err := clearFigures(c.Figures(p.ID)); err != nil {
			return nil, err
		}
	}
	run := figures.Run{
		Paper:  p.ID,
		PDF:    file,
		Dir:    c.Figures(p.ID),
		Budget: figures.Default(),
		Dry:    dry,
	}
	return run.Do(ctx, pages)
}

// clearFigures removes the PNGs a previous run of this command wrote, so
// that a paper cropped again does not keep the figures the detector no
// longer finds. Only the files this command names are removed, which is
// f01.png and its numbered siblings, and a directory holding anything else
// keeps it.
func clearFigures(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !figureName.MatchString(e.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

var figureName = regexp.MustCompile(`^f[0-9]+\.png$`)
