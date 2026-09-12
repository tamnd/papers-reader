package extract

import (
	"sort"

	"github.com/tamnd/papers-reader/poppler"
)

// The numbers here are all fractions of the page width, never points. A
// paper from 1967 is set on a page 567 points wide and one from 2017 on one
// 612 points wide, and a threshold in points would mean one of them was
// measured and the other was guessed.
const (
	// minGutter is how wide the channel between two columns has to be. It is
	// narrower than the gutter a typesetter would name, because the bins at
	// the edges of the channel are painted by the side bearings of the words
	// either side of it and what is left is the part nothing reaches into.
	minGutter = 0.015
	// clearShare is how much ink a bin may hold and still count as part of a
	// channel, as a share of the busiest bin on the page. Zero is the obvious
	// threshold and it is the wrong one: the title block of a two column
	// paper is six full width lines and they paint the gutter all the way
	// down the page, after which the page reads as one column and every
	// sentence on it is interleaved with the one beside it.
	clearShare = 0.15
	// minShare is how much of the text a column has to hold to be one. This
	// is what stops a marginal note, a line number down the side or a stray
	// word in the margin from being read as a column of its own.
	minShare = 0.1
	// spanGap is how many ordinary word spaces the gap at a gutter has to be
	// before the words either side of it are on different lines rather than
	// on one line that spans the columns. It is the second of the two tests
	// in cut and the weaker one: what decides is whether the gap covers the
	// whole channel. This is here for the sparse page whose channel comes out
	// narrower than the word space of the face set across it, so it only has
	// to be a number a title clears and a gutter does not.
	spanGap = 1.5
	// minWords is how many words a page needs before its shape says
	// anything. A title page has forty and is one column anyway.
	minWords = 60
	// bins is how finely the page is divided across to look for the
	// channels. One bin is a little under a character wide at these page
	// sizes, and the narrowest gutter in the hundred is three of them.
	bins = 200
)

// Gutters finds the clear vertical channels that divide a page into columns,
// as x positions in page points, left to right. A one column page has none.
// It is the middle of each channel Channels found, which is what a caller
// that only wants to know which column something is in needs.
func Gutters(p poppler.Layout) []float64 {
	ch := Channels(p)
	if len(ch) == 0 {
		return nil
	}
	out := make([]float64, len(ch))
	for i, c := range ch {
		out[i] = (c[0] + c[1]) / 2
	}
	return out
}

// Channels finds the clear vertical channels that divide a page into
// columns, as the pair of x positions each one runs between, left to right.
// A one column page has none.
//
// It is a projection: every word paints the bins it covers, and a column
// boundary is a run of bins almost nothing painted. That is a different test
// from clustering the lines by their centres and it is the right one.
// Centres say that a single column page whose section numbers hang in the
// left margin is two columns, because the numbers cluster left and the body
// clusters middle and there is a clear gap between the two. A projection
// says the body runs straight through where the boundary would have to be,
// which it does.
//
// Three columns are found the same way as two, which the 1967 proceedings
// need and which costs nothing.
//
// It is a per page test and not a per paper one, because a two column paper
// still sets its title across the full width and puts its references in two
// columns under a heading that spans both.
func Channels(p poppler.Layout) [][2]float64 {
	if p.Width <= 0 {
		return nil
	}
	words := Words(p)
	if len(words) < minWords {
		return nil
	}
	ink := make([]int, bins)
	for _, w := range words {
		from := max(bin(w.XMin, p.Width), 0)
		to := min(bin(w.XMax, p.Width), bins-1)
		for i := from; i <= to; i++ {
			ink[i]++
		}
	}
	peak := 0
	for _, v := range ink {
		peak = max(peak, v)
	}
	clear := int(float64(peak) * clearShare)

	// The margins of the page are clear and are not gutters. The text block
	// is taken as the first and last bin the text really reaches, which is
	// not the first and last bin any word touches: a page number in the
	// corner or a marginal mark would otherwise put the edge out in the
	// margin and make the whole margin a candidate.
	first, last := -1, -1
	for i, v := range ink {
		if v > clear {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 || last-first < bins/4 {
		return nil
	}

	// A channel is kept as the run of bins it covers and not as its middle,
	// because how wide it is is what tells a step across a gutter from the
	// word space of a title set across the page.
	var runs [][2]int
	for i := first + 1; i < last; i++ {
		if ink[i] > clear {
			continue
		}
		j := i
		for j < last && ink[j] <= clear {
			j++
		}
		if float64(j-i)/bins >= minGutter {
			runs = append(runs, [2]int{i, j})
		}
		i = j
	}

	// A channel with almost no text on one side of it is not a column
	// boundary. Dropping the thinnest and measuring again matters on a page
	// where a marginal note has left a wide channel beside the body: the
	// note is not a column, and the gutter between the real columns still
	// is.
	for len(runs) > 0 {
		share := shares(words, runs, p.Width)
		worst, at := 1.0, -1
		for i, s := range share {
			if s < worst {
				worst, at = s, i
			}
		}
		if worst >= minShare {
			break
		}
		// A thin column at the left is the fault of the cut to its right,
		// and one at the right is the fault of the cut to its left.
		if at == 0 {
			runs = runs[1:]
		} else if at == len(share)-1 {
			runs = runs[:len(runs)-1]
		} else {
			runs = append(runs[:at-1], runs[at:]...)
		}
	}

	out := make([][2]float64, len(runs))
	for i, r := range runs {
		out[i] = [2]float64{float64(r[0]) / bins * p.Width, float64(r[1]) / bins * p.Width}
	}
	return out
}

func bin(x, width float64) int { return int(x / width * bins) }

// shares is how much of the text each column holds.
func shares(words []poppler.Word, runs [][2]int, width float64) []float64 {
	counts := make([]float64, len(runs)+1)
	for _, w := range words {
		n := 0
		for _, r := range runs {
			if bin(w.XMid(), width) >= (r[0]+r[1])/2 {
				n++
			}
		}
		counts[n]++
	}
	for i := range counts {
		counts[i] /= float64(len(words))
	}
	return counts
}

// Words is every word of a page, in no particular order.
//
// The lines pdftotext groups them into are not used, and this is why. It
// groups by the row a word sits on, and a row of a two column page crosses
// both columns, so one of its lines can be the end of a sentence in the left
// column followed by the start of an unrelated one in the right. That is the
// same failure that makes pdftotext -layout unusable on a two column paper,
// and reading its lines instead of its text does not fix it.
func Words(p poppler.Layout) []poppler.Word {
	all := words(p)
	h := medianHeight(all)
	out := make([]poppler.Word, 0, len(all))
	for _, w := range all {
		if !sideways(w, h) {
			out = append(out, w)
		}
	}
	return out
}

// Sideways is the text on a page that is set rotated, which comes back from
// pdftotext as a word in a box taller than the page's text ever is.
//
// It is kept out of the page and kept here rather than thrown away. On this
// corpus it is almost always the stamp a preprint server prints down the
// left margin, which is furniture of the plainest kind: it is not the paper,
// it was not written by the author, and it turns up in the middle of the
// abstract if it is left in. Returned rather than dropped because the day
// there is a paper with a rotated table in it, this is where to look.
func Sideways(p poppler.Layout) []poppler.Word {
	all := words(p)
	h := medianHeight(all)
	var out []poppler.Word
	for _, w := range all {
		if sideways(w, h) {
			out = append(out, w)
		}
	}
	return out
}

// sidewaysFactor is how much taller than the page's usual word a box has to
// be before it is a line of text turned on its side. Three is high enough
// that the tall brackets of a displayed matrix stay in the text, and a
// rotated run is a whole line's worth of characters stacked, which is ten
// times the height of a word and not three.
const sidewaysFactor = 3

func sideways(w poppler.Word, median float64) bool {
	return median > 0 && w.Height() > median*sidewaysFactor && w.Height() > w.Width()
}

func words(p poppler.Layout) []poppler.Word {
	var out []poppler.Word
	for _, b := range p.Blocks {
		for _, l := range b.Lines {
			out = append(out, l.Words...)
		}
	}
	return out
}

func medianHeight(words []poppler.Word) float64 {
	var hs []float64
	for _, w := range words {
		if h := w.Height(); h > 0 {
			hs = append(hs, h)
		}
	}
	return median(hs)
}

// Lines is the text of a page rebuilt from its words, in the order a person
// reads them: each column top to bottom, left to right.
//
// A line that spans the columns ends the band above it. That is what a
// section heading set across the page, a wide display and a full width
// figure do, and reading the whole left column before it would move several
// paragraphs past a heading that introduces them.
func Lines(p poppler.Layout) []poppler.TextLine {
	all := rows(Words(p))
	channels := Channels(p)
	if len(channels) == 0 {
		return all
	}
	cuts := make([]float64, len(channels))
	for i, c := range channels {
		cuts[i] = (c[0] + c[1]) / 2
	}
	page := medianWordGap(all)

	var spans, inColumn []poppler.TextLine
	for _, l := range all {
		// A row with enough words is measured against its own spacing, which
		// is what keeps a title set in eighteen point from being cut in half
		// at the gutter: its word spaces are wide, and next to the page's
		// body spacing they look like the step across a column.
		//
		// A quarter of the way up its gaps and not the middle of them,
		// because the row that most needs cutting is the one that already
		// runs across every column of the page, and the gaps in that row are
		// three columns' worth of justification plus the two gutter steps
		// themselves. The middle of that mixture is wider than a word space
		// and on the last body row of Amdahl's first page it came out at
		// eight points against a five point page, which lifted the threshold
		// just over the twelve point gutter and published three columns
		// interleaved in one line. A title has one spacing all the way
		// across, so a quarter of the way up its gaps is the same number as
		// the middle of them and it is protected either way.
		gap := page
		if len(l.Words) > 4 {
			gap = wordGap([]poppler.TextLine{l}, lowGaps)
		}
		for _, piece := range cut(l, channels, spanGap*gap) {
			if crosses(piece.Box, cuts) {
				spans = append(spans, piece)
			} else {
				inColumn = append(inColumn, piece)
			}
		}
	}

	type placed struct {
		band, col int
		line      poppler.TextLine
	}
	band := func(l poppler.TextLine) int {
		n := 0
		for _, s := range spans {
			if s.YMid() < l.YMid() {
				n++
			}
		}
		return n
	}
	var out []placed
	for _, l := range inColumn {
		out = append(out, placed{band(l), Column(l, cuts), l})
	}
	for i, s := range spans {
		// A spanning line closes the band it is in, so it sorts after every
		// column of that band and before anything below it.
		out = append(out, placed{i, len(cuts) + 1, s})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].band != out[j].band {
			return out[i].band < out[j].band
		}
		if out[i].col != out[j].col {
			return out[i].col < out[j].col
		}
		return out[i].line.YMin < out[j].line.YMin
	})
	lines := make([]poppler.TextLine, len(out))
	for i, pl := range out {
		lines[i] = pl.line
	}
	return lines
}

// cut splits a row where it steps across a gutter.
//
// This is the whole of the two column problem. A row that crosses a gutter is
// either a title set across the page or the end of a line in one column
// followed by the start of an unrelated line in the next, and what tells them
// apart is the gap at the crossing: a word space in the first, the whole
// width of the channel in the second.
//
// The test is that the gap covers the channel, end to end, and that is the
// part that has to be exact. Measuring it as a multiple of the page's word
// space instead was close enough for a modern two column preprint and wrong
// on the 1967 proceedings, which set three columns with a twelve point gutter
// and a four and a half point word space: two and a half word spaces, under
// any threshold that leaves a title alone. Every body row of Amdahl's first
// page came through uncut, each one became a line that spans the columns,
// each spanning line closed a band, and the page was published with its three
// columns interleaved a sentence at a time.
//
// The multiple is kept as well and lowered to something a title comfortably
// clears, because a channel found on a sparse page can be narrower than the
// word space of the display face set across it.
func cut(l poppler.TextLine, channels [][2]float64, tol float64) []poppler.TextLine {
	var out []poppler.TextLine
	from := 0
	for i := 1; i < len(l.Words); i++ {
		left, right := l.Words[i-1].XMax, l.Words[i].XMin
		if right-left <= tol || !covers(left, right, channels) {
			continue
		}
		out = append(out, line(l.Words[from:i]))
		from = i
	}
	if from == 0 {
		return []poppler.TextLine{l}
	}
	return append(out, line(l.Words[from:]))
}

// covers says whether the gap from left to right holds a whole channel.
func covers(left, right float64, channels [][2]float64) bool {
	for _, c := range channels {
		if left <= c[0] && right >= c[1] {
			return true
		}
	}
	return false
}

// line makes a text line out of a run of words that are already in order.
func line(words []poppler.Word) poppler.TextLine {
	l := poppler.TextLine{Box: words[0].Box, Words: words}
	for _, w := range words[1:] {
		l.Box = union(l.Box, w.Box)
	}
	return l
}

func crosses(b poppler.Box, cuts []float64) bool {
	for _, c := range cuts {
		if b.XMin < c && b.XMax > c {
			return true
		}
	}
	return false
}

// medianWordGap is the usual distance between two words of this page, which
// is the unit a gutter crossing is measured in.
func medianWordGap(lines []poppler.TextLine) float64 { return wordGap(lines, 0.5) }

// wordGap is a quantile of the distance between two words.
func wordGap(lines []poppler.TextLine, q float64) float64 {
	var gaps []float64
	for _, l := range lines {
		for i := 1; i < len(l.Words); i++ {
			if g := l.Words[i].XMin - l.Words[i-1].XMax; g >= 0 {
				gaps = append(gaps, g)
			}
		}
	}
	return quantile(gaps, q)
}

// rows groups words into the lines they were set on.
//
// A word joins the row whose band it falls in, and the band is not the box
// of everything in the row so far. That is the mistake to avoid and it is
// not obvious until it happens: a row that grows to hold a tall bracket, a
// fraction or the ascender of an integral reaches the line below, the first
// word of that line joins it, the row grows again, and a page of prose comes
// out as one row of two hundred words sorted left to right. The Transformer
// paper's abstract came out that way.
//
// So the band narrows rather than grows. It starts as the first word's box
// and each word that joins intersects it, which leaves the band the height
// of the shortest thing on the line and holds it where the line really is.
func rows(words []poppler.Word) []poppler.TextLine {
	if len(words) == 0 {
		return nil
	}
	sorted := append([]poppler.Word(nil), words...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].YMid() != sorted[j].YMid() {
			return sorted[i].YMid() < sorted[j].YMid()
		}
		return sorted[i].XMin < sorted[j].XMin
	})

	var out []poppler.TextLine
	var bands []poppler.Box
	for _, w := range sorted {
		if n := len(out); n > 0 && sameRow(bands[n-1], w.Box) {
			out[n-1].Words = append(out[n-1].Words, w)
			out[n-1].Box = union(out[n-1].Box, w.Box)
			bands[n-1] = overlap(bands[n-1], w.Box)
			continue
		}
		out = append(out, poppler.TextLine{Box: w.Box, Words: []poppler.Word{w}})
		bands = append(bands, w.Box)
	}
	for i := range out {
		ws := out[i].Words
		sort.SliceStable(ws, func(a, b int) bool { return ws[a].XMin < ws[b].XMin })
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].YMin < out[j].YMin })
	return out
}

// sameRow says whether a word belongs to a row, given the row's band.
//
// The first test is the one that does the work: the word's centre is inside
// the band. The second is for a superscript, which is a small box set high
// whose own centre is above the band and which overlaps it all the same.
// Only a box smaller than the band qualifies, because the same shape one
// size larger is the tall delimiter of the display below.
func sameRow(band, word poppler.Box) bool {
	if word.YMid() >= band.YMin && word.YMid() <= band.YMax {
		return true
	}
	return word.Height() < band.Height() &&
		band.YMid() >= word.YMin && band.YMid() <= word.YMax
}

// overlap is the part of two boxes that is in both, vertically. It keeps the
// left and right edges of the first, because nothing asks a band about x.
func overlap(band, w poppler.Box) poppler.Box {
	band.YMin = max(band.YMin, w.YMin)
	band.YMax = min(band.YMax, w.YMax)
	if band.YMax < band.YMin {
		band.YMax = band.YMin
	}
	return band
}

// Column says which column a line is in, counting from zero at the left. It
// is what tells a paragraph that continues at the top of the next column
// from one that continues on the next line.
func Column(l poppler.TextLine, cuts []float64) int {
	n := 0
	for _, c := range cuts {
		if l.XMid() >= c {
			n++
		}
	}
	return n
}
