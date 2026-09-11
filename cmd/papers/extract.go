package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/poppler"
)

func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "extract these papers only, comma separated")
	field := fs.String("field", "", "extract one field only")
	all := fs.Bool("all", false, "extract every paper on disk whose path is implemented")
	path := fs.String("path", "", "force an extraction path instead of the measured one")
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
manifests/sources.yaml. Only the native path is implemented so far.

    native    pdftotext reads the file and no model is in the path
    layout    a layout model reads the geometry              (milestone M2)
    vision    a vision model reads a picture of the page     (milestone M3)

The native path asks pdftotext for the word boxes rather than for text,
because pdftotext -layout interleaves the columns of a two column paper
into nonsense. The columns, the reading order, the running heads and the
paragraphs are worked out here from the geometry.

Every page is checked against the eight acceptance rules before it is
written, and a page that breaks one is reported and not written. On this
path there is nothing to retry with: pdftotext is not going to read the
page differently the second time, so a refused page is a page for a person
to look at, and it stays in the report until somebody does.

A page that is already extracted is left alone, so running this twice costs
one pdftotext run and no writes. Pass --again to do the work over.

A restricted paper is read as far as its first page and no further. Nothing
but its front matter and a short abstract can ever be published, so the
rest of it would be work done to fill a directory nobody may read.

This needs poppler. Run papers doctor to see whether it is installed.

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
	if *path != "" && classify.Path(*path) != classify.PathNative {
		return fmt.Errorf("the %s path arrives in a later milestone", *path)
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
		n, err := extractOne(ctx, c, p, rec, math, classify.Path(*path), first, last, *again, *dry, *long)
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

func extractOne(ctx context.Context, c *corpus.Corpus, p corpus.Paper, rec *corpus.Source, math *katex.Renderer, forced classify.Path, first, last int, again, dry, long bool) (count, error) {
	var n count
	if rec == nil {
		return n, fmt.Errorf("no licence record, so nothing may be published from it: run papers resolve")
	}
	switch rec.Access {
	case corpus.AccessUnknown, "":
		return n, fmt.Errorf("nothing is known about what may be published from it, so it is not read")
	case corpus.AccessRestricted:
		// Front matter and an abstract under 250 words is the whole of what
		// this paper can ever publish, and both are on the first page.
		last = 1
	}
	file := c.PDF(p.ID)
	if _, err := os.Stat(file); err != nil {
		return n, fmt.Errorf("not fetched yet")
	}
	switch layer := classify.Layer(rec.TextLayer); {
	case forced == classify.PathNative:
		// A person asked for this path for this paper. That is what the flag
		// is for and it is the honest way to extract a paper the measurement
		// sent to the layout path only because it has figures to place: the
		// text of such a paper extracts perfectly well, and the figures are a
		// separate stage with its own command.
	case layer == classify.Native:
	case layer == "":
		return n, fmt.Errorf("not classified yet: run papers classify")
	default:
		return n, fmt.Errorf("its text layer is %s, and only the native path is implemented", layer)
	}

	if last == 0 {
		last = rec.Pages
	}
	if last == 0 {
		doc, err := poppler.Info(ctx, file)
		if err != nil {
			return n, err
		}
		last = doc.Pages
	}
	if first == 0 {
		first = 1
	}
	if last < first {
		return n, fmt.Errorf("page %d comes before page %d", last, first)
	}

	store := extract.Store{Dir: c.Work(p.ID, "pages")}
	todo := store.Missing(first, last)
	if again {
		todo = nil
		for page := first; page <= last; page++ {
			todo = append(todo, page)
		}
	}
	n.already = last - first + 1 - len(todo)
	if len(todo) == 0 {
		return n, nil
	}

	// The whole range is read in one pdftotext run whatever pages are
	// missing, because the running heads are learned from the paper and a
	// single page read on its own has no way to know what is furniture.
	native, err := extract.ReadNative(ctx, file, first, last)
	if err != nil {
		return n, err
	}
	read := native.All()

	checker := &extract.Checker{Math: math, Map: extract.LearnMap(read)}
	// The pages are checked in order and the ones that are already done are
	// checked too, because acceptance rule A5 is about how long this paper's
	// pages usually are and a resumed run that only saw the last three pages
	// would have no idea.
	want := map[int]bool{}
	for _, page := range todo {
		want[page] = true
	}
	for _, page := range read {
		text := page.Text()
		if !want[page.Number] {
			if done, err := store.Read(page.Number); err == nil {
				checker.Check(page.Number, done)
			}
			continue
		}
		if faults := checker.Check(page.Number, text); len(faults) > 0 {
			n.refused++
			for _, f := range faults {
				fmt.Printf("    %s page %d: %s\n", p.ID, page.Number, f)
			}
			continue
		}
		if long {
			fmt.Printf("    %s page %d: %d columns, %d paragraphs\n", p.ID, page.Number, page.Columns, len(page.Paragraphs))
		}
		if dry {
			n.written++
			continue
		}
		if err := store.Write(page.Number, text); err != nil {
			return n, err
		}
		n.written++
	}
	return n, nil
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
