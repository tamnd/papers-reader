package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// page draws a page with a band of ink across the top of it, covering the
// share of the sheet asked for. It stands in for a page of a scan, where the
// letters are grey and not black: 120 is about as dark as the strokes on the
// Cook scan come out.
func page(share float64) []byte {
	const w, h = 200, 300
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := uint8(255)
		if float64(y) < float64(h)*share {
			v = 120
		}
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func TestInkIsTheShareOfThePageThatIsCovered(t *testing.T) {
	for _, share := range []float64{0, 0.05, 0.5, 1} {
		got, err := Ink(page(share))
		if err != nil {
			t.Fatalf("a page %.2f covered: %v", share, err)
		}
		if d := got - share; d > 0.01 || d < -0.01 {
			t.Errorf("a page %.2f covered measured %.4f", share, got)
		}
	}
}

// Paper is not white and a scanner does not pretend it is. A sheet of grey
// paper with nothing printed on it has to measure as empty, or every page of
// a scan measures as full and the rule that reads this learns nothing.
func TestGreyPaperIsNotInk(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.SetGray(x, y, color.Gray{Y: 235})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	got, err := Ink(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("a blank grey sheet measured %.4f ink", got)
	}
}

func TestInkSaysWhatItCouldNotRead(t *testing.T) {
	if _, err := Ink([]byte("this is not a PNG")); err == nil {
		t.Error("something that is not an image measured without complaint")
	}
}
