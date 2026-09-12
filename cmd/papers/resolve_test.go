package main

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

func manifestOf(ids ...string) *corpus.Papers {
	m := &corpus.Papers{}
	for _, id := range ids {
		m.Papers = append(m.Papers, corpus.Paper{ID: id, Field: "networks"})
	}
	return m
}

func sourcesOf(recs ...corpus.Source) *corpus.Sources {
	return &corpus.Sources{Sources: recs}
}

func picked(t *testing.T, got []corpus.Paper, err error, want ...string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("picked %v, want %v", ids, want)
	}
}

func TestChoosingByIdAndByField(t *testing.T) {
	m := manifestOf("chiu-1989-aimd", "codd-1970-relational", "jacobson-1988-congestion")
	m.Papers[1].Field = "databases"

	got, err := choosePapers(m, "jacobson-1988-congestion,chiu-1989-aimd", "")
	picked(t, got, err, "chiu-1989-aimd", "jacobson-1988-congestion")

	got, err = choosePapers(m, "", "databases")
	picked(t, got, err, "codd-1970-relational")

	got, err = choosePapers(m, "", "")
	picked(t, got, err, "chiu-1989-aimd", "codd-1970-relational", "jacobson-1988-congestion")
}

func TestAnIdThatIsNotInTheManifestIsAnError(t *testing.T) {
	_, err := choosePapers(manifestOf("codd-1970-relational"), "codd-1970-relatonal", "")
	if err == nil {
		t.Fatal("a misspelled id was accepted")
	}
	if !strings.Contains(err.Error(), "codd-1970-relatonal") {
		t.Errorf("the error does not say which id: %v", err)
	}
}

// The regression. selectPapers used to do the manifest lookup and the
// skipping in one pass, so a paper it found and then skipped came out of the
// far end as a paper that does not exist. Naming a hand recorded paper was
// answered with "there is no paper called chiu-1989-aimd in the manifest"
// over a paper sitting in the manifest, and papers classify, which shared the
// picker, could not measure an adopted PDF at all.
func TestASkippedPaperIsNotAMissingPaper(t *testing.T) {
	m := manifestOf("chiu-1989-aimd")
	recorded := sourcesOf(corpus.Source{ID: "chiu-1989-aimd", By: corpus.ByHand, URL: "https://example.org/paper.pdf"})

	got, err := selectPapers(m, recorded, "chiu-1989-aimd", "", true)
	if err != nil {
		t.Fatalf("a hand recorded paper was reported as missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("selectPapers offered %v, want a hand record left alone", got)
	}
	// The same paper is there for a command that reads the file rather than
	// going to the network, which is the whole reason for the split.
	got, err = choosePapers(m, "chiu-1989-aimd", "")
	picked(t, got, err, "chiu-1989-aimd")
}

func TestAResolvedPaperIsOnlyOfferedAgainOnRequest(t *testing.T) {
	m := manifestOf("codd-1970-relational")
	recorded := sourcesOf(corpus.Source{ID: "codd-1970-relational", URL: "https://example.org/codd.pdf"})

	got, err := selectPapers(m, recorded, "", "", false)
	picked(t, got, err)

	got, err = selectPapers(m, recorded, "", "", true)
	picked(t, got, err, "codd-1970-relational")
}

func TestAPaperWithNoRecordIsAlwaysOffered(t *testing.T) {
	m := manifestOf("codd-1970-relational")
	got, err := selectPapers(m, sourcesOf(), "", "", false)
	picked(t, got, err, "codd-1970-relational")
}
