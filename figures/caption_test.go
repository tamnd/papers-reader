package figures

import (
	"testing"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

func par(y0, y1 float64, text string) extract.Paragraph {
	return extract.Paragraph{
		Box:  poppler.Box{XMin: colLeft, YMin: y0, XMax: colRight, YMax: y1},
		Text: text,
	}
}

func onlyCaption(t *testing.T, text string) Caption {
	t.Helper()
	got := Captions(extract.Page{Number: 1, Paragraphs: []extract.Paragraph{par(400, 424, text)}}, nil)
	if len(got) != 1 {
		t.Fatalf("%q gave %d captions, want 1", text, len(got))
	}
	return got[0]
}

// The hundred write their captions every way there is, and a pattern that
// insisted on one of them would lose every figure of the papers that use
// another.
func TestTheCaptionOpeningsThePapersUse(t *testing.T) {
	for _, c := range []struct {
		text   string
		kind   string
		number string
	}{
		{"Figure 1: a diagram of the thing", "figure", "1"},
		{"Figure 2. a diagram of the thing", "figure", "2"},
		{"Fig. 3 A diagram of the thing", "figure", "3"},
		{"Fig 4 - a diagram of the thing", "figure", "4"},
		{"FIGURE 5: a diagram of the thing", "figure", "5"},
		{"FIG. 6. a diagram of the thing", "figure", "6"},
		{"Figure 3-a: the left half of it", "figure", "3-a"},
		{"Table 1: what was measured", "table", "1"},
		{"TABLE II. what was measured", "table", "II"},
		{"Algorithm 2: how it is done", "algorithm", "2"},
		{"Listing 1: the loop", "listing", "1"},
	} {
		got := onlyCaption(t, c.text)
		if got.Kind != c.kind {
			t.Errorf("%q is a %q, want %q", c.text, got.Kind, c.kind)
		}
		if got.Number != c.number {
			t.Errorf("%q is numbered %q, want %q", c.text, got.Number, c.number)
		}
	}
}

// A sentence of the body that happens to begin "Figure 3 shows" is a cross
// reference. Taking it for a caption puts the paragraph under the figure
// and leaves the real caption for the next region along.
func TestASentenceAboutAFigureIsNotACaption(t *testing.T) {
	for _, text := range []string{
		"Figure 3 shows that the loss falls away after the first epoch.",
		"Fig. 2 gives the same result for the smaller model.",
		"Table 4 lists every setting that was tried.",
	} {
		got := Captions(extract.Page{Number: 1, Paragraphs: []extract.Paragraph{par(400, 424, text)}}, nil)
		if len(got) != 0 {
			t.Errorf("%q was read as a caption", text)
		}
	}
}

// A caption is three lines of prose and only the first starts with the word
// Figure, so it is read out of the paragraph and the whole paragraph is
// kept.
func TestTheWholeCaptionIsKept(t *testing.T) {
	text := "Figure 7: the whole of the caption, which runs to a second line " +
		"and says a good deal more than the first line does."
	if got := onlyCaption(t, text).Text; got != text {
		t.Fatalf("the caption came back as %q", got)
	}
}

// The Gamma paper captions all nineteen of its figures with the word and the
// number and nothing else, and a caption that is only its own opening is not
// a sentence carrying on about a figure somewhere else.
func TestACaptionWithNothingAfterTheNumberIsStillACaption(t *testing.T) {
	for _, c := range []struct {
		text   string
		number string
	}{
		{"Figure 19", "19"},
		{"Fig. 4", "4"},
		{"Table 5", "5"},
	} {
		got := onlyCaption(t, c.text)
		if got.Number != c.number {
			t.Errorf("%q is numbered %q, want %q", c.text, got.Number, c.number)
		}
	}
}

// The number is what the corpus files a figure under and what rule F09 reads
// to say a figure the prose mentions is missing, so the one case above needs
// one. A word on its own is not enough to go on.
func TestTheBareWordWithNoNumberAndNothingAfterItIsNotACaption(t *testing.T) {
	for _, text := range []string{"Figure", "Table", "Chart"} {
		if got := Captions(extract.Page{Number: 1, Paragraphs: []extract.Paragraph{par(400, 424, text)}}, nil); len(got) != 0 {
			t.Errorf("%q was read as a caption", text)
		}
	}
}

func TestACaptionWithNoNumberIsStillACaption(t *testing.T) {
	got := onlyCaption(t, "Figure: the one diagram in the paper")
	if got.Kind != "figure" {
		t.Fatalf("kind = %q, want figure", got.Kind)
	}
	if got.Number != "" {
		t.Fatalf("number = %q, want none", got.Number)
	}
}

func pairOne(t *testing.T, region poppler.Box, caps []Caption) *Caption {
	t.Helper()
	got := Pair([]Candidate{{Page: 1, Box: region}}, caps, leading)
	if len(got) != 1 {
		t.Fatalf("Pair returned %d results for one candidate", len(got))
	}
	return got[0].Caption
}

func TestTheCaptionUnderTheFigureIsTheFiguresCaption(t *testing.T) {
	region := poppler.Box{XMin: colLeft, YMin: 200, XMax: colRight, YMax: 400}
	caps := []Caption{{
		Page: 1, Kind: "figure", Number: "1", Text: "Figure 1: below it",
		Box: poppler.Box{XMin: colLeft, YMin: 404, XMax: colRight, YMax: 428},
	}}
	got := pairOne(t, region, caps)
	if got == nil {
		t.Fatal("the region came back with no caption")
	}
	if got.Number != "1" {
		t.Fatalf("caption = %q, want figure 1", got.Text)
	}
}

// Papers caption their figures below and their tables above and are not
// consistent about either, so both are looked at and below wins a tie.
func TestTheCaptionBelowWinsOverTheOneAbove(t *testing.T) {
	region := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 400}
	caps := []Caption{
		{Page: 1, Number: "1", Text: "Figure 1: above it",
			Box: poppler.Box{XMin: colLeft, YMin: 288, XMax: colRight, YMax: 298}},
		{Page: 1, Number: "2", Text: "Figure 2: below it",
			Box: poppler.Box{XMin: colLeft, YMin: 402, XMax: colRight, YMax: 412}},
	}
	got := pairOne(t, region, caps)
	if got == nil || got.Number != "2" {
		t.Fatalf("the caption chosen was %+v, want the one below", got)
	}
}

// A figure at the top of a column and the first paragraph of prose under it
// are not a figure and its caption, and there is nothing else on the page
// to tell them apart.
func TestACaptionTooFarAwayIsNotThisFiguresCaption(t *testing.T) {
	region := poppler.Box{XMin: colLeft, YMin: 200, XMax: colRight, YMax: 300}
	caps := []Caption{{
		Page: 1, Number: "1", Text: "Figure 1: a long way below",
		Box: poppler.Box{XMin: colLeft, YMin: 500, XMax: colRight, YMax: 512},
	}}
	if got := pairOne(t, region, caps); got != nil {
		t.Fatalf("a caption %v points away was taken: %+v", 200, got)
	}
}

func TestACaptionInTheOtherColumnIsNotThisFiguresCaption(t *testing.T) {
	region := poppler.Box{XMin: 72, YMin: 300, XMax: 290, YMax: 400}
	caps := []Caption{{
		Page: 1, Number: "1", Text: "Figure 1: in the right column",
		Box: poppler.Box{XMin: 320, YMin: 404, XMax: 540, YMax: 416},
	}}
	if got := pairOne(t, region, caps); got != nil {
		t.Fatalf("a caption in the other column was taken: %+v", got)
	}
}

// One caption belongs to one figure. Two regions stacked over the same
// caption is two halves of one figure that did not merge, and giving both
// of them the caption would commit the same picture twice under one name.
func TestACaptionIsOnlyUsedOnce(t *testing.T) {
	// Both are near enough to the caption to claim it, so the one that
	// keeps it is decided by the rule and not by the reach.
	cands := []Candidate{
		{Page: 1, Box: poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 380}},
		{Page: 1, Box: poppler.Box{XMin: colLeft, YMin: 384, XMax: colRight, YMax: 400}},
	}
	caps := []Caption{{
		Page: 1, Number: "1", Text: "Figure 1: under the lower one",
		Box: poppler.Box{XMin: colLeft, YMin: 404, XMax: colRight, YMax: 416},
	}}
	got := Pair(cands, caps, leading)
	taken := 0
	for _, f := range got {
		if f.Caption != nil {
			taken++
		}
	}
	if taken != 1 {
		t.Fatalf("%d of the two regions got the one caption", taken)
	}
}

func TestACaptionOnAnotherPageIsNotThisFiguresCaption(t *testing.T) {
	region := poppler.Box{XMin: colLeft, YMin: 300, XMax: colRight, YMax: 400}
	caps := []Caption{{
		Page: 2, Number: "1", Text: "Figure 1: on the next page",
		Box: poppler.Box{XMin: colLeft, YMin: 404, XMax: colRight, YMax: 416},
	}}
	if got := pairOne(t, region, caps); got != nil {
		t.Fatalf("a caption from another page was taken: %+v", got)
	}
}
