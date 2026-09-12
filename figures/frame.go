package figures

import (
	"sort"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// A Frame is where a paper puts its type: the rectangle its text block
// occupies on a normal page.
//
// It exists because of the figure at the top of a page. The hole a figure
// leaves is the space between the text above it and the text below, and a
// figure at the very top of a page has no text above it. Measuring from the
// top of that page's own text finds a hole of zero height and loses the
// figure, which on the Transformer paper is figure 1.
//
// So the frame is learned from the paper, the same way the running heads
// are, and for the same reason: every journal sets its margins differently
// and a number in the code would be right for one of them.
type Frame struct {
	Top, Bottom, Left, Right float64
}

// FindFrame measures the text block over every page.
//
// Quartiles rather than the extremes. A paper has a title page whose text
// starts two inches down, a page whose last line is the first line of a
// section, and a page with a footnote rule under the body, and any of those
// as the answer would put the frame somewhere no page actually is.
func FindFrame(pages []poppler.Layout, f *extract.Furniture) Frame {
	var tops, bottoms, lefts, rights []float64
	for _, p := range pages {
		lines := f.Lines(p)
		if len(lines) < minLines {
			continue
		}
		top, bottom := lines[0].YMin, lines[0].YMax
		left, right := lines[0].XMin, lines[0].XMax
		for _, l := range lines {
			top = min(top, l.YMin)
			bottom = max(bottom, l.YMax)
			left = min(left, l.XMin)
			right = max(right, l.XMax)
		}
		tops = append(tops, top)
		bottoms = append(bottoms, bottom)
		lefts = append(lefts, left)
		rights = append(rights, right)
	}
	if len(tops) == 0 {
		return Frame{}
	}
	return Frame{
		Top:    quartile(tops, 0.25),
		Bottom: quartile(bottoms, 0.75),
		Left:   quartile(lefts, 0.25),
		Right:  quartile(rights, 0.75),
	}
}

// Empty says the frame was never measured, which is what a caller gets for
// a paper of one short page. Find falls back to the page's own text.
func (f Frame) Empty() bool { return f.Bottom <= f.Top }

func quartile(v []float64, q float64) float64 {
	sort.Float64s(v)
	i := int(float64(len(v)-1) * q)
	return v[i]
}
