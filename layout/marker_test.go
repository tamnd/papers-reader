package layout

import (
	"strings"
	"testing"
)

// Marker's shape, invented words. One page with a heading, a paragraph
// holding inline mathematics, a display equation, a figure group and a table
// group, plus a running head to be dropped.
const markerFixture = `{
  "id": "/page/0/Page/0",
  "block_type": "Document",
  "children": [
    {
      "id": "/page/0/Page/0",
      "block_type": "Page",
      "bbox": [0, 0, 612, 792],
      "children": [
        {"id": "/page/0/PageHeader/0", "block_type": "PageHeader", "bbox": [72, 40, 540, 55],
         "html": "<p>Journal of Invented Results</p>"},
        {"id": "/page/0/SectionHeader/0", "block_type": "SectionHeader", "bbox": [72, 90, 540, 110],
         "html": "<h2>3.1 Gated units</h2>"},
        {"id": "/page/0/Text/0", "block_type": "Text", "bbox": [72, 120, 540, 200],
         "html": "<p>The error <span class=\"math\">E_n</span> falls with n, and a < b throughout.</p>"},
        {"id": "/page/0/Equation/0", "block_type": "Equation", "bbox": [72, 210, 540, 250],
         "html": "<p>$$E_n = 1 / n$$</p>"},
        {"id": "/page/0/FigureGroup/0", "block_type": "FigureGroup", "bbox": [72, 260, 540, 500],
         "children": [
           {"id": "/page/0/Figure/0", "block_type": "Figure", "bbox": [80, 265, 530, 460],
            "html": "<p><img src=\"_page_0_Figure_1.jpeg\"></p>"},
           {"id": "/page/0/Caption/0", "block_type": "Caption", "bbox": [80, 465, 530, 495],
            "html": "<p>Figure 1: the shape of the thing</p>"}
         ]},
      {"id": "/page/0/TableGroup/0", "block_type": "TableGroup", "bbox": [72, 510, 540, 700],
         "children": [
           {"id": "/page/0/Caption/1", "block_type": "Caption", "bbox": [80, 510, 530, 530],
            "html": "<p>Table 1: invented numbers</p>"},
           {"id": "/page/0/Table/0", "block_type": "Table", "bbox": [80, 535, 530, 695],
            "html": "<table><tr><th>model</th><th>score</th></tr><tr><td>first</td><td>1.0</td></tr></table>"}
         ]},
        {"id": "/page/0/Code/0", "block_type": "Code", "bbox": [72, 705, 540, 760],
         "html": "<pre>def f(x):\n    return x + 1</pre>"},
        {"id": "/page/0/Whatsit/0", "block_type": "Whatsit", "bbox": [72, 765, 540, 780],
         "html": "<p>something new in the next release</p>"}
      ]
    }
  ]
}`

func TestMarkerReadsAPage(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != Marker {
		t.Errorf("tool is %q, want %q", d.Tool, Marker)
	}
	if got, want := len(d.Pages), 1; got != want {
		t.Fatalf("got %d pages, want %d", got, want)
	}
	p, _ := d.Page(1)
	if got, want := p.Width, 612.0; got != want {
		t.Errorf("page width is %v, want %v", got, want)
	}
}

func TestMarkerReadsTheHeadingLevelFromTheTag(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	var h Block
	for _, b := range d.Blocks() {
		if b.Kind == Heading {
			h = b
		}
	}
	if got, want := h.Level, 2; got != want {
		t.Errorf("level is %d, want %d", got, want)
	}
	if got, want := h.Text, "3.1 Gated units"; got != want {
		t.Errorf("title is %q, want %q", got, want)
	}
}

func TestMarkerKeepsALessThanSignInProse(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	var prose string
	for _, b := range d.Blocks() {
		if b.Kind == Text {
			prose = b.Text
			break
		}
	}
	if got, want := prose, "The error $E_n$ falls with n, and a < b throughout."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkerUnwrapsADisplayEquation(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	var eq Block
	for _, b := range d.Blocks() {
		if b.Kind == Equation {
			eq = b
		}
	}
	if got, want := eq.Text, "E_n = 1 / n"; got != want {
		t.Errorf("got %q, want %q: the delimiters are added once, when it is written", got, want)
	}
}

func TestMarkerPairsAFigureGroup(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	figures := d.Figures()
	if got, want := len(figures), 1; got != want {
		t.Fatalf("got %d figures, want %d", got, want)
	}
	if got, want := figures[0].Image, "_page_0_Figure_1.jpeg"; got != want {
		t.Errorf("image is %q, want %q", got, want)
	}
	if got, want := figures[0].Caption, "Figure 1: the shape of the thing"; got != want {
		t.Errorf("caption is %q, want %q", got, want)
	}
}

func TestMarkerPairsATableGroupWhoseCaptionComesFirst(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	var table Block
	for _, b := range d.Blocks() {
		if b.Kind == Table {
			table = b
		}
	}
	if got, want := table.Caption, "Table 1: invented numbers"; got != want {
		t.Errorf("caption is %q, want %q", got, want)
	}
	if got, want := len(table.Rows), 2; got != want {
		t.Fatalf("got %d rows, want %d", got, want)
	}
	if got, want := table.Rows[0][0], "model"; got != want {
		t.Errorf("first cell is %q, want %q", got, want)
	}
}

func TestMarkerKeepsWhitespaceInAListing(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	var code Block
	for _, b := range d.Blocks() {
		if b.Kind == Code {
			code = b
		}
	}
	if got, want := code.Text, "def f(x):\n    return x + 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarkerKeepsAnUnknownBlockAsProse(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range d.Blocks() {
		if b.Kind == Text && strings.Contains(b.Text, "next release") {
			found = true
		}
	}
	if !found {
		t.Error("the unknown block was thrown away rather than kept as prose")
	}
	said := false
	for _, n := range d.Notes {
		if strings.Contains(n, "Whatsit") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing was said about it, notes were %v", d.Notes)
	}
}

func TestMarkerDropsTheRunningHead(t *testing.T) {
	d, err := Parse([]byte(markerFixture))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	if strings.Contains(p.Text(), "Journal of Invented Results") {
		t.Error("the running head reached the page text")
	}
}

func TestMarkerFallsBackToThePolygon(t *testing.T) {
	n := markerNode{Polygon: [][]float64{{80, 460}, {530, 460}, {530, 265}, {80, 265}}}
	want := Box{X0: 80, Y0: 265, X1: 530, Y1: 460}
	if got := markerBox(n); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMarkerRefusesAFileWithNoPages(t *testing.T) {
	if _, err := parseMarker([]byte(`{"block_type": "Document", "children": []}`)); err == nil {
		t.Fatal("a file with no pages was accepted")
	}
}
