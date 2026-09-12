package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/layout"
	"github.com/tamnd/papers-reader/poppler"
)

func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "extract these papers only, comma separated")
	field := fs.String("field", "", "extract one field only")
	all := fs.Bool("all", false, "extract every paper on disk whose path is implemented")
	path := fs.String("path", "", "force an extraction path instead of the measured one")
	tool := fs.String("tool", "", "which layout tool to run: mineru, marker or docling")
	pages := fs.String("pages", "", "extract these pages only, as N or N-M")
	again := fs.Bool("again", false, "extract pages that are already done too")
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
    vision    a vision model reads a picture of the page     (milestone M3)

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

Every page is checked against the eight acceptance rules before it is
written, and a page that breaks one is reported and not written. Neither
path has anything to retry with: the same program over the same file reads
the page the same way the second time, so a refused page is a page for a
person to look at, and it stays in the report until somebody does. The
thing to try is the other path, with --path.

A page that is already extracted is left alone, so running this twice costs
one read of the file and no writes. Pass --again to do the work over.

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
	case "", classify.PathNative, classify.PathLayout:
	default:
		return fmt.Errorf("the %s path arrives in a later milestone", *path)
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

	// One renderer for the whole run. Building it reads and hashes the KaTeX
	// bundle, and acceptance rule A4 asks it about every math span of every
	// page. A machine without a usable build gets the other seven rules and
	// is told which one did not run, rather than a run that stops.
	math, err := katex.New()
	if err != nil {
		fmt.Printf("KaTeX is not usable here, so acceptance rule A4 will not run: %v\n", err)
	}

	ctx := context.Background()
	var done, refused, skipped int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		run := extraction{
			corpus: c, paper: p, source: rec, math: math,
			forced: classify.Path(*path), tool: layout.Tool(*tool),
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

	got, err := e.read(ctx, path, file)
	if err != nil {
		return n, err
	}
	read, checker := got.pages, got.checker

	// The pages are checked in order and the ones that are already done are
	// checked too, because acceptance rule A5 is about how long this paper's
	// pages usually are and a resumed run that only saw the last three pages
	// would have no idea.
	want := map[int]bool{}
	for _, page := range todo {
		want[page] = true
	}
	for _, page := range read {
		if !want[page.number] {
			if done, err := store.Read(page.number); err == nil {
				checker.Check(page.number, done)
			}
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
		return nil, fmt.Errorf("its text layer is %s, and the reader that can do anything with that arrives later in milestone M3: papers render will rasterise it in the meantime", e.source.TextLayer)
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
