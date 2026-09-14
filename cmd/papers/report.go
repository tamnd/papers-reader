package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/llm/ledger"
	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
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
	case "coverage":
		return runReportCoverage(args[1:])
	case "graph":
		return runReportGraph(args[1:])
	case "all":
		return runReportAll(args[1:])
	case "-h", "--help", "help":
		reportUsage(os.Stdout)
		return nil
	}
	reportUsage(os.Stderr)
	return fmt.Errorf("there is no report %s", args[0])
}

func reportUsage(w *os.File) {
	fmt.Fprint(w, `usage: papers report <all|coverage|graph|usage> [flags]

    all        every report below, and the audit, written in one pass
    coverage   how much of each paper is published, and what the rest waits on
    graph      the citations between papers in the corpus
    usage      what the machine time cost, by stage

Run papers report coverage -h, and the same for the others, for the flags.
`)
}

func runReportCoverage(args []string) error {
	fs := flag.NewFlagSet("report coverage", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	write := fs.Bool("write", false, "write reports/coverage.md as well as printing")
	quiet := fs.Bool("quiet", false, "print the summary line only")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers report coverage [flags]

Counts how much of each paper the corpus publishes, per field and per
language, and what the rest is waiting on.

A paper is full, stub or none. Full is a paper whose body is here. Stub is
the front matter and a short abstract, which is the whole of what a
restricted paper may ever have, so it counts as done rather than as a
shortfall. None is a paper nothing is published of yet.

The last table is the one to act on: it names every paper that is not done
and what it is waiting on, which is a fetch, a licence check, a layout tool
or a vision model. The count of papers behind one missing tool is the
number that decides whether to go and get it.

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
	cov, err := report.BuildCoverage(c)
	if err != nil {
		return err
	}
	if *quiet {
		fmt.Println(cov.Summary())
	} else {
		fmt.Print(cov.Markdown())
	}
	if !*write {
		return nil
	}
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	// No keep() here, unlike the usage report. This one is counted off the
	// corpus itself, so a second checkout builds the same file as the first
	// and there is nothing on one machine that the other cannot see.
	out := filepath.Join(c.Reports(), "coverage.md")
	if err := os.WriteFile(out, []byte(cov.Markdown()), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func runReportGraph(args []string) error {
	fs := flag.NewFlagSet("report graph", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	write := fs.Bool("write", false, "write reports/graph.md as well as printing")
	quiet := fs.Bool("quiet", false, "print the summary line only")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers report graph [flags]

Draws the citations that run between papers of the corpus: which paper
cites which, which are cited most, and which are connected to nothing yet.

An edge is a bibliography entry of one paper here that was resolved to
another paper here. Almost every reference points somewhere else, which is
what a hundred papers spread over eighty years looks like, so the edge
count is small next to the reference count and that is the shape rather
than a shortfall.

The last table is the one to act on. It names the papers outside the
corpus that two or more papers inside it cite, which is the reading list
for deciding what to add next.

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
	g, err := report.BuildGraph(c)
	if err != nil {
		return err
	}
	if *quiet {
		fmt.Println(g.Summary())
	} else {
		fmt.Print(g.Markdown())
	}
	if !*write {
		return nil
	}
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	// No keep() here, for the same reason coverage has none: this is
	// counted off the corpus, so a second checkout builds the same file.
	out := filepath.Join(c.Reports(), "graph.md")
	if err := os.WriteFile(out, []byte(g.Markdown()), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func runReportUsage(args []string) error {
	fs := flag.NewFlagSet("report usage", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	write := fs.Bool("write", false, "write reports/usage.md as well as printing")
	prices := fs.String("prices", "", "a JSON file of dollars per million tokens by model (default manifests/prices.json)")
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

The money column is a dash for a model that is not in the price table. The
table is the corpus one unless -prices names another, and every rate in it
carries a sentence saying where the rate came from, printed under the model
table. Most of this fleet is a subscription or a local GPU and is priced at
zero, which is the marginal cost of an ask and not the cost of the fleet. A
model nobody has priced gets a dash rather than a zero, because those two
things are not the same and the difference is the whole point of the
column.

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
	// The corpus is opened whether or not the report is being written,
	// because the pages that were read without a model are counted off the
	// content and they are most of the pages there are.
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	// The corpus carries the price table for its own fleet, so the ordinary
	// run needs no flag and two people rebuilding the report get the same
	// money out of it. The flag is for pricing the same ledger against
	// somebody else's rate card.
	rates := *prices
	if rates == "" {
		rates = c.PricesManifest()
	}
	table, err := report.LoadPrices(rates)
	if err != nil {
		return err
	}
	if *prices != "" && table == nil {
		return fmt.Errorf("there is no price table at %s", *prices)
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

// runReportAll writes every report the corpus carries in one pass.
//
// The four are built from three different places and the order here is the
// order a person would want them in. Coverage and the graph are counted off
// the content, so they are the same on any checkout. The audit is the same
// again. Usage is read from the ledger, which lives beside the route file on
// the machine that did the work, so it is the one that can be missing, and
// a run on a second checkout writes the other three and says why it left
// that one alone rather than overwriting a night of numbers with zeroes.
//
// It never fails on a finding. A report that refused to be written while
// the corpus had a problem in it would be a report nobody could use to
// diagnose the problem. `papers audit -hard` is the gate and this is not.
func runReportAll(args []string) error {
	fs := flag.NewFlagSet("report all", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers report all [flags]

Writes reports/coverage.md, reports/graph.md, reports/audit.md and
reports/usage.md, and prints a summary line for each.

This is the step before publishing. Running the four commands by hand is
four chances to forget one, and a corpus whose coverage says ninety seven
per cent while its audit was written a week ago is worse than one with no
reports at all.

The usage report is the one that can be skipped. It is read from the
ledger, which lives on the machine that did the work rather than in the
corpus, so on a second checkout there is nothing to build it from. The run
says so and leaves the committed one alone.

Findings do not fail this command. Use papers audit -hard for that.

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
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}

	cov, err := report.BuildCoverage(c)
	if err != nil {
		return err
	}
	if err := saved(c, "coverage.md", cov.Markdown(), cov.Summary()); err != nil {
		return err
	}

	g, err := report.BuildGraph(c)
	if err != nil {
		return err
	}
	if err := saved(c, "graph.md", g.Markdown(), g.Summary()); err != nil {
		return err
	}

	in, err := audit.Load(c)
	if err != nil {
		return err
	}
	rep := audit.Run(in, false)
	for _, err := range rep.Errors() {
		fmt.Fprintf(os.Stderr, "papers: %v\n", err)
	}
	if err := saved(c, "audit.md", rep.Markdown(), rep.Summary()); err != nil {
		return err
	}

	work.Configure()
	from := ledger.DefaultPath()
	entries, err := ledger.Read(from)
	if err != nil {
		return err
	}
	table, err := report.LoadPrices(c.PricesManifest())
	if err != nil {
		return err
	}
	reads, err := report.ReadPages(c)
	if err != nil {
		return err
	}
	u := report.BuildUsage(entries, report.UsageOptions{
		Prices: table,
		Stages: askingStages(),
		Reads:  reads,
	})
	path := filepath.Join(c.Reports(), "usage.md")
	if err := keep(path, u, false); err != nil {
		fmt.Printf("usage.md: left alone, %v\n", err)
		return nil
	}
	return saved(c, "usage.md", u.Markdown(), u.Summary())
}

// saved puts one report in the corpus and says what went in it, so that a
// run of four of them reads as four lines rather than as silence.
func saved(c *corpus.Corpus, name, body, summary string) error {
	path := filepath.Join(c.Reports(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", name, summary)
	return nil
}
