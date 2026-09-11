package refs

import (
	"strings"
	"testing"
)

// one parses a single entry in one style, which is what the field tests are
// about. The label is already off by the time fields runs.
func one(s Style, raw string) Entry {
	e := Entry{Key: "1", Raw: raw}
	fields(s, &e)
	return e
}

func TestTheThreeFieldsOfAPlainEntry(t *testing.T) {
	e := one(StyleBracket, "A. Nkemelu, B. Oyelaran, and C. Fairweather. A theory of slow indexes. Journal of Made Up Results, 1991.")
	if got := strings.Join(e.Authors, "; "); got != "A. Nkemelu; B. Oyelaran; C. Fairweather" {
		t.Errorf("the authors are %q", got)
	}
	if e.Title != "A theory of slow indexes" {
		t.Errorf("the title is %q", e.Title)
	}
	if e.Venue != "Journal of Made Up Results" {
		t.Errorf("the venue is %q", e.Venue)
	}
	if e.Year != 1991 {
		t.Errorf("the year is %d", e.Year)
	}
}

func TestAnInitialIsNotTheEndOfTheAuthors(t *testing.T) {
	// The full stop after "V" would cut the author list in half, and the
	// title would come out as "Le. A theory of slow indexes".
	e := one(StyleBracket, "A. Nkemelu and Quoc V. Le. A theory of slow indexes. Journal of Made Up Results, 1991.")
	if e.Title != "A theory of slow indexes" {
		t.Errorf("the title is %q", e.Title)
	}
	if got := strings.Join(e.Authors, "; "); got != "A. Nkemelu; Quoc V. Le" {
		t.Errorf("the authors are %q", got)
	}
}

func TestAnAbbreviationInTheVenueIsNotTheEndOfAField(t *testing.T) {
	e := one(StyleBracket, "A. Nkemelu. A theory of slow indexes. In Proc. Symposium on Indexing, vol. 2, pages 1-12, 1991.")
	if e.Title != "A theory of slow indexes" {
		t.Errorf("the title is %q", e.Title)
	}
	if e.Venue != "In Proc. Symposium on Indexing, vol. 2, pages 1-12" {
		t.Errorf("the venue is %q", e.Venue)
	}
}

func TestAQuotedTitleIsTakenAsPrinted(t *testing.T) {
	e := one(StyleBracket, `A. Nkemelu, "A theory of slow indexes, revisited," In Proceedings of Nowhere, 1991.`)
	if e.Title != "A theory of slow indexes, revisited" {
		t.Errorf("the title is %q", e.Title)
	}
	if got := strings.Join(e.Authors, "; "); got != "A. Nkemelu" {
		t.Errorf("the authors are %q", got)
	}
	if e.Venue != "In Proceedings of Nowhere" {
		t.Errorf("the venue is %q", e.Venue)
	}
}

func TestASurnameFirstAuthorListIsPutBackInOrder(t *testing.T) {
	e := one(StyleBracket, "Nkemelu, A. B. and Oyelaran, C. A theory of slow indexes. 1991.")
	if got := strings.Join(e.Authors, "; "); got != "A. B. Nkemelu; C. Oyelaran" {
		t.Errorf("the authors are %q", got)
	}
}

func TestEtAlIsNotAnAuthor(t *testing.T) {
	e := one(StyleBracket, "A. Nkemelu et al. A theory of slow indexes. 1991.")
	for _, name := range e.Authors {
		if strings.Contains(strings.ToLower(name), "et al") {
			t.Errorf("the authors are %v", e.Authors)
		}
	}
}

func TestAnEntryThatBeginsWithItsTitleKeepsItsTitle(t *testing.T) {
	// A standard or a manual has no author, and reading the first field as
	// one would file the title as a person.
	e := one(StyleBracket, "A guide to slow indexing: the standard. Institute of Made Up Standards, 1991.")
	if e.Title != "A guide to slow indexing: the standard" {
		t.Errorf("the title is %q", e.Title)
	}
	if len(e.Authors) != 0 {
		t.Errorf("the authors are %v", e.Authors)
	}
}

func TestTheIdentifiersAreReadOffTheEntry(t *testing.T) {
	e := one(StyleBracket, "A. Nkemelu. A theory of slow indexes. arXiv preprint arXiv:1607.06450, 2016.")
	if e.ArXiv != "1607.06450" {
		t.Errorf("the arXiv id is %q", e.ArXiv)
	}
	e = one(StyleBracket, "A. Nkemelu. A theory of slow indexes. CoRR, abs/1409.0473, 2014.")
	if e.ArXiv != "1409.0473" {
		t.Errorf("the report series id is %q", e.ArXiv)
	}
	e = one(StyleBracket, "A. Nkemelu. A theory of slow indexes. Journal of Made Up Results, 1991. doi:10.1145/359340.359342.")
	if e.DOI != "10.1145/359340.359342" {
		t.Errorf("the DOI is %q", e.DOI)
	}
	e = one(StyleBracket, `A. Nkemelu, "A theory of slow indexes," http://example.org/slow.pdf, 1991.`)
	if e.URL != "http://example.org/slow.pdf" {
		t.Errorf("the URL is %q", e.URL)
	}
}

func TestThePageRangeIsOnlyReadWhereTheEntrySaysItIsOne(t *testing.T) {
	if got := one(StyleBracket, "A. Nkemelu. A theory. Journal, pages 99-111, 1991.").Pages; got != "99-111" {
		t.Errorf("the pages are %q", got)
	}
	if got := one(StyleBracket, "A. Nkemelu. A theory. Journal, pp. 99-111, 1991.").Pages; got != "99-111" {
		t.Errorf("the pages are %q", got)
	}
	// A bare pair of numbers is as likely to be a volume range or part of a
	// report number, and a wrong page range is a mistake nobody catches by
	// reading.
	if got := one(StyleBracket, "A. Nkemelu. A theory. Technical Report 12-14, 1991.").Pages; got != "" {
		t.Errorf("the pages are %q", got)
	}
}

func TestTheYearIsTheLastOneInTheEntry(t *testing.T) {
	// The volume year comes first and the publication year comes last.
	e := one(StyleBracket, "A. Nkemelu. A theory of slow indexes. Journal of Made Up Results, 27, 1948.")
	if e.Year != 1948 {
		t.Errorf("the year is %d", e.Year)
	}
}

func TestTheVenueDropsWhatIsAlreadyAFieldOfItsOwn(t *testing.T) {
	if got := one(StyleBracket, `A. Nkemelu, "A theory," http://example.org/slow.txt, 1991.`).Venue; got != "" {
		t.Errorf("the venue is %q, want the URL and the bare year gone", got)
	}
	if got := one(StyleBracket, `A. Nkemelu, "A theory," 1957.`).Venue; got != "" {
		t.Errorf("the venue is %q, want a bare year dropped", got)
	}
}

func TestAnAuthorYearEntryIsParsedFromItsBrackets(t *testing.T) {
	e := one(StyleAuthorYear, "Nkemelu, A. and Oyelaran, B. (1991). A theory of slow indexes. Journal of Made Up Results, 27, 379-423.")
	if got := strings.Join(e.Authors, "; "); got != "A. Nkemelu; B. Oyelaran" {
		t.Errorf("the authors are %q", got)
	}
	if e.Title != "A theory of slow indexes" {
		t.Errorf("the title is %q", e.Title)
	}
	if e.Year != 1991 {
		t.Errorf("the year is %d", e.Year)
	}
}

func TestAnEntryThatParsesIntoNothingStillHasItsRaw(t *testing.T) {
	e := one(StyleBracket, "see the discussion above")
	if e.Raw != "see the discussion above" {
		t.Errorf("raw reads %q", e.Raw)
	}
}
