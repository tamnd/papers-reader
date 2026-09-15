package extract

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/poppler"
)

// margin is how much of the top and the bottom of a page the furniture can
// be in. Running heads and folios are set in the margin by definition, and
// looking for them in the body is how the first line of a section that
// happens to repeat gets deleted from every page it is on.
const margin = 0.12

// minPages is how many pages a paper needs before a repeated line means
// anything. Two pages that share a line share it by coincidence as often as
// not, and a two page paper loses nothing by keeping its header.
const minPages = 3

// detach is how far a line has to stand off the rest of the type on the page,
// in lines of that page's own type, before the white space around it is read
// as margin. Two lines of white is already more than any paragraph break in
// the hundred papers and less than any page ever leaves between its body and
// its folio.
const detach = 2.0

// minRows is how many rows of type a page needs before its own typesetting is
// allowed to say where the margin is. A title page, a page holding one figure
// and a page of nothing but a table are all white space with a little type in
// it, and asking them where the body ends gets an answer about the layout of
// that one page rather than about the margin.
const minRows = 8

// Furniture is the running heads, folios and rules of one paper: the text a
// journal prints on the page that the author did not write.
//
// It is learned from the pages rather than matched against a list, because
// every journal in the hundred does it differently. The Communications of the
// ACM put the volume and the month at the top, the AFIPS proceedings put the
// paper's own title there, an arXiv preprint puts nothing at the top and a
// bare number at the bottom, and a photographed thesis has the page number
// written in by hand in a place no rule would look.
//
// The zero Furniture keeps everything, which is what a paper of one page
// should do.
type Furniture struct {
	// repeated is the folded text of a line and the rounded band it sits in,
	// for every line that turned up on more than half the pages. The band is
	// part of the key because a paper that says "Introduction" in its running
	// head also says it once in the body, and only one of those is furniture.
	repeated map[string]bool
	pages    int
	// folios is the page number each page prints, as it printed it, for the
	// pages whose margin number counts with the file. It is learned from the
	// whole paper by folios below, and it is empty for a paper whose margin
	// numbers do not count.
	folios map[int]string
	// frames is where the body sits on each page it was shown, so that a page
	// is measured once rather than once per line.
	frames map[int]frame
}

// A frame is where the body of one page ends and the margin begins, as a
// fraction of the page height. Only what is outside the frame can be
// furniture.
//
// It starts at the fixed band and is then widened by the page's own
// typesetting, because the band on its own is not enough. Razborov's Natural
// Proofs is set on US letter with the text block ending three quarters of the
// way down, and prints its folio at 0.81 of the page height: thirty eight
// points of white above it, on a page whose lines are twelve points apart,
// and still nowhere near the bottom eighth. Read by the band alone that folio
// stayed in the text, and audit rule T10 found the page number sitting in the
// prose in sixteen places in that one paper.
//
// Widening the frame does not delete anything on its own. It only lets Is
// look at a line, and Is still wants the line to be a bare number or to be
// repeated down the paper before it takes it out.
type frame struct {
	top, bottom float64
}

// FindFurniture learns what is furniture from the pages it is given.
//
// Give it the whole paper, or a sample spread across the paper. A run of
// consecutive pages from the middle would learn the running head correctly
// and learn nothing about the first page, which is the page that has a
// different one.
func FindFurniture(pages []poppler.Layout) *Furniture {
	frames := framesOf(pages)
	f := &Furniture{
		repeated: map[string]bool{},
		pages:    len(pages),
		folios:   foliosOf(pages, frames),
		frames:   frames,
	}
	if len(pages) < minPages {
		return f
	}
	// The lines are the rebuilt ones and not pdftotext's, because those are
	// what Is will be asked about later. A running foot that carries the page
	// number at the left margin and the journal's name across the middle is
	// one line to pdftotext and two to the column reader, and learning it in
	// one shape and looking for it in the other finds nothing.
	seen := map[string]map[int]bool{}
	for _, p := range pages {
		for _, l := range Lines(p) {
			k, ok := frames[p.Number].key(p, l)
			if !ok {
				continue
			}
			if seen[k] == nil {
				seen[k] = map[int]bool{}
			}
			seen[k][p.Number] = true
		}
	}
	// A third of the pages, and never fewer than two. Half looks like the
	// safer number and it is wrong: a journal sets one running head on the
	// recto and another on the verso, so each of them is on half the pages
	// and neither is on more than half.
	least := max(len(pages)/3, 2)
	for k, on := range seen {
		if len(on) >= least {
			f.repeated[k] = true
		}
	}
	return f
}

// Is reports whether a line is furniture and should not be in the text.
func (f *Furniture) Is(p poppler.Layout, l poppler.TextLine) bool {
	if f == nil {
		return false
	}
	k, ok := f.frame(p).key(p, l)
	if !ok {
		return false
	}
	if f.repeated[k] {
		return true
	}
	// A bare number alone in the margin is a page number whether or not it
	// repeats, and it never repeats because it counts. Roman numerals are
	// here for the front matter of a thesis, which is the one place in the
	// hundred they are used for pages.
	return folio.MatchString(strings.TrimSpace(l.Text()))
}

// frame is where the body of a page ends and the margin begins. It is the
// measurement taken when the furniture was learned, and a fresh one for a
// page that was not in that sample.
func (f *Furniture) frame(p poppler.Layout) frame {
	if fr, ok := f.frames[p.Number]; ok {
		return fr
	}
	return frameOf(p)
}

// framesOf measures every page and then gives each page the typical page's
// answer as well as its own.
//
// One page on its own is not steady enough. The Natural Proofs pages all
// leave forty points between the last line and the folio and are set
// thirteen points apart, so most of them stand the folio off the text and
// the few with a displayed equation near the foot do not, and a folio that
// is furniture on twenty pages and text on six is worse than either answer.
//
// Typical is the median of every page and not the average of the pages that
// widened, because a title page has a block of white under the authors and
// a page carrying one figure has white wherever the figure is not. Counted
// among all the pages those two are outvoted, which is what should happen to
// them: the Gamma paper has one of each in its first eight pages, and read
// as evidence about the paper they moved the top margin to nearly half the
// page.
//
// Each page then keeps whichever of the two frames is the wider, because a
// journal that walks the folio down the page as the text block grows, which
// the victim cache paper does by half a point a page, would otherwise have
// its own pages overruled by the middle of the others.
func framesOf(pages []poppler.Layout) map[int]frame {
	out := map[int]frame{}
	if len(pages) == 0 {
		return out
	}
	var tops, bottoms []float64
	for _, p := range pages {
		fr := frameOf(p)
		out[p.Number] = fr
		tops = append(tops, fr.top)
		bottoms = append(bottoms, fr.bottom)
	}
	paper := frame{median(tops), median(bottoms)}
	for n, fr := range out {
		out[n] = frame{max(fr.top, paper.top), min(fr.bottom, paper.bottom)}
	}
	return out
}

// frameOf measures one page on its own.
func frameOf(p poppler.Layout) frame {
	fr := frame{margin, 1 - margin}
	if p.Height <= 0 {
		return fr
	}
	rows, pitch := rowsOn(p)
	if len(rows) < minRows || pitch <= 0 {
		return fr
	}
	if gap := rows[1] - rows[0]; gap >= detach*pitch {
		fr.top = max(fr.top, (rows[0]+gap/2)/p.Height)
	}
	if gap := rows[len(rows)-1] - rows[len(rows)-2]; gap >= detach*pitch {
		fr.bottom = min(fr.bottom, (rows[len(rows)-1]-gap/2)/p.Height)
	}
	return fr
}

// rowsOn is the centre of every row of type on a page, in order down the
// page, with the page's line spacing.
//
// Rows and not lines, because the two columns of a two column page put two
// lines at the same height and the question here is only whether anything at
// all is set at that height. Measuring the gaps between lines instead gives
// every two column page a typical gap of nothing, since the line beside a
// line is no distance below it.
//
// The spacing is the gap from one row to the next and not the height of a
// line, because pdftotext measures a line of mathematics from the top of the
// superscripts to the bottom of the subscripts. A page of proofs has a
// typical line height of twenty one points and sets its text thirteen points
// apart, and the height is the wrong unit to count white space in.
func rowsOn(p poppler.Layout) ([]float64, float64) {
	var ys, hs []float64
	for _, l := range p.Lines() {
		if len(l.Words) == 0 {
			continue
		}
		ys = append(ys, l.YMid())
		hs = append(hs, l.Box.Height())
	}
	if len(ys) < 2 {
		return nil, 0
	}
	slices.Sort(ys)
	// Two lines are the same row when they overlap, which is what half a line
	// height apart means, so the height is still what the grouping is done by.
	h := median(hs)
	var rows []float64
	for _, y := range ys {
		if len(rows) == 0 || y-rows[len(rows)-1] > h/2 {
			rows = append(rows, y)
		}
	}
	if len(rows) < 2 {
		return rows, 0
	}
	var gaps []float64
	for i := 1; i < len(rows); i++ {
		gaps = append(gaps, rows[i]-rows[i-1])
	}
	return rows, median(gaps)
}

// Lines returns the text lines of a page with the furniture taken out, in
// reading order.
func (f *Furniture) Lines(p poppler.Layout) []poppler.TextLine {
	var out []poppler.TextLine
	for _, l := range Lines(p) {
		if len(l.Words) == 0 || f.Is(p, l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// Count is how many distinct running heads were learned, for the report. A
// paper that learned none and has a header on every page is a paper whose
// pages were handed in one at a time.
func (f *Furniture) Count() int {
	if f == nil {
		return 0
	}
	return len(f.repeated)
}

// Printed is the page number a page prints, as it printed it, and empty for
// a page that prints none.
//
// It is read off the page before the furniture is stripped, because the
// folio is furniture and is gone by the time anything else looks. The
// toolchain wants it for the page map: a paper pulled out of a journal
// starts at 483 and its PDF starts at 1, and the reading app shows the
// reader the number that is on the paper.
func (f *Furniture) Printed(p poppler.Layout) string {
	if f == nil {
		return ""
	}
	return f.folios[p.Number]
}

// foliosOf works out which of the numbers in the margins are page numbers.
//
// A bare number in the margin is not enough on its own. A two column paper
// sets its footnotes at the foot of the columns and a footnote marker is a
// bare number in the bottom margin, which is how this used to read pages 3
// to 6 of the BERT paper as printing 4, 6, 8 and 10. That paper prints no
// page numbers anywhere, and the map built from those four markers put its
// first page at page 3 of a paper with no page 3.
//
// What separates a folio from a footnote marker is that a folio counts with
// the file: the number on the page after this one is this one plus one. So
// every margin number on every page is a candidate, the offsets between the
// candidates and the page they are on are counted, and the offset the most
// pages agree on wins. A page keeps the candidate that agrees with it and
// drops the rest, which is also what picks the folio out of a page that
// prints a folio and a footnote marker both.
//
// How many pages have to agree is the same third of the paper that a
// running head has to be on, and for a related reason. Two agreeing is not
// enough here: a paper with a footnote on page nine and the next footnote on
// page ten has two markers a page apart, which is exactly what a folio looks
// like, and that pair should not be allowed to number a sixteen page paper.
// A folio is on nearly every page of the paper that has one, so a third is
// already generous.
func foliosOf(pages []poppler.Layout, frames map[int]frame) map[int]string {
	type candidate struct {
		text   string
		offset int
	}
	found := map[int][]candidate{}
	agree := map[int]int{}
	for _, p := range pages {
		for _, l := range Lines(p) {
			if _, ok := frames[p.Number].key(p, l); !ok {
				continue
			}
			s := strings.TrimSpace(l.Text())
			n, ok := counts(s)
			if !ok {
				continue
			}
			found[p.Number] = append(found[p.Number], candidate{s, n - p.Number})
			agree[n-p.Number]++
		}
	}
	best, most := 0, 0
	for offset, n := range agree {
		// Ties go to the smaller offset, so that a run over a paper twice
		// gives the same answer twice. Ranging over a map does not.
		if n > most || (n == most && offset < best) {
			best, most = offset, n
		}
	}
	if most < max(len(pages)/3, 2) {
		return nil
	}
	out := map[int]string{}
	for page, cs := range found {
		for _, c := range cs {
			if c.offset == best {
				out[page] = c.text
				break
			}
		}
	}
	return out
}

// counts is the value of a margin number for the purpose of counting pages
// with. Roman numerals count, because the front matter of a thesis is
// numbered in them, and they are still not what PrintedNumber gives back: a
// page printed "iv" prints "iv" and not 4.
func counts(s string) (int, bool) {
	s = strings.Trim(s, "-[]() ")
	if !folio.MatchString(s) {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	return roman(s)
}

// roman reads a roman numeral. It is here for the front matter of a thesis,
// which is the one place in the hundred papers they are used for pages, and
// it is deliberately strict: "iiii" is four to a lenient reader and is a
// misread scan to this one.
func roman(s string) (int, bool) {
	values := map[rune]int{'i': 1, 'v': 5, 'x': 10, 'l': 50, 'c': 100, 'd': 500, 'm': 1000}
	// The five, the fifty and the five hundred are written once or not at
	// all. Nobody ever wrote "vv" for ten, and a scan that reads one is a
	// scan that read something else.
	once := map[rune]bool{'v': true, 'l': true, 'd': true}
	n, last, repeat := 0, 0, 0
	for _, r := range reverse(strings.ToLower(s)) {
		v, ok := values[r]
		if !ok {
			return 0, false
		}
		switch {
		case v == last:
			repeat++
			if repeat > 2 || once[r] {
				return 0, false
			}
			n += v
		case v < last:
			// The subtractive pair, which is only ever the next two symbols
			// up: iv and ix and not il, and never twice over.
			if v*10 < last {
				return 0, false
			}
			n -= v
		default:
			repeat = 0
			n += v
		}
		if v >= last {
			last = v
		}
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

// reverse is a string back to front, which is how a roman numeral is read:
// a symbol is subtracted when something bigger came after it.
func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

var folio = regexp.MustCompile(`^[-\[\(]?\s*(?:[0-9]{1,4}|[ivxlcdmIVXLCDM]{1,7})\s*[-\]\)]?$`)

// key is how a line is recognised on another page: what it says, with the
// numbers folded out, and roughly where it sits. It returns false for a line
// that is inside the frame, which is to say not in the margin at all.
//
// The numbers are folded because a running head that reads "483" on one page
// reads "484" on the next and is the same piece of furniture, and because a
// header that carries the volume and the page carries both in one line.
func (fr frame) key(p poppler.Layout, l poppler.TextLine) (string, bool) {
	if p.Height <= 0 {
		return "", false
	}
	y := l.YMid() / p.Height
	if y > fr.top && y < fr.bottom {
		return "", false
	}
	text := fold(l.Text())
	if text == "" {
		return "", false
	}
	// Four bands down each margin. Finer than this and a header that moves by
	// a point between pages stops matching itself; coarser and a header and
	// the first line of the body land in the same band.
	band := int(y / (margin / 4))
	return text + "\x00" + string(rune('a'+band)), true
}

// fold reduces a line to what makes it the same line on another page: lower
// case, one space between words, every run of digits replaced by a hash, and
// no punctuation.
func fold(s string) string {
	var b strings.Builder
	digit := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsDigit(r):
			if !digit {
				b.WriteByte('#')
				digit = true
			}
		case unicode.IsLetter(r):
			b.WriteRune(r)
			digit = false
		case unicode.IsSpace(r):
			b.WriteByte(' ')
			digit = false
		default:
			digit = false
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
