package extract

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/grid"
	"github.com/tamnd/papers-reader/poppler"
)

// A Table is a run of lines of one page that were set as a grid, and the
// Markdown they come out as.
//
// Lines is which lines of the page the table was set on, as indices into the
// slice it was found in and in ascending order, so that the caller can take
// them out of the paragraph flow. They are not always a contiguous run: a
// table whose cells pdftotext emitted one at a time comes back with its
// indices scattered over the page, which is what the page's text layer says
// and not a defect here.
type Table struct {
	poppler.Box
	Lines []int
	// Rows and Columns are the shape of the grid that was read, and are zero
	// for a table that is written out as it was printed.
	Rows, Columns int
	// Fenced says the grid did not survive: some cell of it stands over more
	// than one column, and what is in Text is the table laid out the way the
	// page laid it out, inside a fence.
	Fenced bool
	Text   string
}

// The numbers a table is found by. The horizontal ones are in multiples of
// the page's own word space with a floor in points, because a table is found
// by the gaps in it and a gap only means something next to the gaps around it.
const (
	// cellGap is how many times the page's word space a gap has to be before
	// it is a gap between two cells rather than a gap between two words. Four
	// is low, and it is low because the narrowest column gap in the Transformer
	// paper's first table is thirteen points against a word space of two, while
	// the widest word space in a justified line of the same page is five.
	cellGap = 4
	// minCell is the floor in points, for a page whose word space came out
	// tiny or zero. Eight points is three characters of a body face and no
	// typesetter puts that between two words.
	minCell = 8
	// minGridded is how many rows of the run have to carry the full count of
	// cells. Three, so that two lines of prose that happen to have a wide space
	// in the same place are not a table. A table with a heading and one row is
	// missed by this and that is the trade: the corpus is better off with a
	// small table read as prose than with a paragraph read as a table.
	minGridded = 3
	// narrow is how wide a row carrying a single cell may be, as a share of
	// the widest row of the run, before it is a line of prose rather than a
	// row of the table with holes in it.
	//
	// A run has to be allowed to pass over a row with one cell on it: a
	// column heading set on two lines is one, and so is a row of an ablation
	// table that left every column but one blank. What it must not pass over
	// is the first line of the paragraph under the table. Counting them does
	// not tell the two apart, because a table can have four holes in a row
	// and a paragraph is one line at a time. What tells them apart is that a
	// line of prose is set to the measure and a cell is not.
	narrow = 0.6
	// reachUp is how far above the first row of a run its caption may be, in
	// rows, and reachDown how far below the last. A caption wraps to three
	// lines often and to five rarely, and it sits under the table about as
	// often as over it.
	reachUp   = 5
	reachDown = 3
	// onALine is how much of the shorter of two lines has to be level with the
	// other before they are one row of the table. Half. A superscript set
	// beside a cell overlaps it by most of itself, and the line under it
	// overlaps it by nothing.
	onALine = 0.5
	// wordy is how many words the middle cell of a table may hold. A cell
	// holds a value, a name or a measurement, and six words is a generous
	// reading of all three: the widest cell in the Transformer paper's
	// parsing table is "Vinyals & Kaiser el al. (2014) [37]", which is seven,
	// and the middle cell of that table is two.
	//
	// This is the guard against the worst thing this file could do. A two
	// column page whose gutter the column finder missed is a grid: every line
	// has two cells, they line up perfectly, and there may well be a table
	// caption within five lines. Read as a table it would come out as a page
	// of prose in a two column grid, which is worse than the interleaved
	// sentences it started as. The difference between that page and a table
	// is not in the geometry, it is that a table's cells are not sentences. A
	// half line of a two column page runs to nine or ten words.
	wordy = 6
)

// caption is a table caption as the journals in this corpus set one: the
// word, the number, and something between them and the sentence. Roman
// numerals because the older journals number their tables in them.
//
// A figure caption deliberately does not match. A figure is a picture and the
// lines under it are a legend, and reading a legend as a grid would file the
// words of a diagram as data.
var caption = regexp.MustCompile(`(?i)^tab(?:le|\.)\s*(?:[0-9]{1,3}|[ivxlc]{1,6})\b`)

// Tables finds the tables among the lines of one page.
//
// A table is found where the page says there is one. The test is a run of
// rows whose words are grouped into the same columns by gaps far wider than
// the page's word space, next to a line that starts "Table 1". The caption is
// required, and it is the defence against the false positive that matters:
// the author block of a two column paper, a list of symbols set in two
// columns and the numbers down the side of a figure are all grids, and none
// of them is a table. Anchoring to the caption means a table set without one
// is missed, which is the direction to be wrong in.
//
// What comes back is in the order the tables start in, and no line belongs to
// two of them.
func Tables(lines []poppler.TextLine, cuts []float64) []Table {
	if len(lines) < minGridded {
		return nil
	}
	gap := math.Max(cellGap*wordSpace(lines), minCell)
	pitch := medianPitch(lines)

	var out []Table
	for _, col := range columnsOf(lines, cuts) {
		rows := rowsOf(lines, col, gap)
		for i := 0; i < len(rows); i++ {
			if len(rows[i].cells) < 2 {
				continue
			}
			last := extent(rows, i, pitch)
			if last-i+1 < minGridded || !captioned(rows, i, last) {
				continue
			}
			t, ok := build(rows, i, last)
			if !ok {
				continue
			}
			out = append(out, t)
			i = last
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lines[0] < out[j].Lines[0] })
	return out
}

// A cell is a run of words with nothing but word spaces in it.
type cell struct {
	poppler.Box
	Text string
}

// A row is one row of the page: the words that are level with one another,
// cut into cells, and which lines of the page they came from.
//
// It is not the same thing as a line, and the difference is the reason this
// file works. pdftotext emits a line per run of text set in one size, so a
// row of a table that has a subscript in it arrives in three pieces, and the
// third table of the Transformer paper arrives one cell at a time, column by
// column, forty pieces for nine rows. Putting them back by what they are
// level with is the only way to see the grid.
type row struct {
	poppler.Box
	cells []cell
	from  []int
}

func (r row) text() string {
	parts := make([]string, len(r.cells))
	for i, c := range r.cells {
		parts[i] = c.Text
	}
	return strings.Join(parts, " ")
}

// columnsOf groups the lines of a page by the column they are in, left to
// right, each group holding the indices of its lines.
func columnsOf(lines []poppler.TextLine, cuts []float64) [][]int {
	out := make([][]int, len(cuts)+1)
	for i, l := range lines {
		c := Column(l, cuts)
		out[c] = append(out[c], i)
	}
	return out
}

// rowsOf puts the lines of one column back into the rows they were printed
// as, in the order they appear down the page.
func rowsOf(lines []poppler.TextLine, idx []int, gap float64) []row {
	at := append([]int(nil), idx...)
	sort.Slice(at, func(i, j int) bool {
		a, b := lines[at[i]], lines[at[j]]
		if a.YMin != b.YMin {
			return a.YMin < b.YMin
		}
		return a.XMin < b.XMin
	})

	var out []row
	var words []poppler.Word
	var box poppler.Box
	var from []int
	flush := func() {
		if len(from) == 0 {
			return
		}
		sort.Slice(words, func(i, j int) bool { return words[i].XMin < words[j].XMin })
		sort.Ints(from)
		out = append(out, row{Box: box, cells: split(words, gap), from: from})
		words, from = nil, nil
	}
	for _, i := range at {
		l := lines[i]
		if len(from) > 0 && !level(box, l.Box) {
			flush()
		}
		if len(from) == 0 {
			box = l.Box
		} else {
			box = union(box, l.Box)
		}
		words = append(words, l.Words...)
		from = append(from, i)
	}
	flush()
	return out
}

// level says whether a line is on the same row of the page as what has been
// gathered so far.
func level(a, b poppler.Box) bool {
	h := math.Min(a.Height(), b.Height())
	if h <= 0 {
		return a.YMin == b.YMin
	}
	over := math.Min(a.YMax, b.YMax) - math.Max(a.YMin, b.YMin)
	return over >= onALine*h
}

// split cuts a row of words into cells at every gap wider than gap. A row
// with no such gap is one cell, which is what every line of prose is.
func split(words []poppler.Word, gap float64) []cell {
	var out []cell
	var cur cell
	for i, w := range words {
		if i > 0 && w.XMin-words[i-1].XMax > gap && cur.Text != "" {
			out = append(out, cur)
			cur = cell{}
		}
		if cur.Text == "" {
			cur = cell{Box: w.Box, Text: w.Text}
			continue
		}
		cur.Box = union(cur.Box, w.Box)
		cur.Text += " " + w.Text
	}
	if cur.Text != "" {
		out = append(out, cur)
	}
	for i := range out {
		out[i].Text = Ligatures(out[i].Text)
	}
	return out
}

// wordSpace is the usual gap between two words on this page. It is measured
// and not assumed for the same reason the line pitch is: the space of a nine
// point face and the space of a twelve point face are different numbers, and
// a table is found by comparing a gap against this one.
//
// The gaps on a page come in two families, the ones between two words of a
// sentence and the ones between two cells of a table, and this wants the
// first family. The middle of the two together is the wrong answer on any
// page where the second family is the bigger one, which is what a page with a
// full page table on it is: the middle gap of the page is then a cell gap,
// four times a cell gap is wider than the table, and the table is invisible.
// A quarter of the way up is the same answer on a page of prose and the right
// one on a page of table, and it only goes wrong on a page where three
// quarters of the gaps are cell gaps, which the ceiling below catches.
const lowGaps = 0.25

func wordSpace(lines []poppler.TextLine) float64 {
	var gaps, glyphs []float64
	for _, l := range lines {
		for i, w := range l.Words {
			if n := len([]rune(w.Text)); n > 0 && w.Width() > 0 {
				glyphs = append(glyphs, w.Width()/float64(n))
			}
			if i == 0 {
				continue
			}
			if d := w.XMin - l.Words[i-1].XMax; d >= 0 {
				gaps = append(gaps, d)
			}
		}
	}
	space := quantile(gaps, lowGaps)
	// The ceiling, for the page that has no word gaps on it at all because
	// every line of it is a row of one word cells. A space is narrower than a
	// character in every face that was ever cut, so the width of a character
	// is as wide as a space can be.
	if em := median(glyphs); em > 0 && (space <= 0 || space > em) {
		space = em
	}
	return space
}

// extent is the last row of the run that starts at first.
//
// The run steps down the page a row at a time and is allowed to pass over a
// row carrying a single narrow cell, which is how a stacked column heading
// and a row with holes in it arrive. It ends at the last row that carries a
// grid, so that the paragraph under the table is not part of it even when its
// first line was short enough to be passed over.
func extent(rows []row, first int, pitch float64) int {
	last := first
	wide := rows[first].Width()
	for j := first + 1; j < len(rows); j++ {
		step := rows[j].YMin - rows[j-1].YMin
		if pitch > 0 && (step < 0 || step > pitch*2.5) {
			break
		}
		switch n := len(rows[j].cells); {
		case n >= 2:
			last = j
			wide = math.Max(wide, rows[j].Width())
		case n == 1 && rows[j].Width() <= narrow*wide:
			// A hole in the table, or the second line of a heading.
		default:
			return last
		}
	}
	return last
}

// captioned says whether one of the rows around this run is the first line of
// a table caption.
func captioned(rows []row, first, last int) bool {
	for i := max(0, first-reachUp); i <= min(len(rows)-1, last+reachDown); i++ {
		if i >= first && i <= last {
			continue
		}
		if caption.MatchString(strings.TrimSpace(rows[i].text())) {
			return true
		}
	}
	return false
}

// A band is the horizontal extent of one column of a table.
type band struct{ XMin, XMax float64 }

func (c band) overlap(b poppler.Box) float64 {
	return math.Min(c.XMax, b.XMax) - math.Max(c.XMin, b.XMin)
}

// build turns a run of rows into a table, or says it is not one.
func build(rows []row, first, last int) (Table, bool) {
	gridded := 0
	var words []float64
	for i := first; i <= last; i++ {
		if len(rows[i].cells) >= 2 {
			gridded++
		}
		for _, c := range rows[i].cells {
			words = append(words, float64(len(strings.Fields(c.Text))))
		}
	}
	cols := bandsOf(rows, first, last)
	if len(cols) < 2 || gridded < minGridded || median(words) > wordy {
		return Table{}, false
	}

	t := Table{Box: rows[first].Box}
	for i := first; i <= last; i++ {
		t.Box = union(t.Box, rows[i].Box)
		t.Lines = append(t.Lines, rows[i].from...)
	}
	sort.Ints(t.Lines)

	cells, ok := gridOf(rows, cols, first, last)
	if !ok || len(cells) < 2 {
		// A cell that stands over two columns cannot be written as a pipe
		// table, and a pipe table with that cell dropped, or moved into one of
		// the columns it stands over, says something the paper does not. So
		// the page gets the table as it was printed, which a reader can read
		// and a later pass over the corpus can pick up.
		t.Fenced = true
		t.Text = grid.Fence("text", printed(rows, first, last))
		return t, t.Text != ""
	}
	t.Rows, t.Columns = len(cells), len(cols)
	t.Text = grid.Pipe(cells)
	return t, t.Text != ""
}

// bandsOf is where the columns of a table stand, left to right.
//
// They are found by putting together the cells that are over one another and
// not by counting the cells in a row, because half the tables in this corpus
// have a row with a hole in it. The ablation table of the Transformer paper
// is nine rows of thirteen columns and only one of the rows has a figure in
// every column, so a column count taken from the widest row would find one
// row of the table and a count taken from the commonest would find four
// columns out of thirteen.
//
// Two cells that touch are in one column. A heading that stands over two
// columns therefore joins them into one band, which is not the truth about
// the table but is the truth about what can be written down: the rows under
// it then have two cells in one column, gridOf says so, and the table is
// written out as it was printed instead.
func bandsOf(rows []row, first, last int) []band {
	var all []cell
	for i := first; i <= last; i++ {
		all = append(all, rows[i].cells...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].XMin < all[j].XMin })

	var out []band
	for _, c := range all {
		if n := len(out); n > 0 && c.XMin < out[n-1].XMax {
			out[n-1].XMax = math.Max(out[n-1].XMax, c.XMax)
			continue
		}
		out = append(out, band{XMin: c.XMin, XMax: c.XMax})
	}
	return out
}

// gridOf places every cell of the run in a column.
//
// A row holding a single cell that is not in the first column is not a row of
// its own. It is the rest of the row above, which is how a column heading set
// on two lines arrives: the Transformer paper's first table has "Sequential"
// on one line and "Operations" under it, and read as a row of its own it
// would push a row of empty cells into the middle of the table. It is merged
// into the row above rather than dropped, because something that was printed
// on the page belongs in the page.
//
// A row holding several cells is a row even when the first columns of it are
// empty, which is the other half of the same problem: the rows of an ablation
// table leave the stub blank and say what changed in the column it changed
// in, and merging those into the row above would put four experiments in one
// row.
func gridOf(rows []row, cols []band, first, last int) ([][]string, bool) {
	var out [][]string
	for i := first; i <= last; i++ {
		cs := rows[i].cells
		if len(cs) == 0 {
			continue
		}
		at := make([]int, len(cs))
		for k, c := range cs {
			n, ok := place(cols, c.Box)
			if !ok {
				return nil, false
			}
			// Two cells of one row in one column means the columns were read
			// wrong, and a table read wrong is worse than a table not read.
			if k > 0 && n <= at[k-1] {
				return nil, false
			}
			at[k] = n
		}
		if len(cs) > 1 || at[0] == 0 || len(out) == 0 {
			line := make([]string, len(cols))
			for k, c := range cs {
				line[at[k]] = c.Text
			}
			out = append(out, line)
			continue
		}
		line := out[len(out)-1]
		for k, c := range cs {
			line[at[k]] = strings.TrimSpace(line[at[k]] + " " + c.Text)
		}
	}
	return out, true
}

// place is the column a cell sits in.
//
// Most of the cell has to be inside it and next to nothing of the cell may be
// inside any other, which is how a cell standing over two columns is caught.
// The tests are shares of the cell and not point distances, so a one
// character cell and a cell holding a name are held to the same standard.
func place(cols []band, b poppler.Box) (int, bool) {
	best, second, at := 0.0, 0.0, -1
	for i, c := range cols {
		switch o := c.overlap(b); {
		case o > best:
			best, second, at = o, best, i
		case o > second:
			second = o
		}
	}
	w := math.Max(b.Width(), 1)
	if at < 0 || best < 0.6*w || second > 0.25*w {
		return 0, false
	}
	return at, true
}

// printed lays a run of rows out the way the page laid them out, in spaces.
//
// This is the honest fallback, and it is honest because nothing is decided:
// the words are where they were, to the nearest character. The character
// width is the page's own, measured from the words, so a table set in a small
// face comes out at the width it was set at rather than at the width of a
// face this function guessed.
func printed(rows []row, first, last int) string {
	em := glyph(rows, first, last)
	left := math.Inf(1)
	for i := first; i <= last; i++ {
		for _, c := range rows[i].cells {
			left = math.Min(left, c.XMin)
		}
	}
	var out []string
	for i := first; i <= last; i++ {
		// Runes and not bytes: a table of complexities is full of middle dots
		// and a column counted in bytes would drift right by one for each of
		// them.
		var line []rune
		for _, c := range rows[i].cells {
			at := int(math.Round((c.XMin - left) / em))
			if len(line) > 0 && at <= len(line) {
				at = len(line) + 1
			}
			for len(line) < at {
				line = append(line, ' ')
			}
			line = append(line, []rune(c.Text)...)
		}
		out = append(out, strings.TrimRight(string(line), " "))
	}
	return strings.Join(out, "\n")
}

// glyph is how wide one character of this table is, on average. Measured from
// the cells rather than assumed, and never zero: a run of words with no width
// at all would divide the layout by nothing.
func glyph(rows []row, first, last int) float64 {
	var widths []float64
	for i := first; i <= last; i++ {
		for _, c := range rows[i].cells {
			if n := len([]rune(c.Text)); n > 0 && c.Width() > 0 {
				widths = append(widths, c.Width()/float64(n))
			}
		}
	}
	if w := median(widths); w > 0.5 {
		return w
	}
	return 5 // points, about a character of a ten point face
}
