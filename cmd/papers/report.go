package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/llm/ledger"
	"github.com/tamnd/papers-reader/report"
	"github.com/tamnd/papers-reader/work"
)

func runReport(args []string) error {
	if len(args) == 0 {
		reportUsage(os.Stderr)
		return fmt.Errorf("say which report: usage")
	}
	switch args[0] {
	case "usage":
		return runReportUsage(args[1:])
	case "-h", "--help", "help":
		reportUsage(os.Stdout)
		return nil
	case "coverage", "graph":
		return fmt.Errorf("the %s report arrives in milestone M7", args[0])
	}
	reportUsage(os.Stderr)
	return fmt.Errorf("there is no report %s", args[0])
}

func reportUsage(w *os.File) {
	fmt.Fprint(w, `usage: papers report <usage> [flags]

    usage      what the machine time cost, by stage
    coverage   how much of each paper is done (milestone M7)
    graph      the citations between papers in the corpus (milestone M7)

Run papers report usage -h for the flags.
`)
}

func runReportUsage(args []string) error {
	fs := flag.NewFlagSet("report usage", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	write := fs.Bool("write", false, "write reports/usage.md as well as printing")
	prices := fs.String("prices", "", "a JSON file of dollars per million tokens by model")
	since := fs.String("since", "", "count the asks since a date (2026-09-01) or a window (168h)")
	path := fs.String("ledger", "", "read this ledger rather than the one beside the route file")
	quiet := fs.Bool("quiet", false, "print the summary line only")
	force := fs.Bool("force", false, "write a report with no asks in it over one that has some")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers report usage [flags]

Counts what the corpus cost in machine time: asks, pages, tokens, waiting
and money, by stage, by model and by paper.

It is read from the ledger, which is one line per ask and lives beside the
route file rather than in the corpus, because it names the hosts that were
asked. The report does not: it is committed to a public repository, so it
carries counts and no host names and no paths with a user name in them.

A stage that has never asked a model anything is still a row, with zeros in
it. A corpus extracted entirely through the native path has never asked a
model anything, and a report that left the stage out would read like a
report that lost it.

The money column is a dash for a model that is not in the price table, and
the table is empty unless -prices names one. A subscription and a free
gateway have no price per token, and printing zero dollars for them would
be claiming a measurement nobody made.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cutoff, err := when(*since)
	if err != nil {
		return err
	}
	table, err := report.LoadPrices(*prices)
	if err != nil {
		return err
	}
	if *prices != "" && table == nil {
		return fmt.Errorf("there is no price table at %s", *prices)
	}

	// The corpus is opened whether or not the report is being written,
	// because the pages that were read without a model are counted off the
	// content and they are most of the pages there are.
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	reads, err := report.ReadPages(c)
	if err != nil {
		return err
	}

	work.Configure()
	from := *path
	if from == "" {
		from = ledger.DefaultPath()
	}
	entries, err := ledger.Read(from)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		// Not an error. It is what a machine that has only run the native
		// path looks like, and the report says so in its own words.
		fmt.Fprintf(os.Stderr, "papers: nothing has been asked of a model on this machine (%s)\n", from)
	}
	if !cutoff.IsZero() {
		entries = ledger.Filter(entries, cutoff)
	}

	u := report.BuildUsage(entries, report.UsageOptions{
		Prices: table,
		Stages: askingStages(),
		Since:  cutoff,
		Reads:  reads,
	})
	if *quiet {
		fmt.Println(u.Summary())
	} else {
		fmt.Print(u.Markdown())
	}
	if !*write {
		return nil
	}
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	out := filepath.Join(c.Reports(), "usage.md")
	if err := keep(out, u, *force); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(u.Markdown()), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

// keep refuses to write a report with nothing in it over one that has a
// night of work in it.
//
// The ledger is on the machine that did the work and the report is in the
// corpus, so the two come apart as soon as anybody pulls the corpus onto a
// second machine. Running this there would otherwise turn a report full of
// numbers into a page of zeroes, and it would look like a stage that had
// been reverted rather than a report that had been rebuilt from nothing.
func keep(path string, u *report.Usage, force bool) error {
	if force || u.Asks > 0 {
		return nil
	}
	old, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(string(old), report.NoAsks) {
		return nil
	}
	return fmt.Errorf("%s counts asks this machine has no record of, and this run has none to put there: run it where the ledger is, or pass -force to overwrite it", path)
}

// askingStages is every stage that puts a question to a model, so that the
// table holds a row for each of them whether or not it has run.
func askingStages() []string {
	out := make([]string, 0, len(work.Stages))
	for _, s := range work.Stages {
		out = append(out, string(s))
	}
	return out
}

// when reads the window. A date is the common case and a duration is the
// one somebody types in a loop, so both are taken rather than making a
// person convert one into the other.
func when(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t.UTC(), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("-since takes a date like 2026-09-01 or a window like 168h, not %q", s)
	}
	if d < 0 {
		d = -d
	}
	return time.Now().UTC().Add(-d), nil
}
