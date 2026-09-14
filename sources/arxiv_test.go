package sources

import (
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// The primary category is one of three the submission is filed under, and it
// is the one the author picked.
func TestTheArXivPrimaryCategoryIsReadAndTheCrossListingsAreNot(t *testing.T) {
	cands, err := parseArXiv([]byte(arxivAtom))
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].Category != "cs.CL" {
		t.Errorf("the category is %q, want cs.CL", cands[0].Category)
	}
	if cands[0].Venue != "arXiv" {
		t.Errorf("the venue is %q", cands[0].Venue)
	}
}

func TestFieldOfReadsACategoryTheCorpusHasOneAnswerFor(t *testing.T) {
	for _, c := range []struct {
		category string
		want     corpus.Field
		known    bool
	}{
		{"cs.CL", corpus.AIML, true},
		{"cs.NI", corpus.Networks, true},
		{"stat.ML", corpus.AIML, true},
		// Not a category the corpus reads one way. Better to be told.
		{"cs.IT", "", false},
		{"math.PR", "", false},
		{"", "", false},
	} {
		got, ok := FieldOf(c.category)
		if ok != c.known {
			t.Errorf("FieldOf(%q) known is %v, want %v", c.category, ok, c.known)
		}
		if got != c.want {
			t.Errorf("FieldOf(%q) is %q, want %q", c.category, got, c.want)
		}
	}
}

// Crossref is the rung that knows the venue, which is most of why the
// resolver goes there even for a paper it has already found on arXiv.
func TestCrossrefCarriesTheVenue(t *testing.T) {
	cand, err := parseCrossrefWork([]byte(crossrefWorkJSON))
	if err != nil {
		t.Fatal(err)
	}
	if cand.Venue != "" {
		t.Errorf("the fixture has no container-title and the venue came back as %q", cand.Venue)
	}
	cand, err = parseCrossrefWork([]byte(`{"message":{"DOI":"10.1145/1","title":["A Paper"],"container-title":["Communications of the\n ACM"],"issued":{"date-parts":[[1970]]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if cand.Venue != "Communications of the ACM" {
		t.Errorf("the venue is %q", cand.Venue)
	}
}
