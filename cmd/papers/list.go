package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tamnd/papers-reader/corpus"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	field := fs.String("field", "", "only papers in this field: "+fieldNames())
	collection := fs.String("collection", "", "only papers in this reading list")
	status := fs.String("status", "", "only papers at this status")
	count := fs.Bool("count", false, "print counts per field instead of rows")
	ids := fs.Bool("ids", false, "print bare ids, one per line, for piping")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers list [flags]

Lists what the corpus holds.

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
	m, err := c.LoadPapers()
	if err != nil {
		return err
	}

	selected := m.Papers
	if *collection != "" {
		cols, err := c.LoadCollections()
		if err != nil {
			return err
		}
		col, ok := cols.ByID(*collection)
		if !ok {
			return fmt.Errorf("there is no collection called %s", *collection)
		}
		members, err := col.Resolve(m)
		if err != nil {
			return err
		}
		selected = nil
		for _, id := range members {
			if p, ok := m.ByID(id); ok {
				selected = append(selected, *p)
			}
		}
	}

	if *field != "" {
		f, err := corpus.ParseField(*field)
		if err != nil {
			return fmt.Errorf("%w: the fields are %s", err, fieldNames())
		}
		selected = filter(selected, func(p corpus.Paper) bool { return p.Field == f })
	}
	if *status != "" {
		s := corpus.Status(*status)
		if !s.Valid() {
			return fmt.Errorf("%q is not a status", *status)
		}
		selected = filter(selected, func(p corpus.Paper) bool { return p.Status == s })
	}

	if *count {
		return printCounts(selected)
	}
	if *ids {
		for _, p := range selected {
			fmt.Println(p.ID)
		}
		return nil
	}
	return printRows(c, selected)
}

func filter(ps []corpus.Paper, keep func(corpus.Paper) bool) []corpus.Paper {
	var out []corpus.Paper
	for _, p := range ps {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

// printRows prints one line per paper. The access column is the fact from
// sources.yaml and not the guess in the manifest, so a fresh corpus shows
// every paper as unknown, which is what it is.
func printRows(c *corpus.Corpus, ps []corpus.Paper) error {
	src, err := c.LoadSources()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, p := range ps {
		number := ""
		if p.Number > 0 {
			number = fmt.Sprint(p.Number)
		}
		access := corpus.AccessUnknown
		if src != nil {
			access = src.Access(p.ID)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			number, p.ID, p.Year, p.Field, p.Status, access, truncate(p.Title, 60))
	}
	return w.Flush()
}

func printCounts(ps []corpus.Paper) error {
	counts := map[corpus.Field]int{}
	for _, p := range ps {
		counts[p.Field]++
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	total := 0
	for _, f := range corpus.Fields {
		fmt.Fprintf(w, "%s\t%d\t%s\n", f, counts[f], f.Title())
		total += counts[f]
	}
	fmt.Fprintf(w, "total\t%d\t\n", total)
	return w.Flush()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-3]), " ") + "..."
}
