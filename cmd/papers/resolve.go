package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/sources"
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
	merged, err := mergeSources(c, recorded, results)
	if err != nil {
		return err
	}
	report := filepath.Join(c.Reports(), "resolve.md")
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(report, []byte(sources.Markdown(wholeCorpus(manifest, merged, results))), 0o644); err != nil {
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

// choosePapers is the papers the flags name, in manifest order, and an error
// for an id that is not in the manifest.
//
// This is the whole of what a command that reads a file on disk needs, which
// is extract, classify and anything else that asks nobody anything. It is
// separate from selectPapers because the rules that follow that one are about
// not asking somebody else's free service twice for an answer already on
// disk, and one of them skips every paper that has a record at all, which is
// every paper those commands can work on.
func choosePapers(manifest *corpus.Papers, ids, field string) ([]corpus.Paper, error) {
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

// selectPapers is choosePapers for a command that goes out to the network,
// less the papers it should not ask about.
//
// The skipping happens after the manifest lookup and not during it, which is
// the point of the split. Doing both at once meant a paper that was found and
// then skipped came back as a paper that does not exist, so naming a hand
// recorded paper on the command line was answered with "there is no paper
// called chiu-1989-aimd in the manifest" over a paper sitting in the manifest
// two lines from the one before it.
func selectPapers(manifest *corpus.Papers, recorded *corpus.Sources, ids, field string, again bool) ([]corpus.Paper, error) {
	chosen, err := choosePapers(manifest, ids, field)
	if err != nil {
		return nil, err
	}
	var out []corpus.Paper
	for _, p := range chosen {
		if rec, ok := recorded.ByID(p.ID); ok {
			// A record a person decided is left alone whatever the flags say.
			// --again means ask the services again, not throw away somebody's
			// afternoon of reading a publisher's terms.
			if rec.Hand() {
				continue
			}
			// A paper with a record has been resolved. Doing it again costs a
			// request to somebody else's free service for an answer already
			// on disk, so it takes --again to ask for it.
			if !again && rec.URL != "" {
				continue
			}
		}
		out = append(out, p)
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
// the same rule as the top of the ladder and for the same reason.
func mergeSources(c *corpus.Corpus, recorded *corpus.Sources, results []*sources.Result) (map[string]corpus.Source, error) {
	byID := index(recorded)
	for _, res := range results {
		if !res.OK() {
			continue
		}
		rec := res.Record
		if old, ok := byID[rec.ID]; ok {
			// Twice, because this is the one thing in the toolchain that can
			// quietly undo a person's work. selectPapers does not offer a
			// hand record to the ladder in the first place, and if one gets
			// here anyway it is not written over.
			if old.Hand() {
				continue
			}
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
	return byID, writeSources(c, byID)
}

// wholeCorpus is every paper in the manifest, as a result, so that the report
// describes the corpus and not the run.
//
// A paper this run resolved contributes its result, with its rung and its
// near misses. A paper it did not touch contributes whatever sources.yaml
// already says about it. A paper with neither is in the manifest and has
// never been looked at, which is the most important line in the report and
// the easiest one to lose.
func wholeCorpus(manifest *corpus.Papers, merged map[string]corpus.Source, results []*sources.Result) []*sources.Result {
	fresh := map[string]*sources.Result{}
	for _, res := range results {
		fresh[res.ID] = res
	}
	all := make([]*sources.Result, 0, len(manifest.Papers))
	for _, p := range manifest.Papers {
		if res, ok := fresh[p.ID]; ok {
			all = append(all, res)
			continue
		}
		if rec, ok := merged[p.ID]; ok {
			all = append(all, sources.Prior(rec))
			continue
		}
		all = append(all, &sources.Result{
			ID:    p.ID,
			Notes: []string{"no record in sources.yaml, so nothing has ever looked for it"},
		})
	}
	return all
}
