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
	// Fenced says the text is a code fence and every character in it is
	// literal. Nothing that rewrites prose may touch it, starting with the
	// dollar escaping: a dollar inside a fence is printed as a dollar and a
	// backslash in front of it would be printed too.
	Fenced bool
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
		if !par.Fenced {
			parts[i] = escapeDollars(par.Text)
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// escapeDollars writes every dollar sign on a natively read page as \$.
//
// This path produces no TeX. pdftotext hands over the glyphs the PDF holds
// and nothing turns a run of them into a formula, so a dollar in the text is
// a dollar somebody printed and never a delimiter. Left bare it opens a math
// span: the ACM copyright line at the foot of the Paxos paper's first page
// reads "0000-0000/98/0000-0000 $00.00", and that one dollar sign was enough
// to fail acceptance rule A2 and stop the paper being extracted at all.
//
// The layout path is not this function's business. There a dollar is usually
// a delimiter the tool wrote on purpose, and escaping those would turn every
// formula in the paper into prose.
func escapeDollars(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return strings.ReplaceAll(s, "$", `\$`)
}

// Ligatures writes the Latin typographic ligatures out as the letters they
// stand for.
//
// A ligature is a decision the typesetter made about two letters that collide
// in a particular face, and a PDF from a Type 1 era journal puts the decision
// in the text layer: the Paxos paper's editorial note comes off pdftotext as
// "behind a ﬁling cabinet in the TOCS editorial oﬃce". Nothing downstream
// wants it. A search for "filing" misses it, a spell check flags it, a
// translator is handed a character it has no word for, and a reader who
// copies a sentence out of the corpus pastes a glyph that will not match
// anything. The author wrote "filing" and the corpus should say so.
//
// Only the six Latin ligatures in the alphabetic presentation forms block are
// folded. Æ and œ are letters in the languages that use them and not
// typesetting, and the Greek and Arabic presentation forms are somebody
// else's problem and not one this corpus has.
func Ligatures(s string) string {
	if !strings.ContainsFunc(s, isLigature) {
		return s
	}
	return ligatures.Replace(s)
}

func isLigature(r rune) bool { return r >= 'ﬀ' && r <= 'ﬆ' }

var ligatures = strings.NewReplacer(
	"ﬀ", "ff",
	"ﬁ", "fi",
	"ﬂ", "fl",
	"ﬃ", "ffi",
	"ﬄ", "ffl",
	"ﬅ", "st", // the long s ligature, which a 19th century reprint sets
	"ﬆ", "st",
)

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
	out := Page{Number: p.Number, Columns: len(cuts) + 1, Printed: f.Printed(p)}
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
	// The tables are found over the whole page first, because a table is
	// recognised by what is around it and the paragraph loop only ever sees
	// the line before. Their lines then leave the flow: a table read as prose
	// is a paragraph of the words in the order the text layer holds them,
	// with the columns interleaved, which is worse than no table at all.
	//
	// A table is written where its first line would have gone. Its other
	// lines are dropped from the flow wherever they turn up, which for a table
	// whose cells arrived one at a time is all over the page.
	tables := Tables(lines, cuts)
	owner := make(map[int]int, len(tables))
	for at, t := range tables {
		for _, i := range t.Lines {
			owner[i] = at
		}
	}
	for i := 0; i < len(lines); i++ {
		at, taken := owner[i]
		if taken && tables[at].Lines[0] == i {
			t := tables[at]
			flush()
			out.Paragraphs = append(out.Paragraphs, Paragraph{
				Box:    t.Box,
				Text:   t.Text,
				Lines:  len(t.Lines),
				Column: Column(lines[i], cuts),
				Fenced: t.Fenced,
			})
			continue
		}
		if taken {
			continue
		}
		if len(cur) > 0 && breaks(cur[len(cur)-1], lines[i], pitch, cuts) {
			flush()
		}
		cur = append(cur, lines[i])
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

func median(v []float64) float64 { return quantile(v, 0.5) }

// quantile is the value q of the way up a sample, and it sorts what it is
// given, which every caller here is done with by the time it asks.
func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	at := int(float64(len(v)) * q)
	return v[min(at, len(v)-1)]
}

// join makes one paragraph out of a run of lines, healing the hyphens.
func join(lines []poppler.TextLine, cuts []float64) Paragraph {
	p := Paragraph{Box: lines[0].Box, Lines: len(lines), Column: Column(lines[0], cuts)}
	var b strings.Builder
	for i, l := range lines {
		p.Box = union(p.Box, l.Box)
		text := Ligatures(strings.TrimSpace(l.Text()))
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
