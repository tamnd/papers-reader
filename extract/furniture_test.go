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
