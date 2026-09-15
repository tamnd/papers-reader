// Package classify measures what a PDF's text layer is worth.
//
// It measures rather than guesses. The year a paper was published is a good
// hint and it is only a hint: there are 1980 papers with a clean text layer
// because somebody retypeset them, and there are 2005 papers that are
// photographs of a printout. So this reads the file.
//
// The answer is two things. The text layer is what the file's own text is
// worth, and the path is which extraction machinery will be pointed at it.
// They are recorded separately because the second one changes when the
// toolchain changes and the first one never does.
package classify

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/poppler"
)

// Layer is what the file's own text is worth.
type Layer string

const (
	// Native is real text and real mathematics. Nothing is guessed.
	Native Layer = "native"
	// Digital is real text whose mathematics comes out as mush, or a paper
	// with figures that have to be placed, or both.
	Digital Layer = "digital"
	// OCR is a scan somebody has already run OCR over. The headings are
	// legible, the mathematics is not, and neither is trustworthy.
	OCR Layer = "ocr"
	// None is a scan with no text layer at all.
	None Layer = "none"
)

// Path is which extraction machinery the paper goes to.
type Path string

const (
	// PathNative is pdftotext and nothing else. The only text in the corpus
	// that no model ever guessed at.
	PathNative Path = "native"
	// PathLayout sends page geometry to a layout model, which is what finds
	// figure bounding boxes and rebuilds mangled mathematics.
	PathLayout Path = "layout"
	// PathVision sends a page image to a vision model, because there is
	// nothing else to send.
	PathVision Path = "vision"
)

// Path is the extraction path a measured text layer sends a paper down.
//
// The mapping lives here rather than in each stage that needs it because
// three of them need it: the extractor picks the reader, the rasteriser
// decides whether the paper has any use for a picture of itself, and the
// coverage report counts papers per path. Three copies of a four line switch
// is three places for the answer to drift, and the one that drifts is always
// the one nobody was looking at.
//
// A layer nobody has measured yet maps to nothing, which is not the same as
// mapping to the safest path: a paper that has not been classified should
// stop a run and say so.
func (l Layer) Path() Path {
	switch l {
	case Native:
		return PathNative
	case Digital:
		return PathLayout
	case OCR, None:
		return PathVision
	}
	return ""
}

// Thresholds. Every one of these was set by running the measurement over
// real papers rather than by choosing a round number, and the numbers that
// justify them are in the tests.
const (
	// MinChars is the characters per page below which there is no text layer
	// worth the name. A scan with no OCR gives a handful of characters per
	// page from the odd stamp or watermark.
	MinChars = 200
	// MinMaths is the mathematical glyphs per page below which a paper that
	// clearly contains mathematics is not extracting it. Attention Is All You
	// Need scores 4.9 on a born digital file. The 1936 Turing scan, which is
	// nothing but mathematics, scores 0.06, because the glyphs came out as
	// "e^S, S3, a" and similar.
	MinMaths = 1.0
	// Covered is how much of a page one image has to fill before the page is
	// a scan rather than a page with a picture on it. A scanned page is
	// slightly smaller than the sheet it came from, so this is not 1.
	Covered = 0.8
	// MathShare is how much of a paper's fonts have to be mathematical before
	// it is taken to contain mathematics. One symbol font out of eight is a
	// paper that sets its bullet points in CMSY, and the MapReduce paper
	// carries ten of them across sixty and has no equations in it at all.
	// The papers that really are mathematical run from a third to two thirds.
	MathShare = 0.25
)

// Measurement is what was counted. It is a plain struct with no methods that
// touch a disk, so the rule that turns it into an answer can be tested
// without a PDF, and so a person can read the numbers that produced a verdict.
type Measurement struct {
	Pages int
	// First and Last are the band of pages that was sampled.
	First, Last int
	// Chars is the non whitespace characters in the band.
	Chars int
	// Maths is the glyphs in the band that only turn up when mathematics has
	// extracted correctly: Greek, the operators and the relation signs.
	Maths int
	// MathFonts is how many of the fonts in the band are mathematical. This
	// is the other half of the mathematics test: glyphs missing from a paper
	// with no mathematics in it means nothing, and glyphs missing from a
	// paper carrying CMMI and MSBM means the extraction is dropping them.
	MathFonts int
	// Fonts is how many fonts the band uses, and Embedded is how many of them
	// the file carries inside itself. That second number is the scan test. A
	// paper typeset by its author embeds subsets of the fonts it was set in,
	// with names like LICAEO+CMMI10. An OCR layer written over a photograph
	// embeds nothing and names Times-Roman, because the words are not really
	// there and whatever the reader has to hand will do to hold them.
	Fonts, Embedded int
	// Bitmap is how many of the fonts are nameless Type 3 bitmaps. See
	// Transliterated for what that does to the text.
	Bitmap int
	// Unmapped is how many fonts have no ToUnicode map. This is reported
	// because it is worth seeing and it decides nothing: pdfTeX omits the map
	// on a font with a builtin encoding, and half the born digital papers in
	// the corpus have no ToUnicode map on any font and extract perfectly.
	Unmapped int
	// Scans is how many band pages are one image covering the page.
	Scans int
	// Captions is how many figure and table captions were found. Counting
	// captions rather than images is deliberate: most figures in a paper
	// written in LaTeX are vector drawings and are not images at all, so
	// pdfimages reports nothing for a paper full of diagrams.
	Captions int
}

// BandPages is how many pages were sampled.
func (m Measurement) BandPages() int {
	if m.Last < m.First {
		return 0
	}
	return m.Last - m.First + 1
}

// Mathematical reports whether the paper really contains mathematics.
//
// Carrying one mathematical font is not enough. A systems paper sets its
// bullet points in CMSY and its section numbers in CMR and has no equation
// anywhere, and treating that as evidence of mathematics sends every such
// paper down the expensive path with a reason that is not true. So this asks
// what share of the fonts are mathematical, which is the difference between
// a paper that uses a symbol and a paper that is written in symbols.
func (m Measurement) Mathematical() bool {
	return m.Fonts > 0 && float64(m.MathFonts)/float64(m.Fonts) >= MathShare
}

// Transliterated reports whether the file is set entirely in nameless Type 3
// bitmap fonts, which means the text pdftotext prints is not the text the
// paper set.
//
// A bitmap font holds little pictures of glyphs and no name to say which
// glyph is which, so a reader has nothing to map a character code to and
// falls back on printing the code. What comes out is ASCII and it is the
// wrong ASCII: the Razborov paper writes the set "{x : not x}" and the file
// gives up "f x : : x g", because those are the CMSY positions of the brace,
// the colon and the negation. It reads as text, it passes every test for
// being text, and it is a transliteration of the shapes on the page.
//
// Every font and not most of them. Two papers in the corpus set a diagram
// label or a logo in a nameless Type 3 and are otherwise perfectly readable,
// and sending those down the expensive path for a caption would be paying
// for nothing. A file where there is no other kind of font is a file from
// the years when TeX was published through dvips, and all of it is pictures.
func (m Measurement) Transliterated() bool { return m.Fonts > 0 && m.Bitmap == m.Fonts }

// Verdict is the answer, and why.
type Verdict struct {
	Layer Layer
	Path  Path
	Why   string
}

// Classify turns a measurement into an answer.
//
// The order of the tests is the whole thing. A scan is a scan whatever its
// OCR layer says, a paper with no text cannot be read with pdftotext however
// modern it is, and native is what is left when nothing else is wrong. When
// in doubt the answer is the more expensive path, because the cost of
// getting this wrong in the cheap direction is a corpus full of "e^S, S3, a"
// presented as though somebody had read it.
func (m Measurement) Classify() Verdict {
	pages := m.BandPages()
	if pages == 0 {
		return Verdict{None, PathVision, "there was nothing to sample"}
	}
	perPage := m.Chars / pages

	switch {
	case perPage < MinChars:
		return Verdict{None, PathVision, "there is no text layer, only pictures of text"}
	case m.Scans*2 >= pages:
		return Verdict{OCR, PathVision, "the pages are images with an OCR layer over them, so the text is somebody's reading of a scan and not the paper"}
	case m.Fonts > 0 && m.Embedded == 0:
		return Verdict{OCR, PathVision, "the file embeds none of the fonts it names, which is what an OCR layer written over a photograph looks like from the outside"}
	case m.Transliterated():
		return Verdict{Digital, PathLayout, "every font in the file is a bitmap with no name, so what pdftotext prints is each glyph's position in its font and not the character the paper set"}
	case m.Mathematical() && float64(m.Maths)/float64(pages) < MinMaths:
		return Verdict{Digital, PathLayout, "the mathematical fonts are in the file and the mathematical glyphs are not in the text, so the extraction is mangling the mathematics"}
	case m.Captions > 0:
		return Verdict{Digital, PathLayout, "the text extracts cleanly and there are figures to find bounding boxes for"}
	}
	return Verdict{Native, PathNative, "the text and the mathematics both extract cleanly and there are no figures to place"}
}

// Band is the pages worth measuring.
//
// The first page is the title, the authors and the abstract, and the last
// few are the bibliography. Neither is representative: a title page is mostly
// large type and a bibliography is mostly names, and a paper can have a
// perfect title page and an unreadable body. What is left is sampled up to
// the width, because measuring seventy pages to answer a question that twelve
// answers is seventy processes where twelve would do.
//
// How much of the end to drop scales with the length, because a bibliography
// is a share of a paper and not a fixed number of pages. Dropping three from
// an eight page conference paper throws away a third of the body along with
// the figures in it, which is how the P4 paper came out as having no figures
// when it has six.
func Band(pages, width int) (first, last int) {
	if pages <= 3 {
		return 1, pages
	}
	first, last = 2, pages-refPages(pages)
	if width > 0 && last-first+1 > width {
		// Take the band from the middle, where the body is densest.
		middle := (first + last) / 2
		first = middle - width/2
		last = first + width - 1
	}
	return first, last
}

// refPages is how many pages at the end are bibliography. One page on a
// conference paper, three on a journal article, six on something book
// length, which is roughly what the shelf says.
func refPages(pages int) int {
	if n := pages / 12; n > 1 {
		return n
	}
	return 1
}

// DefaultBand is how many pages are sampled. Twelve is enough that one odd
// page cannot swing the answer and few enough that the whole corpus measures
// in a couple of minutes.
const DefaultBand = 12

// Measure reads a PDF and counts everything Classify needs.
func Measure(ctx context.Context, path string) (Measurement, Verdict, error) {
	doc, err := poppler.Info(ctx, path)
	if err != nil {
		return Measurement{}, Verdict{}, err
	}
	first, last := Band(doc.Pages, DefaultBand)
	m := Measurement{Pages: doc.Pages, First: first, Last: last}

	text, err := poppler.Page(ctx, path, first, last)
	if err != nil {
		return m, Verdict{}, err
	}
	m.Chars, m.Maths = count(text)
	m.Captions = len(caption.FindAllString(text, -1))

	fonts, err := poppler.FontList(ctx, path, first, last)
	if err != nil {
		return m, Verdict{}, err
	}
	for _, f := range fonts {
		m.Fonts++
		if f.Embedded {
			m.Embedded++
		}
		if !f.Unicode {
			m.Unmapped++
		}
		if MathFont(f.Name) {
			m.MathFonts++
		}
		if f.Bitmap() {
			m.Bitmap++
		}
	}

	images, err := poppler.ImageList(ctx, path, first, last)
	if err != nil {
		return m, Verdict{}, err
	}
	m.Scans = scans(images, doc)

	return m, m.Classify(), nil
}

// count is the characters and the mathematical glyphs in a stretch of text.
func count(text string) (chars, maths int) {
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		chars++
		if IsMath(r) {
			maths++
		}
	}
	return chars, maths
}

// mathRunes is the set of glyphs that only appear when mathematics has come
// out of a PDF correctly. Greek, the operators, the relations, the set
// theory, and the brackets that are not ordinary brackets.
//
// Latin letters are not in it and neither are digits, because "x = 2" is
// mathematics and extracts fine from almost anything, so counting it would
// measure nothing.
const mathRunes = "∑∏∫∮√∞≤≥≠≈≡∼≃≅±×÷∈∉∋⊂⊆⊃⊇∪∩∀∃∄¬∧∨⊕⊗⊥∅∂∇→←↔⇒⇔↦∘·∙′″‖⟨⟩⌈⌉⌊⌋ℝℕℤℚℂℵ"

// IsMath reports whether a rune is one of the glyphs that only a correct
// mathematical extraction produces.
func IsMath(r rune) bool {
	switch {
	case r >= 'α' && r <= 'ω', r >= 'Α' && r <= 'Ω':
		return true
	case strings.ContainsRune(mathRunes, r):
		return true
	}
	return false
}

// mathFont matches the font families mathematics is set in. Computer Modern
// maths and symbols, the AMS families, and the Unicode maths fonts that came
// later.
var mathFont = regexp.MustCompile(`(?i)(cmmi|cmsy|cmex|cmbsy|msam|msbm|eusm|eufm|eufb|rsfs|stixmath|xitsmath|asanamath|latinmodernmath|lmmath|texgyre\w*math|cambriamath|mathematicalpi|symbol|mtmi|mtsy|mathpack)`)

// MathFont reports whether a font name is a mathematical family.
func MathFont(name string) bool {
	// Subset prefixes look like "LICAEO+CMMI10". The prefix is random and
	// carries nothing, so it goes.
	if i := strings.IndexByte(name, '+'); i >= 0 {
		name = name[i+1:]
	}
	return mathFont.MatchString(name)
}

// caption matches the line a figure or a table starts with. It is anchored to
// the start of a line so that a sentence mentioning Figure 3 does not count
// as a figure.
var caption = regexp.MustCompile(`(?mi)^[ \t]*(figure|fig\.|table|algorithm)[ \t]+\d+`)

// scans counts the band pages that are one image the size of the page.
//
// Pixel dimensions on their own say nothing: 2288 pixels across is a full
// page at 400 dots per inch and a thumbnail at 4000. So this converts back to
// points through the density poppler reports and compares that to the page.
func scans(images []poppler.Image, doc poppler.Doc) int {
	if doc.Width <= 0 || doc.Height <= 0 {
		return 0
	}
	pages := map[int]bool{}
	for _, img := range images {
		if img.XPPI <= 0 || img.YPPI <= 0 {
			continue
		}
		w := float64(img.Width) / float64(img.XPPI) * 72
		h := float64(img.Height) / float64(img.YPPI) * 72
		if w/doc.Width >= Covered && h/doc.Height >= Covered {
			pages[img.Page] = true
		}
	}
	return len(pages)
}
