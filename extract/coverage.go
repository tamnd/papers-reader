package extract

import (
	"regexp"
	"strings"
)

// Coverage is how well a model's reading of a page accounts for the page's own
// text layer, measured over the worst stretch of it, and the words of that
// stretch the reading does not have.
//
// It is the one check on a model's reading that does not depend on anybody's
// opinion of the answer. A born digital paper carries the words it was typeset
// from, and however badly pdftotext lays them out it has them all. So a page
// the model read correctly contains those words, in some order, and a page it
// read half of does not. Nothing else in the acceptance rules can see that.
// Rule A5 catches a page that came back short in characters, and a model that
// drops the last paragraph under a figure is not short by enough to notice.
//
// It found exactly that, which is why it exists. Page 5 of the Bitcoin paper
// ends with a paragraph set under the transaction diagram, the reader stopped
// at the diagram, and the paragraph was published as missing with a clean
// audit over it and nothing anywhere saying so.
//
// The measure is over a stretch and not over the page because over the page it
// does not separate. That dropped paragraph is a fifteenth of its page, and
// there are pages read perfectly well that lose a fifteenth of their words to
// mathematics alone: the text layer of the multi head attention page is full of
// tokens like "rdmodel" that no correct reading of it would ever contain. Whole
// page coverage puts the dropped paragraph at 0.93 and that page at 0.96, which
// is no gap at all. Over the worst twenty words the dropped paragraph is 0.30
// and that page is 0.55, because the mathematics is scattered through the page
// and a dropped paragraph is all in one place.
//
// Within a stretch the comparison is against the whole answer and not against
// the matching stretch of it, because the two things being compared do not
// agree about order and are not supposed to. pdftotext reads a two column page
// straight across and the model reads it column by column, which is the entire
// reason the model is being asked. Order is A6's business and the reading order
// tests', not this one's.
//
// Only words of four letters or more, and only letters. A short token is either
// a word so common that every page has it, which measures nothing, or a piece
// of mathematics, and the text layer's version of a formula is not the model's
// version of a formula and should not be.
func Coverage(layer, answer string) (float64, []string) {
	want := layerWords(layer)
	if len(want) == 0 {
		// Nothing to compare against is not a failure. A page that is one
		// full page figure has a text layer of a caption and a folio.
		return 1, nil
	}
	got := map[string]bool{}
	for _, w := range layerWords(answer) {
		got[w] = true
	}
	// A page with less than a stretch on it is judged as the one stretch it is.
	width := Window
	if len(want) < width {
		width = len(want)
	}
	worst, at := 1.0, 0
	for i := 0; i+width <= len(want); i++ {
		hits := 0
		for _, w := range want[i : i+width] {
			if got[w] {
				hits++
			}
		}
		if share := float64(hits) / float64(width); share < worst {
			worst, at = share, i
		}
	}
	var missing []string
	for _, w := range want[at : at+width] {
		if !got[w] {
			missing = append(missing, w)
		}
	}
	return worst, missing
}

// Window is how long a stretch of the text layer has to be before how much of
// it is missing means anything.
//
// Twenty words is about two lines of a one column paper and about three of a
// two column one, so it is short enough to be all inside a paragraph and long
// enough that a formula or a running head cannot fill it. Wider windows blur
// the thing this is looking for: at forty the dropped paragraph is 0.60 and an
// ordinary page of mathematics is 0.75, and the gap has halved.
const Window = 20

// MinCoverage is how much of a stretch of the text layer a reading has to
// account for.
//
// Half, which is to say a fault when a model has no more than nine of twenty
// consecutive words the page was typeset from. Measured rather than chosen.
// Over the pages of the two born digital papers read this way the worst stretch
// runs from 0.30 to 1.00, and 0.30 is the page whose last paragraph the reader
// dropped. Everything else is 0.55 or better, and the two worst of those are a
// page of matrix algebra and a page that is one large figure with words inside
// it that the picture shows and the answer is right not to transcribe.
//
// So the rule sits three words below the defect it was written for and two
// words above the worst page that is fine, and both of those margins are
// narrower than anybody would like. It is set where it is because the cost of
// the two mistakes is not the same. A page refused wrongly is read again, costs
// a minute, and comes back to a human who can say it was fine. A paragraph
// dropped quietly is published missing and nothing ever says so.
//
// Half a window is also about the smallest hole this can see at all, which is
// a sentence or two. A dropped clause goes past it and always will. The rule
// is for a reader that stopped at a figure or forgot a column, and those lose
// paragraphs.
const MinCoverage = 0.5

var (
	// letters is a word: four or more letters and nothing else. Digits are
	// left out along with everything shorter, because a number in a table is
	// exactly the thing the two readings are most likely to lay out
	// differently and least likely to disagree about.
	letters = regexp.MustCompile(`\p{L}{4,}`)
	// hyphenBreak is a word a line break split in two. A text layer keeps the
	// paper's own hyphenation and a model writes the word, so without this
	// every hyphenated word in the paper counts as missing.
	hyphenBreak = regexp.MustCompile("[-\\x{00ad}\\x{2010}]\\s*\n\\s*")
)

// layerWords is the words of a page in the order they were read off it, which
// the window needs and the set membership test does not.
func layerWords(s string) []string {
	return letters.FindAllString(strings.ToLower(hyphenBreak.ReplaceAllString(s, "")), -1)
}
