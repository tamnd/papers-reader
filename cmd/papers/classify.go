package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/papers-reader/classify"
)

func runClassify(args []string) error {
	fs := flag.NewFlagSet("classify", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "classify these papers only, comma separated")
	field := fs.String("field", "", "classify one field only")
	all := fs.Bool("all", false, "classify every paper on disk that has no measurement yet")
	again := fs.Bool("again", false, "measure papers that already have a text layer recorded too")
	dry := fs.Bool("dry-run", false, "print the measurements and write nothing")
	long := fs.Bool("v", false, "print the numbers behind each verdict")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers classify [flags]

Measures what each fetched PDF's text layer is worth and writes the answer to
manifests/sources.yaml. It measures rather than guessing from the year,
because there are 1980 papers with a clean text layer and 2005 papers that
are photographs of a printout.

    native    real text and real mathematics, extracted with pdftotext alone
    digital   real text, but mangled mathematics or figures to place
    ocr       a scan somebody has already run OCR over
    none      a scan with no text layer at all

A band of body pages is sampled, skipping the title pages and the
bibliography, and four things are counted over it: characters, mathematical
glyphs, mathematical fonts and full page images. What the numbers mean is in
`+"`papers classify -v`"+`, which prints them.

This needs poppler. Run papers doctor to see whether it is installed.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to classify: --id, --field or --all")
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
	todo, err := selectPapers(manifest, recorded, *ids, *field, true)
	if err != nil {
		return err
	}

	byID := index(recorded)
	ctx := context.Background()
	counts := map[classify.Layer]int{}
	var done, missing int

	for _, p := range todo {
		rec, ok := recorded.ByID(p.ID)
		if !ok {
			continue
		}
		if rec.TextLayer != "" && !*again {
			counts[classify.Layer(rec.TextLayer)]++
			continue
		}
		path := c.PDF(p.ID)
		if _, err := os.Stat(path); err != nil {
			missing++
			continue
		}

		m, v, err := classify.Measure(ctx, path)
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		}
		fmt.Printf("  %-34s %-8s %-7s %s\n", p.ID, v.Layer, v.Path, v.Why)
		if *long {
			fmt.Printf("  %-34s pages %d, sampled %d to %d, %d characters a page, %d mathematical glyphs, %d of %d fonts embedded, %d mathematical, %d with no character map, %d scanned pages, %d captions\n",
				"", m.Pages, m.First, m.Last, m.Chars/max(1, m.BandPages()), m.Maths, m.Embedded, m.Fonts, m.MathFonts, m.Unmapped, m.Scans, m.Captions)
		}
		counts[v.Layer]++
		done++

		out := byID[p.ID]
		out.Pages = m.Pages
		out.TextLayer = string(v.Layer)
		byID[p.ID] = out
	}

	if *dry {
		fmt.Println("dry run, nothing written")
		return nil
	}
	if done > 0 {
		if err := writeSources(c, byID); err != nil {
			return err
		}
		fmt.Println("wrote", c.SourcesManifest())
	}
	fmt.Printf("%d measured, %d not on disk. %s\n", done, missing, tally(counts))
	return nil
}

// tally prints the distribution, which is the number this milestone is
// really about: it says how much of the corpus needs a model and how much
// can be read with pdftotext and no guessing at all.
func tally(counts map[classify.Layer]int) string {
	var parts []string
	for _, l := range []classify.Layer{classify.Native, classify.Digital, classify.OCR, classify.None} {
		if n := counts[l]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, l))
		}
	}
	if len(parts) == 0 {
		return "nothing measured"
	}
	return strings.Join(parts, ", ")
}
