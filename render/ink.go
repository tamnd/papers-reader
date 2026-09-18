package render

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // the store writes PNGs and nothing else
)

// Dark is how dark a pixel has to be to count as ink, out of 255.
//
// A scan of a 1971 proceedings is not black on white. The paper is grey, the
// letters are grey, and the edges of every letter are greyer still, so a
// threshold set near black counts the middle of a stroke and misses its
// sides, and the same page scanned twice at two exposures comes out with two
// different amounts of ink on it. 200 is well below the paper of every scan
// in this corpus and well above the lightest stroke of any of them, which is
// what makes the measurement about the letters and not about the scanner.
const Dark = 200

// Step is how many pixels are looked at, one in Step across and one in Step
// down. The measurement is a fraction of a page and a page at 300 dpi is
// eight million pixels, so a quarter of them is the same answer to four
// decimal places for a quarter of the work.
const Step = 2

// Ink is the fraction of a rendered page that is covered in ink.
//
// It is here for rule A5, which asks whether a reading of a page is far
// shorter than this paper's pages usually are. That rule has good evidence
// on a born digital paper, where the file's own text layer says how much
// prose is on the page, and no evidence at all on a scan, where there is no
// layer and every page is a picture. On a scan the rule can only compare a
// page against the average of the paper, and a page that is genuinely short
// is refused for being genuinely short.
//
// Page 8 of Cook's paper is the case that paid for this. It is the last page
// of the bibliography and it carries two references and a folio, and the
// reader transcribed it correctly three times at three resolutions and was
// refused all three times for coming back at 287 characters against a paper
// averaging 3679. The page is 0.5 per cent ink where the paper averages 5.6
// per cent, and that is the whole of the story: there is nothing on it.
//
// What this measures is a ceiling and not an estimate. A page covered in a
// half tone photograph is nearly all ink and holds no words at all, so a lot
// of ink says nothing about how much text to expect. Little ink does: a page
// cannot print two thousand characters in a twentieth of the ink its
// neighbours spend on two thousand characters. So the measurement may lower
// what A5 expects of a reading and must never raise it.
func Ink(png []byte) (float64, error) {
	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return 0, fmt.Errorf("measuring the ink on a page: %w", err)
	}
	b := img.Bounds()
	dark, seen := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += Step {
		for x := b.Min.X; x < b.Max.X; x += Step {
			r, g, bl, _ := img.At(x, y).RGBA()
			// Rec. 601 luminance, and the shift takes the 16 bit channels
			// RGBA returns back down to the 8 bits Dark is written in.
			lum := (299*r + 587*g + 114*bl) / 1000 >> 8
			seen++
			if lum < Dark {
				dark++
			}
		}
	}
	if seen == 0 {
		return 0, nil
	}
	return float64(dark) / float64(seen), nil
}
