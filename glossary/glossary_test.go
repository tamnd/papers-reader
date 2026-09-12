package glossary

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

func TestAKeptTermRendersAsItsEnglish(t *testing.T) {
	// Transformer is Transformer in all three, and that is a decision
	// somebody made rather than a gap in the table.
	term := Term{En: "Transformer", Keep: true}
	for _, l := range []corpus.Lang{corpus.VI, corpus.ZH, corpus.JA} {
		got, ok := term.Rendering(l)
		if !ok || got != "Transformer" {
			t.Errorf("%s renders as %q, %v", l, got, ok)
		}
	}
}

func TestATermWithNoRenderingSaysSo(t *testing.T) {
	term := Term{En: "hash table", Vi: "bảng băm"}
	if got, ok := term.Rendering(corpus.VI); !ok || got != "bảng băm" {
		t.Errorf("vi renders as %q, %v", got, ok)
	}
	if _, ok := term.Rendering(corpus.JA); ok {
		t.Error("ja has a rendering it was never given")
	}
	if _, ok := (Term{En: "x", Zh: "   "}).Rendering(corpus.ZH); ok {
		t.Error("a rendering of spaces counts as a rendering")
	}
}

func TestAScopedTermIsOnlyOfferedToItsField(t *testing.T) {
	// "reduction" is one thing in complexity theory and another in
	// compilers. A rendering offered to the wrong paper is worse than none,
	// because the translator will take it.
	g := &Glossary{Terms: []Term{
		{En: "reduction", Field: corpus.Theory},
		{En: "reduction", Field: corpus.Languages},
		{En: "throughput"},
	}}
	got := g.For(corpus.Theory)
	if len(got) != 2 {
		t.Fatalf("%d terms offered to theory, want the global one and its own", len(got))
	}
	for _, term := range got {
		if term.Field == corpus.Languages {
			t.Errorf("a compilers term was offered to a theory paper")
		}
	}
}

func TestTheLongestTermComesFirst(t *testing.T) {
	// A prompt is read in order. "hash table" has to be decided before
	// "table" is, or the two words of the phrase are rendered separately
	// and the phrase comes out as neither.
	g := &Glossary{Terms: []Term{{En: "table"}, {En: "hash table"}, {En: "set"}}}
	got := g.For(corpus.Algorithms)
	if got[0].En != "hash table" {
		t.Errorf("the list starts %q", got[0].En)
	}
}

func TestCoverageCountsAKeptTermAsCovered(t *testing.T) {
	g := &Glossary{Terms: []Term{
		{En: "MapReduce", Keep: true},
		{En: "throughput", Vi: "thông lượng"},
		{En: "cache"},
	}}
	have, total := g.Coverage(corpus.VI)
	if have != 2 || total != 3 {
		t.Errorf("coverage is %d of %d, want 2 of 3", have, total)
	}
	missing := g.Missing(corpus.VI)
	if len(missing) != 1 || missing[0].En != "cache" {
		t.Errorf("missing is %v", missing)
	}
}

func TestAGlossaryThatIsNotThereIsEmptyAndNotAnError(t *testing.T) {
	g, err := Load(filepath.Join(t.TempDir(), "glossary.yaml"))
	if err != nil {
		t.Fatalf("a missing glossary is an error: %v", err)
	}
	if g.Version != 0 || len(g.Terms) != 0 {
		t.Errorf("a missing glossary loaded as %+v", g)
	}
}

func TestAGlossaryIsReadFromTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.yaml")
	const text = `version: 3
terms:
  - en: attention
    vi: cơ chế chú ý
    field: ai-ml
    note: the mechanism, not the ordinary word
  - en: MapReduce
    keep: true
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if g.Version != 3 || len(g.Terms) != 2 {
		t.Fatalf("read version %d and %d terms", g.Version, len(g.Terms))
	}
	if g.Terms[0].Field != corpus.AIML || g.Terms[0].Vi != "cơ chế chú ý" {
		t.Errorf("the first term is %+v", g.Terms[0])
	}
	if !g.Terms[1].Keep {
		t.Errorf("keep did not survive the file")
	}
	if !g.Has("ATTENTION") || g.Has("cache") {
		t.Errorf("Has does not match the way the extractor counts")
	}
}
