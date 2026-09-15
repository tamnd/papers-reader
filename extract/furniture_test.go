package extract

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// paper builds n pages that all carry the same body, and lets the caller add
// whatever furniture the journal in question would have printed.
func paper(n int, head func(page int) []poppler.TextLine) []poppler.Layout {
	var pages []poppler.Layout
	for i := 1; i <= n; i++ {
		groups := [][]poppler.TextLine{body(60, 120, 20, "a line of the body of this paper")}
		if head != nil {
			groups = append(groups, head(i))
		}
		pages = append(pages, page(i, groups...))
	}
	return pages
}

func TestARunningHeadOnEveryPageIsFurniture(t *testing.T) {
	pages := paper(8, func(int) []poppler.TextLine {
		return []poppler.TextLine{put(60, 30, "Journal of Nothing in Particular")}
	})
	f := FindFurniture(pages)
	if f.Count() != 1 {
		t.Fatalf("learned %d pieces of furniture, want 1", f.Count())
	}
	for _, p := range pages {
		for _, l := range f.Lines(p) {
			if strings.Contains(l.Text(), "Journal") {
				t.Fatalf("page %d kept its running head", p.Number)
			}
		}
	}
}

func TestAHeadThatAlternatesBetweenRectoAndVersoIsStillFurniture(t *testing.T) {
	// The case a half the pages threshold gets wrong. A journal sets its own
	// name on the left hand page and the paper's title on the right, so each
	// head is on exactly half the pages and neither is on more than half.
	pages := paper(10, func(i int) []poppler.TextLine {
		if i%2 == 0 {
			return []poppler.TextLine{put(60, 30, "Journal of Nothing in Particular")}
		}
		return []poppler.TextLine{put(60, 30, "On the Reading of Two Column Pages")}
	})
	f := FindFurniture(pages)
	if f.Count() != 2 {
		t.Fatalf("learned %d pieces of furniture, want both heads", f.Count())
	}
	for _, p := range pages {
		for _, l := range f.Lines(p) {
			if strings.Contains(l.Text(), "Journal") || strings.Contains(l.Text(), "Two Column") {
				t.Fatalf("page %d kept its running head: %q", p.Number, l.Text())
			}
		}
	}
}

func TestAHeadThatCarriesThePageNumberIsStillTheSameHead(t *testing.T) {
	// "Communications of the ACM 481" and "Communications of the ACM 482" are
	// one piece of furniture, so the digits are folded out of the key before
	// the pages are counted.
	pages := paper(6, func(i int) []poppler.TextLine {
		return []poppler.TextLine{put(60, 30, "Communications of Something "+itoa(480+i))}
	})
	f := FindFurniture(pages)
	if f.Count() != 1 {
		t.Fatalf("learned %d heads, want the numbered one counted once", f.Count())
	}
}

func TestABarePageNumberGoesEvenWhenItNeverRepeats(t *testing.T) {
	pages := paper(6, func(i int) []poppler.TextLine {
		return []poppler.TextLine{put(300, 740, itoa(480+i))}
	})
	f := FindFurniture(pages)
	for _, p := range pages {
		for _, l := range f.Lines(p) {
			if len(l.Words) == 1 && folio.MatchString(l.Text()) {
				t.Fatalf("page %d kept its folio %q", p.Number, l.Text())
			}
		}
	}
}

func TestALineInTheBodyThatRepeatsIsNotFurniture(t *testing.T) {
	// Every page of the fixture carries the same body line, and none of it is
	// in the margin. Deleting it would be deleting the paper.
	pages := paper(8, nil)
	f := FindFurniture(pages)
	if f.Count() != 0 {
		t.Errorf("learned %d pieces of furniture from the body", f.Count())
	}
	if n := len(f.Lines(pages[0])); n != 20 {
		t.Errorf("page 1 kept %d lines, want all 20", n)
	}
}

func TestAShortPaperKeepsItsHeader(t *testing.T) {
	// Two pages that share a line share it by coincidence as often as not.
	pages := paper(2, func(int) []poppler.TextLine {
		return []poppler.TextLine{put(60, 30, "Some Heading or Other")}
	})
	if f := FindFurniture(pages); f.Count() != 0 {
		t.Errorf("learned furniture from %d pages", len(pages))
	}
}

func TestTheZeroFurnitureKeepsEverything(t *testing.T) {
	var f *Furniture
	p := page(1, body(60, 120, 5, "a line of the body"))
	if n := len(f.Lines(p)); n != 5 {
		t.Errorf("a nil Furniture kept %d of 5 lines", n)
	}
	if f.Count() != 0 {
		t.Errorf("a nil Furniture counted %d", f.Count())
	}
}

func TestFoldMakesTwoPrintingsOfOneHeadTheSame(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"Communications of the ACM 481", "Communications  of the ACM   4823"},
		{"IEEE Transactions, Vol. 3", "IEEE Transactions Vol 17"},
		{"THE JOURNAL", "the journal"},
	} {
		if fold(c.a) != fold(c.b) {
			t.Errorf("fold(%q) is %q and fold(%q) is %q", c.a, fold(c.a), c.b, fold(c.b))
		}
	}
	if fold("Introduction") == fold("Conclusion") {
		t.Error("fold made two different heads the same")
	}
}

func TestAFolioIsRecognisedHoweverItIsPrinted(t *testing.T) {
	for _, s := range []string{"7", "481", "[12]", "(9)", "- 34 -", "iv", "XVII"} {
		if !folio.MatchString(s) {
			t.Errorf("%q was not read as a page number", s)
		}
	}
	// A four digit number is allowed because a thesis runs to four digits,
	// and that means a year set alone in the margin is read as a folio. That
	// is the right trade: a line that is nothing but a year is furniture on
	// every page of the hundred where it occurs.
	for _, s := range []string{"3.1", "et", "12 words", "1a", "12345"} {
		if folio.MatchString(s) {
			t.Errorf("%q was read as a page number", s)
		}
	}
}

func TestThePageNumbersAPaperPrintsAreRead(t *testing.T) {
	// A paper pulled out of a volume: the file starts at 1 and the paper
	// starts at 481.
	pages := paper(6, func(i int) []poppler.TextLine {
		return []poppler.TextLine{put(300, 740, itoa(480+i))}
	})
	f := FindFurniture(pages)
	for _, p := range pages {
		want := itoa(480 + p.Number)
		if got := f.Printed(p); got != want {
			t.Errorf("page %d printed %q, want %q", p.Number, got, want)
		}
	}
}

func TestAFootnoteMarkerIsNotAPageNumber(t *testing.T) {
	// The BERT paper, which prints no page numbers at all. Its footnotes are
	// at the foot of a column and their markers are bare numbers in the
	// bottom margin, and reading those as folios put page 1 of the paper at
	// page 3. A footnote marker does not count with the file, which is the
	// whole of what makes a folio a folio.
	//
	// Two of these agree, because a footnote on one page and the next
	// footnote on the page after it are a page apart and that is what a
	// folio looks like. A pair is not a page numbering.
	markers := map[int]string{3: "4", 4: "6", 5: "8", 6: "10", 9: "12", 10: "13"}
	pages := paper(12, func(i int) []poppler.TextLine {
		if markers[i] == "" {
			return nil
		}
		return []poppler.TextLine{put(320, 745, markers[i])}
	})
	f := FindFurniture(pages)
	for _, p := range pages {
		if got := f.Printed(p); got != "" {
			t.Errorf("page %d was read as printing %q", p.Number, got)
		}
	}
	if m := LearnMap(readAll(pages, f)); m.Known {
		t.Errorf("learned an offset of %d from a paper with no page numbers", m.Offset)
	}
}

func TestAFolioIsPickedOutOfAPageThatAlsoCarriesAMarker(t *testing.T) {
	// The common case in a two column paper that does number its pages: the
	// folio is centred at the foot and the footnote marker is at the foot of
	// a column, and both are bare numbers in the bottom margin.
	markers := []string{"1", "2", "2", "3", "5", "8"}
	pages := paper(6, func(i int) []poppler.TextLine {
		return []poppler.TextLine{
			put(80, 720, markers[i-1]),
			put(300, 760, itoa(100+i)),
		}
	})
	f := FindFurniture(pages)
	for _, p := range pages {
		want := itoa(100 + p.Number)
		if got := f.Printed(p); got != want {
			t.Errorf("page %d printed %q, want %q", p.Number, got, want)
		}
	}
}

func TestOneMisreadFolioDoesNotMoveTheRest(t *testing.T) {
	// A scan reads one folio wrong now and then. The page that disagrees
	// with the paper keeps no number rather than a wrong one, and the pages
	// around it are untouched.
	pages := paper(6, func(i int) []poppler.TextLine {
		if i == 3 {
			return []poppler.TextLine{put(300, 740, "9")}
		}
		return []poppler.TextLine{put(300, 740, itoa(480+i))}
	})
	f := FindFurniture(pages)
	if got := f.Printed(pages[2]); got != "" {
		t.Errorf("page 3 printed %q, want nothing", got)
	}
	if got := f.Printed(pages[3]); got != "484" {
		t.Errorf("page 4 printed %q, want 484", got)
	}
}

func TestTheFrontMatterOfAThesisCountsInRomanNumerals(t *testing.T) {
	numerals := []string{"i", "ii", "iii", "iv", "v", "vi"}
	pages := paper(6, func(i int) []poppler.TextLine {
		return []poppler.TextLine{put(300, 740, numerals[i-1])}
	})
	f := FindFurniture(pages)
	for _, p := range pages {
		if got := f.Printed(p); got != numerals[p.Number-1] {
			t.Errorf("page %d printed %q, want %q", p.Number, got, numerals[p.Number-1])
		}
	}
}

func TestARomanNumeralIsReadTheWayRomansWroteThem(t *testing.T) {
	for _, c := range []struct {
		s string
		n int
	}{
		{"i", 1}, {"iv", 4}, {"ix", 9}, {"xiv", 14}, {"xl", 40},
		{"XVII", 17}, {"mcmxcix", 1999},
	} {
		if n, ok := roman(c.s); !ok || n != c.n {
			t.Errorf("roman(%q) is %d %v, want %d", c.s, n, ok, c.n)
		}
	}
	// A scan reads "iii" as "iiii" and "ix" as "ic" often enough to be worth
	// refusing. A numeral nobody wrote is a misread, not a page.
	for _, s := range []string{"iiii", "ic", "vv", "abc", "", "1"} {
		if n, ok := roman(s); ok {
			t.Errorf("roman(%q) is %d, want a refusal", s, n)
		}
	}
}

// readAll is every page read, which is what LearnMap wants.
func readAll(pages []poppler.Layout, f *Furniture) []Page {
	out := make([]Page, 0, len(pages))
	for _, p := range pages {
		out = append(out, Read(p, f))
	}
	return out
}

// itoa keeps the fixtures readable without pulling strconv into the test for
// one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// lowFolio is a paper whose text block ends two thirds of the way down the
// page with the folio set a little under it: forty points of white below the
// last line of the body, which is a long way from the body and nowhere near
// the bottom eighth of the page.
func lowFolio(n int) []poppler.Layout {
	var pages []poppler.Layout
	for i := 1; i <= n; i++ {
		text := body(60, 120, 36, "a line of the body of this paper")
		foot := []poppler.TextLine{put(300, 580, itoa(i+1))}
		pages = append(pages, page(i, text, foot))
	}
	return pages
}

func TestAFolioUnderTheTextBlockIsFurnitureWhereverItSits(t *testing.T) {
	// Natural Proofs is set this way and the fixed band alone kept every one
	// of its page numbers, which rule T10 then found sitting in the prose in
	// sixteen places in one paper.
	pages := lowFolio(8)
	band := frame{margin, 1 - margin}
	f := FindFurniture(pages)
	for _, p := range pages {
		for _, l := range f.Lines(p) {
			if folio.MatchString(l.Text()) {
				t.Fatalf("page %d kept its folio %q", p.Number, l.Text())
			}
		}
		if got, want := f.Printed(p), itoa(p.Number+1); got != want {
			t.Errorf("page %d printed %q, want %q", p.Number, got, want)
		}
		last := Lines(p)[len(Lines(p))-1]
		if _, ok := band.key(p, last); ok {
			t.Errorf("page %d sets its folio inside the fixed band, so this fixture proves nothing", p.Number)
		}
	}
}

func TestARunningHeadJustUnderTheTopBandIsFurniture(t *testing.T) {
	// A 1972 journal that sets its head an eighth of an inch lower than the
	// band allows for. Four of the hundred papers do, and each of them
	// carried its head into every page of the Markdown.
	var pages []poppler.Layout
	for i := 1; i <= 8; i++ {
		head := []poppler.TextLine{put(60, 100, "Journal of Nothing in Particular "+itoa(106+i))}
		pages = append(pages, page(i, head, body(60, 150, 30, "a line of the body of this paper")))
	}
	f := FindFurniture(pages)
	for _, p := range pages {
		for _, l := range f.Lines(p) {
			if strings.Contains(l.Text(), "Journal") {
				t.Fatalf("page %d kept its running head", p.Number)
			}
		}
	}
}

func TestALineStandingOffTheTextBlockIsKeptWhenItIsNotFurniture(t *testing.T) {
	// Widening the frame only lets Is look at a line. A footnote set under
	// the text block stands as far off the body as a folio does, and it is
	// neither a bare number nor the same line twice, so it stays.
	words := []string{"one", "two", "three", "four", "five", "six", "seven", "eight"}
	var pages []poppler.Layout
	for i := 1; i <= 8; i++ {
		note := []poppler.TextLine{put(60, 580, "a note on the "+words[i-1]+" point made above")}
		pages = append(pages, page(i, body(60, 120, 36, "a line of the body of this paper"), note))
	}
	f := FindFurniture(pages)
	for _, p := range pages {
		kept := false
		for _, l := range f.Lines(p) {
			kept = kept || strings.Contains(l.Text(), "a note on the")
		}
		if !kept {
			t.Errorf("page %d lost its footnote", p.Number)
		}
	}
}

func TestAPageOfWhiteSpaceDoesNotMoveTheMarginOfTheRest(t *testing.T) {
	// A title page has a block of white under the authors and a page built
	// around one figure has white wherever the figure is not. Read as
	// evidence about where the paper sets its margin they are misleading,
	// and the Gamma paper has one of each: taken on their own the two of
	// them moved the top margin to nearly half the page, which put a
	// footnote marker standing beside a line of prose into the margin and
	// then took it out of the text.
	sparse := func(n int, head string) poppler.Layout {
		return page(n, []poppler.TextLine{put(60, 60, head)}, body(60, 500, 12, "a line under a lot of white"))
	}
	pages := []poppler.Layout{sparse(1, "A Title and Some Authors"), sparse(2, "Figure 1: a caption")}
	for i := 3; i <= 8; i++ {
		marker := []poppler.TextLine{put(300, 256, "1")}
		pages = append(pages, page(i, body(60, 120, 30, "a line of the body of this paper"), marker))
	}
	// The marker is set beside a line of the body and comes back joined to
	// it, which is what the reader does with a superscript.
	frames := framesOf(pages)
	if got := frames[5].top; got != margin {
		t.Errorf("a full page of text starts its body at %.3f, want the fixed band at %.3f", got, margin)
	}
	f := FindFurniture(pages)
	for _, p := range pages[2:] {
		kept := false
		for _, l := range f.Lines(p) {
			kept = kept || strings.HasSuffix(l.Text(), "paper 1")
		}
		if !kept {
			t.Errorf("page %d lost the footnote marker from the middle of its text", p.Number)
		}
	}
}
