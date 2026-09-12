package figures

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// canvas is a render that came back from pdftoppm: white paper, with
// whatever the test draws on it.
func canvas(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	return img
}

func ink(img *image.RGBA, r image.Rectangle) {
	draw.Draw(img, r, image.NewUniform(color.Black), image.Point{}, draw.Src)
}

func encode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return buf.Bytes()
}

// near compares points, which are floats that came out of a division
// by 300 and will not be equal to anything written down.
func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// pdftoppm crops on whole pixels, so a region has to be rounded outwards.
// Rounding in loses a pixel off two sides of every figure in the corpus.
func TestTheCropWindowIsRoundedOutwards(t *testing.T) {
	p := poppler.Layout{Width: pageWide, Height: pageTall}
	x, y, w, h := pixels(poppler.Box{XMin: 10.1, YMin: 10.1, XMax: 20.9, YMax: 30.9}, p, 72)
	if x != 10 || y != 10 {
		t.Fatalf("the window starts at %d,%d, want 10,10", x, y)
	}
	if w != 11 || h != 21 {
		t.Fatalf("the window is %dx%d, want 11x21", w, h)
	}
}

func TestTheCropWindowIsInPixelsAtTheRenderingResolution(t *testing.T) {
	p := poppler.Layout{Width: pageWide, Height: pageTall}
	x, y, w, h := pixels(poppler.Box{XMin: 72, YMin: 72, XMax: 144, YMax: 144}, p, 300)
	if x != 300 || y != 300 || w != 300 || h != 300 {
		t.Fatalf("the window is %d,%d %dx%d, want 300,300 300x300", x, y, w, h)
	}
}

// A region that came out of the geometry can be a hair outside the paper,
// and pdftoppm answers a window that runs off the page with a picture of
// nothing.
func TestTheCropWindowStopsAtTheEdgeOfThePaper(t *testing.T) {
	p := poppler.Layout{Width: pageWide, Height: pageTall}
	_, _, w, h := pixels(poppler.Box{XMin: 600, YMin: 780, XMax: 700, YMax: 900}, p, 72)
	if w != 12 {
		t.Fatalf("the window is %d points wide, want the 12 left on the page", w)
	}
	if h != 12 {
		t.Fatalf("the window is %d points tall, want the 12 left on the page", h)
	}
}

// The white round a figure is the hole it was found in and not the figure.
// Committing it makes page_fraction a measurement of the hole, which is the
// number rule F06 is about.
func TestTrimTakesTheWhiteOff(t *testing.T) {
	img := canvas(300, 300)
	ink(img, image.Rect(100, 120, 200, 220))
	window := poppler.Box{XMin: 0, YMin: 0, XMax: 72, YMax: 72}
	data, box, err := Trim(encode(t, img), window, 300)
	if err != nil {
		t.Fatalf("trimming: %v", err)
	}
	w, h, err := Size(data)
	if err != nil {
		t.Fatalf("reading the trimmed file: %v", err)
	}
	// The drawn rectangle plus three pixels of bleed on each side.
	if w != 106 || h != 106 {
		t.Fatalf("the trimmed render is %dx%d, want 106x106", w, h)
	}
	pt := 72.0 / 300
	if !near(box.XMin, 97*pt) || !near(box.YMin, 117*pt) {
		t.Fatalf("the trimmed box starts at %.2f,%.2f, want %.2f,%.2f", box.XMin, box.YMin, 97*pt, 117*pt)
	}
	if !near(box.XMax, 203*pt) || !near(box.YMax, 223*pt) {
		t.Fatalf("the trimmed box ends at %.2f,%.2f, want %.2f,%.2f", box.XMax, box.YMax, 203*pt, 223*pt)
	}
}

// The box is what goes in figures.yaml, so it has to be the box on the page
// and not the box within the crop.
func TestTheTrimmedBoxIsWhereTheFigureIsOnThePage(t *testing.T) {
	img := canvas(300, 300)
	ink(img, image.Rect(100, 100, 200, 200))
	window := poppler.Box{XMin: 144, YMin: 216, XMax: 216, YMax: 288}
	_, box, err := Trim(encode(t, img), window, 300)
	if err != nil {
		t.Fatal(err)
	}
	if !near(box.XMin, 144+97*72.0/300) {
		t.Fatalf("xmin = %.2f, want the window's own origin plus the trim", box.XMin)
	}
	if box.XMax > window.XMax || box.YMax > window.YMax {
		t.Fatalf("the trimmed box %v is outside the window %v", box, window)
	}
}

// A figure that fills its region is trimmed to itself, and the file that
// comes back should be the one that went in rather than a re-encoding of it.
func TestAFullRegionIsLeftAlone(t *testing.T) {
	img := canvas(80, 80)
	ink(img, image.Rect(0, 0, 80, 80))
	in := encode(t, img)
	window := poppler.Box{XMin: 0, YMin: 0, XMax: 72, YMax: 72}
	out, box, err := Trim(in, window, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, out) {
		t.Fatal("a render with no white on it was re-encoded")
	}
	if box != window {
		t.Fatalf("the box moved to %v from %v", box, window)
	}
}

// Nothing on it means nothing to trim to. Trim leaves it and Blank is what
// throws it out, because a caller that trimmed an empty render to an empty
// rectangle would have a PNG of no pixels to explain.
func TestTrimLeavesABlankRenderAlone(t *testing.T) {
	in := encode(t, canvas(120, 120))
	window := poppler.Box{XMin: 10, YMin: 10, XMax: 40, YMax: 40}
	out, box, err := Trim(in, window, 300)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, out) || box != window {
		t.Fatal("a blank render was trimmed rather than left for Blank to refuse")
	}
}

func TestTrimSaysSoWhenTheRenderIsNotAPNG(t *testing.T) {
	if _, _, err := Trim([]byte("not a png"), poppler.Box{}, 300); err == nil {
		t.Fatal("bytes that are not a PNG were trimmed without complaint")
	}
}

// The detector finds every hole in a column, and a hole left by a table set
// with no rules or by the page simply ending is a hole with nothing in it.
func TestBlankKnowsAnEmptyRender(t *testing.T) {
	if !Blank(encode(t, canvas(200, 200))) {
		t.Fatal("a white render was not called blank")
	}
}

func TestBlankKnowsARenderWithSomethingOnIt(t *testing.T) {
	img := canvas(200, 200)
	ink(img, image.Rect(90, 90, 110, 110))
	if Blank(encode(t, img)) {
		t.Fatal("a render with a square on it was called blank")
	}
}

// A black plate is one flat colour too. Blank is about a region with nothing
// in it, and both of those are regions with nothing in them.
func TestBlankKnowsAFlatBlackRender(t *testing.T) {
	img := canvas(200, 200)
	ink(img, img.Bounds())
	if !Blank(encode(t, img)) {
		t.Fatal("a flat black render was not called blank")
	}
}

// Bytes that do not decode are not blank. They are a render that went wrong,
// which is a different complaint, and calling them blank would drop the
// figure without anybody seeing why.
func TestBlankDoesNotGuessAtBytesItCannotRead(t *testing.T) {
	if Blank([]byte("not a png")) {
		t.Fatal("bytes that are not a PNG were called blank")
	}
}

func TestSizeReadsTheHeader(t *testing.T) {
	w, h, err := Size(encode(t, canvas(321, 123)))
	if err != nil {
		t.Fatal(err)
	}
	if w != 321 || h != 123 {
		t.Fatalf("size = %dx%d, want 321x123", w, h)
	}
}

func TestSizeSaysSoWhenTheBytesAreNotAPNG(t *testing.T) {
	if _, _, err := Size([]byte("not a png")); err == nil {
		t.Fatal("bytes that are not a PNG were measured without complaint")
	}
}
