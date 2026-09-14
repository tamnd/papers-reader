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
		{En: "reduction", Fields: []corpus.Field{corpus.Theory}},
		{En: "reduction", Fields: []corpus.Field{corpus.Languages}},
		{En: "throughput"},
	}}
	got := g.For(corpus.Theory)
	if len(got) != 2 {
		t.Fatalf("%d terms offered to theory, want the global one and its own", len(got))
	}
	for _, term := range got {
		if !term.Offered(corpus.Theory) {
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

// A sense usually spans a few fields. "log" is the append only record in
// systems and in databases, and the logarithm in a paper about generative
// models, where offering the record's rendering is how a translator ends up
// writing the Vietnamese for a log file into a likelihood.
func TestASenseThatSpansAFewFieldsIsOneTerm(t *testing.T) {
	g := &Glossary{Terms: []Term{
		{En: "log", Vi: "nhật ký", Fields: []corpus.Field{corpus.Systems, corpus.Databases}},
		{En: "throughput", Vi: "thông lượng"},
	}}
	for _, f := range []corpus.Field{corpus.Systems, corpus.Databases} {
		if len(g.For(f)) != 2 {
			t.Errorf("%s was offered %d terms, want both", f, len(g.For(f)))
		}
	}
	if got := g.For(corpus.AIML); len(got) != 1 || got[0].En != "throughput" {
		t.Errorf("an ai-ml paper was offered %v", got)
	}
	if TermsSHA(g, corpus.Systems, corpus.VI) == TermsSHA(g, corpus.AIML, corpus.VI) {
		t.Error("two fields offered different terms and hashed the same")
	}
}

func TestAGlossaryIsReadFromTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.yaml")
	const text = `version: 3
terms:
  - en: attention
    vi: cơ chế chú ý
    fields: [ai-ml]
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
	if !g.Terms[0].Offered(corpus.AIML) || g.Terms[0].Offered(corpus.Theory) || g.Terms[0].Vi != "cơ chế chú ý" {
		t.Errorf("the first term is %+v", g.Terms[0])
	}
	if !g.Terms[1].Keep {
		t.Errorf("keep did not survive the file")
	}
	if !g.Has("ATTENTION") || g.Has("cache") {
		t.Errorf("Has does not match the way the extractor counts")
	}
}

func TestTheTermsHashTracksWhatAPaperWasTranslatedAgainst(t *testing.T) {
	// The version says the glossary moved. This says whether it moved under
	// this paper's feet, and without it every version bump queues the whole
	// corpus for translating again.
	g := &Glossary{Version: 1, Terms: []Term{
		{En: "attention", Vi: "cơ chế chú ý", Fields: []corpus.Field{corpus.AIML}},
		{En: "reduction", Vi: "phép rút gọn", Fields: []corpus.Field{corpus.Languages}},
		{En: "graph", Vi: "đồ thị"},
	}}
	ml := TermsSHA(g, corpus.AIML, corpus.VI)

	if TermsSHA(g, corpus.Languages, corpus.VI) == ml {
		t.Error("two fields that are offered different terms hash the same")
	}
	if TermsSHA(g, corpus.AIML, corpus.ZH) == ml {
		t.Error("two languages hash the same, and one of them has no renderings at all")
	}

	g.Version = 2
	if TermsSHA(g, corpus.AIML, corpus.VI) != ml {
		t.Error("a version bump that changed no rendering moved the hash")
	}
	g.Terms[1].Vi = "phép quy giản"
	if TermsSHA(g, corpus.AIML, corpus.VI) != ml {
		t.Error("a rendering in another field moved this field's hash")
	}
	g.Terms[2].Vi = "biểu đồ"
	if TermsSHA(g, corpus.AIML, corpus.VI) == ml {
		t.Error("a global term changed and the hash stood still")
	}
}
