package layout

import (
	"strings"
	"testing"
)

// Docling's shape, invented words. The body is the reading order and the
// lists are not, which is the whole reason this parser walks rather than
// reads straight through: here the caption is listed before the heading it
// comes after.
const doclingFixture = `{
  "schema_name": "DoclingDocument",
  "version": "1.0.0",
  "name": "invented",
  "pages": {
    "1": {"page_no": 1, "size": {"width": 612, "height": 792}},
    "2": {"page_no": 2, "size": {"width": 612, "height": 792}}
  },
  "body": {
    "self_ref": "#/body",
    "children": [
      {"$ref": "#/texts/0"},
      {"$ref": "#/texts/1"},
      {"$ref": "#/texts/2"},
      {"$ref": "#/texts/3"},
      {"$ref": "#/texts/4"},
      {"$ref": "#/groups/0"},
      {"$ref": "#/pictures/0"},
      {"$ref": "#/tables/0"}
    ]
  },
  "groups": [
    {"self_ref": "#/groups/0", "children": [{"$ref": "#/texts/6"}, {"$ref": "#/texts/7"}]}
  ],
  "texts": [
    {"self_ref": "#/texts/5", "label": "caption", "text": "Figure 1: the shape of the thing",
     "prov": [{"page_no": 2, "bbox": {"l": 80, "t": 465, "r": 530, "b": 495, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/0", "label": "page_header", "text": "Journal of Invented Results",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 40, "r": 540, "b": 55, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/1", "label": "section_header", "text": "3.1 Gated units", "level": 2,
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 90, "r": 540, "b": 110, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/2", "label": "text", "text": "A paragraph of invented prose.",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 120, "r": 540, "b": 200, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/3", "label": "formula", "text": "$$E_n = 1 / n$$",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 210, "r": 540, "b": 250, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/4", "label": "code", "text": "def f(x):\n    return x + 1", "code_language": "Python",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 260, "r": 540, "b": 320, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/6", "label": "list_item", "text": "the first point",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 330, "r": 540, "b": 345, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/7", "label": "list_item", "text": "the second point",
     "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 350, "r": 540, "b": 365, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/8", "label": "caption", "text": "Table 1: invented numbers",
     "prov": [{"page_no": 2, "bbox": {"l": 80, "t": 510, "r": 530, "b": 530, "coord_origin": "TOPLEFT"}}]},
    {"self_ref": "#/texts/9", "label": "whatsit", "text": "something new in the next release",
     "prov": [{"page_no": 2, "bbox": {"l": 80, "t": 700, "r": 530, "b": 715, "coord_origin": "TOPLEFT"}}]}
  ],
  "pictures": [
    {"self_ref": "#/pictures/0", "label": "picture",
     "captions": [{"$ref": "#/texts/5"}],
     "image": {"uri": "invented_artifacts/image_000001.png"},
     "prov": [{"page_no": 2, "bbox": {"l": 80, "t": 327, "r": 530, "b": 527, "coord_origin": "BOTTOMLEFT"}}]}
  ],
  "tables": [
    {"self_ref": "#/tables/0", "label": "table",
     "captions": [{"$ref": "#/texts/8"}],
     "prov": [{"page_no": 2, "bbox": {"l": 80, "t": 535, "r": 530, "b": 695, "coord_origin": "TOPLEFT"}}],
     "data": {"grid": [
       [{"text": "model", "col_span": 1, "column_header": true}, {"text": "score", "col_span": 1, "column_header": true}],
       [{"text": "first", "col_span": 1}, {"text": "1.0", "col_span": 1}]
     ]}}
  ]
}`

func TestDoclingReadsTheBodyInReadingOrder(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != Docling {
		t.Errorf("tool is %q, want %q", d.Tool, Docling)
	}
	p, ok := d.Page(1)
	if !ok {
		t.Fatal("page one went missing")
	}
	want := []Kind{Furniture, Heading, Text, Equation, Code, Text, Text}
	if len(p.Blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d", len(p.Blocks), len(want))
	}
	for i, k := range want {
		if p.Blocks[i].Kind != k {
			t.Errorf("block %d is %q, want %q", i, p.Blocks[i].Kind, k)
		}
	}
}

func TestDoclingReadsTheHeadingLevel(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	if got, want := p.Blocks[1].Level, 2; got != want {
		t.Errorf("level is %d, want %d", got, want)
	}
}

func TestDoclingUnwrapsAFormula(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
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
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDoclingKeepsWhitespaceInAListing(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
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
	if got, want := code.Lang, "python"; got != want {
		t.Errorf("language is %q, want %q", got, want)
	}
}

func TestDoclingDoesNotRepeatACaptionAsAParagraph(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range d.Blocks() {
		if b.Kind == Text && strings.HasPrefix(b.Text, "Figure 1:") {
			t.Error("the figure caption was written twice, once as a paragraph")
		}
	}
	figures := d.Figures()
	if got, want := len(figures), 1; got != want {
		t.Fatalf("got %d figures, want %d", got, want)
	}
	if got, want := figures[0].Caption, "Figure 1: the shape of the thing"; got != want {
		t.Errorf("caption is %q, want %q", got, want)
	}
}

func TestDoclingTurnsABottomLeftBoxOver(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
	if err != nil {
		t.Fatal(err)
	}
	// The fixture has the picture at t 327 and b 527 from the bottom of a
	// 792 point page, which is 265 to 465 from the top.
	want := Box{X0: 80, Y0: 265, X1: 530, Y1: 465}
	if got := d.Figures()[0].Box; got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDoclingReadsATableGrid(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
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
	if got, want := table.Rows[1][1], "1.0"; got != want {
		t.Errorf("cell 1,1 is %q, want %q", got, want)
	}
}

func TestDoclingIgnoresWhatTheBodyDoesNotPointAt(t *testing.T) {
	d, err := Parse([]byte(doclingFixture))
	if err != nil {
		t.Fatal(err)
	}
	// texts/9 is in the list and not in the body, which is how Docling
	// writes something it decided not to keep.
	for _, b := range d.Blocks() {
		if strings.Contains(b.Text, "next release") {
			t.Error("a text nothing pointed at was read anyway")
		}
	}
}

func TestDoclingSurvivesAGroupThatPointsAtItself(t *testing.T) {
	const loop = `{
	  "schema_name": "DoclingDocument",
	  "body": {"self_ref": "#/body", "children": [{"$ref": "#/groups/0"}]},
	  "groups": [{"self_ref": "#/groups/0", "children": [{"$ref": "#/groups/0"}, {"$ref": "#/texts/0"}]}],
	  "texts": [{"self_ref": "#/texts/0", "label": "text", "text": "reached at last",
	             "prov": [{"page_no": 1, "bbox": {"l": 0, "t": 0, "r": 1, "b": 1}}]}]
	}`
	d, err := Parse([]byte(loop))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(d.Blocks()), 1; got != want {
		t.Fatalf("got %d blocks, want %d", got, want)
	}
}

func TestDoclingRefusesAnEmptyFile(t *testing.T) {
	if _, err := parseDocling([]byte(`{"schema_name": "DoclingDocument"}`)); err == nil {
		t.Fatal("a file with nothing in it was accepted")
	}
}
