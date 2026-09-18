package extract

import (
	"strings"
	"testing"
)

// layer is a page's text layer the way pdftotext hands one over: the words in
// the order it read them off the page, wrapped at whatever width it liked.
// None of the text in this file is out of a paper. It is written here so that
// the corpus's licensing has nothing to do with the test suite.
const layer = `A Note On Counting Words
Every measure of agreement between two readings of the same page has to
decide what counts as the same word, and this note takes the simplest
answer available, which is that two words are the same when they are
spelled the same way. That is wrong about proper nouns and wrong about
inflection and it does not matter, because the question being asked is
not whether the readings agree but whether one of them has a hole in it.

It should be said that a hole small enough to hide inside a window is a
hole this cannot find, and there is no arrangement of these numbers that
would change that. What it sets out to catch is a reader that stopped at
a figure and never came back, or one that answered about a column and
forgot the other. Both of those lose whole paragraphs, and a paragraph
is longer than any window anybody would want to measure over.`

func TestCoverageIsOneWhenThereIsNothingToCompareAgainst(t *testing.T) {
	share, missing := Coverage("", "anything at all")
	if share != 1 || missing != nil {
		t.Errorf("Coverage of an empty layer is %v %q, want 1 and nothing", share, missing)
	}
	// A page whose layer is a folio and nothing else has no words in it by
	// the four letter rule, and is the same case.
	share, _ = Coverage("7", "a whole page figure")
	if share != 1 {
		t.Errorf("Coverage of a layer with no words is %v, want 1", share)
	}
}

func TestAReadingThatHasEverythingCoversEverything(t *testing.T) {
	share, missing := Coverage(layer, layer)
	if share != 1 || missing != nil {
		t.Errorf("Coverage of a layer against itself is %v %q, want 1 and nothing", share, missing)
	}
}

func TestOrderDoesNotCount(t *testing.T) {
	// pdftotext reads a two column page straight across and a model reads it
	// column by column. The two answers are the same reading.
	words := strings.Fields(layer)
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	share, missing := Coverage(layer, strings.Join(words, " "))
	if share != 1 {
		t.Errorf("Coverage of a layer against itself backwards is %v, missing %q, want 1", share, missing)
	}
}

// The defect the rule was written for. A reader that stopped at a figure and
// left the paragraph under it out, on a page it otherwise read correctly.
func TestADroppedParagraphIsFound(t *testing.T) {
	first, second, ok := strings.Cut(layer, "\n\n")
	if !ok {
		t.Fatal("the fixture no longer has two paragraphs in it")
	}
	share, missing := Coverage(layer, first)
	if share >= MinCoverage {
		t.Errorf("a dropped paragraph covers %v, want under %v", share, MinCoverage)
	}
	// The words reported are the ones somebody has to recognise the dropped
	// passage by, so they have to come out of it.
	for _, w := range missing {
		if !strings.Contains(strings.ToLower(second), w) {
			t.Errorf("the missing words include %q, which is not in the dropped paragraph", w)
		}
	}
}

// A hole of a few words is a hole this does not find, and saying so here is
// cheaper than somebody discovering it from a paper. Half a window of words
// is about where it starts to bite, so a clause is safe and a sentence or two
// is not.
func TestAFewDroppedWordsAreNotFound(t *testing.T) {
	const dropped = " and there is no arrangement of these numbers that\nwould change that"
	if !strings.Contains(layer, dropped) {
		t.Fatal("the fixture has changed and no longer contains the passage this drops")
	}
	if share, _ := Coverage(layer, strings.Replace(layer, dropped, "", 1)); share < MinCoverage {
		t.Errorf("a dropped sentence covers %v, and this rule is not supposed to be able to see it", share)
	}
}

func TestScatteredDisagreementIsNotADroppedParagraph(t *testing.T) {
	// Mathematics and ligatures are wrong all over a page and nowhere for
	// long. This drops every sixth word, which is a sixth of the page and far
	// more than any real reading loses, and it still has to pass.
	words := strings.Fields(layer)
	var kept []string
	for i, w := range words {
		if i%6 != 0 {
			kept = append(kept, w)
		}
	}
	share, missing := Coverage(layer, strings.Join(kept, " "))
	if share < MinCoverage {
		t.Errorf("a page with every sixth word gone covers %v, want at least %v, missing %q", share, MinCoverage, missing)
	}
}

func TestHyphenationIsNotADisagreement(t *testing.T) {
	// The layer keeps the paper's own hyphenation and the model writes the
	// word, so without unhyphenating, every broken word counts as missing.
	broken := "The relation-\nship between the two read-\nings of one page."
	share, missing := Coverage(broken, "The relationship between the two readings of one page.")
	if share != 1 {
		t.Errorf("a hyphenated layer covers %v, missing %q, want 1", share, missing)
	}
}

func TestShortWordsAndNumbersAreNotCounted(t *testing.T) {
	// A table of numbers laid out two ways is the thing the two readings are
	// most likely to disagree about and least likely to be wrong about.
	share, _ := Coverage("28.4 41.8 26.9 3.9 100 2.3", "no numbers here at all")
	if share != 1 {
		t.Errorf("a layer of numbers covers %v, want 1", share)
	}
}

func TestAPageShorterThanAWindowIsJudgedWhole(t *testing.T) {
	const short = "Acknowledgments. The author wishes to thank his colleagues."
	if len(layerWords(short)) >= Window {
		t.Fatalf("the fixture has %d words, and this test wants fewer than %d", len(layerWords(short)), Window)
	}
	share, missing := Coverage(short, "Acknowledgments.")
	if share >= MinCoverage {
		t.Errorf("a short page read as one word covers %v, want under %v", share, MinCoverage)
	}
	if len(missing) == 0 {
		t.Error("a short page that lost its words reports none missing")
	}
	share, _ = Coverage(short, short)
	if share != 1 {
		t.Errorf("a short page read correctly covers %v, want 1", share)
	}
}

// Averaging over the page is what this replaced, and the difference between
// the two is the point. The same hole in a longer page is the same hole, and
// a measure that divides it by the length of the page stops seeing it.
func TestALongerPageDoesNotHideTheSameHole(t *testing.T) {
	first, _, _ := strings.Cut(layer, "\n\n")
	short, _ := Coverage(layer, first)
	long, _ := Coverage(layer+"\n\n"+first+"\n\n"+first, first+"\n\n"+first+"\n\n"+first)
	if short != long {
		t.Errorf("the same hole covers %v on a short page and %v on a long one", short, long)
	}
	if long >= MinCoverage {
		t.Errorf("the hole covers %v on the long page, want under %v", long, MinCoverage)
	}
}

// The ACM bibliography. A surname set in small capitals comes out of the
// text layer with its first letter as a token of its own, and a model that
// read the page correctly wrote the name. Page 12 of the Chord paper was
// refused three times over this and then lost, and it holds the entries
// twenty five of that paper's citations point at.
func TestASurnameInSmallCapitalsIsNotAMissingWord(t *testing.T) {
	layer := "[12] K UBIATOWICZ , J., B INDEL , D., C ZERWINSKI , S., " +
		"E ATON , P., G EELS , D., G UMMADI , R., R HEA , S., " +
		"W EATHERSPOON , H., W EIMER , W., W ELLS , C., AND Z HAO , B. " +
		"OceanStore: An architecture for global-scale persistent storage."
	answer := "12. Kubiatowicz, J., Bindel, D., Czerwinski, S., Eaton, P., Geels, D., " +
		"Gummadi, R., Rhea, S., Weatherspoon, H., Weimer, W., Wells, C., and Zhao, B. " +
		"OceanStore: An architecture for global-scale persistent storage."
	share, missing := Coverage(layer, answer)
	if share < MinCoverage {
		t.Errorf("a bibliography read correctly came out at %.0f%%, missing %v", share*100, missing)
	}
}

// The other half. Taking a letter off the front is not allowed to excuse a
// paragraph the reader really did drop.
func TestADroppedParagraphIsStillDropped(t *testing.T) {
	layer := "The network is reliable and the latency is zero. Bandwidth is infinite " +
		"and the topology does not change. There is one administrator and the " +
		"transport cost is nothing. The network is homogeneous throughout."
	answer := "The network is reliable and the latency is zero."
	share, _ := Coverage(layer, answer)
	if share >= MinCoverage {
		t.Errorf("a dropped paragraph came out at %.0f%%, want it refused", share*100)
	}
}
