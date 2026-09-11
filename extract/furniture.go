package extract

import (
	"regexp"
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
}

// FindFurniture learns what is furniture from the pages it is given.
//
// Give it the whole paper, or a sample spread across the paper. A run of
// consecutive pages from the middle would learn the running head correctly
// and learn nothing about the first page, which is the page that has a
// different one.
func FindFurniture(pages []poppler.Layout) *Furniture {
	f := &Furniture{repeated: map[string]bool{}, pages: len(pages)}
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
			k, ok := key(p, l)
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
	k, ok := key(p, l)
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
func Printed(p poppler.Layout) string {
	for _, l := range Lines(p) {
		if _, ok := key(p, l); !ok {
			continue
		}
		if s := strings.TrimSpace(l.Text()); folio.MatchString(s) {
			return s
		}
	}
	return ""
}

var folio = regexp.MustCompile(`^[-\[\(]?\s*(?:[0-9]{1,4}|[ivxlcdmIVXLCDM]{1,7})\s*[-\]\)]?$`)

// key is how a line is recognised on another page: what it says, with the
// numbers folded out, and roughly where it sits. It returns false for a line
// that is not in the margin at all.
//
// The numbers are folded because a running head that reads "483" on one page
// reads "484" on the next and is the same piece of furniture, and because a
// header that carries the volume and the page carries both in one line.
func key(p poppler.Layout, l poppler.TextLine) (string, bool) {
	if p.Height <= 0 {
		return "", false
	}
	y := l.YMid() / p.Height
	if y > margin && y < 1-margin {
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
