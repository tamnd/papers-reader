package figures

import (
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

func page() poppler.Layout {
	return poppler.Layout{Number: 1, Width: pageWide, Height: pageTall}
}

// bitmap is an embedded image of a given size in points, placed at a given
// resolution. pdfimages reports pixels and density, so the fixture is
// written the way a person thinks about it and converted here.
func bitmap(w, h float64, ppi int) poppler.Image {
	return poppler.Image{
		Page:  1,
		Width: int(w / 72 * float64(ppi)), Height: int(h / 72 * float64(ppi)),
		XPPI: ppi, YPPI: ppi,
	}
}

func region(w, h float64) Candidate {
	return Candidate{Page: 1, Box: poppler.Box{XMin: 100, YMin: 100, XMax: 100 + w, YMax: 100 + h}}
}

// A drawing has nothing under it, so there is no resolution to match and 300
// is as good as the region will ever get.
func TestARegionOverNothingIsADrawing(t *testing.T) {
	got := Classify(region(300, 200), nil, page())
	if got.Method != Vector {
		t.Fatalf("method = %v, want vector", got.Method)
	}
	if got.PPI != 0 {
		t.Fatalf("a drawing was given a resolution of %d", got.PPI)
	}
}

// A region over a bitmap of its own size is that bitmap, and rendering it at
// 300 when it was placed at 600 throws away half of what the paper has.
func TestARegionOverABitmapTakesTheBitmapsResolution(t *testing.T) {
	got := Classify(region(300, 200), []poppler.Image{bitmap(300, 200, 600)}, page())
	if got.Method != Raster {
		t.Fatalf("method = %v, want raster", got.Method)
	}
	if got.PPI != 600 {
		t.Fatalf("ppi = %d, want the bitmap's 600", got.PPI)
	}
}

// The region is measured from the white round the figure, so it carries
// whatever margin the typesetter left and is always a little larger than the
// bitmap inside it.
func TestASmallMarginRoundTheBitmapIsStillTheBitmap(t *testing.T) {
	got := Classify(region(320, 214), []poppler.Image{bitmap(300, 200, 300)}, page())
	if got.Method != Raster {
		t.Fatalf("method = %v, want raster for a bitmap 7 percent smaller than its region", got.Method)
	}
}

// A bitmap the width of the region and half its height is the top panel of a
// figure and not the figure, and rendering the whole region at that one
// panel's resolution is a guess about the rest of it.
func TestAPanelOfTheFigureIsNotTheFigure(t *testing.T) {
	got := Classify(region(300, 400), []poppler.Image{bitmap(300, 200, 600)}, page())
	if got.Method != Vector {
		t.Fatalf("method = %v, want vector when only half the region is a bitmap", got.Method)
	}
}

// A scanned page is one image the size of the page. Every region of it is a
// piece of that scan and no region of it can be better than the scan was,
// which is worth recording rather than quietly calling it a raster.
func TestARegionOfAScannedPageIsACrop(t *testing.T) {
	scan := bitmap(pageWide, pageTall, 300)
	got := Classify(region(300, 200), []poppler.Image{scan}, page())
	if got.Method != Crop {
		t.Fatalf("method = %v, want crop on a scanned page", got.Method)
	}
	if got.PPI != 300 {
		t.Fatalf("ppi = %d, want the scan's 300", got.PPI)
	}
}

// The scan wins wherever it is in the list, because a region that matches a
// small bitmap on a page that is itself a scan is still a piece of the scan.
func TestTheScanIsFoundBehindASmallerImage(t *testing.T) {
	images := []poppler.Image{bitmap(300, 200, 600), bitmap(pageWide, pageTall, 300)}
	if got := Classify(region(300, 200), images, page()); got.Method != Crop {
		t.Fatalf("method = %v, want crop", got.Method)
	}
}

// An image on another page tells us nothing about this one. pdfimages is run
// over a range, so the list a page is classified against holds its
// neighbours too.
func TestABitmapOnAnotherPageIsIgnored(t *testing.T) {
	other := bitmap(300, 200, 600)
	other.Page = 2
	if got := Classify(region(300, 200), []poppler.Image{other}, page()); got.Method != Vector {
		t.Fatalf("method = %v, want vector when the only bitmap is on another page", got.Method)
	}
}

// pdfimages reports a density of zero for an image it could not place, and
// dividing by that is how a figure ends up with a bounding box of infinity.
func TestAnUnplaceableImageIsIgnored(t *testing.T) {
	broken := poppler.Image{Page: 1, Width: 1250, Height: 833}
	if got := Classify(region(300, 200), []poppler.Image{broken}, page()); got.Method != Vector {
		t.Fatalf("method = %v, want vector when the bitmap has no resolution", got.Method)
	}
}

func TestARegionWithNoSizeMatchesNothing(t *testing.T) {
	flat := Candidate{Page: 1, Box: poppler.Box{XMin: 100, YMin: 100, XMax: 100, YMax: 300}}
	if got := Classify(flat, []poppler.Image{bitmap(300, 200, 600)}, page()); got.Method != Vector {
		t.Fatalf("method = %v, want vector for a region with no width", got.Method)
	}
}
