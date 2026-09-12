// Package grid writes a table as Markdown.
//
// It is here rather than in the package that found the table because three
// paths find tables and all three have to write them the same way. A table
// that pdftotext's word boxes said was a grid, one that a layout model handed
// over as rows of cells, and one that a vision model transcribed from a
// picture are three different problems, and a reader of the corpus should not
// be able to tell which of them produced the page.
package grid

import "strings"

// Pipe writes rows as a GitHub Flavored Markdown table, the first row being
// the header.
//
// A row shorter than the widest is padded with empty cells, because a pipe
// table with a ragged row does not render as a table at all: the renderer
// keeps the header's column count and quietly drops whatever is past it. The
// caller decides whether a table with ragged rows should be written this way
// or fenced. Padding here is for the row that is genuinely short, which is a
// cell the paper left empty.
func Pipe(rows [][]string) string {
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return ""
	}
	var b strings.Builder
	for i, row := range rows {
		b.WriteString(line(row, width))
		b.WriteByte('\n')
		if i == 0 {
			b.WriteString(rule(width))
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func line(cells []string, width int) string {
	out := make([]string, width)
	for i := range out {
		if i < len(cells) {
			out[i] = Escape(cells[i])
		}
	}
	return "| " + strings.Join(out, " | ") + " |"
}

func rule(width int) string {
	out := make([]string, width)
	for i := range out {
		out[i] = "---"
	}
	return "| " + strings.Join(out, " | ") + " |"
}

// Escape keeps a cell that contains a pipe from becoming two cells. A paper
// about shell pipelines has one in a table and it is not a column separator.
func Escape(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// Fence writes a block of text in a code fence, tagged with a language.
//
// It is what a table that a pipe table cannot carry is written as, and it is
// also how a listing is written, which is the same problem: a block of text
// whose spacing is the content and must not be reflowed.
//
// An empty tag is written as text. A wrong language tag is worse than none,
// because a renderer colours it and a reader believes the colouring.
func Fence(lang, body string) string {
	body = strings.Trim(body, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if lang == "" {
		lang = "text"
	}
	// A listing that itself contains a fence needs a longer one. Real in a
	// paper about Markdown and free to support.
	fence := "```"
	for strings.Contains(body, fence) {
		fence += "`"
	}
	return fence + lang + "\n" + body + "\n" + fence
}
