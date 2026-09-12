// Package figures crops the diagrams out of the pages.
//
// A figure is a cropped diagram. It is never a whole page, it is capped in
// bytes, and it is capped as a fraction of the page it came from. Those caps
// are what separate this corpus from a mirror of copyrighted PDFs, and they
// are checked here before a file is written as well as by audit rule F06
// after it, because a rule that only runs in CI is a rule that runs after the
// commit that broke it.
//
// Nothing here decides what may be published. That is the licence record in
// sources.yaml, and the caller checks it before asking for a crop at all: a
// restricted paper commits no figures whatever their size.
package figures

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/tamnd/papers-reader/poppler"
)

// Method says how the crop was made, and the three are not equal in quality.
//
// Vector re-renders the region out of the PDF, which is what a born digital
// paper gets and is lossless in effect. Raster is the same render at the
// resolution of the bitmap that is actually sitting there, which is what a
// paper whose figure is a photograph or a scanned plate gets. Crop is a
// region of a page that is itself one big image, which is what a scanned
// paper gets and is the worst of the three.
type Method string

const (
	Vector Method = "vector"
	Raster Method = "raster"
	Crop   Method = "crop"
)

// A Candidate is a region of a page that might be a figure. It is what a
// detector returns and it is not a figure yet: it has no caption, and a
// region with no caption is not committed.
type Candidate struct {
	Page int
	// Box is in the same coordinates as everything else the toolchain reads
	// off a page: points, origin at the top left, y increasing downward.
	Box    poppler.Box
	Method Method
	// PPI is the resolution of the bitmap under the region, for a raster. It
	// is zero for a region that is drawn rather than placed.
	PPI int
}

// A Figure is one committed figure, as figures.yaml records it.
//
// The bounding box is written bottom left because that is the PDF's own
// convention and figures.yaml is the file somebody opens next to the PDF in
// a viewer. Everything inside the toolchain reads boxes top left, so the
// conversion happens once, here, on the way out.
type Figure struct {
	Paper string `yaml:"paper"`
	// ID is f01, f02 and so on, and is the file name without the extension.
	// It is written as "figure" because the manifest is a flat list over the
	// whole corpus and "the figure of this paper" is what the field means.
	ID     string `yaml:"figure"`
	Number string `yaml:"number,omitempty"`
	Page   int    `yaml:"page"`
	Box    Box    `yaml:"bbox,flow"`
	// Caption is what the paper printed under the figure, and it is the part
	// of a figure that gets translated.
	Caption string `yaml:"caption"`
	SHA256  string `yaml:"sha256"`
	Method  Method `yaml:"method"`
	// Fraction is the area of the box over the area of the page, and it is
	// the number rule F06 is about.
	Fraction float64 `yaml:"page_fraction"`
	// Scale is how far the render had to be taken down to fit the byte cap,
	// as a share of the resolution it would otherwise have had. It is absent
	// for a figure that fitted at full size, which is most of them.
	Scale  float64 `yaml:"scale,omitempty"`
	Width  int     `yaml:"width"`
	Height int     `yaml:"height"`
	Bytes  int     `yaml:"bytes"`
}

// Box is a bounding box as figures.yaml writes it: xmin, ymin, xmax, ymax in
// PDF points with the origin at the bottom left.
type Box [4]float64

// pdfBox turns a top left box into the bottom left one figures.yaml records.
func pdfBox(b poppler.Box, pageHeight float64) Box {
	return Box{
		round(b.XMin),
		round(pageHeight - b.YMax),
		round(b.XMax),
		round(pageHeight - b.YMin),
	}
}

// round is two decimal places, which is a hundredth of a point and is finer
// than any PDF places anything. A box printed to fifteen digits is a box
// nobody reads and a diff nobody can review.
func round(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

// Name is the file the figure is committed as, under figures/<id>.
func (f Figure) Name() string { return f.ID + ".png" }

// The budget. One line of the spec, and the only one of the four numbers
// that is about anything but quality is the fraction.
const (
	// MinPixels is the shortest side a committed figure may have. Below this
	// it is an inline glyph the extraction should have set as mathematics,
	// not a diagram.
	MinPixels = 100
	// MaxBytes is the cap on one figure. Half a megabyte of PNG is a large
	// diagram at 300 dots per inch.
	MaxBytes = 512 << 10
	// MaxFraction is the rule that keeps this a corpus and not a mirror.
	//
	// A cropped diagram is a figure. A page image committed because the
	// extraction was hard is a scan of a copyrighted paper in a public
	// repository, and to git those two look identical. There is no licence
	// under which this corpus republishes a page of somebody's paper as an
	// image, so the check is here at the point of writing and again in the
	// audit, and neither one is a warning.
	MaxFraction = 0.75
)

// A Budget is the caps a figure has to meet. The zero Budget is not the
// default: use Default, so that a caller who forgot to set one gets the real
// caps rather than no caps at all.
type Budget struct {
	MinPixels   int
	MaxBytes    int
	MaxFraction float64
}

// Default is the budget from the spec.
func Default() Budget {
	return Budget{MinPixels: MinPixels, MaxBytes: MaxBytes, MaxFraction: MaxFraction}
}

// Check applies the budget to a figure that is about to be written. The
// error says which cap it broke and by how much, because "too big" is not
// something anybody can act on.
func (b Budget) Check(f Figure) error {
	switch {
	case f.Fraction > b.MaxFraction:
		return fmt.Errorf("it covers %.0f%% of the page and the cap is %.0f%%, which makes it a page image and not a figure",
			f.Fraction*100, b.MaxFraction*100)
	case f.Width < b.MinPixels || f.Height < b.MinPixels:
		return fmt.Errorf("it is %dx%d pixels and the floor is %d on a side, which makes it a glyph and not a diagram",
			f.Width, f.Height, b.MinPixels)
	case f.Bytes > b.MaxBytes:
		return fmt.Errorf("it is %d KB and the cap is %d KB, even after taking the render down",
			f.Bytes>>10, b.MaxBytes>>10)
	}
	return nil
}

// Fraction is the area of a region over the area of the page it came from.
//
// Area and not either dimension on its own, because a full width band an
// inch high is a figure and a column wide band the height of the page is
// most of somebody's paper.
func Fraction(b poppler.Box, page poppler.Layout) float64 {
	area := page.Width * page.Height
	if area <= 0 {
		return 0
	}
	return b.Width() * b.Height() / area
}

// SHA256 is the hash of the bytes as they will be committed, which is what
// the manifest records and what rule F05 deduplicates on.
func SHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
