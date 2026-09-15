package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/poppler"
	"github.com/tamnd/papers-reader/render"
)

func runRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "render these papers only, comma separated")
	field := fs.String("field", "", "render one field only")
	all := fs.Bool("all", false, "render every fetched paper that needs a model to read it")
	dpi := fs.Int("dpi", render.Base, "resolution to render at")
	colour := fs.String("colour", "auto", "gray, rgb, or auto to ask the file which pages have colour on them")
	pages := fs.String("pages", "", "render these pages only, as N or N-M")
	again := fs.Bool("again", false, "render pages that are already rendered too")
	dry := fs.Bool("dry-run", false, "print what would be rendered and write nothing")
	long := fs.Bool("v", false, "print every page")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers render [flags]

Rasterises the pages of a fetched PDF to images/<id>/<profile>/NNNN.png,
which is what the vision path sends to a reader.

This is the one stage that asks nothing of anybody. It is pdftoppm and
arithmetic, it needs no network and no model, and the answer for a page at
a resolution is the same every time. So it can run days ahead of the stage
that consumes it, and it should: the readers are slow and rationed, and
standing in a queue for one with the rasteriser still to run is the scarce
thing waiting on the plentiful one.

A profile is the resolution and whether the colour was kept, and it is part
of the path rather than lost: a page that came back unreadable at 300 is
asked again at 400, and both renders stay on disk so a later run can tell a
page that has been tried twice from one that has been tried once.

With --colour auto, which is the default, pdfimages is asked which pages
carry a colour image and only those are rendered in colour. A colour render
of black ink on white paper is three times the upload for the same words.

With --all or --field only the papers that need a model are rendered, since
a paper pdftotext can read has no use for a picture of itself. Naming a
paper with --id renders it whatever its path, which is how to look at one.

Nothing here is committed. images/ is gitignored, and a page raster is a
picture of a copyrighted paper whatever the rules about crops say.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to render: --id, --field or --all")
	}
	first, last, err := pageRange(*pages)
	if err != nil {
		return err
	}
	gray, auto, err := colourMode(*colour)
	if err != nil {
		return err
	}
	if err := (render.Profile{DPI: *dpi}).Valid(); err != nil {
		return err
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

	ctx := context.Background()
	var drawn, already, skipped int
	for _, p := range todo {
		run := rasterise{
			corpus: c, paper: p, source: sourceOf(recorded, p.ID),
			dpi: *dpi, gray: gray, auto: auto,
			first: first, last: last,
			named: *ids != "",
			again: *again, dry: *dry, long: *long,
		}
		n, err := run.do(ctx)
		switch {
		case err != nil:
			fmt.Printf("  %-34s %v\n", p.ID, err)
			skipped++
		default:
			drawn += n.drawn
			already += n.already
			if n.drawn > 0 || *long {
				fmt.Printf("  %-34s %s\n", p.ID, n)
			}
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d pages rendered, %d already there, %d papers skipped\n", drawn, already, skipped)
	return nil
}

// sourceOf is the licence record of one paper, or nil. The nil is the
// interesting case and every caller has to handle it: a paper with no record
// is a paper nobody has looked up, and nothing is read from it.
func sourceOf(recorded *corpus.Sources, id string) *corpus.Source {
	rec, _ := recorded.ByID(id)
	return rec
}

// colourMode reads the --colour flag. auto means ask the file, and then gray
// is what a page with no colour image on it gets.
func colourMode(s string) (gray, auto bool, err error) {
	switch s {
	case "auto":
		return true, true, nil
	case "gray", "grey":
		return true, false, nil
	case "rgb", "colour", "color":
		return false, false, nil
	}
	return false, false, fmt.Errorf("--colour takes gray, rgb or auto, not %q", s)
}

// A rasterise is one paper's run.
type rasterise struct {
	corpus *corpus.Corpus
	paper  corpus.Paper
	source *corpus.Source

	dpi  int
	gray bool
	// auto asks the file which pages have colour on them rather than
	// rendering the whole paper one way.
	auto bool

	first, last int
	// named says a person asked for this paper by id, which is what lets a
	// paper off the rule that only the vision path needs pictures.
	named bool
	again bool
	dry   bool
	long  bool
}

// drew is what one paper's run came to.
type drew struct {
	drawn, already int
	// profiles is the profiles the run touched, for the line it prints. A
	// paper with one colour plate in it touches two.
	profiles []string
}

func (n drew) String() string {
	parts := []string{fmt.Sprintf("%d pages", n.drawn)}
	if n.already > 0 {
		parts = append(parts, fmt.Sprintf("%d already there", n.already))
	}
	if len(n.profiles) > 0 {
		parts = append(parts, strings.Join(n.profiles, " and "))
	}
	return strings.Join(parts, ", ")
}

func (r *rasterise) do(ctx context.Context) (drew, error) {
	var n drew
	if r.source == nil {
		return n, fmt.Errorf("no licence record, so nothing may be published from it: run papers resolve")
	}
	if !r.corpus.PublishesWhole() {
		switch r.source.Access {
		case corpus.AccessUnknown, "":
			return n, fmt.Errorf("nothing is known about what may be published from it, so it is not read")
		case corpus.AccessRestricted:
			// The same cap the extractor keeps. Rendering a page that may
			// never be read is CPU and disk spent on a directory nobody may
			// open.
			if r.last == 0 || r.last > restrictedPages {
				r.last = restrictedPages
			}
		}
	}
	if !r.named && classify.Layer(r.source.TextLayer).Path() != classify.PathVision {
		return n, nil
	}
	file := r.corpus.PDF(r.paper.ID)
	if _, err := os.Stat(file); err != nil {
		return n, fmt.Errorf("not fetched yet")
	}
	if err := r.pageRange(ctx, file); err != nil {
		return n, err
	}

	// Which pages are in colour is one call for the whole range rather than
	// one per page, and it is skipped entirely when the mode was given, so a
	// forced run needs nothing from pdfimages.
	colour := map[int]bool{}
	if r.auto {
		found, err := render.Colour(ctx, file, r.first, r.last)
		if err != nil {
			return n, err
		}
		colour = found
	}

	store := render.Store{Dir: r.corpus.Images(r.paper.ID)}
	seen := map[string]bool{}
	for page := r.first; page <= r.last; page++ {
		profile := render.Profile{DPI: r.dpi, Gray: r.gray && !colour[page]}
		if !seen[profile.Name()] {
			seen[profile.Name()] = true
			n.profiles = append(n.profiles, profile.Name())
		}
		if store.Has(profile, page) && !r.again {
			n.already++
			continue
		}
		if r.dry {
			n.drawn++
			if r.long {
				fmt.Printf("    %s page %d at %s\n", r.paper.ID, page, profile)
			}
			continue
		}
		if r.again {
			// A page being rendered over is a page somebody thinks came out
			// wrong, so the old one goes rather than being left for Best to
			// find.
			_ = os.Remove(store.Path(profile, page))
		}
		made, err := store.Render(ctx, file, profile, page)
		if err != nil {
			return n, err
		}
		if !made {
			n.already++
			continue
		}
		n.drawn++
		if r.long {
			w, h, err := store.Size(profile, page)
			if err != nil {
				return n, err
			}
			fmt.Printf("    %s page %d at %s: %dx%d\n", r.paper.ID, page, profile, w, h)
		}
	}
	return n, nil
}

// pageRange fills in the ends of the range that were not given.
func (r *rasterise) pageRange(ctx context.Context, file string) error {
	if r.last == 0 {
		r.last = r.source.Pages
	}
	if r.last == 0 {
		doc, err := poppler.Info(ctx, file)
		if err != nil {
			return err
		}
		r.last = doc.Pages
	}
	if r.first == 0 {
		r.first = 1
	}
	if r.last < r.first {
		return fmt.Errorf("page %d comes before page %d", r.last, r.first)
	}
	return nil
}
