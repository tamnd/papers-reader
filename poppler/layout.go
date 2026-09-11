package poppler

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// A Box is a rectangle on a page, in PDF points, with y growing downward.
//
// Downward is pdftotext's convention and not the PDF's, and it is kept
// because everything in this toolchain that reads a Box is asking a reading
// order question: is this line above that one. Flipping it here would mean
// every caller writing the comparison the other way round and one of them
// eventually getting it wrong.
type Box struct {
	XMin, YMin, XMax, YMax float64
}

// Width and Height of a box.
func (b Box) Width() float64  { return b.XMax - b.XMin }
func (b Box) Height() float64 { return b.YMax - b.YMin }

// XMid is the horizontal centre, which is what columns are clustered on. A
// line's left edge is the wrong thing to cluster: an indented first line of a
// paragraph and a flush one belong to the same column and start in different
// places, and their centres do not.
func (b Box) XMid() float64 { return (b.XMin + b.XMax) / 2 }

// YMid is the vertical centre.
func (b Box) YMid() float64 { return (b.YMin + b.YMax) / 2 }

// A Word is one word and where it sits.
type Word struct {
	Box
	Text string
}

// A TextLine is a row of words as pdftotext grouped them.
type TextLine struct {
	Box
	Words []Word
}

// Text is the line as a string, with single spaces between the words.
//
// The gaps between words are not preserved, and that is deliberate: in a
// justified column they are whatever the typesetter needed to make the line
// come out even, and carrying them into the Markdown would be carrying the
// journal's typesetting rather than the author's text. A code listing is the
// exception and it does not come through here.
func (l TextLine) Text() string {
	parts := make([]string, len(l.Words))
	for i, w := range l.Words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

// A Block is what pdftotext thinks is a paragraph.
//
// It is usually right and it is not trusted for anything that matters. A
// block that spans both columns of a two column page is the common failure
// and it is why the column clustering in package extract works from the
// lines rather than from these.
type Block struct {
	Box
	Lines []TextLine
}

// A Layout is one page with its geometry.
type Layout struct {
	Number        int
	Width, Height float64
	Blocks        []Block
}

// Lines is every line of the page in the order pdftotext emitted them, which
// is flow order and is not reading order on a two column page.
func (p Layout) Lines() []TextLine {
	var out []TextLine
	for _, b := range p.Blocks {
		out = append(out, b.Lines...)
	}
	return out
}

// Layouts reads the geometry of a range of pages.
//
// This is the first thing the native path does. Plain pdftotext -layout
// interleaves the columns of a two column paper into nonsense, and almost
// every paper from 1970 onward is two column, so the toolchain asks for the
// boxes and works out the reading order itself.
func Layouts(ctx context.Context, path string, first, last int) ([]Layout, error) {
	out, err := run(ctx, "pdftotext", "reads where the words are on the page",
		"-bbox-layout", "-f", strconv.Itoa(first), "-l", strconv.Itoa(last), path, "-")
	if err != nil {
		return nil, err
	}
	pages, err := ParseLayout(out)
	if err != nil {
		return nil, err
	}
	// pdftotext numbers the pages it printed from one, so the range has to be
	// put back. A caller that asked for page 12 and got a page numbered 1
	// would write it to the wrong file.
	for i := range pages {
		pages[i].Number = first + i
	}
	return pages, nil
}

// ParseLayout reads the XHTML that pdftotext -bbox-layout prints.
//
// The document has a DOCTYPE and an XHTML namespace and is otherwise well
// formed, so the stdlib decoder reads it with Strict off. Off rather than on
// because the only thing strictness would buy is a failure on a file poppler
// itself wrote, and a page of mathematics that ends up with a stray entity in
// a word is a page the acceptance rules will catch anyway.
func ParseLayout(out []byte) ([]Layout, error) {
	d := xml.NewDecoder(bytes.NewReader(scrub(out)))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var pages []Layout
	var page *Layout
	var block *Block
	var line *TextLine
	var word *Word
	var text strings.Builder

	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("reading the page geometry: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "page":
				pages = append(pages, Layout{
					Number: len(pages) + 1,
					Width:  attrFloat(t, "width"),
					Height: attrFloat(t, "height"),
				})
				page = &pages[len(pages)-1]
			case "block":
				if page == nil {
					continue
				}
				page.Blocks = append(page.Blocks, Block{Box: box(t)})
				block = &page.Blocks[len(page.Blocks)-1]
			case "line":
				if block == nil {
					continue
				}
				block.Lines = append(block.Lines, TextLine{Box: box(t)})
				line = &block.Lines[len(block.Lines)-1]
			case "word":
				if line == nil {
					continue
				}
				word = &Word{Box: box(t)}
				text.Reset()
			}
		case xml.CharData:
			if word != nil {
				text.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "page":
				page, block, line = nil, nil, nil
			case "block":
				block, line = nil, nil
			case "line":
				line = nil
			case "word":
				if word != nil && line != nil {
					// A word of nothing but space is what an empty <word/>
					// comes back as, and it would turn into a double space in
					// the line. Dropped here rather than in every caller.
					if s := strings.TrimSpace(text.String()); s != "" {
						word.Text = s
						line.Words = append(line.Words, *word)
					}
				}
				word = nil
			}
		}
	}
	return pages, nil
}

func box(t xml.StartElement) Box {
	return Box{
		XMin: attrFloat(t, "xMin"),
		YMin: attrFloat(t, "yMin"),
		XMax: attrFloat(t, "xMax"),
		YMax: attrFloat(t, "yMax"),
	}
}

// attrFloat reads one attribute, case insensitively because the decoder
// lowercases nothing and poppler writes xMin while a hand written fixture is
// as likely to write xmin.
func attrFloat(t xml.StartElement, name string) float64 {
	for _, a := range t.Attr {
		if strings.EqualFold(a.Name.Local, name) {
			v, _ := strconv.ParseFloat(a.Value, 64)
			return v
		}
	}
	return 0
}

// scrub takes out the characters XML does not allow, which pdftotext prints
// anyway.
//
// A PDF whose font maps a glyph to nothing leaves the control character in
// the text layer, and poppler passes it through into the XHTML it writes. The
// decoder is then right to refuse the document, and refusing it would lose
// the whole paper over a character that is not on the page: Razborov 1997 has
// a U+0003 on page 3 and thirteen pages of readable mathematics around it.
//
// They are dropped rather than replaced with a space. A control character in
// a text layer sits inside a word, and a space there would cut the word in
// two, which is a worse reading of the page than the one with the character
// gone.
func scrub(b []byte) []byte {
	clean := true
	for _, c := range b {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			clean = false
			break
		}
	}
	if clean {
		return b
	}
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			continue
		}
		out = append(out, c)
	}
	return out
}
