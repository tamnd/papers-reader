package layout

import (
	"fmt"
	"strings"

	"github.com/tamnd/papers-reader/grid"
)

// Text is one page written the way the page store holds it: blocks one to a
// paragraph, blank lines between them.
//
// The output has to be the same shape the native path writes, because
// assemble and split read both and neither of them should be able to tell
// which path a paper took. That is also why a heading comes out as an ATX
// heading rather than as a bare line: the native path has to guess at its
// headings from the numbering and the typography, and a layout model does
// not have to guess, so it says so and the splitter believes it.
func (p Page) Text() string {
	var parts []string
	for _, b := range p.Blocks {
		if s := b.Markdown(); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// Markdown is one block.
func (b Block) Markdown() string {
	switch b.Kind {
	case Furniture:
		// Dropped here, where something still knows it is a running head.
		// Downstream all that is left is a short line at the top of a page,
		// which is also what a heading looks like.
		return ""
	case Heading:
		return b.heading()
	case Equation:
		return b.equation()
	case Code:
		return b.code()
	case Table:
		return b.table()
	case Figure:
		return b.figure()
	}
	return collapse(b.Text)
}

// heading writes a heading as ATX, with the number the paper printed left in
// the title where the paper printed it.
//
// The number stays because the splitter files a section under the paper's
// own number rather than under its position, so a paper whose section 3 is
// the fourth heading keeps 3, and a reader following a cross reference from
// another paper finds it.
func (b Block) heading() string {
	title := collapse(b.Text)
	if title == "" {
		return ""
	}
	level := b.Level
	switch {
	case level < 1:
		level = 1
	case level > 6:
		// Markdown stops at six and a layout model occasionally reports an
		// eighth level heading on a page of nested numbering. Six reads the
		// same and is valid.
		level = 6
	}
	return strings.Repeat("#", level) + " " + title
}

// equation writes a display equation as $$..$$ on lines of its own.
//
// The TeX is kept as the tool gave it, on one line, with any $$ or \[ the
// tool wrapped it in taken off first. Reflowing TeX is how a matrix becomes
// a syntax error, and validating it is acceptance rule A4's job and not
// this function's.
func (b Block) equation() string {
	tex := strings.TrimSpace(b.Text)
	tex = strings.TrimSpace(strings.Trim(tex, "$"))
	tex = strings.TrimSpace(strings.TrimPrefix(tex, `\[`))
	tex = strings.TrimSpace(strings.TrimSuffix(tex, `\]`))
	if tex == "" {
		return ""
	}
	return "$$\n" + tex + "\n$$"
}

// code writes a listing in a fence, with every space it arrived with.
//
// A code block is never reflowed, never wrapped and never dedented. Papers
// reference "line 7", and a line number column the journal printed stays
// inside the fence for the same reason.
func (b Block) code() string { return grid.Fence(b.Lang, b.Text) }

// table writes a table as GitHub Flavored Markdown, with the caption above
// it as an italic paragraph.
//
// Above, because that is where a table's caption is printed and because the
// caption is the part that gets translated and a translator reading the
// caption after the table has already read the table.
func (b Block) table() string {
	var parts []string
	if c := collapse(b.Caption); c != "" {
		parts = append(parts, "*"+c+"*")
	}
	if len(b.Rows) == 0 {
		// A table the tool could only give as a picture. Not committed as
		// one: a table as an image cannot be read by a screen reader,
		// searched, or translated. The note is the report.
		return strings.Join(parts, "\n\n")
	}
	// A cell arrives from the tool with whatever line breaks the printed
	// table had in it, and a line break inside a pipe table ends the row. The
	// escaping and the padding are grid's, so this path and the native one
	// cannot drift apart.
	rows := make([][]string, len(b.Rows))
	for i, row := range b.Rows {
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			rows[i][j] = collapse(cell)
		}
	}
	parts = append(parts, grid.Pipe(rows))
	return strings.Join(parts, "\n\n")
}

// figure writes the picture and its caption.
//
// The path is the one the tool wrote and is rewritten to figures/<id>/ by
// the figure stage, which is also what checks the size and the page fraction
// cap. Nothing here is committable on its own, and that is deliberate: rule
// F06 is the line between a corpus and a mirror of copyrighted PDFs, and it
// is enforced in one place by one rule rather than wherever a path is
// written.
func (b Block) figure() string {
	var parts []string
	alt := collapse(b.Caption)
	if alt == "" {
		alt = "figure"
	}
	if b.Image != "" {
		parts = append(parts, fmt.Sprintf("![%s](%s)", escapeBrackets(alt), b.Image))
	}
	if c := collapse(b.Caption); c != "" {
		parts = append(parts, "*"+c+"*")
	}
	return strings.Join(parts, "\n\n")
}

func escapeBrackets(s string) string {
	s = strings.ReplaceAll(s, "[", `\[`)
	return strings.ReplaceAll(s, "]", `\]`)
}
