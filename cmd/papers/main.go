// Command papers builds and checks the corpus at github.com/tamnd/papers.
//
// It is one binary with one subcommand per stage of the pipeline. Every
// subcommand is idempotent: run it twice on a finished paper and the second
// run does nothing.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
)

// command is one subcommand.
type command struct {
	name string
	// milestone is the milestone that brings the command. A command that has
	// not arrived yet says so and exits non-zero, rather than printing nothing
	// and returning success, because a pipeline script that silently skips a
	// stage is worse than one that stops.
	milestone string
	summary   string
	run       func(args []string) error
}

// commands is the whole command set, in pipeline order rather than
// alphabetical order, because the order is the documentation.
var commands = []command{
	{"version", "", "print the version", runVersion},
	{"list", "", "list papers in the corpus", runList},
	{"audit", "", "check the corpus against the numbered rules", runAudit},
	{"add", "M9", "add a paper to the manifest from an arXiv id or a DOI", nil},
	{"resolve", "M1", "find where each paper can be fetched, and its licence", runResolve},
	{"fetch", "M1", "download the PDFs that resolved, and hash them", runFetch},
	{"adopt", "M1", "record a PDF a person fetched by hand, and where from", runAdopt},
	{"classify", "M1", "measure what each PDF's text layer is worth", runClassify},
	{"render", "M2", "rasterise pages for the pages that need a model", nil},
	{"extract", "M2", "turn pages into Markdown with LaTeX mathematics", nil},
	{"figures", "M3", "crop the diagrams out of the pages", nil},
	{"pagemap", "M3", "record which figure came from which page and box", nil},
	{"refs", "M4", "parse bibliographies and link the citations", nil},
	{"assemble", "M5", "join the extracted pages into one document", nil},
	{"split", "M5", "cut a paper into one file per section", nil},
	{"tags", "M5", "hand out permanent identifiers", nil},
	{"glossary", "M6", "manage the controlled vocabulary", nil},
	{"translate", "M6", "produce Vietnamese, Chinese and Japanese", nil},
	{"emit", "M8", "build the JSON the reading app consumes", nil},
	{"graph", "M7", "build the citation graph over the corpus", nil},
	{"report", "M7", "write the coverage, audit and usage reports", nil},
	{"queue", "M1", "show and drain the work queue", nil},
	{"routes", "M1", "show the model routing table", nil},
	{"doctor", "M1", "check that the tools and the routes are usable", runDoctor},
	{"suggest", "M9", "list works cited often and not yet in the corpus", nil},
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "papers: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return nil
	case "-v", "--version":
		return runVersion(nil)
	}
	if strings.HasPrefix(args[0], "-") {
		usage(os.Stderr)
		return fmt.Errorf("%s is not a flag of papers itself, it belongs to a subcommand", args[0])
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		if c.run == nil {
			return fmt.Errorf("%s arrives in milestone %s and is not implemented yet", c.name, c.milestone)
		}
		return c.run(args[1:])
	}
	usage(os.Stderr)
	return fmt.Errorf("there is no %s command", args[0])
}

func usage(w *os.File) {
	fmt.Fprint(w, `papers builds and checks the corpus at github.com/tamnd/papers.

usage:
    papers <command> [flags]

commands:
`)
	width := 0
	for _, c := range commands {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	for _, c := range commands {
		note := ""
		if c.run == nil {
			note = fmt.Sprintf("  (milestone %s)", c.milestone)
		}
		fmt.Fprintf(w, "    %-*s  %s%s\n", width, c.name, c.summary, note)
	}
	fmt.Fprintf(w, `
The corpus is found with the -corpus flag, else %s, else by walking up from
the working directory until a manifests/papers.yaml turns up.

Run papers <command> -h for the flags of one command.
`, corpus.EnvRoot)
}

// openCorpus is how every subcommand finds the corpus.
func openCorpus(root string) (*corpus.Corpus, error) { return corpus.Open(root) }

func runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Printf("papers %s %s %s/%s\n", papers.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return nil
}

// fieldNames is the twelve fields as a comma separated list, for flag help
// and for error messages.
func fieldNames() string {
	names := make([]string, 0, len(corpus.Fields))
	for _, f := range corpus.Fields {
		names = append(names, string(f))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
