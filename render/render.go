package render

import (
	"context"
	"fmt"
	"image"
	_ "image/png" // for image.DecodeConfig on a rendered page
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/poppler"
)

// A Profile is one way of rasterising a page: the resolution, and whether the
// colour is kept.
//
// It is part of the cache key rather than a detail of the run. A page that
// came back unreadable at 300 is asked again at 400, and if the second render
// overwrote the first there would be no way to tell a page that has been
// tried twice from one that has been tried once, which is exactly what the
// retry logic needs to know.
type Profile struct {
	DPI  int
	Gray bool
}

// Base is what a page is rendered at first.
//
// 300 dots per inch is where scanned text stops being a guess. Below it the
// thin strokes of a 1970 journal face close up and an e becomes an o, and
// above it the file grows as the square of the resolution for detail no
// reader needs. It is also the resolution almost every scan in this corpus
// was made at, so rendering higher is interpolating pixels that were never
// there.
const Base = 300

// Ladder is the resolutions a page is tried at, in order.
//
// A page is not asked again at the same resolution, because a reader given
// the same picture twice has no new information and answers the same way. It
// is asked again with more pixels, which is the one thing that changes what
// the reader can see. Three rungs and then the page is a page for a person to
// look at: at 600 dots per inch a letter page is 5100 by 6600 and anything
// still unreadable is unreadable in the paper.
var Ladder = []int{Base, 400, 600}

// Name is the profile as a directory name, which is how two renders of the
// same page at different resolutions sit side by side on disk.
func (p Profile) Name() string {
	if p.Gray {
		return strconv.Itoa(p.DPI) + "-gray"
	}
	return strconv.Itoa(p.DPI) + "-rgb"
}

func (p Profile) String() string { return p.Name() }

// Raster is the profile as poppler takes it.
func (p Profile) Raster() poppler.Raster { return poppler.Raster{DPI: p.DPI, Gray: p.Gray} }

// Next is the profile to try after this one, and false when the ladder has
// run out. Colour is not changed by an escalation: a page that is grey is
// grey, and rendering the same ink in three channels adds nothing to read.
func (p Profile) Next() (Profile, bool) {
	for i, dpi := range Ladder {
		if dpi == p.DPI && i+1 < len(Ladder) {
			return Profile{DPI: Ladder[i+1], Gray: p.Gray}, true
		}
	}
	// A resolution somebody chose by hand is off the ladder, so the ladder has
	// nothing to say about what comes next.
	return p, false
}

// Valid reports what is wrong with a profile, for a flag a person typed.
func (p Profile) Valid() error {
	switch {
	case p.DPI < 72:
		return fmt.Errorf("%d dots per inch is below the resolution of a screen, and a page rendered there is not text", p.DPI)
	case p.DPI > 1200:
		return fmt.Errorf("%d dots per inch would render a letter page at over a hundred megapixels", p.DPI)
	}
	return nil
}

// A Store is the page images of one paper, under images/<id>/.
//
// One directory per profile and one file per page inside it, zero padded so
// that ls and a glob agree about the order. The layout is the whole of the
// caching story: a render is expensive in CPU and free to keep, so a page is
// rendered once and read as many times as the reader needs to be asked.
type Store struct {
	Dir string

	// Draw turns one page into one PNG. Nil means poppler, which is what it
	// is everywhere except a test: a test that shells out to pdftoppm is a
	// test about poppler, and this package is not.
	Draw func(ctx context.Context, pdf string, page int, r poppler.Raster, out string) error
}

// Path is the file one page at one profile is written to.
func (s Store) Path(p Profile, page int) string {
	return filepath.Join(s.Dir, p.Name(), fmt.Sprintf("%04d.png", page))
}

// Has reports whether a page has been rendered at this profile.
func (s Store) Has(p Profile, page int) bool {
	info, err := os.Stat(s.Path(p, page))
	return err == nil && info.Size() > 0
}

// Read is the bytes of one rendered page, which is what goes up to a model.
func (s Store) Read(p Profile, page int) ([]byte, error) {
	return os.ReadFile(s.Path(p, page))
}

// Size is how many pixels a rendered page came to. It reads the PNG header
// and not the image, so it costs nothing to ask about every page of a run.
func (s Store) Size(p Profile, page int) (width, height int, err error) {
	f, err := os.Open(s.Path(p, page))
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", s.Path(p, page), err)
	}
	return cfg.Width, cfg.Height, nil
}

// Pages is every page rendered at this profile, in order.
func (s Store) Pages(p Profile) ([]int, error) {
	entries, err := os.ReadDir(filepath.Join(s.Dir, p.Name()))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".png") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".png"))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

// Missing is the pages of a range that have not been rendered yet.
func (s Store) Missing(p Profile, first, last int) []int {
	var out []int
	for page := first; page <= last; page++ {
		if !s.Has(p, page) {
			out = append(out, page)
		}
	}
	return out
}

// Best is the highest profile a page has already been rendered at, which is
// the one a reader should be given. A page that has been escalated twice is
// on disk three times and the 600 is the one that was asked for.
func (s Store) Best(page int, gray bool) (Profile, bool) {
	best, found := Profile{}, false
	for _, dpi := range Ladder {
		p := Profile{DPI: dpi, Gray: gray}
		if s.Has(p, page) {
			best, found = p, true
		}
	}
	return best, found
}

// Render rasterises one page, or does nothing if it is already there.
//
// It returns whether it did the work, so a caller can tell a run that
// rendered forty pages from one that found forty pages already rendered
// without counting files itself.
func (s Store) Render(ctx context.Context, pdf string, p Profile, page int) (bool, error) {
	if s.Has(p, page) {
		return false, nil
	}
	if err := p.Valid(); err != nil {
		return false, err
	}
	draw := s.Draw
	if draw == nil {
		draw = poppler.ToPNG
	}
	if err := draw(ctx, pdf, page, p.Raster(), s.Path(p, page)); err != nil {
		return false, fmt.Errorf("page %d at %s: %w", page, p, err)
	}
	return true, nil
}

// Colour is the pages of a range that carry a colour image, which are the
// only pages worth rendering in colour.
func Colour(ctx context.Context, pdf string, first, last int) (map[int]bool, error) {
	images, err := poppler.ImageList(ctx, pdf, first, last)
	if err != nil {
		return nil, err
	}
	return ColourPages(images), nil
}

// ColourPages is the judgement Colour makes, over a list somebody else read.
//
// It is deliberately one-sided. Anything poppler does not call grey counts as
// colour, including an indexed palette that may hold two shades of black,
// because being wrong this way costs a bigger file on one page and being
// wrong the other way loses the one figure in the paper that was in colour.
// A page with no image on it at all is text, and text is grey.
func ColourPages(images []poppler.Image) map[int]bool {
	out := map[int]bool{}
	for _, img := range images {
		switch img.Colour {
		case "gray", "-", "":
		default:
			out[img.Page] = true
		}
	}
	return out
}
