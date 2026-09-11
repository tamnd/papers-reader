package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/sources"

	"gopkg.in/yaml.v3"
)

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "resolve these papers only, comma separated")
	field := fs.String("field", "", "resolve one field only")
	all := fs.Bool("all", false, "resolve every paper that has no record yet")
	again := fs.Bool("again", false, "resolve papers that already have a record too")
	only := fs.String("only", "", "use these rungs of the ladder only, comma separated")
	noCache := fs.Bool("no-cache", false, "ignore the response cache and ask the services again")
	dry := fs.Bool("dry-run", false, "print what would be written and change nothing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers resolve [flags]

Finds where each paper can legally be fetched from and writes the answer to
manifests/sources.yaml. The ladder is tried in order and stops at the first
candidate that is verified and has somewhere to fetch from:

    %s

Every candidate that is not pinned or seeded in the manifest has to pass three
checks before it is accepted: the titles must be at least %.2f similar, the
years must be within %d, and an author must be shared. A candidate that fails
any of them is written to reports/resolve.md as a near miss and the paper
stays unresolved, which is a fine state and a much better one than confidently
wrong.

The access class comes from the licence and from nothing else. A paper whose
licence nobody has a rule for stays unknown, and unknown publishes nothing.

`, strings.Join(sources.Rungs, " -> "), sources.TitleFloor, sources.YearSlack)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to resolve: --id, --field or --all")
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

	todo, err := selectPapers(manifest, recorded, *ids, *field, *again)
	if err != nil {
		return err
	}
	if len(todo) == 0 {
		fmt.Println("nothing to resolve")
		return nil
	}

	cache := c.Work("resolve")
	if *noCache {
		cache = ""
	}
	client := sources.NewClient(papers.Version, cache)
	r := &sources.Resolver{Client: client, Skip: skipped(*only)}

	ctx := context.Background()
	results := make([]*sources.Result, 0, len(todo))
	for _, p := range todo {
		res := r.Resolve(ctx, p)
		results = append(results, res)
		fmt.Println(line(res))
	}

	if *dry {
		fmt.Println("dry run, nothing written")
		return nil
	}
	if err := mergeSources(c, recorded, results); err != nil {
		return err
	}
	report := filepath.Join(c.Reports(), "resolve.md")
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(report, []byte(sources.Markdown(results)), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", c.SourcesManifest())
	fmt.Println("wrote", report)

	var stuck int
	for _, res := range results {
		if !res.OK() {
			stuck++
		}
	}
	fmt.Printf("%d papers, %d resolved, %d not\n", len(results), len(results)-stuck, stuck)
	return nil
}

// line is the one line printed per paper while the resolver runs, because a
// hundred papers at a second each is a minute and a half of staring at
// nothing otherwise.
func line(res *sources.Result) string {
	if !res.OK() {
		if n := len(res.Misses); n > 0 {
			return fmt.Sprintf("  %-34s unresolved, %d near misses", res.ID, n)
		}
		return fmt.Sprintf("  %-34s unresolved", res.ID)
	}
	return fmt.Sprintf("  %-34s %-10s %-14s %s", res.ID, res.Rung, res.Record.Access, res.Record.URL)
}

// selectPapers works out which papers to run.
func selectPapers(manifest *corpus.Papers, recorded *corpus.Sources, ids, field string, again bool) ([]corpus.Paper, error) {
	var want map[string]bool
	if ids != "" {
		want = map[string]bool{}
		for _, id := range strings.Split(ids, ",") {
			want[strings.TrimSpace(id)] = true
		}
	}

	var out []corpus.Paper
	for _, p := range manifest.Papers {
		switch {
		case want != nil && !want[p.ID]:
			continue
		case field != "" && string(p.Field) != field:
			continue
		}
		if !again {
			// A paper with a record has been resolved. Doing it again costs a
			// request to somebody else's free service for an answer already
			// on disk, so it takes --again to ask for it.
			if rec, ok := recorded.ByID(p.ID); ok && rec.URL != "" {
				continue
			}
		}
		out = append(out, p)
		if want != nil {
			delete(want, p.ID)
		}
	}
	for id := range want {
		return nil, fmt.Errorf("there is no paper called %s in the manifest", id)
	}
	return out, nil
}

func skipped(only string) map[string]bool {
	if only == "" {
		return nil
	}
	keep := map[string]bool{}
	for _, r := range strings.Split(only, ",") {
		keep[strings.TrimSpace(r)] = true
	}
	skip := map[string]bool{}
	for _, r := range sources.Rungs {
		if !keep[r] {
			skip[r] = true
		}
	}
	return skip
}

// mergeSources writes the new records into manifests/sources.yaml without
// losing the ones already there.
//
// A record that was edited by hand wins over one this run produced, which is
// the same rule as the top of the ladder and for the same reason. The file is
// rewritten sorted by id so that two runs in different orders produce the
// same diff.
func mergeSources(c *corpus.Corpus, recorded *corpus.Sources, results []*sources.Result) error {
	byID := map[string]corpus.Source{}
	for _, rec := range recorded.Sources {
		byID[rec.ID] = rec
	}
	for _, res := range results {
		if !res.OK() {
			continue
		}
		rec := res.Record
		if old, ok := byID[rec.ID]; ok {
			// Keep everything the fetcher and the classifier wrote, because
			// this command did not learn any of it and must not erase it.
			rec.Fetched, rec.SHA256, rec.Pages = old.Fetched, old.SHA256, old.Pages
			rec.TextLayer, rec.FiguresLicence = old.TextLayer, old.FiguresLicence
			if old.Note != "" {
				rec.Note = old.Note
			}
		}
		byID[rec.ID] = rec
	}

	out := corpus.Sources{Sources: make([]corpus.Source, 0, len(byID))}
	for _, rec := range byID {
		out.Sources = append(out.Sources, rec)
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })

	var b strings.Builder
	b.WriteString("# Where each paper was found and what may be published from it.\n")
	b.WriteString("#\n")
	b.WriteString("# Written by `papers resolve`. Hand edits survive a re-run, so a record corrected by a person stays corrected.\n")
	b.WriteString("# A paper with no entry here, or with access: unknown, publishes nothing at all.\n\n")

	enc, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	b.Write(enc)
	return os.WriteFile(c.SourcesManifest(), []byte(b.String()), 0o644)
}
