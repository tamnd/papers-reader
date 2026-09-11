package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/poppler"
)

// runDoctor reports whether this machine can do the work, before a run that
// takes an evening finds out on paper ninety.
//
// It is deliberately read only and deliberately quiet about what it cannot
// check. A tool that is present might still be the wrong version and doctor
// does not pretend otherwise: it says what it found and what each thing is
// needed for, and leaves the judgement to the person reading.
func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers doctor [flags]

Checks that the corpus is findable and that the tools each stage needs are
installed. Nothing is downloaded, nothing is written and no model is called.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ok := true

	fmt.Printf("papers %s\n\n", papers.Version)

	fmt.Println("corpus")
	c, err := openCorpus(*root)
	switch {
	case err != nil:
		fmt.Printf("  %-14s %s\n", "not found", err)
		fmt.Printf("  %-14s set %s or run this inside a checkout of tamnd/papers\n", "", corpus.EnvRoot)
		ok = false
	default:
		fmt.Printf("  %-14s %s\n", "root", c.Root)
		manifest, err := c.LoadPapers()
		if err != nil {
			fmt.Printf("  %-14s %v\n", "papers.yaml", err)
			ok = false
		} else {
			fmt.Printf("  %-14s %d papers\n", "papers.yaml", len(manifest.Papers))
		}
		recorded, err := c.LoadSources()
		if err != nil {
			fmt.Printf("  %-14s %v\n", "sources.yaml", err)
			ok = false
		} else {
			fmt.Printf("  %-14s %d records, %d with an access class\n", "sources.yaml", len(recorded.Sources), recorded.Resolved())
		}
		if err := ignored(c); err != nil {
			fmt.Printf("  %-14s %v\n", "pdf/", err)
			ok = false
		} else {
			fmt.Printf("  %-14s ignored by git, as it must be\n", "pdf/")
		}
	}

	fmt.Println("\npoppler, for reading PDFs")
	for _, tool := range poppler.Tools {
		if !poppler.Have(tool) {
			fmt.Printf("  %-14s not installed, %s\n", tool, needs[tool])
			ok = false
			continue
		}
		version, err := poppler.Version(ctx, tool)
		if err != nil {
			version = "installed, version unknown"
		}
		fmt.Printf("  %-14s %s\n", tool, version)
	}

	fmt.Println("\nlayout models, for the papers pdftotext cannot read on its own")
	for _, tool := range []string{"mineru", "marker_single", "docling"} {
		if path, err := exec.LookPath(tool); err == nil {
			fmt.Printf("  %-14s %s\n", tool, path)
		} else {
			fmt.Printf("  %-14s not installed\n", tool)
		}
	}
	fmt.Println("  none of these is required yet. They arrive with the layout path in M2.")

	fmt.Println()
	if !ok {
		return fmt.Errorf("something above needs attention")
	}
	fmt.Println("everything this stage needs is here")
	return nil
}

// needs says what each program is for, so that "not installed" comes with a
// reason to install it.
var needs = map[string]string{
	"pdfinfo":   "which is how the page count is read",
	"pdftotext": "which is how the text layer is read, and the whole native path",
	"pdffonts":  "which is how a scan is told from a born digital file",
	"pdfimages": "which is the other half of that test",
	"pdftoppm":  "which is how pages are rasterised for the vision path in M3",
}
