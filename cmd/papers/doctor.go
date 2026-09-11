package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tamnd/llm/route"
	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/layout"
	"github.com/tamnd/papers-reader/poppler"
	"github.com/tamnd/papers-reader/work"
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
	offline := fs.Bool("offline", false, "do not ask the hosts in the route file anything")
	deep := fs.Bool("deep", false, "put one trivial question to each host as well")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers doctor [flags]

Checks that the corpus is findable, that the tools each stage needs are
installed, and that the hosts in the route file are answering. Nothing is
downloaded and nothing is written.

Asking a host whether it is up is a GET and costs nothing, so it is done
by default; -offline skips it. Asking it a question costs a call, so -deep
is opt in, and it is the only way to find out that an account has been
quietly moved down to a smaller model than the route file names.

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
	for _, tool := range layout.Tools {
		if !layout.Have(tool) {
			fmt.Printf("  %-14s not installed\n", tool)
			continue
		}
		version, err := layout.Version(ctx, tool)
		if err != nil {
			version = "installed, version unknown"
		}
		fmt.Printf("  %-14s %s\n", tool, version)
	}
	switch have := layout.Installed(); {
	case len(have) == 0:
		// Not an error. Most of the hundred take the native path and a
		// machine that only ever runs those needs none of these.
		fmt.Println("  none installed, so papers extract --path layout has nothing to run")
	default:
		fmt.Printf("  %-14s %s, unless --tool says otherwise\n", "the default", have[0])
	}

	if !routesOK(*offline, *deep) {
		ok = false
	}

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

// routesOK reports on the fleet: which hosts are configured, and whether any
// of them is answering.
//
// A machine with no route file is fine and says so. The native and layout
// paths ask no model anything, so a checkout that only ever runs those needs
// no route file, and a doctor that fails over a file that is not needed
// teaches people to ignore it.
//
// A machine with a route file and nothing answering is not fine. That is the
// state a rebuild will sit in all night, and it is the whole reason to run
// this before starting one.
func routesOK(offline, deep bool) bool {
	fmt.Println("\nthe fleet, for the stages that put questions to a model")
	registry, path, err := work.Routes("")
	if err != nil {
		fmt.Printf("  %-14s %v\n", "routes", err)
		return false
	}
	if len(registry.Routes) == 0 {
		fmt.Printf("  %-14s none configured, run papers routes init to write a template\n", "routes")
		fmt.Printf("  %-14s the native and layout paths need none, the vision path needs one\n", "")
		return true
	}
	fmt.Printf("  %-14s %d, from %s\n", "routes", len(registry.Routes), path)
	if err := registry.Validate(); err != nil {
		fmt.Printf("  %-14s %v\n", "", err)
		return false
	}
	pool := work.Pool(registry)
	if pool.Empty() {
		fmt.Printf("  %-14s every route is disabled or is a reader, so nothing can be asked anything\n", "")
		return false
	}
	fmt.Printf("  %-14s %d calls at once across %d hosts\n", "lanes", pool.Lanes(), len(pool.Routes()))
	if offline {
		fmt.Printf("  %-14s not asked, this run is offline\n", "hosts")
		return true
	}

	pool.Prober = route.Prober{Deep: deep}
	ctx, cancel := context.WithTimeout(context.Background(), probeFor(deep))
	defer cancel()
	results := pool.ProbeAll(ctx)
	fmt.Println()
	for line := range strings.SplitSeq(strings.TrimRight(route.Table(results), "\n"), "\n") {
		fmt.Printf("  %s\n", line)
	}
	live := 0
	for _, health := range results {
		if health.State.Usable() {
			live++
		}
	}
	if live == 0 {
		fmt.Printf("\n  %-14s nothing is answering, so a run started now would wait rather than work\n", "")
		return false
	}
	return true
}

// probeFor is how long the whole fleet gets to answer. A health check is
// milliseconds and a real question through a browser session has been
// measured at about two and a half minutes, so the two bounds are not
// remotely the same number.
func probeFor(deep bool) time.Duration {
	if deep {
		return 10 * time.Minute
	}
	return time.Minute
}
