package poppler

import (
	"strings"
	"testing"
)

// The fixture is what pdftotext -bbox-layout writes, cut down to two pages.
// It is written out rather than captured from a paper because every paper in
// the corpus is under someone's copyright and a test file is the last place
// to put one.
const bbox = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" lang="" xml:lang="">
<head>
  <title>a paper</title>
</head>
<body>
<doc>
  <page width="612.000000" height="792.000000">
    <flow>
      <block xMin="60.000000" yMin="100.000000" xMax="300.000000" yMax="124.000000">
        <line xMin="60.000000" yMin="100.000000" xMax="300.000000" yMax="110.000000">
          <word xMin="60.000000" yMin="100.000000" xMax="90.000000" yMax="110.000000">The</word>
          <word xMin="95.000000" yMin="100.000000" xMax="160.000000" yMax="110.000000">first</word>
          <word xMin="165.000000" yMin="100.000000" xMax="300.000000" yMax="110.000000">line&#8212;here</word>
        </line>
        <line xMin="60.000000" yMin="112.000000" xMax="240.000000" yMax="122.000000">
          <word xMin="60.000000" yMin="112.000000" xMax="120.000000" yMax="122.000000">second</word>
          <word xMin="125.000000" yMin="112.000000" xMax="240.000000" yMax="122.000000">line</word>
          <word xMin="245.000000" yMin="112.000000" xMax="245.000000" yMax="122.000000"> </word>
        </line>
      </block>
    </flow>
  </page>
  <page width="612.000000" height="792.000000">
    <flow>
      <block xMin="60.000000" yMin="100.000000" xMax="200.000000" yMax="110.000000">
        <line xMin="60.000000" yMin="100.000000" xMax="200.000000" yMax="110.000000">
          <word xMin="60.000000" yMin="100.000000" xMax="200.000000" yMax="110.000000">alone</word>
        </line>
      </block>
    </flow>
  </page>
</doc>
</body>
</html>
`

func TestParseLayoutReadsThePagesAndTheirGeometry(t *testing.T) {
	pages, err := ParseLayout([]byte(bbox))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("read %d pages, want 2", len(pages))
	}
	p := pages[0]
	if p.Number != 1 || p.Width != 612 || p.Height != 792 {
		t.Errorf("page 1 is %d at %gx%g", p.Number, p.Width, p.Height)
	}
	if len(p.Blocks) != 1 || len(p.Blocks[0].Lines) != 2 {
		t.Fatalf("page 1 has %d blocks", len(p.Blocks))
	}
	if got := p.Blocks[0].Box; got.XMin != 60 || got.YMax != 124 {
		t.Errorf("the block is %+v", got)
	}

	lines := p.Lines()
	if got := lines[0].Text(); got != "The first line—here" {
		t.Errorf("the first line reads %q, want the entity decoded", got)
	}
	// An empty word is what a PDF with a stray space in its text layer gives,
	// and carrying it would double a space in the line.
	if got := lines[1].Text(); got != "second line" {
		t.Errorf("the second line reads %q, want the empty word dropped", got)
	}
	if got := lines[0].Words[0].Box; got != (Box{XMin: 60, YMin: 100, XMax: 90, YMax: 110}) {
		t.Errorf("the first word is at %+v", got)
	}
	if pages[1].Number != 2 || pages[1].Lines()[0].Text() != "alone" {
		t.Errorf("the second page reads %q", pages[1].Lines()[0].Text())
	}
}

func TestParseLayoutSaysNothingAboutAFileWithNoPages(t *testing.T) {
	// A PDF whose pages are all images comes back as a well formed document
	// with nothing in it, and that is the signal the native path uses to hand
	// the paper to the vision path. It is not an error.
	pages, err := ParseLayout([]byte("<html><body><doc></doc></body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Errorf("read %d pages from an empty document", len(pages))
	}
}

func TestParseLayoutRefusesSomethingThatIsNotTheDocument(t *testing.T) {
	if _, err := ParseLayout([]byte("<page><word>")); err == nil {
		t.Error("read a truncated document without complaining")
	}
}

func TestABoxKnowsItsOwnShape(t *testing.T) {
	b := Box{XMin: 10, YMin: 20, XMax: 40, YMax: 60}
	if b.Width() != 30 || b.Height() != 40 {
		t.Errorf("%+v measures %gx%g", b, b.Width(), b.Height())
	}
	if b.XMid() != 25 || b.YMid() != 40 {
		t.Errorf("the centre of %+v is %g,%g", b, b.XMid(), b.YMid())
	}
}

func TestAControlCharacterInTheTextLayerDoesNotLoseThePage(t *testing.T) {
	// A font that maps a glyph to nothing leaves the control character in the
	// text layer and poppler prints it into the XHTML, which no XML decoder
	// will accept. Losing fifteen readable pages over it is the wrong answer.
	broken := strings.Replace(bbox, ">second<", ">sec\x03ond<", 1)
	pages, err := ParseLayout([]byte(broken))
	if err != nil {
		t.Fatalf("a page with a control character in it did not parse: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages came back")
	}
	got := pages[0].Lines()[1].Text()
	if !strings.Contains(got, "second") {
		t.Errorf("the line reads %q, want the word with the character gone", got)
	}
}
