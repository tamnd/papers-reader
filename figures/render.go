package figures

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os/exec"
	"strconv"

	"github.com/tamnd/papers-reader/poppler"
)

// DPI is what a region is rendered at. Three hundred dots per inch is what
// a journal asks an author for and is the point past which a diagram stops
// getting better on a screen.
const DPI = 300

// MaxDPI is the ceiling on rendering a region at the resolution of the
// bitmap underneath it. A paper with a 1200 dpi scanned plate in it would
// otherwise produce a figure nobody can open, and the byte cap would take
// it straight back down again.
const MaxDPI = 600

// Render rasterises one region of one page to PNG.
//
// Rendered with pdftoppm rather than pulled out with pdfimages, even where
// the region is an embedded bitmap and pdfimages would give it at its own
// resolution. Two reasons, and the second is the one that decides it.
//
// A figure is usually not one image. The Transformer paper's figure 1 is a
// bitmap with a soft mask over it, and pdfimages writes those as two files,
// the picture and the mask, and composites neither. A figure elsewhere in
// the hundred is a photograph with vector arrows drawn over it, and
// pdfimages does not see the arrows at all. Rendering the region is the
// only thing that produces what the page actually shows.
//
// And the region is what the budget is about. A crop is a crop of the page,
// so the pixels and the bounding box are the same measurement, and rule F06
// is checking the thing that was committed rather than something adjacent
// to it.
func Render(ctx context.Context, pdf string, c Candidate, page poppler.Layout, dpi int) ([]byte, error) {
	if dpi <= 0 {
		dpi = DPI
	}
	x, y, w, h := pixels(c.Box, page, dpi)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("the region on page %d has no area at %d dpi", c.Page, dpi)
	}
	out, err := exec.CommandContext(ctx, "pdftoppm",
		"-png", "-r", strconv.Itoa(dpi),
		"-f", strconv.Itoa(c.Page), "-l", strconv.Itoa(c.Page), "-singlefile",
		"-x", strconv.Itoa(x), "-y", strconv.Itoa(y),
		"-W", strconv.Itoa(w), "-H", strconv.Itoa(h),
		pdf).Output()
	if err != nil {
		return nil, fmt.Errorf("pdftoppm could not render page %d of %s: %w", c.Page, pdf, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pdftoppm rendered page %d of %s as nothing", c.Page, pdf)
	}
	return out, nil
}

// pixels turns a region in points into the pixel window pdftoppm crops
// with, which is measured from the top left of the page at the rendering
// resolution.
func pixels(b poppler.Box, page poppler.Layout, dpi int) (x, y, w, h int) {
	scale := float64(dpi) / 72
	x = int(math.Floor(b.XMin * scale))
	y = int(math.Floor(b.YMin * scale))
	w = int(math.Ceil(b.XMax*scale)) - x
	h = int(math.Ceil(b.YMax*scale)) - y
	// Clamped to the page, because a region that came out of the geometry
	// can be a hair outside it and pdftoppm answers a window that runs off
	// the page with a picture of nothing.
	if wide := int(page.Width*scale) - x; w > wide {
		w = wide
	}
	if tall := int(page.Height*scale) - y; h > tall {
		h = tall
	}
	return x, y, w, h
}

// A Rendered is one region rasterised, trimmed and inside the byte cap.
type Rendered struct {
	Data []byte
	// Box is the region the bytes actually show, in points with the origin
	// at the top left. It is the candidate's box with the white margin
	// taken off, and it is what goes in figures.yaml: the box a reader
	// checks against the PDF should be the box of the picture and not the
	// box of the hole the picture was found in.
	Box           poppler.Box
	Width, Height int
	// Scale is the share of the full resolution this settled at, and is one
	// for a figure that fitted the cap first time.
	Scale float64
	// Method is how the crop was made, decided from what is under the
	// trimmed box rather than under the hole it was found in.
	Method Method
}

// Fit renders a region, trims it and brings it inside the byte cap.
//
// The cap is met by rendering again at a lower resolution rather than by
// re-encoding what was rendered. A PNG of a line diagram is already
// lossless and there is nothing to squeeze out of it; the only thing that
// makes it smaller is fewer pixels, and asking pdftoppm for fewer pixels
// gives a better picture than scaling one down afterwards, because it
// antialiases from the vectors.
//
// The scale it settled on is recorded in figures.yaml, because a reader
// who finds a diagram unreadable should be able to see that it was taken
// down rather than wonder whether the paper printed it that way.
func Fit(ctx context.Context, pdf string, c Candidate, page poppler.Layout, images []poppler.Image, b Budget) (*Rendered, error) {
	// The first render is at 300 and its only job is to find out what is
	// actually in the region. A hole in a column is always larger than the
	// figure in it, so matching an embedded bitmap against the hole is
	// matching against the wrong rectangle: the Transformer paper's figure
	// 1 is a bitmap 219 points wide sitting in a hole 396 points wide, and
	// the two do not look like the same thing until the white is off.
	first, err := render(ctx, pdf, c, page, DPI)
	if err != nil {
		return nil, err
	}
	c = Classify(Candidate{Page: c.Page, Box: first.Box}, images, page)
	dpi := DPI
	if c.Method == Raster && c.PPI > DPI {
		dpi = min(c.PPI, MaxDPI)
	}
	full := dpi

	out := first
	for try := 0; try < tries; try++ {
		// The first pass is reused where nothing about it would change. A
		// drawing wanted 300 and was rendered at 300, so rendering it again
		// is a second of process for the same bytes.
		if dpi != DPI || try > 0 {
			out, err = render(ctx, pdf, c, page, dpi)
			if err != nil {
				return nil, err
			}
		}
		out.Method = c.Method
		out.Scale = float64(dpi) / float64(full)
		if len(out.Data) <= b.MaxBytes {
			return out, nil
		}
		// PNG size goes roughly with the number of pixels, so the
		// resolution wants the square root of the overshoot. A tenth off is
		// taken as well, because the estimate is optimistic on a photograph
		// and a second render costs a second.
		next := int(float64(dpi) * 0.9 * math.Sqrt(float64(b.MaxBytes)/float64(len(out.Data))))
		if next >= dpi {
			next = dpi - 25
		}
		if next < minDPI {
			next = minDPI
		}
		if next == dpi {
			break
		}
		dpi = next
	}
	return out, nil
}

// render is one pass over a region: rasterise it, cut the white off and
// measure what is left.
func render(ctx context.Context, pdf string, c Candidate, page poppler.Layout, dpi int) (*Rendered, error) {
	data, err := Render(ctx, pdf, c, page, dpi)
	if err != nil {
		return nil, err
	}
	// Trimmed against the window pdftoppm actually cropped rather than
	// against the region that was asked for. The window is whole pixels, so
	// it begins a fraction of a point outside the region, and measuring the
	// trim from the region would put every box out by that fraction.
	x, y, w, h := pixels(c.Box, page, dpi)
	pt := 72 / float64(dpi)
	window := poppler.Box{
		XMin: float64(x) * pt,
		YMin: float64(y) * pt,
		XMax: float64(x+w) * pt,
		YMax: float64(y+h) * pt,
	}
	data, box, err := Trim(data, window, dpi)
	if err != nil {
		return nil, err
	}
	px, py, err := Size(data)
	if err != nil {
		return nil, err
	}
	return &Rendered{Data: data, Box: box, Width: px, Height: py, Scale: 1}, nil
}

// Trim cuts the white margin off a render and says what is left.
//
// A region is the hole a figure left in a column, and a figure rarely
// fills its hole: it is centred in it, or it is a wide diagram with a
// short caption, and what comes out of the renderer has an inch of white
// along one side. Committing that white is committing nothing, and worse,
// it makes page_fraction a measurement of the hole rather than of the
// picture, which is the number rule F06 is about.
//
// The crop is exact, so the box comes back adjusted rather than
// approximated. Cropping loses no quality: the pixels that are kept are
// the pixels that were rendered.
func Trim(data []byte, box poppler.Box, dpi int) ([]byte, poppler.Box, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, box, err
	}
	ink := inked(img)
	if ink.Empty() {
		// Nothing on it. Left as it is, and Blank throws it out.
		return data, box, nil
	}
	b := img.Bounds()
	if ink == b {
		return data, box, nil
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return data, box, nil
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, sub.SubImage(ink)); err != nil {
		return nil, box, err
	}
	pt := 72 / float64(dpi)
	return buf.Bytes(), poppler.Box{
		XMin: box.XMin + float64(ink.Min.X-b.Min.X)*pt,
		YMin: box.YMin + float64(ink.Min.Y-b.Min.Y)*pt,
		XMax: box.XMin + float64(ink.Max.X-b.Min.X)*pt,
		YMax: box.YMin + float64(ink.Max.Y-b.Min.Y)*pt,
	}, nil
}

// inked is the smallest rectangle holding everything that is not the
// background, with a hair of margin left on so that the outermost stroke
// of a diagram does not sit against the edge of the file.
func inked(img image.Image) image.Rectangle {
	b := img.Bounds()
	if b.Empty() {
		return image.Rectangle{}
	}
	back := img.At(b.Min.X, b.Min.Y)
	out := image.Rectangle{Min: b.Max, Max: b.Min}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if same(img, x, y, back) {
				continue
			}
			out.Min.X = min(out.Min.X, x)
			out.Min.Y = min(out.Min.Y, y)
			out.Max.X = max(out.Max.X, x+1)
			out.Max.Y = max(out.Max.Y, y+1)
		}
	}
	if out.Empty() {
		return image.Rectangle{}
	}
	return out.Inset(-bleed).Intersect(b)
}

// bleed is how many pixels of background are kept around the ink. At 300
// dots per inch this is a hundredth of an inch, which is not visible and
// which keeps a box drawn at the edge of a diagram from looking clipped.
const bleed = 3

// tries is how many renders one figure is worth. Each one is a process and
// a page of PDF, and the estimate converges in two.
const tries = 5

// minDPI is the floor. Below this a diagram with text in it is a diagram
// whose labels cannot be read, and a figure nobody can read is worse than a
// figure that is not there: the file is committed, the audit passes, and
// the reader is the one who finds out.
const minDPI = 72

// Size is the pixel size of an encoded PNG, read from the header rather
// than by decoding it.
func Size(data []byte) (w, h int, err error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// Blank reports whether a render came back as one flat colour, which is
// what a region that was really white space renders as.
//
// The whitespace detector finds every hole in a column, and a hole left by
// a table set with no rules, by a display equation that was too tall, or by
// the page simply ending is a hole with nothing in it. Committing a blank
// PNG is the kind of thing nobody notices until the corpus has four hundred
// of them.
func Blank(data []byte) bool {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	b := img.Bounds()
	if b.Empty() {
		return true
	}
	first := img.At(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y += step(b) {
		for x := b.Min.X; x < b.Max.X; x += step(b) {
			if !same(img, x, y, first) {
				return false
			}
		}
	}
	return true
}

// step is how coarsely a render is sampled looking for ink. Every pixel of
// a 2000 by 3000 render is six million calls to At through an interface,
// and a diagram that is invisible at this sampling is a diagram with one
// stray pixel in it.
func step(b image.Rectangle) int {
	n := max(b.Dx(), b.Dy()) / 400
	if n < 1 {
		return 1
	}
	return n
}

func same(img image.Image, x, y int, want color.Color) bool {
	r1, g1, b1, a1 := img.At(x, y).RGBA()
	r2, g2, b2, a2 := want.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}
