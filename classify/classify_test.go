package classify

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// The numbers in these cases are the ones real papers produced. Attention Is
// All You Need measures 2538 characters and 4.9 mathematical glyphs a page
// over its body. The 1936 Turing scan measures 1473 characters and 0.06
// glyphs a page, and its pages are one 2288 by 3539 image each.
func TestClassify(t *testing.T) {
	cases := []struct {
		name  string
		m     Measurement
		layer Layer
		path  Path
	}{
		{
			"a born digital paper with figures",
			Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Maths: 49, MathFonts: 5, Fonts: 12, Embedded: 12, Captions: 7},
			Digital, PathLayout,
		},
		{
			"a born digital paper with no figures and no mathematics",
			Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Fonts: 6, Embedded: 6},
			Native, PathNative,
		},
		{
			"a born digital paper whose mathematics extracts",
			Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Maths: 49, MathFonts: 5, Fonts: 12, Embedded: 12},
			Native, PathNative,
		},
		{
			"a born digital paper whose mathematics does not extract",
			Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Maths: 2, MathFonts: 4, Fonts: 10, Embedded: 10},
			Digital, PathLayout,
		},
		{
			"a paper published through dvips, set entirely in nameless bitmaps",
			Measurement{Pages: 9, First: 2, Last: 8, Chars: 17430, Fonts: 27, Embedded: 27, Bitmap: 27, Unmapped: 27},
			Digital, PathLayout,
		},
		{
			"a paper with one bitmap among its real fonts",
			Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Fonts: 12, Embedded: 12, Bitmap: 1},
			Native, PathNative,
		},
		{
			"a scan with an OCR layer",
			Measurement{Pages: 36, First: 12, Last: 23, Chars: 17676, Maths: 1, Fonts: 4, Unmapped: 4, Scans: 12},
			OCR, PathVision,
		},
		{
			"a scan with an OCR layer and no images poppler can see",
			Measurement{Pages: 36, First: 12, Last: 23, Chars: 17676, Fonts: 4, Unmapped: 4},
			OCR, PathVision,
		},
		{
			"a scan with nothing on it",
			Measurement{Pages: 20, First: 3, Last: 14, Chars: 60, Scans: 12},
			None, PathVision,
		},
		{
			"nothing measured at all",
			Measurement{Pages: 0},
			None, PathVision,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.m.Classify()
			if got.Layer != tc.layer || got.Path != tc.path {
				t.Errorf("came out %s on the %s path, want %s on %s (%s)", got.Layer, got.Path, tc.layer, tc.path, got.Why)
			}
			if got.Why == "" {
				t.Error("no reason was given")
			}
		})
	}
}

// A paper with no mathematics in it has no mathematical glyphs, and that
// says nothing about the extraction. The test has to be both halves: the
// fonts are there and the glyphs are not.
func TestNoMathsIsNotBadMaths(t *testing.T) {
	m := Measurement{Pages: 15, First: 3, Last: 12, Chars: 25380, Maths: 0, MathFonts: 0, Fonts: 5, Embedded: 5}
	if v := m.Classify(); v.Layer != Native {
		t.Errorf("a prose paper came out %s: %s", v.Layer, v.Why)
	}
}

// The first version of this called a missing ToUnicode map an OCR layer, and
// it called Attention Is All You Need, GPT-3 and the GAN paper scans. pdfTeX
// leaves the map off a font with a builtin encoding and pdftotext reads those
// perfectly, so the map decides nothing. What separates the two is whether
// the file carries its fonts: the scan of Turing's 1936 paper names
// Times-Roman and embeds nothing, and every paper written in LaTeX embeds a
// subset of everything it uses.
func TestAMissingCharacterMapIsNotAScan(t *testing.T) {
	digital := Measurement{Pages: 9, First: 3, Last: 6, Chars: 9216, Maths: 47, MathFonts: 19, Fonts: 24, Embedded: 24, Unmapped: 24, Captions: 3}
	if v := digital.Classify(); v.Layer == OCR {
		t.Errorf("a paper that embeds all 24 of its fonts came out %s: %s", v.Layer, v.Why)
	}
	scan := Measurement{Pages: 36, First: 12, Last: 23, Chars: 17676, Fonts: 4, Embedded: 0, Unmapped: 4}
	if v := scan.Classify(); v.Layer != OCR {
		t.Errorf("a file that embeds none of its 4 fonts came out %s: %s", v.Layer, v.Why)
	}
}

// A systems paper sets its bullet points in CMSY and has no equation in it.
// The MapReduce paper carries ten mathematical fonts across sixty and zero
// mathematical glyphs, and saying its mathematics is being mangled would be
// a false statement printed next to a correct verdict.
func TestOneSymbolFontIsNotMathematics(t *testing.T) {
	systems := []struct {
		name string
		m    Measurement
	}{
		{"mapreduce", Measurement{Pages: 13, First: 3, Last: 10, Chars: 28784, Fonts: 60, Embedded: 60, MathFonts: 10, Unmapped: 60, Captions: 5}},
		{"p4", Measurement{Pages: 8, First: 3, Last: 5, Chars: 11220, Fonts: 8, Embedded: 8, MathFonts: 1, Unmapped: 8}},
		{"bitcoin", Measurement{Pages: 9, First: 3, Last: 6, Chars: 8904, Fonts: 7, Embedded: 7, MathFonts: 1}},
	}
	for _, tc := range systems {
		if tc.m.Mathematical() {
			t.Errorf("%s was taken to contain mathematics on %d of %d fonts", tc.name, tc.m.MathFonts, tc.m.Fonts)
		}
		if v := tc.m.Classify(); strings.Contains(v.Why, "mangling the mathematics") {
			t.Errorf("%s was told its mathematics is mangled and it has none: %s", tc.name, v.Why)
		}
	}

	// The papers that really are mathematical run from a third upwards.
	maths := []Measurement{
		{Pages: 15, First: 3, Last: 12, Fonts: 22, MathFonts: 12},
		{Pages: 9, First: 3, Last: 6, Fonts: 27, MathFonts: 19},
		{Pages: 12, First: 3, Last: 9, Fonts: 28, MathFonts: 9},
	}
	for _, m := range maths {
		if !m.Mathematical() {
			t.Errorf("a paper with %d of %d fonts mathematical was not taken to contain mathematics", m.MathFonts, m.Fonts)
		}
	}
}

func TestBand(t *testing.T) {
	cases := []struct {
		pages, width      int
		first, last, want int
	}{
		{36, 12, 11, 22, 12},
		{15, 12, 2, 13, 12},
		{9, 12, 2, 8, 7},
		{8, 12, 2, 7, 6},
		{4, 12, 2, 3, 2},
		{3, 12, 1, 3, 3},
		{1, 12, 1, 1, 1},
		{200, 12, 87, 98, 12},
	}
	for _, tc := range cases {
		first, last := Band(tc.pages, tc.width)
		if first != tc.first || last != tc.last {
			t.Errorf("a %d page paper samples %d to %d, want %d to %d", tc.pages, first, last, tc.first, tc.last)
		}
		if got := (Measurement{First: first, Last: last}).BandPages(); got != tc.want {
			t.Errorf("a %d page paper samples %d pages, want %d", tc.pages, got, tc.want)
		}
		if first < 1 || last > tc.pages {
			t.Errorf("a %d page paper samples pages %d to %d, which are not all in it", tc.pages, first, last)
		}
	}
}

func TestMathFont(t *testing.T) {
	yes := []string{"LICAEO+CMMI10", "NSOWGJ+CMSY10", "QDTWCG+MSBM10", "CMEX10", "STIXMath-Regular", "LatinModernMath-Regular", "Symbol"}
	no := []string{"AECCXO+NimbusRomNo9L-Regu", "Times-Roman", "Helvetica", "FUIULY+CMR10", "Courier-Bold"}
	for _, name := range yes {
		if !MathFont(name) {
			t.Errorf("%s is a mathematical font and was not recognised", name)
		}
	}
	for _, name := range no {
		if MathFont(name) {
			t.Errorf("%s is not a mathematical font and was", name)
		}
	}
}

// Latin letters and digits are not counted, because "x = 2" extracts fine
// from almost anything and counting it would measure nothing.
func TestMathGlyphs(t *testing.T) {
	chars, maths := count("Where the projections are WiQ ∈ Rdmodel ×dk and α ≤ β.")
	if chars == 0 {
		t.Fatal("nothing was counted")
	}
	if maths != 5 {
		t.Errorf("counted %d mathematical glyphs, want 5", maths)
	}
	if _, maths := count("x = 2 and y = 3, so x + y = 5."); maths != 0 {
		t.Errorf("ordinary arithmetic counted as %d mathematical glyphs", maths)
	}
}

func TestCaptionsAreLines(t *testing.T) {
	text := `As shown in Figure 2, the encoder has six layers.

Figure 2: The Transformer architecture.

    Table 1  Maximum path lengths.
`
	if n := len(caption.FindAllString(text, -1)); n != 2 {
		t.Errorf("found %d captions, want 2: a sentence mentioning a figure is not a figure", n)
	}
}

// A scanned page is one image the size of the page. The pixel count on its
// own decides nothing, which is why the density has to be read as well.
func TestScansAreFullPageImages(t *testing.T) {
	page := poppler.Doc{Width: 451.44, Height: 697.68}
	scan := []poppler.Image{
		{Page: 5, Width: 2288, Height: 3539, XPPI: 400, YPPI: 400},
		{Page: 6, Width: 2308, Height: 3514, XPPI: 400, YPPI: 400},
	}
	if n := scans(scan, page); n != 2 {
		t.Errorf("found %d scanned pages, want 2", n)
	}

	figure := []poppler.Image{
		{Page: 5, Width: 2288, Height: 3539, XPPI: 4000, YPPI: 4000},
		{Page: 5, Width: 300, Height: 200, XPPI: 150, YPPI: 150},
	}
	if n := scans(figure, page); n != 0 {
		t.Errorf("found %d scanned pages among the figures, want 0", n)
	}
	if n := scans(scan, poppler.Doc{}); n != 0 {
		t.Error("a document of unknown size produced a scan count")
	}
}

func TestParsePoppler(t *testing.T) {
	// Whether the toolchain can read what poppler prints is not something to
	// find out halfway through a hundred papers.
	const fonts = `name                                 type              encoding         emb sub uni object ID
------------------------------------ ----------------- ---------------- --- --- --- ---------
LICAEO+CMMI10                        Type 1            Builtin          yes yes yes    139  0
Times-Roman                          Type 1            WinAnsi          no  no  no     221  0
`
	got := poppler.ParseFonts([]byte(fonts))
	if len(got) != 2 {
		t.Fatalf("parsed %d fonts, want 2", len(got))
	}
	if !got[0].Embedded || !got[0].Unicode {
		t.Errorf("the embedded font parsed as %+v", got[0])
	}
	if got[1].Embedded || got[1].Unicode {
		t.Errorf("the OCR font parsed as %+v", got[1])
	}
	if !strings.Contains(got[0].Name, "CMMI10") {
		t.Errorf("the font name parsed as %q", got[0].Name)
	}
}

// Three stages read a recorded text layer and act on it, so the mapping from
// one to a path is one function and this is the test of it. A layer nobody
// has measured maps to nothing rather than to a safe default, because a run
// over an unclassified paper should stop and say so.
func TestLayerPath(t *testing.T) {
	for _, c := range []struct {
		layer Layer
		want  Path
	}{
		{Native, PathNative},
		{Digital, PathLayout},
		{OCR, PathVision},
		{None, PathVision},
		{"", ""},
		{"typewriter", ""},
	} {
		if got := c.layer.Path(); got != c.want {
			t.Errorf("the %q layer takes the %q path, want %q", c.layer, got, c.want)
		}
	}
}

// Every verdict the measurement can reach names the path the layer it
// recorded would have chosen anyway. The two disagreeing would mean a paper
// extracted down one path and reported as being on another.
func TestEveryVerdictAgreesWithItsLayer(t *testing.T) {
	for _, v := range []Verdict{
		{Layer: Native, Path: PathNative},
		{Layer: Digital, Path: PathLayout},
		{Layer: OCR, Path: PathVision},
		{Layer: None, Path: PathVision},
	} {
		if got := v.Layer.Path(); got != v.Path {
			t.Errorf("a %s verdict says %s and its layer says %s", v.Layer, v.Path, got)
		}
	}
}

// The Razborov paper is nine pages of TeX pushed through dvips in 1994 and
// every one of its thirty five fonts is a nameless bitmap. It embeds all of
// them, it has thousands of characters a page, and none of the other tests
// sees anything wrong with it, so before this it was read with pdftotext and
// the set "{x : not x}" landed in the corpus as "f x : : x g". The Vietnamese
// translation of it was refused nine times over, because the model was being
// asked to translate a page of CMSY font positions.
func TestTransliterated(t *testing.T) {
	for _, c := range []struct {
		name string
		m    Measurement
		want bool
	}{
		{"a dvips paper", Measurement{Fonts: 27, Bitmap: 27}, true},
		{"a paper with a bitmap diagram label", Measurement{Fonts: 12, Bitmap: 1}, false},
		{"a paper mostly in bitmaps", Measurement{Fonts: 6, Bitmap: 5}, false},
		{"an ordinary paper", Measurement{Fonts: 12}, false},
		{"a file with no fonts at all", Measurement{}, false},
	} {
		if got := c.m.Transliterated(); got != c.want {
			t.Errorf("%s is transliterated %v, want %v", c.name, got, c.want)
		}
	}
}
