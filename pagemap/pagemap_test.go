package pagemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// journal is a paper pulled out of a volume: a cover sheet the library added
// and then three pages numbered from 483, with the middle one's folio lost
// the way a scanner loses one in the gutter.
func journal() *Map {
	return Build("amdahl-1967-law", []Page{
		{PDF: 1, Size: Size{612, 792}, Columns: 1},
		{PDF: 2, Printed: "483", Size: Size{612, 792}, Columns: 3},
		{PDF: 3, Size: Size{612, 792}, Columns: 3},
		{PDF: 4, Printed: "485", Size: Size{612, 792}, Columns: 3},
	})
}

func TestTheOffsetIsLearnedFromTheFoliosThePagesPrinted(t *testing.T) {
	m := journal()
	if m.Offset == nil {
		t.Fatal("learned no offset from two pages that printed one")
	}
	if *m.Offset != 481 {
		t.Errorf("offset %d, want 481", *m.Offset)
	}
}

func TestAPageThatPrintedNoFolioStillHasAPrintedNumber(t *testing.T) {
	m := journal()
	if got := m.Printed(3); got != "484" {
		t.Errorf("page 3 is printed %q, want 484", got)
	}
	// The cover sheet is page 482 of a volume nobody has, which is what the
	// offset says and is the only answer available. The point of the test is
	// that it is not the empty string and not 1.
	if got := m.Printed(1); got != "482" {
		t.Errorf("page 1 is printed %q, want 482", got)
	}
}

func TestAPaperThatNumbersNothingSaysSoRatherThanGuessing(t *testing.T) {
	m := Build("mccarthy-1960-lisp", []Page{
		{PDF: 1, Size: Size{612, 792}},
		{PDF: 2, Size: Size{612, 792}},
	})
	if m.Offset != nil {
		t.Fatalf("learned offset %d from a paper that printed no numbers", *m.Offset)
	}
	if got := m.Printed(1); got != "" {
		t.Errorf("page 1 is printed %q, want nothing", got)
	}
	if got := m.Span(); got != "" {
		t.Errorf("span %q, want nothing", got)
	}
}

func TestOnePageThatPrintedAFolioIsNotEnoughToLearnFrom(t *testing.T) {
	// One page's folio read wrong is one page's folio read wrong, and there
	// is no second page to disagree with it.
	m := Build("one", []Page{
		{PDF: 1, Printed: "17", Size: Size{612, 792}},
		{PDF: 2, Size: Size{612, 792}},
	})
	if m.Offset != nil {
		t.Fatalf("learned offset %d from a single folio", *m.Offset)
	}
	// What the page itself printed is still what the page printed.
	if got := m.Printed(1); got != "17" {
		t.Errorf("page 1 is printed %q, want 17", got)
	}
}

func TestRomanNumeralsTeachNoOffset(t *testing.T) {
	m := Build("thesis", []Page{
		{PDF: 1, Printed: "i", Size: Size{612, 792}},
		{PDF: 2, Printed: "ii", Size: Size{612, 792}},
		{PDF: 3, Printed: "iii", Size: Size{612, 792}},
	})
	if m.Offset != nil {
		t.Fatalf("learned offset %d from roman numerals", *m.Offset)
	}
	if got := m.Printed(2); got != "ii" {
		t.Errorf("page 2 is printed %q, want ii", got)
	}
}

func TestAPrintedPageIsFoundInTheFile(t *testing.T) {
	m := journal()
	for _, c := range []struct {
		printed int
		pdf     int
		ok      bool
	}{
		{483, 2, true}, // printed on the page itself
		{484, 3, true}, // by the offset, because that page printed none
		{485, 4, true},
		{999, 0, false}, // past the end of the file
	} {
		pdf, ok := m.PDF(c.printed)
		if ok != c.ok || pdf != c.pdf {
			t.Errorf("printed %d is file page %d %v, want %d %v", c.printed, pdf, ok, c.pdf, c.ok)
		}
	}
}

func TestAPageMisnumberedInTheJournalResolvesToThePageThatCarriesIt(t *testing.T) {
	// The offset says page 3 of the file is printed 484 and the journal
	// printed 484 on the page after it as well. What a page actually prints
	// beats what the offset works out, because the offset is a median over
	// the paper and the folio is the page speaking for itself.
	m := Build("misnumbered", []Page{
		{PDF: 1, Printed: "483"},
		{PDF: 2, Printed: "484"},
		{PDF: 3, Printed: "484"},
		{PDF: 4, Printed: "486"},
	})
	pdf, ok := m.PDF(484)
	if !ok || pdf != 2 {
		t.Errorf("printed 484 is file page %d %v, want 2 true", pdf, ok)
	}
}

func TestTheSpanIsThePagesThePaperPrints(t *testing.T) {
	// The cover sheet is page 482 by the offset and is not a page of
	// anybody's paper, so the span starts where the paper does.
	if got := journal().Span(); got != "483-485" {
		t.Errorf("span %q, want 483-485", got)
	}
	one := Build("one", []Page{{PDF: 1, Printed: "7"}})
	if got := one.Span(); got != "7" {
		t.Errorf("span %q, want 7", got)
	}
	// The Kerberos paper sets its folios as "-2-" and "-3-", and prints
	// pages 2 to 3.
	decorated := Build("steiner-1988-kerberos", []Page{
		{PDF: 1},
		{PDF: 2, Printed: "-2-"},
		{PDF: 3, Printed: "-3-"},
	})
	if got := decorated.Span(); got != "2-3" {
		t.Errorf("span %q, want 2-3", got)
	}
}

func TestAFigureIsRecordedAgainstThePageItWasCutFrom(t *testing.T) {
	m := Build("vaswani-2017-attention", []Page{
		{PDF: 1, Size: Size{612, 792}},
		{PDF: 3, Size: Size{612, 792}, Figures: []string{"f01", "f02"}},
	})
	p, ok := m.Page(3)
	if !ok {
		t.Fatal("no page 3")
	}
	if len(p.Figures) != 2 || p.Figures[0] != "f01" {
		t.Errorf("figures %v, want f01 and f02", p.Figures)
	}
	if !strings.Contains(m.String(), "2 figures") {
		t.Errorf("the summary %q does not count the figures", m.String())
	}
}

func TestAMapSurvivesBeingWrittenAndReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages", "amdahl-1967-law.yaml")
	if err := journal().Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Offset == nil || *back.Offset != 481 {
		t.Errorf("offset came back %v, want 481", back.Offset)
	}
	if len(back.Pages) != 4 || back.Pages[1].Printed != "483" {
		t.Errorf("pages came back %v", back.Pages)
	}
	if back.Pages[1].Size.Width() != 612 || back.Pages[1].Size.Height() != 792 {
		t.Errorf("size came back %v", back.Pages[1].Size)
	}
}

func TestAnUnnumberedPaperIsWrittenAsNullAndNotAsZero(t *testing.T) {
	// A preprint numbered from one has an offset of zero and knows it, and a
	// paper nobody could learn an offset for does not. Both of those read
	// back as zero if the field is written with omitempty, and then a reader
	// cannot tell "page 1" from "we have no idea".
	m := Build("nothing", []Page{{PDF: 1}, {PDF: 2}})
	b, err := m.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "offset: null") {
		t.Errorf("wrote\n%s\nwant an explicit null offset", b)
	}
	zero := Build("preprint", []Page{{PDF: 1, Printed: "1"}, {PDF: 2, Printed: "2"}})
	b, err = zero.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "offset: 0") {
		t.Errorf("wrote\n%s\nwant an offset of zero", b)
	}
}

func TestTheFileOpensWithTheHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "amdahl-1967-law.yaml")
	if err := journal().Save(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "# Which page of the file") {
		t.Errorf("the file opens %.40q", b)
	}
}
