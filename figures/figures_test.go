package figures

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// The page every fixture in this package is set on: letter paper, one inch
// margins, ten point type on twelve point leading. Nothing here comes from a
// real paper, because a test fixture that quotes one is a copy of one.
const (
	pageWide = 612.0
	pageTall = 792.0
	colLeft  = 72.0
	colRight = 540.0
	topLine  = 72.0
	botLine  = 720.0
	leading  = 12.0
)

func TestTheBudgetRefusesAPageImage(t *testing.T) {
	b := Default()
	fig := Figure{Width: 2000, Height: 2600, Bytes: 300 << 10, Fraction: 0.94}
	err := b.Check(fig)
	if err == nil {
		t.Fatal("a figure covering 94 percent of its page was accepted")
	}
	if got := err.Error(); got == "" {
		t.Fatal("the refusal says nothing")
	}
}

func TestTheBudgetRefusesAGlyph(t *testing.T) {
	b := Default()
	if err := b.Check(Figure{Width: 64, Height: 64, Bytes: 900, Fraction: 0.01}); err == nil {
		t.Fatal("a 64 by 64 image was accepted as a diagram")
	}
}

// A figure can be a strip. Figure 3 of the GAN paper is one row of digits
// across the column, 1434 by 87 pixels, and it is a diagram by every measure
// except the one that asked both sides to clear the same floor.
func TestTheBudgetTakesAStrip(t *testing.T) {
	b := Default()
	if err := b.Check(Figure{Width: 1434, Height: 87, Bytes: 60 << 10, Fraction: 0.08}); err != nil {
		t.Fatalf("a row of digits across the column was refused: %v", err)
	}
}

// What the short side is for is the hairlines: a rule under a table header,
// a column border, an underline.
func TestTheBudgetRefusesARule(t *testing.T) {
	b := Default()
	err := b.Check(Figure{Width: 1434, Height: 8, Bytes: 400, Fraction: 0.01})
	if err == nil {
		t.Fatal("a two point rule across the column was accepted as a diagram")
	}
	if !strings.Contains(err.Error(), "short side") {
		t.Errorf("the refusal reads %q, and it is the short side that is wrong", err)
	}
}

func TestTheBudgetRefusesAFileOverTheCap(t *testing.T) {
	b := Default()
	if err := b.Check(Figure{Width: 2000, Height: 2000, Bytes: MaxBytes + 1, Fraction: 0.2}); err == nil {
		t.Fatal("a figure over the byte cap was accepted")
	}
}

func TestTheBudgetTakesADiagram(t *testing.T) {
	b := Default()
	if err := b.Check(Figure{Width: 1200, Height: 900, Bytes: 120 << 10, Fraction: 0.22}); err != nil {
		t.Fatalf("an ordinary diagram was refused: %v", err)
	}
}

// The fraction is what separates a corpus from a mirror, so the boundary is
// worth pinning down rather than leaving to whichever way the comparison
// happens to be written.
func TestTheFractionCapIsInclusive(t *testing.T) {
	b := Default()
	if err := b.Check(Figure{Width: 1000, Height: 1000, Bytes: 1000, Fraction: MaxFraction}); err != nil {
		t.Fatalf("a figure exactly at the cap was refused: %v", err)
	}
	if err := b.Check(Figure{Width: 1000, Height: 1000, Bytes: 1000, Fraction: MaxFraction + 0.001}); err == nil {
		t.Fatal("a figure just over the cap was accepted")
	}
}

func TestFractionIsAreaOverArea(t *testing.T) {
	page := poppler.Layout{Width: 600, Height: 800}
	got := Fraction(poppler.Box{XMin: 100, YMin: 100, XMax: 400, YMax: 500}, page)
	if want := 300.0 * 400 / (600 * 800); got != want {
		t.Fatalf("fraction = %v, want %v", got, want)
	}
}

// A full width band an inch high is a figure and a column wide band the
// height of the page is most of somebody's paper, which is why the rule is
// about area and not about either dimension on its own.
func TestATallNarrowBandIsNotMostOfThePage(t *testing.T) {
	page := poppler.Layout{Width: pageWide, Height: pageTall}
	tall := poppler.Box{XMin: 72, YMin: 72, XMax: 300, YMax: 720}
	if f := Fraction(tall, page); f > MaxFraction {
		t.Fatalf("a single column of the page came to %.2f of it", f)
	}
}

func TestFractionOfAPageWithNoSizeIsZero(t *testing.T) {
	if f := Fraction(poppler.Box{XMax: 10, YMax: 10}, poppler.Layout{}); f != 0 {
		t.Fatalf("fraction of a page with no dimensions = %v, want 0", f)
	}
}

// figures.yaml records boxes the way a PDF viewer shows them, so that
// somebody can check one against the file without doing arithmetic.
func TestTheRecordedBoxHasItsOriginAtTheBottomLeft(t *testing.T) {
	got := pdfBox(poppler.Box{XMin: 72, YMin: 100, XMax: 540, YMax: 300}, pageTall)
	want := Box{72, pageTall - 300, 540, pageTall - 100}
	if got != want {
		t.Fatalf("pdfBox = %v, want %v", got, want)
	}
}

func TestTheRecordedBoxIsRoundedToAHundredthOfAPoint(t *testing.T) {
	got := pdfBox(poppler.Box{XMin: 72.123456, YMin: 0, XMax: 100, YMax: 1}, 792)
	if got[0] != 72.12 {
		t.Fatalf("xmin = %v, want 72.12", got[0])
	}
}

func TestTheFileIsNamedAfterTheFigure(t *testing.T) {
	if got := (Figure{ID: "f07"}).Name(); got != "f07.png" {
		t.Fatalf("name = %q, want f07.png", got)
	}
}

func TestTheHashIsOfTheBytesAsCommitted(t *testing.T) {
	a, b := SHA256([]byte("one")), SHA256([]byte("one"))
	if a != b {
		t.Fatal("the same bytes hashed to two different things")
	}
	if a == SHA256([]byte("two")) {
		t.Fatal("different bytes hashed to the same thing")
	}
	if len(a) != 64 {
		t.Fatalf("the hash is %d characters, want 64", len(a))
	}
}

// Shannon's schematic is 44 points by 49 on a page of 612 by 792, which two
// decimal places wrote as 0, and a fraction of 0 is what rule F06 reads as a
// figure whose fraction was never recorded.
func TestATinyFigureStillRecordsAFraction(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want float64
	}{
		{0.00446, 0.0045},
		{0.000001, 0.0001},
		{0.75, 0.75},
		{0.1949, 0.1949},
		{0, 0},
		{-1, 0},
	} {
		if got := roundFraction(c.in); got != c.want {
			t.Errorf("roundFraction(%v) is %v, want %v", c.in, got, c.want)
		}
	}
}
