package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/route"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/roundtrip"
	"github.com/tamnd/papers-reader/translate"
	"github.com/tamnd/papers-reader/work"
)

func runRoundtrip(args []string) error {
	fs := flag.NewFlagSet("roundtrip", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "check these papers only, comma separated")
	field := fs.String("field", "", "check one field only")
	all := fs.Bool("all", false, "check every paper that has a translation")
	langs := fs.String("lang", "", "which languages to check, comma separated, or every one of them")
	routes := fs.String("routes", "", "path to a routing table, instead of the one in the config directory")
	top := fs.Int("top", roundtrip.Default("").Top, "how many papers at the head of the canon are checked whole")
	rate := fs.Int("rate", roundtrip.Default("").Rate, "the percentage of the remaining sections that are checked")
	limit := fs.Int("limit", 0, "stop after this many pages, for a run on a budget")
	dry := fs.Bool("dry-run", false, "print the sample and ask nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers roundtrip [flags]

Puts a sample of the translated pages back into English and asks a judge
whether the two Englishes claim the same things.

This is the only check in the toolchain that reads what a translation
means. Everything else compares form: the audit counts the formulas, the
citations, the listings and the tags, and looks for prose that is still in
English. All of it passes a page that is well formed, fluent and wrong. A
dropped negation, a bound that turned into its converse, a hedge that
hardened into a claim: those are the mistranslations that reach a reader,
because nothing about them looks like a mistake.

Two asks per page, so it runs on a sample. Every abstract, every section of
the papers at the head of the canon, and a percentage of the rest. The
sample is drawn from the hash of the translation prompt, so it does not
move between runs of one build and it is redrawn when the prompt changes.

The back translation is sent somewhere other than the model that wrote the
translation, and the judge somewhere other than that again, as far as the
routing table allows. A model checking its own work agrees with itself. A
page the fleet could only check against itself is still checked and the
report says so on the line.

A verdict of differs-materially is written into the page's front matter,
which puts it back on the translate queue: the next papers translate run
asks for it again. The whole run goes to reports/roundtrip.md with both
Englishes side by side, because a verdict nobody can check is an opinion.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to check: -id, -field or -all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	want, err := languages(*langs)
	if err != nil {
		return err
	}
	p, err := prompt.Get(prompt.Translate)
	if err != nil {
		return err
	}
	policy := roundtrip.Policy{Top: *top, Rate: *rate, Seed: p.SHA}

	pages, err := pagesOf(c, todo, want)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return fmt.Errorf("there is nothing translated to check: run papers translate first")
	}
	sample := roundtrip.Pick(pages, policy)
	if *limit > 0 && len(sample) > *limit {
		sample = sample[:*limit]
	}
	fmt.Printf("%d translated pages, %d in the sample, %d asks\n", len(pages), len(sample), 2*len(sample))
	if *dry {
		for _, s := range sample {
			fmt.Printf("  %s (%s)\n", s.Path(), s.Why)
		}
		fmt.Println("dry run, nothing asked and nothing written")
		return nil
	}

	logf := func(format string, args ...any) { fmt.Printf("    "+format+"\n", args...) }
	ask, from, err := second(*routes, logf)
	if err != nil {
		return err
	}
	fmt.Printf("the questions go to the hosts in the %s routing table\n", from)

	checker := &roundtrip.Checker{Ask: ask, Logf: logf}
	rep := &roundtrip.Report{Run: work.RunID(), Policy: policy}
	ctx := context.Background()
	for _, s := range sample {
		check, err := one(ctx, c, checker, s)
		rep.Usage = sum(rep.Usage, check.Usage)
		if err != nil {
			// One page the fleet could not answer for is not a reason to
			// throw away the pages it did answer for. It is a reason to say
			// so in the report, which is what Failed is.
			fmt.Printf("%s: %v\n", s.Path(), err)
			rep.Failed = append(rep.Failed, fmt.Sprintf("`%s` %v", s.Path(), err))
			continue
		}
		rep.Checks = append(rep.Checks, check)
		if err := record(c, s, check.Verdict, rep.Run); err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", s.Path(), check.Verdict)
	}

	path := filepath.Join(c.Reports(), "roundtrip.md")
	if err := os.MkdirAll(c.Reports(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(rep.Markdown()), 0o644); err != nil {
		return err
	}
	fmt.Println(rep.Summary())
	fmt.Printf("written to %s\n", path)
	if n := len(rep.Material()); n > 0 {
		fmt.Printf("%d pages are back on the translate queue\n", n)
	}
	return nil
}

// pagesOf lists every translated page the check could look at.
func pagesOf(c *corpus.Corpus, papers []corpus.Paper, langs []corpus.Lang) ([]roundtrip.Page, error) {
	var out []roundtrip.Page
	for _, p := range papers {
		for _, l := range langs {
			dir := c.Content(l, p.ID)
			entries, err := os.ReadDir(dir)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var names []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, name := range names {
				b, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					return nil, err
				}
				front, _, err := corpus.ParseFront(b)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
				}
				out = append(out, roundtrip.Page{
					Paper: p.ID, Lang: l, Name: name,
					Kind: front.Kind, Number: p.Number, Model: front.TranslationModel,
				})
			}
		}
	}
	return out, nil
}

// one reads both sides of a page off the disk and checks it.
//
// The English it compares against is the file the translation says it came
// from, not the file of the same name, because a section can be renamed and
// a check that silently compared a page with a different page would be
// worse than no check.
func one(ctx context.Context, c *corpus.Corpus, checker *roundtrip.Checker, s roundtrip.Sample) (roundtrip.Check, error) {
	b, err := os.ReadFile(filepath.Join(c.Root, s.Path()))
	if err != nil {
		return roundtrip.Check{Sample: s}, err
	}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		return roundtrip.Check{Sample: s}, err
	}
	from := front.TranslatedFrom
	if from == "" {
		from = filepath.Join("content", string(corpus.EN), s.Paper, s.Name)
	}
	english, err := os.ReadFile(filepath.Join(c.Root, from))
	if err != nil {
		return roundtrip.Check{Sample: s}, err
	}
	_, source, err := corpus.ParseFront(english)
	if err != nil {
		return roundtrip.Check{Sample: s}, fmt.Errorf("%s: %w", from, err)
	}
	return checker.Run(ctx, s, translate.Paper{
		ID: front.Paper, Title: front.Title, Field: front.Field,
	}, string(source), string(body))
}

// record writes the verdict into the page's front matter.
//
// A verdict of differs-materially is what puts the page back on the
// translate queue, so it has to be on the page and not only in the report:
// the report is rewritten by every run and the queue has to survive one.
func record(c *corpus.Corpus, s roundtrip.Sample, v roundtrip.Verdict, run string) error {
	path := filepath.Join(c.Root, s.Path())
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		return err
	}
	front.Roundtrip, front.RoundtripRun = string(v), run
	out, err := corpus.Render(front, body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// second builds the asker, which steers each question away from a model.
//
// The registry is filtered by model rather than by route name because what
// matters is who answers and not which host relayed it. Three routes
// fronting one model are one opinion. When the filter leaves nothing, the
// question goes to the whole fleet anyway and the check records that it was
// a model marking its own work: a self-check still catches a dropped
// paragraph, and refusing to run is a worse answer than running and saying
// what it is worth.
func second(path string, logf func(string, ...any)) (func(context.Context, string, string, llm.Request) (translate.Reply, error), string, error) {
	registry, from, err := work.Routes(path)
	if err != nil {
		return nil, from, err
	}
	whole, _, err := work.Fleet(work.Roundtrip, path, false, logf)
	if err != nil {
		return nil, from, err
	}
	return func(ctx context.Context, target, avoid string, req llm.Request) (translate.Reply, error) {
		asker := whole
		if other := without(registry, avoid); !work.Pool(other).Empty() {
			up := *whole
			up.Waiter = &work.Waiter{Pool: work.Pool(other), Logf: logf}
			asker = &up
		}
		answer, err := asker.Do(ctx, target, req)
		return translate.Reply{Response: answer.Response, Model: answer.Model, Route: answer.Route}, err
	}, from, nil
}

// without is the routing table with every route serving one model taken out.
func without(registry route.Registry, model string) route.Registry {
	if strings.TrimSpace(model) == "" {
		return registry
	}
	out := route.Registry{}
	for _, r := range registry.Routes {
		if r.Model != model {
			out.Routes = append(out.Routes, r)
		}
	}
	return out
}
