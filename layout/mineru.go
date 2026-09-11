package layout

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MinerU writes two files worth reading. `<name>_middle.json` is the whole
// layout with a bounding box on every block, and `<name>_content_list.json`
// is the same content flattened with the boxes thrown away. The first is
// what the figure stage needs, so it is what is read where there is a
// choice, and the second is read as well because a MinerU run that fell back
// to its simpler pipeline produces only that.

type mineruMiddle struct {
	Info []mineruPage `json:"pdf_info"`
}

type mineruPage struct {
	Blocks []mineruBlock `json:"para_blocks"`
	// Discarded is the page furniture MinerU threw out: running heads,
	// folios, the footnote rule. Read so it can be dropped knowingly rather
	// than never seen.
	Discarded []mineruBlock `json:"discarded_blocks"`
	Index     int           `json:"page_idx"`
	Size      []float64     `json:"page_size"`
}

type mineruBlock struct {
	Type string `json:"type"`
	Box  []any  `json:"bbox"`
	// Blocks is how MinerU nests a figure: an image block holds an
	// image_body and an image_caption, and a table block holds a table_body
	// and a table_caption.
	Blocks []mineruBlock `json:"blocks"`
	Lines  []mineruLine  `json:"lines"`
	Level  int           `json:"level"`
}

type mineruLine struct {
	Spans []mineruSpan `json:"spans"`
}

type mineruSpan struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	HTML      string `json:"html"`
	ImagePath string `json:"image_path"`
}

// The block types MinerU emits. The ones not listed fall through to a note.
const (
	mineruText      = "text"
	mineruTitle     = "title"
	mineruEquation  = "interline_equation"
	mineruImage     = "image"
	mineruTable     = "table"
	mineruIndex     = "index"
	mineruList      = "list"
	mineruDiscarded = "discarded"
)

// parseMinerU reads a middle.json, or a content_list.json if that is what it
// was handed. The two are told apart by their shape rather than by their name
// because the names are prefixed with the PDF's own and a file that has been
// moved or renamed still has to read.
func parseMinerU(b []byte) (*Document, error) {
	if isArray(b) {
		return parseMinerUList(b)
	}
	var m mineruMiddle
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("mineru: %w", err)
	}
	if len(m.Info) == 0 {
		return nil, fmt.Errorf("mineru: the file has no pages in it")
	}
	d := &Document{Tool: MinerU}
	for _, p := range m.Info {
		w, h := 0.0, 0.0
		if len(p.Size) == 2 {
			w, h = p.Size[0], p.Size[1]
		}
		page := p.Index + 1
		// A page with nothing on it still gets a page. A blank verso is a
		// real answer, and a document that left it out would renumber every
		// page after it.
		d.touch(page, w, h)
		for _, b := range p.Blocks {
			for _, out := range d.mineruBlock(b) {
				d.add(page, w, h, out)
			}
		}
		for _, b := range p.Discarded {
			d.add(page, w, h, Block{Kind: Furniture, Box: box(b.Box), Text: mineruText2(b)})
		}
	}
	d.sortPages()
	return d, nil
}

// mineruBlock turns one of MinerU's blocks into ours. It returns a slice
// because a figure whose caption MinerU could not pair comes back as two.
func (d *Document) mineruBlock(b mineruBlock) []Block {
	switch b.Type {
	case mineruText, mineruList, mineruIndex:
		text := mineruText2(b)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []Block{{Kind: Text, Box: box(b.Box), Text: text}}

	case mineruTitle:
		text := mineruText2(b)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		level := b.Level
		if level == 0 {
			level = 1
		}
		return []Block{{Kind: Heading, Box: box(b.Box), Text: text, Level: level}}

	case mineruEquation:
		tex := mineruText2(b)
		if strings.TrimSpace(tex) == "" {
			return nil
		}
		return []Block{{Kind: Equation, Box: box(b.Box), Text: tex}}

	case mineruImage:
		out := Block{Kind: Figure, Box: box(b.Box)}
		for _, sub := range b.Blocks {
			switch sub.Type {
			case "image_body":
				out.Image = mineruImagePath(sub)
				if out.Box.Empty() {
					out.Box = box(sub.Box)
				}
			case "image_caption", "image_footnote":
				out.Caption = join(out.Caption, mineruText2(sub))
			}
		}
		if out.Image == "" {
			// MinerU found a region and wrote no file for it, which happens
			// when the picture is a vector drawing it could not rasterise.
			// Nothing to commit and nothing to say on the page.
			d.note("a figure was found with no image file behind it")
			return nil
		}
		if out.Caption == "" {
			// An uncaptioned large region is a decorative rule, a logo or a
			// signature block far more often than it is a figure, so it is
			// reported and the figure stage decides.
			d.note("a figure was found with no caption")
		}
		return []Block{out}

	case mineruTable:
		out := Block{Kind: Table, Box: box(b.Box)}
		for _, sub := range b.Blocks {
			switch sub.Type {
			case "table_body":
				html, image := mineruTableBody(sub)
				out.Rows = tableRows(html)
				out.Image = image
				if out.Box.Empty() {
					out.Box = box(sub.Box)
				}
			case "table_caption", "table_footnote":
				out.Caption = join(out.Caption, mineruText2(sub))
			}
		}
		if len(out.Rows) == 0 {
			// A table that came back as a picture. Not committed as one: a
			// table as an image cannot be read aloud, searched or
			// translated, and a corpus that commits one has quietly lost the
			// numbers in it.
			d.note("a table came back as a picture and has no rows: %s", firstWords(out.Caption))
		}
		return []Block{out}

	case mineruDiscarded:
		return []Block{{Kind: Furniture, Box: box(b.Box), Text: mineruText2(b)}}
	}
	d.note("mineru block type %q is not one this reads", b.Type)
	return nil
}

// mineruText2 is a block's text with the inline mathematics wrapped.
//
// MinerU gives an inline equation as a span of its own with the TeX bare, so
// this is where $..$ goes round it. A span already wrapped is left alone,
// because MinerU's own Markdown writer wraps some of them and a double wrap
// is an empty math span followed by prose read as TeX.
func mineruText2(b mineruBlock) string {
	var parts []string
	for _, sub := range b.Blocks {
		if s := mineruText2(sub); s != "" {
			parts = append(parts, s)
		}
	}
	for _, line := range b.Lines {
		var words []string
		for _, s := range line.Spans {
			switch s.Type {
			case "inline_equation":
				if tex := strings.TrimSpace(s.Content); tex != "" {
					words = append(words, wrapMath(tex))
				}
			case "interline_equation":
				if tex := strings.TrimSpace(s.Content); tex != "" {
					words = append(words, tex)
				}
			default:
				if c := strings.TrimSpace(s.Content); c != "" {
					words = append(words, c)
				}
			}
		}
		if len(words) > 0 {
			parts = append(parts, strings.Join(words, " "))
		}
	}
	return strings.Join(parts, " ")
}

func mineruImagePath(b mineruBlock) string {
	for _, line := range b.Lines {
		for _, s := range line.Spans {
			if s.ImagePath != "" {
				return s.ImagePath
			}
		}
	}
	return ""
}

func mineruTableBody(b mineruBlock) (html, image string) {
	for _, line := range b.Lines {
		for _, s := range line.Spans {
			if s.HTML != "" && html == "" {
				html = s.HTML
			}
			if s.ImagePath != "" && image == "" {
				image = s.ImagePath
			}
		}
	}
	return html, image
}

// The flattened file, for a run that produced no middle.json.

type mineruItem struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Level int    `json:"text_level"`
	Page  int    `json:"page_idx"`

	ImagePath string `json:"img_path"`
	// MinerU has spelled the caption field three ways across its versions
	// and all three turn up in files people have on disk.
	ImageCaption  []string `json:"image_caption"`
	ImgCaption    []string `json:"img_caption"`
	TableCaption  []string `json:"table_caption"`
	TableBody     string   `json:"table_body"`
	TableFootnote []string `json:"table_footnote"`
}

// parseMinerUList reads a content_list.json, which has the content and none
// of the geometry. A paper read this way extracts and cannot have its
// figures cropped, and the figure stage says so rather than cropping the
// wrong rectangle.
func parseMinerUList(b []byte) (*Document, error) {
	var items []mineruItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, fmt.Errorf("mineru: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("mineru: the file has nothing in it")
	}
	d := &Document{Tool: MinerU}
	d.note("this is a mineru content list, which has no bounding boxes, so no figure can be cropped from it")
	for _, it := range items {
		page := it.Page + 1
		switch it.Type {
		case "text":
			text := collapse(it.Text)
			if text == "" {
				continue
			}
			if it.Level > 0 {
				d.add(page, 0, 0, Block{Kind: Heading, Text: text, Level: it.Level})
				continue
			}
			d.add(page, 0, 0, Block{Kind: Text, Text: text})
		case "equation":
			if tex := strings.TrimSpace(it.Text); tex != "" {
				d.add(page, 0, 0, Block{Kind: Equation, Text: tex})
			}
		case "image":
			caption := join(strings.Join(it.ImageCaption, " "), strings.Join(it.ImgCaption, " "))
			if it.ImagePath == "" {
				d.note("a figure was found with no image file behind it")
				continue
			}
			if caption == "" {
				d.note("a figure was found with no caption")
			}
			d.add(page, 0, 0, Block{Kind: Figure, Image: it.ImagePath, Caption: collapse(caption)})
		case "table":
			rows := tableRows(it.TableBody)
			caption := collapse(strings.Join(it.TableCaption, " "))
			if len(rows) == 0 {
				d.note("a table came back as a picture and has no rows: %s", firstWords(caption))
			}
			d.add(page, 0, 0, Block{Kind: Table, Rows: rows, Caption: caption, Image: it.ImagePath})
		default:
			d.note("mineru item type %q is not one this reads", it.Type)
		}
	}
	d.sortPages()
	return d, nil
}

// box reads MinerU's four numbers. They arrive as JSON numbers but a couple
// of versions write them as strings, so the slice is []any and this is where
// that is dealt with once.
func box(v []any) Box {
	if len(v) != 4 {
		return Box{}
	}
	n := make([]float64, 4)
	for i, x := range v {
		switch f := x.(type) {
		case float64:
			n[i] = f
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
			if err != nil {
				return Box{}
			}
			n[i] = parsed
		default:
			return Box{}
		}
	}
	return Box{X0: n[0], Y0: n[1], X1: n[2], Y1: n[3]}
}

func wrapMath(tex string) string {
	if strings.HasPrefix(tex, "$") && strings.HasSuffix(tex, "$") {
		return tex
	}
	return "$" + tex + "$"
}

func join(a, b string) string {
	switch {
	case strings.TrimSpace(a) == "":
		return strings.TrimSpace(b)
	case strings.TrimSpace(b) == "":
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(a) + " " + strings.TrimSpace(b)
}

// firstWords is enough of a caption to find the thing in the paper, for a
// note that has to be short enough to read in a list of forty.
func firstWords(s string) string {
	s = collapse(s)
	if s == "" {
		return "no caption"
	}
	r := []rune(s)
	if len(r) > 50 {
		return string(r[:50])
	}
	return s
}
