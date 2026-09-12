package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/layout"
	"github.com/tamnd/papers-reader/poppler"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/render"
	"github.com/tamnd/papers-reader/work"
)

func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "extract these papers only, comma separated")
	field := fs.String("field", "", "extract one field only")
	all := fs.Bool("all", false, "extract every paper on disk whose path is implemented")
	path := fs.String("path", "", "force an extraction path instead of the measured one")
	tool := fs.String("tool", "", "which layout tool to run: mineru, marker or docling")
	routes := fs.String("routes", "", "read this route file rather than the usual one")
	pages := fs.String("pages", "", "extract these pages only, as N or N-M")
	again := fs.Bool("again", false, "extract pages that are already done too")
	retidy := fs.Bool("retidy", false, "run the tidier over the pages on disk and ask no model")
	recheck := fs.Bool("recheck", false, "run the acceptance rules over the pages on disk and ask no model")
	dry := fs.Bool("dry-run", false, "print what would be extracted and write nothing")
	long := fs.Bool("v", false, "print every page and every refusal")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers extract [flags]

Turns the pages of a fetched PDF into text, one file per page under
work/<id>/pages/NNNN.txt. Nothing here is committed: these are the
intermediate the assembler joins and the splitter cuts into sections.

Which path a paper takes was measured by papers classify and is recorded in
manifests/sources.yaml.

    native    pdftotext reads the file and no model is in the path
    layout    a layout model reads the geometry
    vision    a vision model reads a picture of the page

The native path asks pdftotext for the word boxes rather than for text,
because pdftotext -layout interleaves the columns of a two column paper
into nonsense. The columns, the reading order, the running heads and the
paragraphs are worked out here from the geometry.

The layout path runs mineru, marker or docling over the whole file once and
reads the JSON it wrote, never its Markdown: the three write three dialects
and the corpus writes its own. The run costs minutes and its output is kept
under work/<id>/layout, so a second run over a paper that is already
converted reads the file it wrote and starts no process. Pass --tool to pick
one; without it the first installed is used, in the order above.

The vision path shows a picture of each page to a model, one page per ask.
The raster comes from papers render, and a page that is not rendered yet is
rendered here, so running that command first is an optimisation and not a
prerequisite. The prompt is pinned and its hash goes in the front matter of
everything the page becomes, and a paper whose notation needs a sentence of
explanation gets one added to it, which is per paper so that editing it does
not mark every other paper's pages stale.

Every page is checked against the eight acceptance rules before it is
written, and a page that breaks one is reported and not written. The native
and layout paths have nothing to retry with: the same program over the same
file reads the page the same way the second time, so a refused page is a
page for a person to look at, and it stays in the report until somebody
does. The thing to try is the other path, with --path. The vision path does
retry, because a model asked twice answers twice: a refused page goes back
at 400 dpi and then at 600, and a page refused at all three is reported and
left alone.

A page that is already extracted is left alone, so running this twice costs
one read of the file and no writes. Pass --again to do the work over.

--retidy rewrites the pages on disk through the tidier and asks no model at
all. It is for the day the tidier learns something new: when Untable was
added, four pages of the Transformer paper were already on disk with their
tables in HTML, and the choice was between re-reading them on a vision
model and running the new step over the text that was already correct.
Re-reading costs minutes of a rationed reader and comes back different, so
a page that was right can come back wrong. Every step in Tidy is a
translation between two spellings and none of them changes a page that is
already in the second spelling, so running it again over a tidy page writes
nothing.

--recheck runs the acceptance rules over the pages on disk, asks no model
and writes nothing. It is for the day a rule is added: every page in the
corpus was accepted by the rules as they stood when it was read, and a new
rule has never seen any of them. A9 was added this way and found a
paragraph that a reader had dropped under a figure four pages into a paper
that was already published.

A restricted paper is read as far as its third page and no further. Nothing
but its front matter and a short abstract can ever be published, so the
rest of it would be work done to fill a directory nobody may read.

This needs poppler, and the layout path needs one of the three tools. Run
papers doctor to see what is installed.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to extract: --id, --field or --all")
	}
	first, last, err := pageRange(*pages)
	if err != nil {
		return err
	}
	switch classify.Path(*path) {
	case "", classify.PathNative, classify.PathLayout, classify.PathVision:
	default:
		return fmt.Errorf("%q is not an extraction path: it is native, layout or vision", *path)
	}
	if *tool != "" && !known(layout.Tool(*tool)) {
		return fmt.Errorf("%q is not a layout tool: run papers doctor to see which are installed", *tool)
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
	todo, err := choosePapers(manifest, *ids, *field)
	if err != nil {
		return err
	}

	if *retidy {
		return retidyPages(c, todo, first, last, *dry, *long)
	}
	if *recheck {
		return recheckPages(c, recorded, todo, first, last, *long)
	}

	// One renderer for the whole run. Building it reads and hashes the KaTeX
	// bundle, and acceptance rule A4 asks it about every math span of every
	// page. A machine without a usable build gets the other seven rules and
	// is told which one did not run, rather than a run that stops.
	math, err := katex.New()
	if err != nil {
		fmt.Printf("KaTeX is not usable here, so acceptance rule A4 will not run: %v\n", err)
	}

	// The fleet is assembled the first time a paper needs it and not before.
	// A native run must work on a machine with no route file at all, and most
	// runs are native runs; building the pool up front would make every one of
	// them fail on a paper that was never going to a model.
	var (
		once  sync.Once
		asker *work.Ask
		fleet error
	)
	ready := func() (*work.Ask, error) {
		once.Do(func() {
			var from string
			asker, from, fleet = work.Fleet(work.Extract, *routes, true, func(format string, args ...any) {
				fmt.Printf("    "+format+"\n", args...)
			})
			if fleet == nil {
				fmt.Printf("the pages go to the hosts in the %s routing table\n", from)
			}
		})
		return asker, fleet
	}

	ctx := context.Background()
	var done, refused, skipped int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		run := extraction{
			corpus: c, paper: p, source: rec, math: math,
			forced: classify.Path(*path), tool: layout.Tool(*tool),
			fleet: ready,
			first: first, last: last,
			again: *again, dry: *dry, long: *long,
		}
		n, err := run.do(ctx)
		switch {
		case err != nil:
			fmt.Printf("  %-34s %v\n", p.ID, err)
			skipped++
		default:
			done += n.written
			refused += n.refused
			if n.written+n.refused > 0 || *long {
				fmt.Printf("  %-34s %s\n", p.ID, n)
			}
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d pages extracted, %d refused, %d papers skipped\n", done, refused, skipped)
	if refused > 0 {
		return fmt.Errorf("%d pages did not pass the acceptance rules", refused)
	}
	return nil
}

// count is what one paper's run came to.
type count struct {
	written, refused, already int
}

func (n count) String() string {
	parts := []string{fmt.Sprintf("%d pages", n.written)}
	if n.already > 0 {
		parts = append(parts, fmt.Sprintf("%d already done", n.already))
	}
	if n.refused > 0 {
		parts = append(parts, fmt.Sprintf("%d refused", n.refused))
	}
	return strings.Join(parts, ", ")
}

// An extraction is one paper's run. It is a struct rather than a dozen
// arguments because the two paths want almost the same things and a reader
// comparing them should not have to count parameters to see where they
// differ.
type extraction struct {
	corpus *corpus.Corpus
	paper  corpus.Paper
	source *corpus.Source
	math   *katex.Renderer
	forced classify.Path
	tool   layout.Tool
	// fleet is the asker, built on demand and once for the whole run. Only
	// the vision path calls it.
	fleet func() (*work.Ask, error)

	first, last int
	again       bool
	dry         bool
	long        bool
}

// restrictedPages is how far into a paper that may not be redistributed this
// program reads.
//
// Everything published from such a paper is its front matter and an abstract
// of at most 250 words, and it used to stop at one page on the reasoning that
// both are on the first. Three of the first eight papers in this corpus put
// something else there: the Paxos paper opens with Lamport's own note about
// where the article appeared, and the abstract is on the page after it, so
// the published file was a title, an author and a citation. Three pages is
// past every cover sheet in the corpus and is still nothing anybody could
// mistake for a copy of the paper. What may be published is capped by
// split.Restrict and by audit rule S07, not by this.
const restrictedPages = 3

// seed feeds the pages already on disk through the checker, and through the
// folio learner where there is one, before anything is asked.
//
// Acceptance rule A5 is about how long this paper's pages usually are and
// rule A6 is about where its printed numbering starts, and neither question
// can be answered out of the pages of one run. A resumed run that saw only
// the last three pages of a paper knows neither.
//
// Every page on disk, and not the range the run was given. Walking the range
// is right for a resumed run over a whole paper and wrong for every other use
// of -pages, which is how a good page came to be thrown away: `-pages 7
// -again` gave the loop a range of exactly one page, that page was the page
// being re-read, so nothing at all was seeded, A5 stood down for want of a
// sample, and an answer that stopped after one sentence replaced a page that
// was correct. A rule that is off whenever it is asked about a single page is
// off exactly when a person is looking hardest at that page.
//
// A page in want is left out, because its old text is what the new text is
// about to replace and a page should not set the standard it is judged
// against.
func seed(store extract.Store, want map[int]bool, checker *extract.Checker, folios *extract.Folios) {
	pages, err := store.Pages()
	if err != nil {
		return
	}
	for _, page := range pages {
		if want[page] {
			continue
		}
		done, err := store.Read(page)
		if err != nil {
			continue
		}
		checker.Check(page, done)
		if folios != nil {
			folios.Add(page, done)
		}
	}
}

// recheckPages runs the acceptance rules over the pages that are already on
// disk, and writes nothing.
//
// Every page in a corpus was accepted by the rules as they stood on the day
// it was read, which means a rule added since has never been applied to any
// of them. Re-extracting to find out costs a model run per page and comes
// back with different text, so the answer would be about the new text and not
// about what is in the corpus. This reads what is there.
//
// The checker is built the way the path that wrote the pages built it. Model
// is on for a paper a model read, because A5 and A9 are about a reader that
// can stop halfway down a page and pdftotext cannot. A6 needs the paper's
// folios, which are learned from the pages themselves, so every page is fed
// in before any of them is judged.
func recheckPages(c *corpus.Corpus, recorded *corpus.Sources, todo []corpus.Paper, first, last int, long bool) error {
	math, err := katex.New()
	if err != nil {
		fmt.Printf("KaTeX is not usable here, so acceptance rule A4 will not run: %v\n", err)
	}
	ctx := context.Background()
	var checked, faulted, skipped int
	for _, p := range todo {
		store := extract.Store{Dir: c.Work(p.ID, "pages")}
		pages, err := store.Pages()
		if err != nil || len(pages) == 0 {
			skipped++
			continue
		}
		record, err := extract.ReadRecord(c.Work(p.ID))
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			skipped++
			continue
		}
		text := map[int]string{}
		folios := &extract.Folios{}
		for _, page := range pages {
			body, err := store.Read(page)
			if err != nil {
				continue
			}
			text[page] = body
			folios.Add(page, body)
		}
		model := record != nil && record.Path != string(classify.PathNative)
		checker := &extract.Checker{Math: math, Model: model, Map: folios.Map()}
		if rec, ok := recorded.ByID(p.ID); ok && model {
			switch classify.Layer(rec.TextLayer) {
			case classify.Native, classify.Digital:
				checker.Layer = pageLayer(ctx, c.PDF(p.ID))
			}
		}
		var n int
		faults := judge(checker, pages, text)
		for _, page := range pages {
			if first > 0 && (page < first || page > last) {
				continue
			}
			checked++
			for _, f := range faults[page] {
				n++
				fmt.Printf("  %-34s page %d: %s\n", p.ID, page, f)
			}
		}
		faulted += n
		if long && n == 0 {
			fmt.Printf("  %-34s %d pages, nothing to report\n", p.ID, len(pages))
		}
	}
	fmt.Printf("%d pages rechecked, %d faults, %d papers skipped\n", checked, faulted, skipped)
	return nil
}

// judge applies the acceptance rules to every page of a paper and says what
// each page broke.
//
// Two passes, because rule A5 is about this paper's pages and has no opinion
// about any of them until it has seen all of them. The first pass only
// observes and the second only asks, which is what extract.Checker.Faults is
// for: one pass that did both would count every page twice, and a sample with
// each page in it twice has the same mean and a narrower variance, so A5 would
// stand further and further off until it stopped firing.
//
// Each page is still measured against a sample that includes itself, which
// widens the mean by one page in however many. That is the price of not asking
// a model, and it is small on any paper long enough for A5 to run at all.
func judge(checker *extract.Checker, pages []int, text map[int]string) map[int][]extract.Fault {
	for _, page := range pages {
		checker.Check(page, text[page])
	}
	out := map[int][]extract.Fault{}
	for _, page := range pages {
		if faults := checker.Faults(page, text[page]); len(faults) > 0 {
			out[page] = faults
		}
	}
	return out
}

// pageLayer is the text layer of one page of a file, for rule A9 outside a
// live extraction run.
func pageLayer(ctx context.Context, file string) func(int) (string, bool) {
	return keepLayer(func(page int) (string, error) {
		return poppler.Page(ctx, file, page, page)
	})
}

// keepLayer reads a page's text layer once and keeps it.
//
// Reading one is a pdftotext process, and judge asks for every page twice, so
// without this a recheck of the corpus would fork twice per page for an answer
// that cannot have changed in between. A page pdftotext could not read is kept
// as absent rather than as empty, so the second ask does not retry it either.
func keepLayer(read func(int) (string, error)) func(int) (string, bool) {
	seen := map[int]string{}
	return func(page int) (string, bool) {
		body, ok := seen[page]
		if !ok {
			var err error
			if body, err = read(page); err != nil {
				body = ""
			}
			seen[page] = body
		}
		return body, body != ""
	}
}

// retidyPages runs the tidier over the pages that are already on disk.
//
// The acceptance rules are not run again and neither is anything else. A page
// on disk has already passed them, and Tidy takes wrapping off rather than
// putting anything in, so a page that passed before passes after. Running the
// checker here would also need the paper's own sample to run rule A5 against,
// and the sample would be half the old spelling and half the new one.
func retidyPages(c *corpus.Corpus, todo []corpus.Paper, first, last int, dry, long bool) error {
	var changed, same, skipped int
	for _, p := range todo {
		store := extract.Store{Dir: c.Work(p.ID, "pages")}
		pages, err := store.Pages()
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			skipped++
			continue
		}
		var n int
		for _, page := range pages {
			if first > 0 && (page < first || page > last) {
				continue
			}
			before, err := store.Read(page)
			if err != nil {
				fmt.Printf("  %-34s page %d: %v\n", p.ID, page, err)
				continue
			}
			// Compared as the file would be written and not as Tidy
			// returns it, because Store.Write puts the final newline on and
			// a page whose only difference is that newline is a page this
			// would rewrite for ever.
			after := extract.Tidy(before)
			if after+"\n" == before {
				same++
				continue
			}
			n++
			if long {
				fmt.Printf("  %-34s page %d: %d characters became %d\n", p.ID, page, len(before), len(after))
			}
			if dry {
				continue
			}
			if err := store.Write(page, after); err != nil {
				return fmt.Errorf("%s page %d: %w", p.ID, page, err)
			}
		}
		changed += n
		if n > 0 || long {
			fmt.Printf("  %-34s %d pages rewritten\n", p.ID, n)
		}
	}
	if dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d pages rewritten, %d already tidy, %d papers skipped\n", changed, same, skipped)
	return nil
}

func (e *extraction) do(ctx context.Context) (count, error) {
	var n count
	if e.source == nil {
		return n, fmt.Errorf("no licence record, so nothing may be published from it: run papers resolve")
	}
	switch e.source.Access {
	case corpus.AccessUnknown, "":
		return n, fmt.Errorf("nothing is known about what may be published from it, so it is not read")
	case corpus.AccessRestricted:
		// Front matter and an abstract under 250 words is the whole of what
		// this paper can ever publish, so the rest of it is never read.
		e.last = restrictedPages
	}
	file := e.corpus.PDF(e.paper.ID)
	if _, err := os.Stat(file); err != nil {
		return n, fmt.Errorf("not fetched yet")
	}
	path, err := e.path()
	if err != nil {
		return n, err
	}
	if err := e.pageRange(ctx, file); err != nil {
		return n, err
	}

	store := extract.Store{Dir: e.corpus.Work(e.paper.ID, "pages")}
	todo := store.Missing(e.first, e.last)
	if e.again {
		todo = nil
		for page := e.first; page <= e.last; page++ {
			todo = append(todo, page)
		}
	}
	n.already = e.last - e.first + 1 - len(todo)
	if len(todo) == 0 {
		return n, nil
	}

	// The vision path reads a page at a time and writes each one as it comes
	// back, because a page takes a minute and a half and a run over a long
	// paper is a run somebody will interrupt. The other two paths read the
	// whole file in one go and have nothing to gain from it.
	if path == classify.PathVision {
		got, err := e.vision(ctx, file, store, todo)
		n.written, n.refused = got.written, got.refused
		return n, err
	}

	got, err := e.read(ctx, path, file)
	if err != nil {
		return n, err
	}
	read, checker := got.pages, got.checker

	want := map[int]bool{}
	for _, page := range todo {
		want[page] = true
	}
	seed(store, want, checker, nil)
	for _, page := range read {
		if !want[page.number] {
			continue
		}
		if faults := checker.Check(page.number, page.text); len(faults) > 0 {
			n.refused++
			for _, f := range faults {
				fmt.Printf("    %s page %d: %s\n", e.paper.ID, page.number, f)
			}
			continue
		}
		if e.long {
			fmt.Printf("    %s page %d: %s\n", e.paper.ID, page.number, page.how)
		}
		if e.dry {
			n.written++
			continue
		}
		if err := store.Write(page.number, page.text); err != nil {
			return n, err
		}
		n.written++
	}
	if n.written > 0 {
		// Written last, so that a run killed halfway leaves a record of
		// pages that are really there rather than of pages it meant to do.
		record := extract.Record{
			Path:  string(path),
			Tool:  got.tool,
			First: e.first,
			Last:  e.last,
			When:  time.Now().UTC().Truncate(time.Second),
		}
		if err := record.Write(e.corpus.Work(e.paper.ID)); err != nil {
			return n, err
		}
	}
	return n, nil
}

// path is the extraction path this paper takes.
func (e *extraction) path() (classify.Path, error) {
	// A person asking for a path for a paper is what the flag is for, and it
	// is the honest way to extract a paper the measurement sent to the layout
	// path only because it has figures to place: the text of such a paper
	// extracts perfectly well, and the figures are a separate stage with its
	// own command.
	if e.forced != "" {
		return e.forced, nil
	}
	layer := classify.Layer(e.source.TextLayer)
	switch path := layer.Path(); path {
	case "":
		return "", fmt.Errorf("not classified yet: run papers classify")
	default:
		return path, nil
	}
}

// layer is what rule A9 compares a model's reading against, and nil for a
// paper that has nothing worth comparing it to.
//
// Only a born digital file. A scan's OCR layer is itself a reading and a bad
// one, so comparing a good reading against it reports the good reading as
// wrong, and those are precisely the papers the vision path exists for. A
// file with no layer at all has nothing to say either way.
//
// pdftotext is asked per page and the answer is kept, because the rule runs
// again on every retry of the same page and a paper is read once per run.
func (e *extraction) layer(ctx context.Context, file string) func(int) (string, bool) {
	switch classify.Layer(e.source.TextLayer) {
	case classify.Native, classify.Digital:
	default:
		return nil
	}
	seen := map[int]string{}
	return func(page int) (string, bool) {
		if text, ok := seen[page]; ok {
			return text, text != ""
		}
		text, err := poppler.Page(ctx, file, page, page)
		if err != nil {
			// A layer that will not come out is a rule that does not run,
			// not a page that is refused. Everything else about the page has
			// already been checked by the time this is asked.
			text = ""
		}
		seen[page] = text
		return text, text != ""
	}
}

// pageRange fills in the ends of the range that were not given.
func (e *extraction) pageRange(ctx context.Context, file string) error {
	if e.last == 0 {
		e.last = e.source.Pages
	}
	if e.last == 0 {
		doc, err := poppler.Info(ctx, file)
		if err != nil {
			return err
		}
		e.last = doc.Pages
	}
	if e.first == 0 {
		e.first = 1
	}
	if e.last < e.first {
		return fmt.Errorf("page %d comes before page %d", e.last, e.first)
	}
	return nil
}

// A rendered page is one page of either path, ready to be checked and
// written. The two paths agree on this much and nothing downstream can tell
// which one produced a page, which is the point of having two.
type rendered struct {
	number int
	text   string
	// how is what to print under --v: the shape of the page as the path that
	// read it saw it.
	how string
}

// A reading is one pass over a file: the pages, the checker that has this
// paper's shape in it, and the name of the program that did the reading, for
// the record the run leaves behind.
type reading struct {
	pages   []rendered
	checker *extract.Checker
	tool    string
}

func (e *extraction) read(ctx context.Context, path classify.Path, file string) (*reading, error) {
	switch path {
	case classify.PathLayout:
		return e.readLayout(ctx, file)
	case classify.PathVision:
		// Handled a page at a time by e.vision, which do calls instead of
		// this. A path that reached here would be a bug rather than a
		// misconfiguration, so it says so.
		return nil, fmt.Errorf("the vision path is not read in one pass")
	}
	// The whole range is read in one pdftotext run whatever pages are
	// missing, because the running heads are learned from the paper and a
	// single page read on its own has no way to know what is furniture.
	native, err := extract.ReadNative(ctx, file, e.first, e.last)
	if err != nil {
		return nil, err
	}
	read := native.All()
	out := make([]rendered, 0, len(read))
	for _, p := range read {
		out = append(out, rendered{
			number: p.Number,
			text:   p.Text(),
			how:    fmt.Sprintf("%d columns, %d paragraphs", p.Columns, len(p.Paragraphs)),
		})
	}
	return &reading{
		pages:   out,
		checker: &extract.Checker{Math: e.math, Map: extract.LearnMap(read)},
		tool:    version(poppler.Version(ctx, "pdftotext")),
	}, nil
}

func (e *extraction) readLayout(ctx context.Context, file string) (*reading, error) {
	dir := e.corpus.Work(e.paper.ID, "layout")
	if e.dry && !layout.Done(dir) {
		return nil, fmt.Errorf("a layout run takes minutes, so a dry run will not start one")
	}
	doc, err := layout.Run(ctx, e.tool, file, dir)
	if err != nil {
		return nil, err
	}
	for _, note := range doc.Notes {
		fmt.Printf("    %s: %s\n", e.paper.ID, note)
	}
	var out []rendered
	for _, p := range doc.Pages {
		if p.Number < e.first || p.Number > e.last {
			continue
		}
		out = append(out, rendered{
			number: p.Number,
			text:   p.Text(),
			how:    fmt.Sprintf("%s, %d blocks", doc.Tool, len(p.Blocks)),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s read the file and found none of pages %d to %d in it", doc.Tool, e.first, e.last)
	}
	// A6 is off because the folio is furniture and the layout model dropped
	// it before this saw the page. A5 is on because a layout model is a
	// model and a model can stop halfway down a page, which is the thing
	// that rule is for.
	return &reading{
		pages:   out,
		checker: &extract.Checker{Math: e.math, Model: true},
		tool:    version(layout.Version(ctx, doc.Tool)),
	}, nil
}

// vision reads the pages one at a time by showing each one to a model.
//
// It is a separate loop from read because everything about it is per page.
// The page is rendered, sent, checked and written before the next one is
// started, so an interrupted run keeps every page it paid for. The page map
// is learned from the pages as they arrive rather than from the whole paper,
// for the same reason.
func (e *extraction) vision(ctx context.Context, file string, store extract.Store, todo []int) (count, error) {
	var n count
	if e.dry {
		// There is nothing to dry run. The cost of this path is the asking,
		// and a run that asked would not be a dry one.
		fmt.Printf("    %s: %d pages would be read by a model\n", e.paper.ID, len(todo))
		return n, nil
	}
	asker, err := e.fleet()
	if err != nil {
		return n, err
	}
	// The prompt is the shared one plus this paper's note, where it has one.
	// Its hash goes in the record and from there into the front matter of
	// every file these pages become.
	pinned, err := prompt.Page(e.paper.ID)
	if err != nil {
		return n, err
	}
	// Which pages have a colour image on them. A page of black text on white
	// paper goes up as grey and costs a third of the bytes, and a page with a
	// colour figure on it goes up in colour because the colours are the
	// figure. A file pdfimages will not read is not a reason to stop: every
	// page goes as grey, which is what the ladder starts at anyway.
	colour, err := render.Colour(ctx, file, e.first, e.last)
	if err != nil {
		fmt.Printf("    %s: the images could not be listed, so every page goes up in grey: %v\n", e.paper.ID, err)
	}

	// A5 is on because these pages came from a model and a model can stop
	// halfway down a page. A6 is on once the paper has taught it where its
	// numbering starts, which is what Folios is for. A9 is on for a paper
	// whose own text layer is worth comparing an answer against.
	checker := &extract.Checker{Math: e.math, Model: true, Layer: e.layer(ctx, file)}
	folios := &extract.Folios{}
	want := map[int]bool{}
	for _, page := range todo {
		want[page] = true
	}
	seed(store, want, checker, folios)

	v := &extract.Vision{
		Ask: func(ctx context.Context, target string, req llm.Request) (extract.Reply, error) {
			answer, err := asker.Do(ctx, target, req)
			return extract.Reply{Response: answer.Response, Model: answer.Model}, err
		},
		Prompt: pinned,
		Store:  render.Store{Dir: e.corpus.Images(e.paper.ID)},
		PDF:    file,
		Paper:  e.paper.ID,
		Colour: colour,
		Check: func(page int, text string) []extract.Fault {
			// Taken fresh on every attempt, because the page before this one
			// may have been what taught the paper its offset.
			checker.Map = folios.Map()
			return checker.Check(page, text)
		},
		Logf: func(format string, args ...any) { fmt.Printf("    "+format+"\n", args...) },
	}

	var tool string
	for _, page := range todo {
		scan, err := v.Page(ctx, page)
		if err != nil {
			return n, fmt.Errorf("page %d: %w", page, err)
		}
		if !scan.OK() {
			n.refused++
			for _, f := range scan.Faults {
				fmt.Printf("    %s page %d: %s\n", e.paper.ID, page, f)
			}
			continue
		}
		if err := store.Write(page, scan.Text); err != nil {
			return n, err
		}
		folios.Add(page, scan.Text)
		tool = scan.Model
		n.written++
		if e.long {
			fmt.Printf("    %s page %d: %s\n", e.paper.ID, page, scan.How())
		}
		// Written after every page rather than at the end, so that a run
		// stopped with control C leaves a record of the pages that are really
		// on disk. Write widens the range rather than replacing it.
		record := extract.Record{
			Path:   string(classify.PathVision),
			Tool:   tool,
			Prompt: pinned.SHA,
			First:  e.first,
			Last:   e.last,
			When:   time.Now().UTC().Truncate(time.Second),
		}
		if err := record.Write(e.corpus.Work(e.paper.ID)); err != nil {
			return n, err
		}
	}
	return n, nil
}

// version is the line a tool prints about itself, or nothing. A run that
// cannot get a version out of a program still extracted the pages, and a
// record that says which program read them is worth more than no record.
func version(s string, err error) string {
	if err != nil {
		return ""
	}
	return s
}

// known says whether a name is one of the layout tools, installed or not. A
// typo should be an error here rather than a tool that is silently not run.
func known(t layout.Tool) bool {
	for _, have := range layout.Tools {
		if have == t {
			return true
		}
	}
	return false
}

// choosePapers picks the papers a run is about. It is deliberately not
// resolve's selectPapers: that one skips a paper that already has a record,
// which is every paper this command can work on.
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

// pageRange reads the --pages flag, which is a page or a range of them.
func pageRange(s string) (first, last int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	from, to, found := strings.Cut(s, "-")
	first, err = strconv.Atoi(strings.TrimSpace(from))
	if err != nil {
		return 0, 0, fmt.Errorf("%q is not a page or a range of pages", s)
	}
	if !found {
		return first, first, nil
	}
	last, err = strconv.Atoi(strings.TrimSpace(to))
	if err != nil {
		return 0, 0, fmt.Errorf("%q is not a page or a range of pages", s)
	}
	return first, last, nil
}
