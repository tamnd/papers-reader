package extract

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
	"github.com/tamnd/papers-reader/poppler"
)

// A Paragraph is a run of lines that belong to one another.
//
// It is the unit the rest of the toolchain works in. A line is a typesetting
// accident, a page is where the paper ran out of room, and a paragraph is
// something the author wrote.
type Paragraph struct {
	poppler.Box
	Text string
	// Lines is how many lines of the page it was set on, which is what tells
	// a heading from a paragraph that happens to be short.
	Lines int
	// Column counts from zero at the left of the page. A
	// paragraph that ends at the foot of a column and one that starts at the
	// head of the next are the same paragraph more often than not, and the
	// assembler needs to know that it changed.
	Column int
	// Continues is set when the paragraph ends without terminal punctuation,
	// which is how the assembler is told to consider joining it to what comes
	// next. It is recorded here rather than worked out later because the
	// hyphen at the end of the last line is healed by then.
	Continues bool
	// Hyphen is set when the join to the next paragraph would have to heal a
	// word broken across the break. A paragraph that ends mid word is a
	// continuation and there is nothing to decide.
	Hyphen bool
}

// A Page is one page of a paper after the geometry has been read, the
// furniture taken out and the columns put in order.
type Page struct {
	Number  int
	Columns int
	// Printed is the page number the page itself prints, as it printed it,
	// and empty when it prints none. It is kept as text and not as an integer
	// because the front matter of a thesis prints roman numerals and because
	// a folio that came back as "48I" from a scan is worth seeing.
	Printed    string
	Paragraphs []Paragraph
}

// Text is the page as the page file on disk: one paragraph per line, blank
// lines between them.
//
// One line per paragraph rather than the wrapping the journal used, because
// the wrapping is the journal's and because a Markdown file whose lines are
// wrapped at the column width of a 1967 proceedings is a file every future
// edit has to rewrap. The assembler joins these across pages.
func (p Page) Text() string {
	parts := make([]string, len(p.Paragraphs))
	for i, par := range p.Paragraphs {
		parts[i] = par.Text
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// PrintedNumber is the page's own number as an integer, for the page map.
// The bool is false for a page that printed none and for the roman numerals
// of a thesis's front matter, which carry no offset worth learning.
func (p Page) PrintedNumber() (int, bool) {
	n, err := strconv.Atoi(strings.Trim(p.Printed, "-[]() "))
	if err != nil {
		return 0, false
	}
	return n, true
}

// Empty reports whether the page carried no text at all, which on the native
// path means a plate, a blank verso, or a page whose text layer is an image
// and whose paper should not have been on this path.
func (p Page) Empty() bool { return len(p.Paragraphs) == 0 }

// Read turns one page of geometry into paragraphs.
//
// The furniture may be nil, in which case nothing is stripped, which is what
// a caller reading a single page in isolation gets and is the honest result:
// a running head that was not learned is better in the text than a body line
// deleted because it looked like one.
func Read(p poppler.Layout, f *Furniture) Page {
	cuts := Gutters(p)
	out := Page{Number: p.Number, Columns: len(cuts) + 1, Printed: Printed(p)}
	lines := f.Lines(p)
	if len(lines) == 0 {
		return out
	}

	pitch := medianPitch(lines)
	var cur []poppler.TextLine
	flush := func() {
		if len(cur) == 0 {
			return
		}
		out.Paragraphs = append(out.Paragraphs, join(cur, cuts))
		cur = nil
	}
	for i, l := range lines {
		if i > 0 && breaks(lines[i-1], l, pitch, cuts) {
			flush()
		}
		cur = append(cur, l)
	}
	flush()
	return out
}

// breaks says whether a new paragraph starts at this line.
//
// Three reasons, and they are in the order of how much they can be trusted:
// the column changed, the vertical gap is bigger than the paper's leading, or
// the line is indented past the left edge of the one above it. The second is
// measured against the paper's own line pitch and not against the gap between
// the boxes, because the gap between two lines of the same paragraph is a
// point and a half in a 1967 proceedings and four in a modern preprint, and a
// threshold on that number is a threshold on the typeface. The third is
// the weakest of the three and it is what a paper that marks its paragraphs
// with a first line indent and no extra leading needs, which is most of the
// hundred. A paper that marks them neither way is read as one paragraph per
// column, and the assembler puts it back together from the punctuation.
func breaks(prev, l poppler.TextLine, pitch float64, cuts []float64) bool {
	if Column(prev, cuts) != Column(l, cuts) {
		return true
	}
	if pitch > 0 && l.YMin-prev.YMin > pitch*1.4 {
		return true
	}
	const indent = 4 // points, about two characters of a body face
	return l.XMin > prev.XMin+indent
}

// medianPitch is how far a page moves down for one line of text: the usual
// distance from one line's top to the next one's. It is the only reliable way
// to know what an unusual distance is, and it is measured and not assumed
// because a paper set in ten point on twelve and a double spaced thesis are
// both in the hundred.
//
// A pair that runs backwards or halfway down the page is the jump from the
// foot of one column to the head of the next and is not a line of text.
//
// A pair that barely moves at all is one row of the page that came back in
// pieces. pdftotext splits a row wherever the pieces are set at different
// sizes, which is what a figure drawn out of type does to every row of
// itself, and the tops of those pieces then sit a thousandth of a point
// apart. Counted as line pitches they are most of the page: the appendix
// pages of the Transformer paper measure a pitch of two thousandths of a
// point and then read each line of the figure's caption as a paragraph of
// its own, which truncates the caption at its first line. So a step that
// covers less of a line than minStep is not a step to the next line.
//
// A page with only a gap or two on it has no median worth the name: one gap
// is always its own median, so a page of two paragraphs set well apart would
// measure its own paragraph break as the normal pitch and come out as one
// paragraph. When the measurement lands at more than three times the height
// of a line of text it is not a line pitch, and the text's own height is the
// better estimate. Three because nothing in the hundred is set looser than
// double spaced, which measures a little over twice the height.
func medianPitch(lines []poppler.TextLine) float64 {
	var pitches, heights []float64
	for i, l := range lines {
		if h := l.Height(); h > 0 {
			heights = append(heights, h)
		}
		if i == 0 {
			continue
		}
		prev := lines[i-1]
		d := l.YMin - prev.YMin
		if d > 0 && d < 60 && d >= minStep*min(prev.Height(), l.Height()) {
			pitches = append(pitches, d)
		}
	}
	height := median(heights)
	pitch := median(pitches)
	if height > 0 && (pitch == 0 || pitch > height*3) {
		return height * 1.2
	}
	return pitch
}

// minStep is how far down the page a pair of lines has to step before the
// step counts as a line pitch, as a share of the shorter of the two. Half,
// because nothing is set tighter than solid and a pair of lines set solid
// steps by the full height of a line. Anything under that is two pieces of
// one row and the distance between them is rounding.
const minStep = 0.5

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

// join makes one paragraph out of a run of lines, healing the hyphens.
func join(lines []poppler.TextLine, cuts []float64) Paragraph {
	p := Paragraph{Box: lines[0].Box, Lines: len(lines), Column: Column(lines[0], cuts)}
	var b strings.Builder
	for i, l := range lines {
		p.Box = union(p.Box, l.Box)
		text := strings.TrimSpace(l.Text())
		if i == 0 {
			b.WriteString(text)
			continue
		}
		switch s := b.String(); {
		case broken(s, text):
			b.Reset()
			b.WriteString(strings.TrimRight(s, hyphens))
			b.WriteString(text)
		case hyphenated(s):
			// A compound the author wrote that the typesetter happened to
			// break at its own hyphen. The hyphen stays and so does the join:
			// a space after it would leave "Newton- Raphson", which is not
			// what is on the page and is not a word.
			b.WriteString(text)
		default:
			b.WriteByte(' ')
			b.WriteString(text)
		}
	}
	p.Text = strings.TrimSpace(b.String())
	p.Hyphen = strings.HasSuffix(p.Text, "-")
	p.Continues = p.Hyphen || !closed(p.Text)
	return p
}

// hyphens is every character a line break hyphen is written with: the plain
// one, the Unicode hyphen, the non-breaking one and the soft one that a PDF
// producer sometimes leaves in the text layer. It is package assemble's set
// rather than a second copy of it, because a word broken at a line end and
// the same word broken across a page have to come out spelled the same way.
const hyphens = assemble.Hyphens

// hyphenated says whether a line ends in a hyphen at all, healed or not.
func hyphenated(s string) bool { return assemble.Hyphenated(s) }

// broken says whether a word was split across these two lines.
//
// The rule is a hyphen at the end and a lower case letter at the start, and
// it is kept that narrow on purpose. A line that ends in a dash and is
// followed by a capital is a compound the author wrote, and healing it turns
// "Newton-Raphson" set across a line break into "NewtonRaphson" and, worse,
// turns a paper's own hyphenated term into two different spellings in the
// same document.
// There is one more case and it is worth the four lines. A compound that is
// broken at one of its own hyphens puts a lower case letter after the break,
// so the capital test does not save it: "English-to-German" set across a
// line comes back as "English-" and "to-German", and healing it gives
// "Englishto-German", which is in the abstract of the Transformer paper. A
// fragment that carries a hyphen of its own is the rest of a compound and
// not the rest of a word.
func broken(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	l := []rune(left)
	if !strings.ContainsRune(hyphens, l[len(l)-1]) {
		return false
	}
	if !unicode.IsLower([]rune(right)[0]) {
		return false
	}
	word, _, _ := strings.Cut(right, " ")
	return !strings.ContainsAny(word, hyphens)
}

// closed says whether a paragraph ends where a sentence ends. A closing
// quote or bracket after the stop counts, and a full stop after an initial
// does not get as far as here because it is not at the end of a paragraph.
func closed(s string) bool {
	s = strings.TrimRight(s, `"'”’)]}`)
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '.', '!', '?', ':', ';':
		return true
	}
	return false
}

func union(a, b poppler.Box) poppler.Box {
	return poppler.Box{
		XMin: min(a.XMin, b.XMin),
		YMin: min(a.YMin, b.YMin),
		XMax: max(a.XMax, b.XMax),
		YMax: max(a.YMax, b.YMax),
	}
}
