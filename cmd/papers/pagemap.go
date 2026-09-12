package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/pagemap"
	"github.com/tamnd/papers-reader/poppler"
)

func runPageMap(args []string) error {
	fs := flag.NewFlagSet("pagemap", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "map these papers only, comma separated")
	field := fs.String("field", "", "map one field only")
	all := fs.Bool("all", false, "map every paper that has been fetched")
	dry := fs.Bool("dry-run", false, "print what would be written and write nothing")
	long := fs.Bool("v", false, "print every paper, including the ones already done")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers pagemap [flags]

Writes manifests/pages/<id>.yaml: which page of the file is which page of
the paper, how big each page is, and which figures were cut from it.

A paper pulled out of a journal starts at page 483 and a preprint starts at
1, and neither of them starts where the PDF does, because the PDF of a
journal article often carries a cover sheet the library added. Everything
the toolchain records about where something came from is in pages of the
file, because that is the only number it can see, and every cross reference
a paper makes to itself is in pages of the paper. This is the table between
the two, and the offset at the top of it is learned from the folios the
pages printed, by the median, which is the same rule acceptance rule A6
applies during extraction and the same code.

The page size is here because a bounding box in figures.yaml is four numbers
in points and four numbers in points mean nothing without the page they were
measured on. The figure list is the same fact from the other end, which page
a crop came off, and it is the one somebody asks when a figure looks wrong
and they want to put it next to the page.

This reads the PDF, so it runs where the PDFs are. A paper whose text layer
is an image prints no folios into the geometry, so its folios are read back
out of the pages extraction already wrote, and a paper that has been neither
extracted nor given a text layer gets a map with sizes and no numbers, which
is honest and is still worth having.

A restricted paper is mapped over the three pages it was read on, wherever
in the file those are.

A run that would write the file it already wrote writes nothing, so this is
cheap to put in a pipeline.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to map: --id, --field or --all")
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
	cropped, err := figures.Load(c.FiguresManifest())
	if err != nil {
		return err
	}
	todo, err := choosePapers(manifest, *ids, *field)
	if err != nil {
		return err
	}

	ctx := context.Background()
	var written, already, skipped int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		m, err := mapPaper(ctx, c, p.ID, rec, cropped)
		if err != nil {
			if *long {
				fmt.Printf("  %-34s %v\n", p.ID, err)
			}
			skipped++
			continue
		}
		path := c.PageMap(p.ID)
		body, err := m.Bytes()
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, body) {
			already++
			if *long {
				fmt.Printf("  %-34s %s, already done\n", p.ID, m)
			}
			continue
		}
		fmt.Printf("  %-34s %s\n", p.ID, m)
		written++
		if *dry {
			continue
		}
		if err := m.Save(path); err != nil {
			return err
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d papers mapped, %d already done, %d skipped\n", written, already, skipped)
	return nil
}

// mapPaper reads one paper's pages.
func mapPaper(ctx context.Context, c *corpus.Corpus, id string, rec *corpus.Source, cropped *figures.Manifest) (*pagemap.Map, error) {
	if rec == nil {
		return nil, fmt.Errorf("no licence record, so it is not read: run papers resolve")
	}
	if rec.Access == corpus.AccessUnknown || rec.Access == "" {
		return nil, fmt.Errorf("nothing is known about what may be published from it, so it is not read")
	}
	file := c.PDF(id)
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("not fetched yet")
	}
	last := rec.Pages
	if last == 0 {
		doc, err := poppler.Info(ctx, file)
		if err != nil {
			return nil, err
		}
		last = doc.Pages
	}
	first := 1
	if rec.Access == corpus.AccessRestricted {
		first, last = restrictedRange(c, id, last)
	}

	// One pdftotext run for the whole paper, because the folio is furniture
	// and furniture is learned from the paper rather than from a page. A
	// scanned paper comes back from this with the page sizes and no words on
	// any page, which is not an error here: the sizes are the half of this
	// file that does not depend on there being a text layer.
	native, err := extract.ReadNative(ctx, file, first, last)
	if err != nil {
		return nil, err
	}
	read := map[int]extract.Page{}
	for _, p := range native.All() {
		read[p.Number] = p
	}
	store := extract.Store{Dir: c.Work(id, "pages")}
	byPage := figuresByPage(cropped, id)

	pages := make([]pagemap.Page, 0, len(native.Layouts))
	for _, l := range native.Layouts {
		page := pagemap.Page{
			PDF:     l.Number,
			Size:    pagemap.Size{round(l.Width), round(l.Height)},
			Figures: byPage[l.Number],
		}
		if p, ok := read[l.Number]; ok {
			page.Printed = p.Printed
			page.Columns = p.Columns
		}
		if page.Printed == "" {
			// A page a model read. The folio is in the text the model wrote,
			// because a model transcribes what it sees and the running heads
			// were never separated out of it.
			if text, err := store.Read(l.Number); err == nil {
				if n, ok := extract.Folio(text); ok {
					page.Printed = strconv.Itoa(n)
				}
			}
		}
		pages = append(pages, page)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].PDF < pages[j].PDF })
	return pagemap.Build(id, pages), nil
}

// restrictedRange is the pages of a restricted paper that are mapped, which
// is the window that was read and nothing else.
//
// It used to be pages 1 to 3, and that is wrong for the reason it was wrong
// in papers extract: three pages from the front of a file is not three pages
// of a paper. The UNC tech report of No Silver Bullet opens with a cover
// sheet, a page whose only line is an equal opportunity notice, and a second
// title page, so it is read from page 4, and its map described three pages
// nothing was published from.
//
// The extraction record is what says where the window is, because where it
// starts is a decision the extraction made and nothing else knows it. A
// paper with no record has not been read at all, and the front of the file
// is then the only guess available.
func restrictedRange(c *corpus.Corpus, id string, pages int) (first, last int) {
	first, last = 1, restrictedWindow(1)
	if r, err := extract.ReadRecord(c.Work(id)); err == nil && r != nil && r.First > 0 {
		first, last = r.First, r.Last
	}
	if pages > 0 && last > pages {
		last = pages
	}
	return first, last
}

// figuresByPage is this paper's committed figures, by the page they were cut
// from and in the order the manifest lists them.
func figuresByPage(m *figures.Manifest, id string) map[int][]string {
	out := map[int][]string{}
	for _, f := range m.Figures {
		if f.Paper == id {
			out[f.Page] = append(out[f.Page], f.ID)
		}
	}
	return out
}

// round is two decimal places, which is finer than any PDF places anything
// and is what figures.yaml writes its boxes to. A page size printed to
// fifteen digits is a diff nobody can review.
func round(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }
