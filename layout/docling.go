package layout

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Docling writes a DoclingDocument: parallel lists of texts, tables and
// pictures, and a body whose children are references into those lists in
// reading order. So the lists are indexed first and then the body is walked,
// because the lists on their own are in no particular order and reading them
// straight through interleaves a caption with the paragraph after it.

type doclingDoc struct {
	Body     doclingGroup           `json:"body"`
	Texts    []doclingText          `json:"texts"`
	Tables   []doclingTable         `json:"tables"`
	Pictures []doclingPicture       `json:"pictures"`
	Groups   []doclingGroup         `json:"groups"`
	Pages    map[string]doclingPage `json:"pages"`
}

type doclingGroup struct {
	SelfRef  string       `json:"self_ref"`
	Children []doclingRef `json:"children"`
}

type doclingRef struct {
	Ref string `json:"$ref"`
}

type doclingPage struct {
	Size struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"size"`
	PageNo int `json:"page_no"`
}

type doclingProv struct {
	PageNo int `json:"page_no"`
	BBox   struct {
		L      float64 `json:"l"`
		T      float64 `json:"t"`
		R      float64 `json:"r"`
		B      float64 `json:"b"`
		Origin string  `json:"coord_origin"`
	} `json:"bbox"`
}

type doclingText struct {
	SelfRef  string        `json:"self_ref"`
	Label    string        `json:"label"`
	Text     string        `json:"text"`
	Orig     string        `json:"orig"`
	Level    int           `json:"level"`
	Prov     []doclingProv `json:"prov"`
	Language string        `json:"code_language"`
}

type doclingTable struct {
	SelfRef  string        `json:"self_ref"`
	Label    string        `json:"label"`
	Prov     []doclingProv `json:"prov"`
	Captions []doclingRef  `json:"captions"`
	Data     struct {
		Grid [][]struct {
			Text     string `json:"text"`
			RowSpan  int    `json:"row_span"`
			ColSpan  int    `json:"col_span"`
			IsHeader bool   `json:"column_header"`
		} `json:"grid"`
	} `json:"data"`
}

type doclingPicture struct {
	SelfRef  string        `json:"self_ref"`
	Label    string        `json:"label"`
	Prov     []doclingProv `json:"prov"`
	Captions []doclingRef  `json:"captions"`
	Image    struct {
		URI string `json:"uri"`
	} `json:"image"`
}

// Docling's labels. The list is longer; the rest are prose.
const (
	doclingTitle    = "title"
	doclingSection  = "section_header"
	doclingText2    = "text"
	doclingPara     = "paragraph"
	doclingList     = "list_item"
	doclingFormula  = "formula"
	doclingCode     = "code"
	doclingCaption  = "caption"
	doclingFooter   = "page_footer"
	doclingHeader   = "page_header"
	doclingFootnote = "footnote"
)

func parseDocling(b []byte) (*Document, error) {
	var doc doclingDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("docling: %w", err)
	}
	if len(doc.Texts) == 0 && len(doc.Tables) == 0 && len(doc.Pictures) == 0 {
		return nil, fmt.Errorf("docling: the file has nothing in it")
	}
	d := &Document{Tool: Docling}

	texts := map[string]doclingText{}
	for _, t := range doc.Texts {
		texts[t.SelfRef] = t
	}
	tables := map[string]doclingTable{}
	for _, t := range doc.Tables {
		tables[t.SelfRef] = t
	}
	pictures := map[string]doclingPicture{}
	for _, p := range doc.Pictures {
		pictures[p.SelfRef] = p
	}
	groups := map[string]doclingGroup{}
	for _, g := range doc.Groups {
		groups[g.SelfRef] = g
	}
	size := map[int][2]float64{}
	for _, p := range doc.Pages {
		size[p.PageNo] = [2]float64{p.Size.Width, p.Size.Height}
	}
	for no := range size {
		d.touch(no, size[no][0], size[no][1])
	}

	// A caption reached through a picture's or a table's captions list is
	// not also a paragraph of its own, and Docling lists it in both places.
	claimed := map[string]bool{}
	for _, p := range doc.Pictures {
		for _, c := range p.Captions {
			claimed[c.Ref] = true
		}
	}
	for _, t := range doc.Tables {
		for _, c := range t.Captions {
			claimed[c.Ref] = true
		}
	}

	w := &doclingWalk{d: d, texts: texts, tables: tables,
		pictures: pictures, groups: groups, size: size, claimed: claimed}
	w.children(doc.Body.Children, 0)
	d.sortPages()
	return d, nil
}

type doclingWalk struct {
	d        *Document
	texts    map[string]doclingText
	tables   map[string]doclingTable
	pictures map[string]doclingPicture
	groups   map[string]doclingGroup
	size     map[int][2]float64
	claimed  map[string]bool
	seen     map[string]bool
}

// children walks a run of references in reading order. The depth guard is
// against a document whose groups refer to each other, which a converted
// file with a damaged structure can produce and which would otherwise be an
// infinite walk.
func (w *doclingWalk) children(refs []doclingRef, depth int) {
	if depth > 16 {
		w.d.note("the docling body nests deeper than sixteen levels and the rest was not read")
		return
	}
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	for _, r := range refs {
		if w.seen[r.Ref] {
			continue
		}
		w.seen[r.Ref] = true
		switch {
		case w.claimed[r.Ref]:
			continue
		case w.text(r.Ref):
		case w.table(r.Ref):
		case w.picture(r.Ref):
		default:
			if g, ok := w.groups[r.Ref]; ok {
				w.children(g.Children, depth+1)
				continue
			}
			w.d.note("the docling body refers to %s, which is not in the document", r.Ref)
		}
	}
}

func (w *doclingWalk) text(ref string) bool {
	t, ok := w.texts[ref]
	if !ok {
		return false
	}
	page, box := w.where(t.Prov)
	body := t.Text
	if body == "" {
		body = t.Orig
	}
	switch t.Label {
	case doclingHeader, doclingFooter:
		w.add(page, Block{Kind: Furniture, Box: box, Text: collapse(body)})
	case doclingTitle:
		w.add(page, Block{Kind: Heading, Box: box, Text: collapse(body), Level: 1})
	case doclingSection:
		level := t.Level
		if level < 1 {
			level = 1
		}
		w.add(page, Block{Kind: Heading, Box: box, Text: collapse(body), Level: level})
	case doclingFormula:
		if tex := unwrapMath(body); tex != "" {
			w.add(page, Block{Kind: Equation, Box: box, Text: tex})
		}
	case doclingCode:
		lang := strings.ToLower(t.Language)
		w.add(page, Block{Kind: Code, Box: box, Text: strings.Trim(body, "\n"), Lang: lang})
	case doclingText2, doclingPara, doclingList, doclingCaption, doclingFootnote, "":
		if s := collapse(body); s != "" {
			w.add(page, Block{Kind: Text, Box: box, Text: s})
		}
	default:
		if s := collapse(body); s != "" {
			w.d.note("docling label %q is not one this reads, and was kept as prose", t.Label)
			w.add(page, Block{Kind: Text, Box: box, Text: s})
		}
	}
	return true
}

func (w *doclingWalk) table(ref string) bool {
	t, ok := w.tables[ref]
	if !ok {
		return false
	}
	page, box := w.where(t.Prov)
	b := Block{Kind: Table, Box: box, Caption: w.caption(t.Captions)}
	for _, row := range t.Data.Grid {
		var cells []string
		for _, c := range row {
			n := c.ColSpan
			if n < 1 {
				n = 1
			}
			for i := 0; i < n; i++ {
				cells = append(cells, collapse(c.Text))
			}
		}
		if len(cells) > 0 {
			b.Rows = append(b.Rows, cells)
		}
	}
	if len(b.Rows) == 0 {
		w.d.note("a table came back as a picture and has no rows: %s", firstWords(b.Caption))
	}
	w.add(page, b)
	return true
}

func (w *doclingWalk) picture(ref string) bool {
	p, ok := w.pictures[ref]
	if !ok {
		return false
	}
	page, box := w.where(p.Prov)
	b := Block{Kind: Figure, Box: box, Caption: w.caption(p.Captions), Image: p.Image.URI}
	if b.Image == "" {
		w.d.note("a figure was found with no image file behind it")
	}
	if b.Caption == "" {
		w.d.note("a figure was found with no caption")
	}
	w.add(page, b)
	return true
}

func (w *doclingWalk) caption(refs []doclingRef) string {
	out := ""
	for _, r := range refs {
		if t, ok := w.texts[r.Ref]; ok {
			body := t.Text
			if body == "" {
				body = t.Orig
			}
			out = join(out, body)
		}
	}
	return collapse(out)
}

// where is the page a block is on and the box it occupies.
//
// Docling reports a bottom-left origin as often as a top-left one, and the
// figure stage crops in the top-left convention the rest of the toolchain
// uses, so a bottom-left box is turned over here using the page height. A
// box left in the wrong convention crops the mirror image of the figure,
// which is a mistake nobody notices until a reader does.
func (w *doclingWalk) where(prov []doclingProv) (int, Box) {
	if len(prov) == 0 {
		return 1, Box{}
	}
	p := prov[0]
	page := p.PageNo
	if page < 1 {
		page = 1
	}
	b := Box{X0: p.BBox.L, Y0: p.BBox.T, X1: p.BBox.R, Y1: p.BBox.B}
	if strings.EqualFold(p.BBox.Origin, "BOTTOMLEFT") {
		if h := w.size[page][1]; h > 0 {
			b.Y0, b.Y1 = h-p.BBox.T, h-p.BBox.B
		}
	}
	if b.Y1 < b.Y0 {
		b.Y0, b.Y1 = b.Y1, b.Y0
	}
	return page, b
}

func (w *doclingWalk) add(page int, b Block) {
	s := w.size[page]
	w.d.add(page, s[0], s[1], b)
}
