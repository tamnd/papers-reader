package refs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifest() *Manifest {
	return &Manifest{
		Paper: "somebody-2020-citing",
		Style: StyleBracket,
		Entries: []Entry{
			{Key: "1", Raw: "A. Nkemelu. A theory of slow indexes. 1991.", Title: "A theory of slow indexes", Year: 1991, ResolvesTo: "nkemelu-1991-slowindexes"},
			{Key: "2", Raw: "B. Oyelaran. Indexes that are slower still. 1994.", Title: "Indexes that are slower still", Year: 1994},
		},
	}
}

func TestAManifestSurvivesTheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refs", "somebody-2020-citing.yaml")
	if err := manifest().Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Paper != "somebody-2020-citing" || back.Style != StyleBracket || len(back.Entries) != 2 {
		t.Fatalf("read back %+v", back)
	}
	if back.Entries[0].ResolvesTo != "nkemelu-1991-slowindexes" || back.Entries[1].ResolvesTo != "" {
		t.Errorf("the entries read back as %+v", back.Entries)
	}
}

func TestAnUnresolvedEntryStillSaysSo(t *testing.T) {
	// resolves_to is written out empty rather than left out, so that a
	// person reading the file can see the question was asked and answered.
	path := filepath.Join(t.TempDir(), "somebody-2020-citing.yaml")
	if err := manifest().Save(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), "resolves_to:") != 2 {
		t.Errorf("the file reads\n%s", b)
	}
}

func TestSavingOverAManifestLeavesOneFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "somebody-2020-citing.yaml")
	for i := 0; i < 2; i++ {
		if err := manifest().Save(path); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files", len(entries))
	}
}

func TestAManifestKnowsWhatItResolved(t *testing.T) {
	m := manifest()
	if m.Resolved() != 1 {
		t.Errorf("counted %d resolved", m.Resolved())
	}
	if got := m.Links(); len(got) != 1 || got["1"] != "nkemelu-1991-slowindexes" {
		t.Errorf("the links are %v", got)
	}
	if e, ok := m.Entry("2"); !ok || e.Title != "Indexes that are slower still" {
		t.Errorf("looking up entry 2 gave %+v", e)
	}
	if _, ok := m.Entry("9"); ok {
		t.Error("found an entry that is not there")
	}
}

func TestAGapIsAnEntryTheManifestSaysIsNotThere(t *testing.T) {
	m := &Manifest{Gaps: []Gap{{Key: "67", Why: "the page runs 66 then 68 in four reads of it"}}}
	if !m.Missing("67") {
		t.Error("the manifest does not know 67 is a gap")
	}
	if m.Missing("68") {
		t.Error("the manifest calls 68 a gap and it is not one")
	}
	if (&Manifest{}).Missing("67") {
		t.Error("a manifest with no gaps at all has one")
	}
}
