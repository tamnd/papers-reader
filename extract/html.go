package extract

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/grid"
)

// Untable rewrites the HTML tables a reader wrote as the pipe tables this
// corpus is written in.
//
// The prompt asks for a pipe table where the cells form a grid and a `text`
// fence where they do not. A model trained on web pages answers with
// `<table>` anyway, the same way olmOCR answers in TeX's own math delimiters
// however the question is worded, and for the same reason: it is writing what
// its training data looks like. Three sections of the Transformer paper came
// back as raw HTML.
//
// Left alone it is worse than it looks. The reading app renders Markdown and
// not HTML, so the table comes out as a column of angle brackets. The
// translator has no idea which parts of it are text. And `<td>1.0 \cdot
// 10^{20}</td>` is mathematics with no dollars round it, so mathtex never sees
// it, the M rules never check it, and rule M14 reads the section as one whose
// formulas were flattened.
//
// Like Dollars this is a translation between two spellings and not a repair.
// Nothing here decides that something is a table: the `<table>` says so. What
// it will not do is guess. A table it cannot read as a rectangle of cells is
// left exactly as it came, where rule T11 reports it and a person looks at it,
// because a table quietly turned into the wrong table is worse than a table
// that is obviously still HTML.
func Untable(s string) string {
	lines := strings.Split(s, "\n")
	// A listing about HTML is full of `<table>` and none of it is a table.
	// Inside counts from one, so the line at index i is at fenced[i+1].
	fenced := code.Inside(s)
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if fenced[i+1] || !opensTable(lines[i]) {
			out = append(out, lines[i])
			continue
		}
		end := closesTable(lines, i, fenced)
		if end < 0 {
			out = append(out, lines[i])
			continue
		}
		pipe, ok := pipeTable(strings.Join(lines[i:end+1], "\n"))
		if !ok {
			out = append(out, lines[i:end+1]...)
			i = end
			continue
		}
		out = append(out, strings.Split(pipe, "\n")...)
		i = end
	}
	return strings.Join(out, "\n")
}

var (
	tableOpen  = regexp.MustCompile(`(?i)^\s*<table\b`)
	tableClose = regexp.MustCompile(`(?i)</table\s*>`)
	rowTag     = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr\s*>`)
	cellTag    = regexp.MustCompile(`(?is)<(th|td)\b([^>]*)>(.*?)</(?:th|td)\s*>`)
	spanAttr   = regexp.MustCompile(`(?i)\b(rowspan|colspan)\s*=\s*"?(\d+)"?`)
	// leftover is any tag at all. A cell that still has one after the
	// conversions below is a cell this function did not understand, and one
	// cell it did not understand is enough to leave the whole table alone.
	leftover = regexp.MustCompile(`<[^>]*>`)
)

func opensTable(line string) bool { return tableOpen.MatchString(line) }

// closesTable is the line the table ends on, and -1 for a table that never
// ends. A table that runs to the end of the page is a page that went wrong
// somewhere else and is not this function's to tidy away.
func closesTable(lines []string, from int, fenced []bool) int {
	for i := from; i < len(lines); i++ {
		if i > from && fenced[i+1] {
			return -1
		}
		if tableClose.MatchString(lines[i]) {
			return i
		}
	}
	return -1
}

// pipeTable reads the HTML and writes the Markdown, and says no rather than
// guessing.
func pipeTable(html string) (string, bool) {
	rows := rowTag.FindAllStringSubmatch(html, -1)
	if len(rows) < 2 {
		// One row is a heading with nothing under it, which is not a table
		// anybody can read, and no rows is markup this function misread.
		return "", false
	}
	// grid is the table being filled in, and held is the cells still coming
	// down from a rowspan above. A cell that spans four rows is written once
	// and the three rows under it get an empty cell in that column, which is
	// what every Markdown renderer does with a span anyway because GitHub
	// Flavored Markdown has no way to write one. The alternative is to drop
	// the row the span came from, and then the table says something the paper
	// does not.
	var out [][]string
	held := map[int]int{}
	for _, r := range rows {
		cells := cellTag.FindAllStringSubmatch(r[1], -1)
		if len(cells) == 0 {
			return "", false
		}
		var row []string
		put := func(text string) {
			for held[len(row)] > 0 {
				held[len(row)]--
				row = append(row, "")
			}
			row = append(row, text)
		}
		for _, c := range cells {
			text, ok := cellText(c[3])
			if !ok {
				return "", false
			}
			down, across := spans(c[2])
			if down > 1 {
				// The column the cell lands in is not known until the held
				// cells before it have been laid down, so the hold is
				// recorded after the put.
				put(text)
				held[len(row)-1] += down - 1
				for i := 1; i < across; i++ {
					put("")
				}
				continue
			}
			put(text)
			for i := 1; i < across; i++ {
				put("")
			}
		}
		for held[len(row)] > 0 {
			held[len(row)]--
			row = append(row, "")
		}
		out = append(out, row)
	}
	// Every row the same width, and no span left hanging over the end of the
	// table. This is the rectangle test and it is the whole of what keeps a
	// wrong table out of the corpus.
	//
	// It is not a formality. A reader that writes spans writes them from
	// looking at a picture, and the Transformer paper's third table came back
	// with `rowspan="4"` over a group of five rows and `colspan="7"` used to
	// mean seven empty columns rather than one cell seven wide. Expanded as
	// written, the numbers land under the wrong headings, and a table whose
	// numbers are under the wrong headings is worse than no table at all
	// because it reads like a table. The same paper's second table uses both
	// kinds of span correctly and converts.
	//
	// grid.Pipe pads a short row, which is right for a cell the paper left
	// empty and wrong here: a ragged row out of a span arithmetic that does
	// not add up is a row this function read wrong, not a row the paper left
	// short.
	for _, row := range out {
		if len(row) != len(out[0]) {
			return "", false
		}
	}
	for _, n := range held {
		if n > 0 {
			return "", false
		}
	}
	return grid.Pipe(out), true
}

// spans reads rowspan and colspan off a cell's attributes. Anything else on
// them is ignored: an align or a style is presentation and this corpus does
// not carry presentation.
func spans(attrs string) (down, across int) {
	down, across = 1, 1
	for _, m := range spanAttr.FindAllStringSubmatch(attrs, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil || n < 1 || n > 64 {
			continue
		}
		if strings.EqualFold(m[1], "rowspan") {
			down = n
		} else {
			across = n
		}
	}
	return down, across
}

var (
	bold      = regexp.MustCompile(`(?is)<(b|strong)\b[^>]*>(.*?)</(?:b|strong)\s*>`)
	italic    = regexp.MustCompile(`(?is)<(i|em)\b[^>]*>(.*?)</(?:i|em)\s*>`)
	linebreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	// script is a subscript or a superscript together with the token it
	// belongs to. A subscript is mathematics wherever it appears in these
	// tables and it is written as mathematics, because the alternative is to
	// leave it as a tag that the renderer will not render and the M rules
	// cannot see.
	script = regexp.MustCompile(`(?is)([\p{L}\p{N}.)\]]+)<(sub|sup)\b[^>]*>(.*?)</(?:sub|sup)\s*>`)
	// texRun is a run of TeX with no dollars round it, which is how a reader
	// writes a number in scientific notation inside a cell: `1.0 \cdot
	// 10^{20}`. The whole reason for converting these tables is that TeX the
	// splitter cannot see is mathematics the audit cannot check, and a cell
	// left like this would be exactly that with the angle brackets taken off.
	//
	// It has to be a control sequence with something on both sides of it,
	// because a lone backslash is an escape and a lone `\cdot` in a column of
	// prose is a typo rather than a formula.
	texRun = regexp.MustCompile(`[0-9A-Za-z.]+(?:\s*\\[a-zA-Z]+\s*[0-9A-Za-z.^_{}]+)+`)
)

// cellText turns the inside of one cell into Markdown, and says no if
// anything is left that it did not understand.
func cellText(s string) (string, bool) {
	s = linebreak.ReplaceAllString(s, " ")
	s = bold.ReplaceAllString(s, "**$2**")
	s = italic.ReplaceAllString(s, "*$2*")
	s = script.ReplaceAllStringFunc(s, func(m string) string {
		p := script.FindStringSubmatch(m)
		mark := "_"
		if strings.EqualFold(p[2], "sup") {
			mark = "^"
		}
		return "$" + p[1] + mark + "{" + strings.TrimSpace(p[3]) + "}$"
	})
	// Only a cell with no dollars anywhere in it. A cell that has some is a
	// cell where the reader already marked the mathematics, and guessing at
	// the rest of it would put dollars inside dollars.
	if !strings.Contains(s, "$") {
		s = texRun.ReplaceAllString(s, "$$$0$$")
	}
	// A pipe inside a cell would end the cell, and grid.Pipe escapes it on
	// the way out. Doing it here as well would escape the backslash.
	s = strings.Join(strings.Fields(s), " ")
	if leftover.MatchString(s) {
		return "", false
	}
	return s, true
}
