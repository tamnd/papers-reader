package figures

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// A Caption is one caption of a paper and where it sits.
type Caption struct {
	Page int
	Box  poppler.Box
	// Kind is the word the caption opens with, normalised: figure, table,
	// algorithm or listing. A table caption is here because a table is not a
	// figure and must not be committed as one, and the only way to refuse a
	// region that is really a table is to recognise the caption under it.
	Kind string
	// Number is what the paper called it, as the paper wrote it, so a paper
	// that numbers its figures 3-a keeps the a.
	Number string
	Text   string
}

// caption is the opening of a caption: the word, then optionally a number.
//
// Deliberately not anchored to a particular numbering, because the hundred
// write Figure 1, Fig. 1, FIG. 1, Figure 1., Figure 1:, Fig 3-a and Table
// I, and a pattern that insisted on one of them would lose every figure of
// the papers that use another.
//
// The abbreviating full stop is outside the first group and after the word
// boundary, so that Fig. 3 keeps its number. Inside the group it takes the
// boundary with it, the number is never reached, and every paper that
// abbreviates loses the one thing the entry is indexed by.
//
// A number may be led by a letter as well as by a digit, because a paper
// that numbers by section numbers its appendix figures G.1 and G.2. Bare
// letters come last in the alternation so that Table IV is read as IV and
// not as I, and the sectioned letter comes before the bare one so that G.1
// is read whole.
var caption = regexp.MustCompile(`^(Fig(?:ure)?|FIG(?:URE)?|Table|TABLE|Algorithm|ALGORITHM|Listing|LISTING|Chart)\b\.?[ \t]*([0-9]+(?:[.\-][0-9a-zA-Z]+)*|[A-Z](?:[.\-][0-9a-zA-Z]+)+|[IVXLC]+|[A-Z])?[ \t]*[.:)]?`)

// Captions is every caption on a page, in reading order.
//
// It reads the paragraphs rather than the lines, because a caption is three
// lines of prose and the second and third do not start with the word
// Figure. The paragraph is what the assembler would have written and it is
// what gets translated.
//
// The lines are the page's own geometry and are used for the cases a
// paragraph on its own cannot answer, which are all of them a caption glued
// to something it is not part of. See twoUp, solo and buried. A caller with
// no geometry to hand may pass nil, and then a caption has to be the whole
// head of a paragraph to be found.
func Captions(page extract.Page, lines []poppler.TextLine) []Caption {
	var out []Caption
	for _, par := range page.Paragraphs {
		if cs := twoUp(par, page.Number, lines); len(cs) > 0 {
			out = append(out, cs...)
			continue
		}
		if c, ok := opens(par.Text, par.Box, page.Number); ok {
			// The paragraph may be the caption and then something the page
			// gave it that is not part of it. The box stays the paragraph's,
			// because that is what the region above it is grown from and the
			// measurements say it is the better seed, but the caption the
			// corpus files and shows is the caption.
			if l, ok := stops(par, lines); ok {
				c.Text = strings.TrimSpace(from(l.Words))
			}
			out = append(out, c)
			continue
		}
		if c, ok := solo(par, page.Number, lines); ok {
			out = append(out, c)
			continue
		}
		if c, ok := buried(par, page.Number, lines); ok {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Box.YMin < out[j].Box.YMin })
	return out
}

// opens reads a caption off the start of some text.
func opens(text string, area poppler.Box, page int) (Caption, bool) {
	m := caption.FindStringSubmatch(text)
	if m == nil {
		return Caption{}, false
	}
	// A sentence of the body that happens to begin "Figure 3 shows" is a
	// reference to a figure and not a caption. The test is the punctuation
	// the typesetter put after the number: a caption has some, a sentence
	// does not.
	if !separated(text, m[0], m[2]) {
		return Caption{}, false
	}
	return Caption{
		Page:   page,
		Box:    area,
		Kind:   kind(m[1]),
		Number: m[2],
		Text:   strings.TrimSpace(text),
	}, true
}

// twoUp reads the two captions of a pair of figures set side by side, and
// nothing at all for every other paragraph.
//
// A journal that sets two small figures across the measure sets a caption
// under each of them, and the two captions are one row of type with a gutter
// down the middle. pdftotext reads that row the way it reads any other, left
// to right across the gutter, so the paragraph comes out with the two
// captions interleaved a line at a time. The TPU paper's page 3 reads
// "Figure 1. TPU Block Diagram. The main computation part is the Figure 2.
// Floor Plan of TPU die." and carries on alternating for four more lines.
//
// Left as it is that costs two things. The paper has no figure 2, because
// nothing on the page begins with it, and rule F09 says so. And figure 1
// gets the whole row as its region, so the committed PNG is both diagrams
// and the committed caption is the interleaved text.
//
// The split has to be certain before it is made, because cutting a
// paragraph of prose in half would put half a sentence under each of two
// pictures. Two things always have to hold: no word crosses the gutter,
// which is what a gutter is and what no interior position in a set
// paragraph manages, and both halves open with a caption of their own
// carrying its own number.
//
// One of those is weak on a row of one line, where the gutter is any gap
// between two words. Such a row needs a third thing, which is that the gap
// is wider than a space. Appendix H of the GPT-3 paper sets a pair of one
// line captions with 88 points between them, ten times the height of the
// type they are set in.
func twoUp(par extract.Paragraph, page int, lines []poppler.TextLine) []Caption {
	inside := within(par.Box, lines)
	if len(inside) == 0 {
		return nil
	}
	for _, at := range openings(inside) {
		if len(inside) < 2 && !wide(inside, at) {
			continue
		}
		left, right, ok := divide(inside, at)
		if !ok {
			continue
		}
		lp, rp := extract.Join(left, nil), extract.Join(right, nil)
		cl, lok := opens(lp.Text, lp.Box, page)
		cr, rok := opens(rp.Text, rp.Box, page)
		if lok && rok && cl.Number != cr.Number {
			return []Caption{cl, cr}
		}
	}
	return nil
}

// openings is every x a caption starts at part way along a line, which is
// where the gutter of a two up row would be.
//
// Taken from the words rather than swept across the page, because the gutter
// between two captions is as narrow as the typesetter could make it. The one
// on page 3 of the TPU paper is three and a half points, which is less than
// two spaces of the face it is set in.
func openings(lines []poppler.TextLine) []float64 {
	var out []float64
	seen := make(map[float64]bool)
	for _, l := range lines {
		for i := 1; i < len(l.Words); i++ {
			if !caption.MatchString(from(l.Words[i:])) {
				continue
			}
			if x := l.Words[i].XMin; !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	return out
}

// from is the text of a line from some word on.
func from(words []poppler.Word) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

// wide says the white before the caption that starts at this x is wider
// than any space the typesetter set, measured in the type's own height.
func wide(lines []poppler.TextLine, at float64) bool {
	for _, l := range lines {
		for i := 1; i < len(l.Words); i++ {
			w := l.Words[i]
			if w.XMin != at {
				continue
			}
			if h := w.Height(); h > 0 && w.XMin-l.Words[i-1].XMax >= spread*h {
				return true
			}
		}
	}
	return false
}

// divide cuts every line of a paragraph at one x, and refuses if any word
// straddles the cut or if either side comes back empty.
func divide(lines []poppler.TextLine, at float64) (left, right []poppler.TextLine, ok bool) {
	for _, l := range lines {
		var a, b []poppler.Word
		for _, w := range l.Words {
			switch {
			case w.XMin < at && w.XMax > at:
				return nil, nil, false
			case w.XMin >= at:
				b = append(b, w)
			default:
				a = append(a, w)
			}
		}
		if len(a) > 0 {
			left = append(left, line(a))
		}
		if len(b) > 0 {
			right = append(right, line(b))
		}
	}
	return left, right, len(left) > 0 && len(right) > 0
}

// line is a run of words as a line, with the box they cover.
func line(words []poppler.Word) poppler.TextLine {
	b := words[0].Box
	for _, w := range words[1:] {
		b.XMin = min(b.XMin, w.XMin)
		b.YMin = min(b.YMin, w.YMin)
		b.XMax = max(b.XMax, w.XMax)
		b.YMax = max(b.YMax, w.YMax)
	}
	return poppler.TextLine{Box: b, Words: words}
}

// solo reads a caption that is a line to itself, which the assembler put in
// the same paragraph as whatever was set under it.
//
// This is the other half of what a caption of nothing but a number costs. The
// Gamma paper's page 15 sets "Figure 7" under the plate and the heading of
// section 3.5 below that, and there is no blank line between them that
// pdftotext can see, so the paragraph reads "Figure 7 3.5. Operating and
// Storage System". The word after the number is then a section number, which
// is not the opening of a caption and is not a sentence carrying on either,
// and the paper lost the figure with rule F09 naming it.
//
// The box here is the line and not the paragraph, because the paragraph is
// mostly the thing the caption is not part of. That is the other way round
// from the paragraph the caption does open, where the box stays the
// paragraph's. Both are what the crops came out best as.
func solo(par extract.Paragraph, page int, lines []poppler.TextLine) (Caption, bool) {
	l, ok := stops(par, lines)
	if !ok {
		return Caption{}, false
	}
	return opens(strings.TrimSpace(from(l.Words)), l.Box, page)
}

// stops is the first line of a paragraph when that line is a caption and the
// caption is finished on it, and there is more in the paragraph under it.
//
// Finished means the line is the caption's own opening and nothing else, and
// that it ends at the number. A paper that sets "Figure 1:" on a line and the
// description under it has a caption of two lines and the colon is the
// typesetter promising the second one, so taking the first line for the whole
// of it would throw the description away. A caption that ends at its number
// promises nothing.
//
// A line of a set paragraph runs the measure, so a line that is two words
// long and happens to be a cross reference is not something a typesetter
// produces.
func stops(par extract.Paragraph, lines []poppler.TextLine) (poppler.TextLine, bool) {
	inside := within(par.Box, lines)
	if len(inside) < 2 {
		return poppler.TextLine{}, false
	}
	head := strings.TrimSpace(from(inside[0].Words))
	m := caption.FindStringSubmatch(head)
	if m == nil || strings.TrimSpace(m[0]) != head || m[2] == "" || !strings.HasSuffix(head, m[2]) {
		return poppler.TextLine{}, false
	}
	return inside[0], true
}

// buried reads a caption that starts part way down a paragraph, because the
// lettering of the figure above it was read as the first line of the same
// paragraph.
//
// This is what happens to a plot with its axes labelled. The last row of the
// ResNet paper's figure 1 is "iter. (1e4) iter. (1e4)", the two axis labels
// side by side, and pdftotext puts them at the head of the paragraph the
// caption is in. Nothing on the page then begins with the word Figure, the
// paper has no figure 1, and rule F09 says so.
//
// The test for it is narrow, because a body paragraph with a line beginning
// "Figure 3. The..." is a cross reference that happened to fall after a line
// break, and reading that as a caption would put the paragraph after it
// inside a PNG. So every line above the one the caption starts on has to
// stand off the paragraph's own left edge. A line of set text does not: the
// measure is what makes it a paragraph, and the one line that may be
// indented is the first, which is never the line this is looking past.
func buried(par extract.Paragraph, page int, lines []poppler.TextLine) (Caption, bool) {
	inside := within(par.Box, lines)
	if len(inside) < 2 {
		return Caption{}, false
	}
	for i, l := range inside {
		if i == 0 {
			// The head of the paragraph, which opens has already read.
			continue
		}
		text, ok := tail(par.Text, l)
		if !ok {
			continue
		}
		c, ok := opens(text, cover(inside[i:]), page)
		if !ok {
			continue
		}
		if !lettered(par.Box, inside[:i]) {
			return Caption{}, false
		}
		return c, true
	}
	return Caption{}, false
}

// lettered says every one of these lines stands off the left edge, which is
// what the lettering of a figure does and what a line of set text does not.
func lettered(par poppler.Box, lines []poppler.TextLine) bool {
	if par.Width() <= 0 {
		return false
	}
	for _, l := range lines {
		if l.XMin-par.XMin < stand*par.Width() {
			return false
		}
	}
	return true
}

// stand is how far off the paragraph's left edge a line has to begin before
// it is the figure's lettering rather than the paper's text, as a share of
// the paragraph's width. A twentieth of a column is about three characters,
// which is wider than any paragraph indent this corpus sets and narrower
// than the margin any label sits at.
const stand = 0.05

// tail is the paragraph's own text from this line's first words on, and
// false when the line is not in it.
//
// Out of the paragraph rather than rebuilt from the lines, so that a word
// the assembler healed across a line break stays healed. The needle is the
// first few words, because the whole line does not survive that healing.
func tail(text string, l poppler.TextLine) (string, bool) {
	head := l.Words
	if len(head) > needle {
		head = head[:needle]
	}
	parts := make([]string, len(head))
	for i, w := range head {
		parts[i] = w.Text
	}
	at := strings.Index(text, strings.Join(parts, " "))
	if at <= 0 {
		return "", false
	}
	return strings.TrimSpace(text[at:]), true
}

// needle is how many words of a line are matched against the paragraph. Two
// is enough to find "Figure 1." and short enough to survive a paragraph
// whose third word was hyphenated across the break.
const needle = 2

// within is the lines of a page that sit inside a paragraph, in reading
// order.
func within(b poppler.Box, lines []poppler.TextLine) []poppler.TextLine {
	var out []poppler.TextLine
	for _, l := range lines {
		if l.YMid() >= b.YMin && l.YMid() <= b.YMax && l.XMid() >= b.XMin && l.XMid() <= b.XMax {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].YMin < out[j].YMin })
	return out
}

// cover is the rectangle a run of lines covers.
func cover(lines []poppler.TextLine) poppler.Box {
	b := lines[0].Box
	for _, l := range lines[1:] {
		b.XMin = min(b.XMin, l.XMin)
		b.YMin = min(b.YMin, l.YMin)
		b.XMax = max(b.XMax, l.XMax)
		b.YMax = max(b.YMax, l.YMax)
	}
	return b
}

// separated says whether what follows the number looks like a caption
// rather than a sentence carrying on.
//
// Three ways it can: the typesetter put a colon, a full stop, a dash or a
// parenthesis after the number, or nothing follows the number at all, or the
// paper sets its captions with nothing but space, in which case the word
// after the number starts a sentence with a capital. "Figure 3 shows" is
// none of them and is a cross reference in the body.
//
// Nothing following the number is the Gamma paper. It captions all nineteen
// of its figures with the word and the number and no description, so every
// caption on every plate reads "Figure 19" and stops. Read as a sentence
// carrying on, that is a sentence with nothing in it, and treating it as one
// cost the paper twelve of its figures: rule F09 named every one of them as
// mentioned in the prose with no picture to show, and the three that did get
// through were pairs set side by side that came out as one crop carrying
// both numbers, "Figure 12 Figure 13". A cross reference is a sentence about
// a figure and a sentence needs more words than this has.
//
// A number is required for that case and only that case. The number is
// optional in the pattern, so text that is the bare word Figure and nothing
// else would otherwise arrive here and be taken as a caption of no figure in
// particular.
//
// What follows the number is read after any rule drawn with type is taken off
// it. See undrawn.
func separated(text, head, number string) bool {
	if strings.ContainsAny(last(head), ".:)") {
		return true
	}
	rest := strings.TrimSpace(text[len(head):])
	if strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "—") {
		return true
	}
	rest = undrawn(rest)
	if rest == "" {
		return number != ""
	}
	word, _, _ := strings.Cut(rest, " ")
	return word != strings.ToLower(word)
}

// undrawn is the text with any rule the typesetter drew with type taken off
// the front of it.
//
// The Gamma paper is set in troff, and troff draws the line that separates a
// footnote from the text above it by repeating one character across the
// measure. pdftotext reads that line the way it reads a word, and where the
// rule falls beside a caption the two land in one paragraph: page 13 comes
// out as "Figure 5" and then the letter h thirty-eight times, and page 15 the
// same under figure 7. Read as the word after the number that is lowercase,
// so both captions were refused as cross references and the paper lost both
// figures with rule F09 naming them.
//
// Taken off the front and not out of the middle, because the only thing being
// decided here is what the first word after the number is.
func undrawn(text string) string {
	for {
		word, rest, _ := strings.Cut(text, " ")
		if !repeated(word) {
			return text
		}
		text = strings.TrimSpace(rest)
	}
}

// repeated says a word is one character over and over, which is how a rule is
// drawn with type and is not how a word is spelled.
func repeated(word string) bool {
	r := []rune(word)
	if len(r) < repeats {
		return false
	}
	for _, c := range r[1:] {
		if c != r[0] {
			return false
		}
	}
	return true
}

// repeats is how many times the character has to come round before the word
// is a rule. Four, because no word of any of the four languages the corpus
// holds spells the same letter four times over, and the shortest rule a paper
// draws runs the width of a column.
const repeats = 4

func last(s string) string {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return ""
	}
	return s[len(s)-1:]
}

func kind(word string) string {
	switch w := strings.ToLower(strings.TrimRight(word, ".")); w {
	case "fig", "figure":
		return "figure"
	default:
		return w
	}
}

// Pair attaches the nearest caption to each candidate region of one page.
//
// Below for preference and above otherwise, because papers caption their
// figures below and their tables above and are not consistent about either.
// The caption has to be within a couple of lines of the region: a figure at
// the top of a column and the first paragraph of prose under it are not a
// figure and its caption, and there is nothing else on the page to tell
// them apart.
//
// A candidate with no caption gets none, and the caller does not commit it.
// In practice an uncaptioned hole in a column is a display equation, a
// table, a decorative rule or a logo far more often than it is a figure
// somebody forgot to caption.
func Pair(cands []Candidate, caps []Caption, pitch float64) []Found {
	out := make([]Found, len(cands))
	taken := make([]bool, len(caps))
	for i, c := range cands {
		out[i] = Found{Candidate: c}
		best, at := 0.0, -1
		for j, text := range caps {
			if taken[j] || text.Page != c.Page || !beside(c.Box, text.Box) {
				continue
			}
			d := distance(c.Box, text.Box)
			if d > reach*pitch {
				continue
			}
			// Below wins over above at the same distance, which is what
			// sorts out a figure with a caption under it sitting directly
			// beneath a table with its caption above.
			if text.Box.YMin >= c.Box.YMax {
				d -= 0.01
			}
			if at < 0 || d < best {
				best, at = d, j
			}
		}
		if at >= 0 {
			taken[at] = true
			out[i].Caption = &caps[at]
		}
	}
	return out
}

// A Found is a region with whatever caption belongs to it, which is nothing
// for most of them.
type Found struct {
	Candidate
	Caption *Caption
}

// reach is how far from the region a caption may be, in lines. Two lines of
// white space is what a journal leaves between a figure and its caption;
// more than that and the thing below the hole is the next paragraph.
const reach = 3

// beside says whether a caption is horizontally where the region is. A
// caption in the right column is not the caption of a figure in the left
// one, however close the two are vertically.
func beside(region, cap poppler.Box) bool {
	return cap.XMid() >= region.XMin && cap.XMid() <= region.XMax
}

func distance(region, cap poppler.Box) float64 {
	switch {
	case cap.YMin >= region.YMax:
		return cap.YMin - region.YMax
	case cap.YMax <= region.YMin:
		return region.YMin - cap.YMax
	}
	// Overlapping, which happens when the caption is set inside the figure's
	// own white space. Nothing is closer than that.
	return 0
}
