package layout

import (
	"strings"
	"testing"
)

// A cut down middle.json with one of everything on two pages. The shape is
// MinerU's; the words are invented.
const mineruMiddleFixture = `{
  "pdf_info": [
    {
      "page_idx": 0,
      "page_size": [612, 792],
      "para_blocks": [
        {
          "type": "title",
          "bbox": [72, 90, 540, 110],
          "level": 1,
          "lines": [{"spans": [{"type": "text", "content": "1 Introduction"}]}]
        },
        {
          "type": "text",
          "bbox": [72, 120, 540, 200],
          "lines": [
            {"spans": [
              {"type": "text", "content": "The error"},
              {"type": "inline_equation", "content": "E_n"},
              {"type": "text", "content": "falls with n."}
            ]}
          ]
        },
        {
          "type": "interline_equation",
          "bbox": [72, 210, 540, 250],
          "lines": [{"spans": [{"type": "interline_equation", "content": "E_n = 1 / n"}]}]
        }
      ],
      "discarded_blocks": [
        {
          "type": "discarded",
          "bbox": [72, 60, 540, 70],
          "lines": [{"spans": [{"type": "text", "content": "Journal of Invented Results"}]}]
        }
      ]
    },
    {
      "page_idx": 1,
      "page_size": [612, 792],
      "para_blocks": [
        {
          "type": "image",
          "bbox": [72, 90, 540, 400],
          "blocks": [
            {
              "type": "image_body",
              "bbox": [80, 95, 530, 360],
              "lines": [{"spans": [{"type": "image", "image_path": "images/a1b2.jpg"}]}]
            },
            {
              "type": "image_caption",
              "bbox": [80, 365, 530, 385],
              "lines": [{"spans": [{"type": "text", "content": "Figure 1: the shape of the thing"}]}]
            }
          ]
        },
        {
          "type": "table",
          "bbox": [72, 420, 540, 600],
          "blocks": [
            {
              "type": "table_caption",
              "bbox": [80, 420, 530, 440],
              "lines": [{"spans": [{"type": "text", "content": "Table 1: invented numbers"}]}]
            },
            {
              "type": "table_body",
              "bbox": [80, 445, 530, 595],
              "lines": [{"spans": [{"type": "table", "html": "<table><tr><th>model</th><th>score</th></tr><tr><td>first</td><td>1.0</td></tr></table>", "image_path": "images/c3d4.jpg"}]}]
            }
          ]
        },
        {
          "type": "whatsit",
          "bbox": [72, 610, 540, 620],
          "lines": [{"spans": [{"type": "text", "content": "something new in the next release"}]}]
        }
      ]
    }
  ]
}`

func TestMinerUReadsAMiddleFile(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != MinerU {
		t.Errorf("tool is %q, want %q", d.Tool, MinerU)
	}
	if got, want := len(d.Pages), 2; got != want {
		t.Fatalf("got %d pages, want %d", got, want)
	}
	// MinerU counts pages from zero and the corpus counts from one.
	if got, want := d.First(), 1; got != want {
		t.Errorf("first page is %d, want %d", got, want)
	}
}

func TestMinerUWrapsInlineMathematics(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	var prose string
	for _, b := range p.Blocks {
		if b.Kind == Text {
			prose = b.Text
		}
	}
	if want := "The error $E_n$ falls with n."; prose != want {
		t.Errorf("got %q, want %q", prose, want)
	}
}

func TestMinerUKeepsTheRunningHeadAsFurniture(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	found := false
	for _, b := range p.Blocks {
		if b.Kind == Furniture {
			found = true
		}
	}
	if !found {
		t.Fatal("the discarded block was never read")
	}
	if strings.Contains(p.Text(), "Journal of Invented Results") {
		t.Error("the running head reached the page text")
	}
}

func TestMinerUPairsAFigureWithItsCaption(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	figures := d.Figures()
	if got, want := len(figures), 1; got != want {
		t.Fatalf("got %d figures, want %d", got, want)
	}
	f := figures[0]
	if got, want := f.Image, "images/a1b2.jpg"; got != want {
		t.Errorf("image is %q, want %q", got, want)
	}
	if got, want := f.Caption, "Figure 1: the shape of the thing"; got != want {
		t.Errorf("caption is %q, want %q", got, want)
	}
	if f.Box.Empty() {
		t.Error("the figure has no box, so nothing can be cropped from it")
	}
}

func TestMinerUReadsATableAsRows(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	var table Block
	for _, b := range d.Blocks() {
		if b.Kind == Table {
			table = b
		}
	}
	want := [][]string{{"model", "score"}, {"first", "1.0"}}
	if got := table.Rows; len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		for j := range want[i] {
			if table.Rows[i][j] != want[i][j] {
				t.Errorf("cell %d,%d is %q, want %q", i, j, table.Rows[i][j], want[i][j])
			}
		}
	}
	if got, want := table.Caption, "Table 1: invented numbers"; got != want {
		t.Errorf("caption is %q, want %q", got, want)
	}
}

func TestMinerUReportsABlockTypeItDoesNotKnow(t *testing.T) {
	d, err := Parse([]byte(mineruMiddleFixture))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range d.Notes {
		if strings.Contains(n, "whatsit") {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing was said about the unknown block type, notes were %v", d.Notes)
	}
}

const mineruListFixture = `[
  {"type": "text", "text": "1 Introduction", "text_level": 1, "page_idx": 0},
  {"type": "text", "text": "A paragraph of invented prose.", "page_idx": 0},
  {"type": "equation", "text": "E_n = 1 / n", "page_idx": 0},
  {"type": "image", "img_path": "images/a1b2.jpg", "img_caption": ["Figure 1: a shape"], "page_idx": 1},
  {"type": "table", "table_body": "<table><tr><td>a</td><td>b</td></tr></table>", "table_caption": ["Table 1: two letters"], "page_idx": 1}
]`

func TestMinerUReadsAContentList(t *testing.T) {
	d, err := Parse([]byte(mineruListFixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != MinerU {
		t.Errorf("tool is %q, want %q", d.Tool, MinerU)
	}
	if got, want := len(d.Pages), 2; got != want {
		t.Fatalf("got %d pages, want %d", got, want)
	}
	first, _ := d.Page(1)
	want := "# 1 Introduction\n\nA paragraph of invented prose.\n\n$$\nE_n = 1 / n\n$$\n"
	if got := first.Text(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAContentListSaysItHasNoBoxes(t *testing.T) {
	d, err := Parse([]byte(mineruListFixture))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range d.Notes {
		if strings.Contains(n, "no bounding boxes") {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing was said about the missing boxes, notes were %v", d.Notes)
	}
	for _, f := range d.Figures() {
		if !f.Box.Empty() {
			t.Error("a content list gave a figure a box it cannot have")
		}
	}
}

func TestABoxWrittenAsStringsIsStillABox(t *testing.T) {
	got := box([]any{"72", "90.5", "540", "110"})
	want := Box{X0: 72, Y0: 90.5, X1: 540, Y1: 110}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if b := box([]any{"not a number", 1.0, 2.0, 3.0}); !b.Empty() {
		t.Errorf("got %+v, want an empty box", b)
	}
	if b := box([]any{1.0, 2.0}); !b.Empty() {
		t.Errorf("got %+v, want an empty box", b)
	}
}

func TestMinerURefusesAFileWithNoPages(t *testing.T) {
	if _, err := parseMinerU([]byte(`{"pdf_info": []}`)); err == nil {
		t.Fatal("a file with no pages was accepted")
	}
}
