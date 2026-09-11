package layout

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Marker returns a tree: a document holding pages holding blocks, each with
// a type, a box and a fragment of HTML. The HTML is the content, and a block
// that holds other blocks says so with an <a href> to each child rather than
// by nesting the text, so the tree has to be walked and not just read.

type markerNode struct {
	ID        string       `json:"id"`
	BlockType string       `json:"block_type"`
	HTML      string       `json:"html"`
	Polygon   [][]float64  `json:"polygon"`
	BBox      []float64    `json:"bbox"`
	Children  []markerNode `json:"children"`
	// Images is the pictures this block holds, keyed by the id of the child
	// block they belong to, with a base64 payload marker writes separately.
	// Only the key is wanted here: the figure stage reads the file marker
	// wrote next to the JSON.
	Images map[string]string `json:"images"`
}

// The block types marker emits that mean something different to the corpus.
// The full list is longer and the rest are prose.
const (
	markerPage       = "Page"
	markerSection    = "SectionHeader"
	markerText       = "Text"
	markerList       = "ListGroup"
	markerListItem   = "ListItem"
	markerEquation   = "Equation"
	markerFigure     = "Figure"
	markerPicture    = "Picture"
	markerFigureGrp  = "FigureGroup"
	markerPictureGrp = "PictureGroup"
	markerTable      = "Table"
	markerTableGrp   = "TableGroup"
	markerCaption    = "Caption"
	markerCode       = "Code"
	markerFootnote   = "Footnote"
	markerHeader     = "PageHeader"
	markerFooter     = "PageFooter"
	markerToC        = "TableOfContents"
)

// parseMarker reads marker's JSON output.
func parseMarker(b []byte) (*Document, error) {
	var root markerNode
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("marker: %w", err)
	}
	pages := collectPages(root)
	if len(pages) == 0 {
		return nil, fmt.Errorf("marker: the file has no pages in it")
	}
	d := &Document{Tool: Marker}
	for i, p := range pages {
		// Marker numbers its pages in the block id, "/page/3/Page/0", and
		// the id is not always present. The position in the tree is, and the
		// pages are in order, so that is what the number comes from.
		number := i + 1
		w, h := markerSize(p)
		d.touch(number, w, h)
		for _, b := range d.markerBlocks(p) {
			d.add(number, w, h, b)
		}
	}
	d.sortPages()
	return d, nil
}

func collectPages(n markerNode) []markerNode {
	if n.BlockType == markerPage {
		return []markerNode{n}
	}
	var out []markerNode
	for _, c := range n.Children {
		out = append(out, collectPages(c)...)
	}
	return out
}

// markerBlocks walks one page. A group block holds its picture and its
// caption as children, so a group is turned into one of ours and the
// children are not visited again.
func (d *Document) markerBlocks(page markerNode) []Block {
	var out []Block
	for _, n := range page.Children {
		out = append(out, d.markerBlock(n)...)
	}
	return out
}

func (d *Document) markerBlock(n markerNode) []Block {
	switch n.BlockType {
	case markerHeader, markerFooter, markerToC:
		return []Block{{Kind: Furniture, Box: markerBox(n), Text: htmlText(n.HTML)}}

	case markerSection:
		title := htmlText(n.HTML)
		if title == "" {
			return nil
		}
		return []Block{{Kind: Heading, Box: markerBox(n), Text: title, Level: htmlLevel(n.HTML)}}

	case markerEquation:
		tex := htmlMath(n.HTML)
		if tex == "" {
			return nil
		}
		return []Block{{Kind: Equation, Box: markerBox(n), Text: tex}}

	case markerCode:
		body := codeText(n.HTML)
		if strings.TrimSpace(body) == "" {
			return nil
		}
		return []Block{{Kind: Code, Box: markerBox(n), Text: body}}

	case markerFigure, markerPicture:
		return []Block{d.markerFigure(n, "")}

	case markerFigureGrp, markerPictureGrp:
		var picture markerNode
		caption := ""
		for _, c := range n.Children {
			switch c.BlockType {
			case markerCaption, markerFootnote:
				caption = join(caption, htmlText(c.HTML))
			default:
				picture = c
			}
		}
		if picture.BlockType == "" {
			return nil
		}
		b := d.markerFigure(picture, caption)
		if b.Box.Empty() {
			b.Box = markerBox(n)
		}
		return []Block{b}

	case markerTable:
		return []Block{d.markerTable(n, "")}

	case markerTableGrp:
		var table markerNode
		caption := ""
		for _, c := range n.Children {
			switch c.BlockType {
			case markerCaption, markerFootnote:
				caption = join(caption, htmlText(c.HTML))
			default:
				table = c
			}
		}
		if table.BlockType == "" {
			return nil
		}
		b := d.markerTable(table, caption)
		if b.Box.Empty() {
			b.Box = markerBox(n)
		}
		return []Block{b}

	case markerList:
		// A list group holds its items and nothing else, and a list that
		// survived two columns and a PDF is a run of paragraphs. Reading it
		// as anything more structured invents structure.
		var out []Block
		for _, c := range n.Children {
			out = append(out, d.markerBlock(c)...)
		}
		return out

	case markerText, markerListItem, markerCaption, markerFootnote, "":
		text := htmlText(n.HTML)
		if text == "" {
			return nil
		}
		return []Block{{Kind: Text, Box: markerBox(n), Text: text}}
	}
	// Marker grows block types between releases and an unknown one is far
	// more likely to be prose than to be nothing.
	if text := htmlText(n.HTML); text != "" {
		d.note("marker block type %q is not one this reads, and was kept as prose", n.BlockType)
		return []Block{{Kind: Text, Box: markerBox(n), Text: text}}
	}
	d.note("marker block type %q is not one this reads", n.BlockType)
	return nil
}

func (d *Document) markerFigure(n markerNode, caption string) Block {
	b := Block{Kind: Figure, Box: markerBox(n), Caption: collapse(caption)}
	b.Image = htmlImage(n.HTML)
	if b.Image == "" {
		for name := range n.Images {
			b.Image = name
			break
		}
	}
	if b.Image == "" {
		d.note("a figure was found with no image file behind it")
	}
	if b.Caption == "" {
		d.note("a figure was found with no caption")
	}
	return b
}

func (d *Document) markerTable(n markerNode, caption string) Block {
	b := Block{Kind: Table, Box: markerBox(n), Caption: collapse(caption), Rows: tableRows(n.HTML)}
	if len(b.Rows) == 0 {
		d.note("a table came back as a picture and has no rows: %s", firstWords(b.Caption))
	}
	return b
}

// markerSize is the page in marker's coordinates, taken from the page
// block's own box.
func markerSize(p markerNode) (w, h float64) {
	b := markerBox(p)
	return b.X1 - b.X0, b.Y1 - b.Y0
}

// markerBox prefers the bbox and falls back to the polygon, because marker
// writes one or the other depending on the renderer.
func markerBox(n markerNode) Box {
	if len(n.BBox) == 4 {
		return Box{X0: n.BBox[0], Y0: n.BBox[1], X1: n.BBox[2], Y1: n.BBox[3]}
	}
	if len(n.Polygon) == 0 {
		return Box{}
	}
	b := Box{X0: n.Polygon[0][0], Y0: n.Polygon[0][1], X1: n.Polygon[0][0], Y1: n.Polygon[0][1]}
	for _, p := range n.Polygon {
		if len(p) != 2 {
			return Box{}
		}
		b.X0 = min(b.X0, p[0])
		b.Y0 = min(b.Y0, p[1])
		b.X1 = max(b.X1, p[0])
		b.Y1 = max(b.Y1, p[1])
	}
	return b
}
