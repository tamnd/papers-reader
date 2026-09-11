package refs

import (
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// The corpus these tests resolve against is three papers that do not exist.
func corpusPapers() []corpus.Paper {
	return []corpus.Paper{
		{
			ID:      "nkemelu-1991-slowindexes",
			Title:   "A Theory of Slow Indexes",
			Authors: []string{"Adaeze Nkemelu", "Bolaji Oyelaran"},
			Year:    1991,
			ArXiv:   "9101.00001",
		},
		{
			ID:      "ravensworth-1996-lastword",
			Title:   "The Last Word on Indexes",
			Authors: []string{"Dilys Ravensworth"},
			Year:    1996,
			DOI:     "10.1145/111111.222222",
		},
		{
			ID:      "sandoval-1999-reconsidered",
			Title:   "Indexes Reconsidered",
			Authors: []string{"Elena Sandoval"},
			Year:    1999,
		},
	}
}

func TestAReferenceThatNamesACorpusPaperResolves(t *testing.T) {
	entries := []Entry{{
		Key:     "1",
		Raw:     "Adaeze Nkemelu and Bolaji Oyelaran. A theory of slow indexes. Journal of Made Up Results, 1991.",
		Authors: []string{"Adaeze Nkemelu", "Bolaji Oyelaran"},
		Title:   "A theory of slow indexes",
		Year:    1991,
	}}
	if n := Resolve(entries, corpusPapers(), "somebody-2020-citing"); n != 1 {
		t.Fatalf("resolved %d", n)
	}
	if entries[0].ResolvesTo != "nkemelu-1991-slowindexes" {
		t.Errorf("resolved to %q", entries[0].ResolvesTo)
	}
}

func TestAReferenceToSomethingElseResolvesToNothing(t *testing.T) {
	entries := []Entry{{
		Key:     "1",
		Raw:     "Fenella Turnbull. Indexes at last. Journal of Made Up Results, 2001.",
		Authors: []string{"Fenella Turnbull"},
		Title:   "Indexes at last",
		Year:    2001,
	}}
	if n := Resolve(entries, corpusPapers(), "somebody-2020-citing"); n != 0 {
		t.Errorf("resolved %d: %q", n, entries[0].ResolvesTo)
	}
}

func TestARightTitleWithTheWrongAuthorsDoesNotResolve(t *testing.T) {
	// Two papers can carry the same title, and a citation graph that says
	// one cited the other because of it is worse than one that says nothing.
	entries := []Entry{{
		Key:     "1",
		Authors: []string{"Fenella Turnbull"},
		Title:   "A theory of slow indexes",
		Year:    1991,
	}}
	if n := Resolve(entries, corpusPapers(), ""); n != 0 {
		t.Errorf("resolved to %q on the title alone", entries[0].ResolvesTo)
	}
}

func TestAReferenceWithTheWrongYearDoesNotResolve(t *testing.T) {
	entries := []Entry{{
		Key:     "1",
		Authors: []string{"Adaeze Nkemelu"},
		Title:   "A theory of slow indexes",
		Year:    1975,
	}}
	if n := Resolve(entries, corpusPapers(), ""); n != 0 {
		t.Errorf("resolved to %q sixteen years out", entries[0].ResolvesTo)
	}
}

func TestAPreprintAndItsJournalVersionAreAYearApart(t *testing.T) {
	// A year either way is allowed, because a preprint and the paper it
	// became are very often dated a year apart and they are one paper.
	entries := []Entry{{
		Key:     "1",
		Authors: []string{"Adaeze Nkemelu"},
		Title:   "A theory of slow indexes",
		Year:    1992,
	}}
	if n := Resolve(entries, corpusPapers(), ""); n != 1 {
		t.Errorf("resolved %d", n)
	}
}

func TestAnArXivIdentifierResolvesOnItsOwn(t *testing.T) {
	// The identifier is not a guess, so it stands even where the parser
	// could read nothing else off the entry.
	entries := []Entry{{Key: "1", Raw: "arXiv:9101.00001v3", ArXiv: "9101.00001v3"}}
	if n := Resolve(entries, corpusPapers(), ""); n != 1 {
		t.Fatalf("resolved %d", n)
	}
	if entries[0].ResolvesTo != "nkemelu-1991-slowindexes" {
		t.Errorf("resolved to %q", entries[0].ResolvesTo)
	}
}

func TestADOIResolvesOnItsOwnWhateverPrefixItIsPrintedWith(t *testing.T) {
	for _, printed := range []string{
		"10.1145/111111.222222",
		"doi:10.1145/111111.222222",
		"https://doi.org/10.1145/111111.222222",
	} {
		entries := []Entry{{Key: "1", DOI: printed}}
		if n := Resolve(entries, corpusPapers(), ""); n != 1 {
			t.Errorf("%q resolved %d", printed, n)
			continue
		}
		if entries[0].ResolvesTo != "ravensworth-1996-lastword" {
			t.Errorf("%q resolved to %q", printed, entries[0].ResolvesTo)
		}
	}
}

func TestNoReferenceResolvesToTheCitingPaper(t *testing.T) {
	// Rule R05. An entry in a paper's own bibliography is never that paper,
	// and a self loop would break the one data-quality check the citation
	// graph exists to support.
	entries := []Entry{{
		Key:     "1",
		Authors: []string{"Adaeze Nkemelu"},
		Title:   "A theory of slow indexes",
		Year:    1991,
		ArXiv:   "9101.00001",
	}}
	if n := Resolve(entries, corpusPapers(), "nkemelu-1991-slowindexes"); n != 0 {
		t.Errorf("a paper resolved a reference to itself: %q", entries[0].ResolvesTo)
	}
}

func TestResolvingTwiceGivesTheSameAnswer(t *testing.T) {
	entries := []Entry{
		{Key: "1", Authors: []string{"Adaeze Nkemelu"}, Title: "A theory of slow indexes", Year: 1991},
		{Key: "2", Authors: []string{"Fenella Turnbull"}, Title: "Indexes at last", Year: 2001},
	}
	first := Resolve(entries, corpusPapers(), "")
	was := []string{entries[0].ResolvesTo, entries[1].ResolvesTo}
	second := Resolve(entries, corpusPapers(), "")
	if first != second || entries[0].ResolvesTo != was[0] || entries[1].ResolvesTo != was[1] {
		t.Errorf("the second run resolved %d and said %v, the first %d and %v",
			second, []string{entries[0].ResolvesTo, entries[1].ResolvesTo}, first, was)
	}
}

func TestAPaperLeavingTheCorpusUnresolvesTheReference(t *testing.T) {
	// Resolution re-runs on every build and is not a record of what was
	// true once. A stale resolves_to pointing at a paper that has been
	// taken out would be a dead link in the reading app.
	entries := []Entry{{
		Key:        "1",
		Authors:    []string{"Adaeze Nkemelu"},
		Title:      "A theory of slow indexes",
		Year:       1991,
		ResolvesTo: "nkemelu-1991-slowindexes",
	}}
	if n := Resolve(entries, nil, ""); n != 0 || entries[0].ResolvesTo != "" {
		t.Errorf("kept %q with nothing in the corpus", entries[0].ResolvesTo)
	}
}
