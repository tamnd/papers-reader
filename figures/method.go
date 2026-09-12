package figures

import (
	"math"

	"github.com/tamnd/papers-reader/poppler"
)

// Classify decides how a region should be rendered, from what pdfimages
// says is under it.
//
// The three methods are not a style choice, they are a claim about what the
// figure is, and the claim goes in figures.yaml where a person can check
// it. A region over an embedded bitmap is a photograph or a plate and wants
// that bitmap's own resolution; a region over nothing is a drawing and
// renders at 300 wherever it is looked at; and a region on a page that is
// itself one big image is a piece of a scan, which is the worst case and is
// the one worth naming.
//
// pdfimages does not say where on the page an image sits, only how big it
// is and how densely it is placed, so the match is by size. That is enough:
// two bitmaps on one page that are the same size to within a fifth are two
// halves of one figure more often than they are a mistake, and the only
// thing this decides is the resolution to render at.
func Classify(c Candidate, images []poppler.Image, page poppler.Layout) Candidate {
	var best poppler.Image
	closest := math.MaxFloat64
	for _, img := range images {
		if img.Page != c.Page || img.XPPI <= 0 || img.YPPI <= 0 {
			continue
		}
		w, h := placed(img)
		if covers(w, h, page) {
			// The page is a scan. Every region of it is a piece of one
			// image and no region of it is better than the scan was.
			c.Method = Crop
			c.PPI = img.XPPI
			return c
		}
		if d := off(w, h, c.Box); d < closest {
			best, closest = img, d
		}
	}
	if closest <= tolerance {
		c.Method = Raster
		c.PPI = max(best.XPPI, best.YPPI)
		return c
	}
	c.Method = Vector
	c.PPI = 0
	return c
}

// placed is how large an image is on the page, in points. The pixel count
// alone says nothing: 2550 pixels is a full page at 300 dots per inch and a
// postage stamp at 3000.
func placed(img poppler.Image) (w, h float64) {
	return float64(img.Width) / float64(img.XPPI) * 72, float64(img.Height) / float64(img.YPPI) * 72
}

// covers says whether an image is the page. A scanned page is one image the
// size of the page, give or take the margin the scanner left.
func covers(w, h float64, page poppler.Layout) bool {
	if page.Width <= 0 || page.Height <= 0 {
		return false
	}
	return w*h/(page.Width*page.Height) > 0.9
}

// off is how far an image's size is from a region's, as a share. Both
// dimensions, and the worse of the two: a bitmap the width of the region
// and half its height is the top half of a figure and not the figure.
func off(w, h float64, b poppler.Box) float64 {
	if b.Width() <= 0 || b.Height() <= 0 {
		return math.MaxFloat64
	}
	return max(math.Abs(w-b.Width())/b.Width(), math.Abs(h-b.Height())/b.Height())
}

// tolerance is how far out the size match may be. A fifth, because the
// region comes from the white space around the figure and carries whatever
// margin the typesetter left, so it is always a little larger than the
// bitmap in it.
const tolerance = 0.25
