package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// stub writes empty content files so that a test can say which files a
// paper's directories hold without caring what is in them.
func stub(t *testing.T, c *corpus.Corpus, l corpus.Lang, id string, names ...string) {
	t.Helper()
	dir := c.Content(l, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("---\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// held is the files a paper's directory holds, for comparing against what a
// test expected to be left.
func held(t *testing.T, c *corpus.Corpus, l corpus.Lang, id string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(c.Content(l, id), "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range names {
		out = append(out, filepath.Base(p))
	}
	return out
}

// Paxos was read again, its second section came back under a different
// title, and the Vietnamese copy of the old one stayed where it was. Two
// files numbered 02 is a hard T04 failure, so the paper never published.
func TestATranslationOfASectionThatIsGoneIsDeleted(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	stub(t, c, corpus.EN, "a-paper", "00_front.md", "02_the_single_decree_synod.md")
	stub(t, c, corpus.VI, "a-paper", "00_front.md", "02_the_single_decree_synod.md", "02_references.md")
	stub(t, c, corpus.ZH, "a-paper", "00_front.md", "02_references.md")

	gone, err := orphans(c, "a-paper")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"vi/02_references.md", "zh/02_references.md"}
	if !equal(gone, want) {
		t.Errorf("deleted %v, want %v", gone, want)
	}
	if got := held(t, c, corpus.VI, "a-paper"); !equal(got, []string{"00_front.md", "02_the_single_decree_synod.md"}) {
		t.Errorf("the Vietnamese directory holds %v", got)
	}
	if got := held(t, c, corpus.EN, "a-paper"); len(got) != 2 {
		t.Errorf("the English directory holds %v and this should not have touched it", got)
	}
}

// A language a paper has no directory for is not an error and not a note.
func TestAPaperWithNoTranslationsHasNoOrphans(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	stub(t, c, corpus.EN, "a-paper", "00_front.md")
	gone, err := orphans(c, "a-paper")
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Errorf("deleted %v", gone)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GPT-3 had its extracted pages cleaned up, so the splitter could not run on
// it at all, and a Vietnamese file numbered 17 with no English beside it was
// out of reach of the only thing that deletes one. It failed three hard rules
// and held the whole paper out of the corpus.
func TestATranslationIsSweptEvenWhenNothingWasSplit(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	stub(t, c, corpus.EN, "a-paper", "18_references.md")
	stub(t, c, corpus.VI, "a-paper", "17_references.md", "18_references.md")

	gone, err := orphans(c, "a-paper")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"vi/17_references.md"}; !equal(gone, want) {
		t.Errorf("deleted %v, want %v", gone, want)
	}
	if got := held(t, c, corpus.VI, "a-paper"); !equal(got, []string{"18_references.md"}) {
		t.Errorf("the Vietnamese directory holds %v", got)
	}
}
