package main

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/sources"
)

// attention is what the arXiv API says about a paper that is already in the
// corpus, which makes it a candidate every test here can reason about without
// having to invent one.
func attention() sources.Candidate {
	return sources.Candidate{
		Title:    "Attention Is All You Need",
		Authors:  []string{"Ashish Vaswani", "Noam Shazeer"},
		Year:     2017,
		ArXiv:    "1706.03762",
		Venue:    "arXiv",
		Category: "cs.CL",
		Source:   "arxiv",
	}
}

func entryOf(t *testing.T, cand sources.Candidate, id, field, aka, idea string, difficulty int) corpus.Paper {
	t.Helper()
	p, err := newEntry(cand, id, field, aka, idea, difficulty)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnEntryIsWhatTheServiceSaid(t *testing.T) {
	p := entryOf(t, attention(), "", "", "", "", 0)
	switch {
	case p.Title != "Attention Is All You Need":
		t.Errorf("the title is %q", p.Title)
	case len(p.Authors) != 2 || p.Authors[0] != "Ashish Vaswani":
		t.Errorf("the authors are %v", p.Authors)
	case p.Year != 2017:
		t.Errorf("the year is %d", p.Year)
	case p.ArXiv != "1706.03762":
		t.Errorf("the arXiv id is %q", p.ArXiv)
	case p.Venue != "arXiv":
		t.Errorf("the venue is %q", p.Venue)
	}
}

// The statuses are a pipeline. An entry arriving as anything but listed would
// be a claim about files that have not been fetched.
func TestANewEntryIsListedAndNothingElse(t *testing.T) {
	if got := entryOf(t, attention(), "", "", "", "", 0).Status; got != corpus.Listed {
		t.Errorf("a new entry arrives as %q", got)
	}
}

// The access class belongs to papers resolve, which is the command that reads
// the licence and writes the fact into sources.yaml. An entry that guessed it
// would be writing a guess into the one field the manifest's own header calls
// a guess.
func TestANewEntryClaimsNothingAboutAccess(t *testing.T) {
	if got := entryOf(t, attention(), "", "", "", "", 0).Expect; got != "" {
		t.Errorf("the entry expects %q", got)
	}
}

// The seed list keeps its numbers so that a reader who arrives from the
// numbered list can find entry 63, and that only works if nothing added later
// is numbered 101.
func TestANewEntryHasNoNumber(t *testing.T) {
	if n := entryOf(t, attention(), "", "", "", "", 0).Number; n != 0 {
		t.Errorf("a new entry was numbered %d", n)
	}
}

func TestTheIdIsSuggestedAndTheFlagWins(t *testing.T) {
	if got := entryOf(t, attention(), "", "", "", "", 0).ID; got != "vaswani-2017-attention" {
		t.Errorf("the suggested id is %q", got)
	}
	if got := entryOf(t, attention(), "vaswani-2017-transformer", "", "", "", 0).ID; got != "vaswani-2017-transformer" {
		t.Errorf("-id gave %q", got)
	}
}

// A service that hands back no author leaves nothing to build an id out of,
// and the error has to say what to do about it rather than what went wrong.
func TestAnEntryWithNoIdToSuggestSaysToPassOne(t *testing.T) {
	cand := attention()
	cand.Authors = nil
	_, err := newEntry(cand, "", "", "", "", 0)
	if err == nil {
		t.Fatal("an entry with no authors and no -id was accepted")
	}
	if !strings.Contains(err.Error(), "pass -id") {
		t.Errorf("the error reads %q", err)
	}
}

func TestAKAIsSplitAndTrimmedAndAnEmptyOneIsDropped(t *testing.T) {
	p := entryOf(t, attention(), "", "", "Transformer,  Attention paper , ", "", 0)
	if strings.Join(p.AKA, "|") != "Transformer|Attention paper" {
		t.Errorf("the other names are %v", p.AKA)
	}
	if got := entryOf(t, attention(), "", "", "", "", 0).AKA; got != nil {
		t.Errorf("no -aka gave %v", got)
	}
}

func TestDifficultyIsOneToFive(t *testing.T) {
	if got := entryOf(t, attention(), "", "", "", "", 4).Difficulty; got != 4 {
		t.Errorf("the difficulty is %d", got)
	}
	for _, bad := range []int{-1, 6} {
		if _, err := newEntry(attention(), "", "", "", "", bad); err == nil {
			t.Errorf("a difficulty of %d was accepted", bad)
		}
	}
}

func TestTheGroupComesFromTheArXivCategory(t *testing.T) {
	if got := entryOf(t, attention(), "", "", "", "", 0).Field; got != corpus.AIML {
		t.Errorf("cs.CL was filed under %q", got)
	}
}

// A category the corpus reads more than one way, and a DOI, both carry no
// answer, and in each case the error has to name the way out.
func TestAGroupThatCannotBeGuessedAsksForTheFlag(t *testing.T) {
	ambiguous := attention()
	ambiguous.Category = "cs.IT"
	if _, err := groupOf("", ambiguous); err == nil || !strings.Contains(err.Error(), "cs.IT") {
		t.Errorf("an unreadable category gave %v", err)
	}

	doi := sources.Candidate{Source: "crossref"}
	err := func() error { _, err := groupOf("", doi); return err }()
	if err == nil {
		t.Fatal("a candidate with no category at all was filed somewhere")
	}
	if !strings.Contains(err.Error(), "-field") {
		t.Errorf("the error reads %q", err)
	}
}

func TestTheFieldFlagBeatsTheCategoryAndHasToBeAGroup(t *testing.T) {
	// cs.CL reads as aiml, and a person saying otherwise is a person who has
	// read the paper.
	if got, err := groupOf("theory", attention()); err != nil || got != corpus.Theory {
		t.Errorf("-field theory gave %q, %v", got, err)
	}
	if _, err := groupOf("machine-learning", attention()); err == nil {
		t.Error("a group the manifest does not have was accepted")
	}
}

// Every group the manifest has is a group the flag takes, which is the part
// of this that breaks quietly when a group is added to corpus.Fields.
func TestEveryGroupOfTheManifestIsAGroupTheFlagTakes(t *testing.T) {
	for _, f := range corpus.Fields {
		if got, err := groupOf(string(f), attention()); err != nil || got != f {
			t.Errorf("-field %s gave %q, %v", f, got, err)
		}
	}
}
