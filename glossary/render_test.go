package glossary

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
)

// answering is a Renderer whose fleet is a function of the question, which is
// what every test here needs and none of them needs twice.
func answering(t *testing.T, reply func(asked string) string) (*Renderer, *[]string) {
	t.Helper()
	var asks []string
	r := &Renderer{
		Ask: func(_ context.Context, _ string, req llm.Request) (Reply, error) {
			asks = append(asks, req.Instructions)
			return Reply{Response: llm.Response{Text: reply(req.Instructions)}}, nil
		},
		Logf: func(string, ...any) {},
	}
	return r, &asks
}

func TestTheRenderingsComeBackIntoTheGlossary(t *testing.T) {
	g := &Glossary{Terms: []Term{{En: "throughput"}, {En: "latency"}}}
	r, _ := answering(t, func(string) string {
		return "throughput :: thông lượng\nlatency :: độ trễ\n"
	})

	filled, err := r.Fill(context.Background(), g, corpus.VI)
	if err != nil {
		t.Fatal(err)
	}
	if filled != 2 {
		t.Errorf("%d terms filled", filled)
	}
	if g.Terms[0].Vi != "thông lượng" || g.Terms[1].Vi != "độ trễ" {
		t.Errorf("the glossary is %+v", g.Terms)
	}
	if g.Terms[0].Zh != "" {
		t.Error("the Vietnamese run wrote Chinese")
	}
}

func TestOnlyTheMissingTermsAreAskedAbout(t *testing.T) {
	// A reviewer's correction has to survive a second run, or the review is
	// work somebody does twice.
	g := &Glossary{Terms: []Term{
		{En: "serializability", Vi: "khả tuần tự"},
		{En: "latency"},
	}}
	r, asks := answering(t, func(string) string {
		return "serializability :: SOMETHING ELSE\nlatency :: độ trễ\n"
	})

	if _, err := r.Fill(context.Background(), g, corpus.VI); err != nil {
		t.Fatal(err)
	}
	if g.Terms[0].Vi != "khả tuần tự" {
		t.Errorf("a rendering that was already there was replaced with %q", g.Terms[0].Vi)
	}
	if strings.Contains((*asks)[0], "serializability") {
		t.Error("a term that already had a rendering was sent up anyway")
	}
}

func TestKeepIsWrittenInAsTheEnglishAndNotAsTheWordKeep(t *testing.T) {
	g := &Glossary{Terms: []Term{{En: "MapReduce"}}}
	r, _ := answering(t, func(string) string { return "MapReduce :: KEEP" })

	if _, err := r.Fill(context.Background(), g, corpus.JA); err != nil {
		t.Fatal(err)
	}
	if g.Terms[0].Ja != "MapReduce" {
		t.Errorf("KEEP came out as %q", g.Terms[0].Ja)
	}
}

func TestKeepInOneLanguageDoesNotDecideTheOtherTwo(t *testing.T) {
	// This is the bug that got the first glossary wrong. The model said
	// cache and server stand in Vietnamese, which is true, the term was
	// flagged as kept outright, and Chinese and Japanese were never asked
	// about it. Chinese says 缓存 and 服务器 and has done for thirty years.
	g := &Glossary{Terms: []Term{{En: "cache"}}}
	r, _ := answering(t, func(string) string { return "cache :: KEEP" })

	if _, err := r.Fill(context.Background(), g, corpus.VI); err != nil {
		t.Fatal(err)
	}
	if g.Terms[0].Keep {
		t.Error("one language's answer flagged the term kept in all three")
	}
	if _, done := g.Terms[0].Rendering(corpus.ZH); done {
		t.Error("Chinese was counted as covered without being asked")
	}
}

func TestATermTheModelSkippedIsLeftEmpty(t *testing.T) {
	// The gap is the point. A guessed rendering is indistinguishable from a
	// looked up one once it is in the file.
	g := &Glossary{Terms: []Term{{En: "throughput"}, {En: "serializability"}}}
	said := []string{}
	r, _ := answering(t, func(string) string { return "throughput :: 吞吐量" })
	r.Logf = func(format string, args ...any) { said = append(said, format) }

	filled, err := r.Fill(context.Background(), g, corpus.ZH)
	if err != nil {
		t.Fatal(err)
	}
	if filled != 1 || g.Terms[1].Zh != "" {
		t.Errorf("%d filled and the second term is %q", filled, g.Terms[1].Zh)
	}
	if len(said) != 1 {
		t.Errorf("the skipped term was not reported: %v", said)
	}
}

func TestTheNotesGoUpWithTheTerms(t *testing.T) {
	// Which sense of an English word the papers mean is the whole difficulty,
	// and the note is where it was decided.
	g := &Glossary{Terms: []Term{{En: "model", Note: "in databases, the description of a system"}}}
	r, asks := answering(t, func(string) string { return "model :: mô hình" })

	if _, err := r.Fill(context.Background(), g, corpus.VI); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains((*asks)[0], "in databases, the description of a system") {
		t.Error("the note was not sent with the term")
	}
	if !strings.Contains((*asks)[0], "Vietnamese") {
		t.Error("the prompt does not say which language it is asking for")
	}
}

func TestTheTermsAreAskedForInBatches(t *testing.T) {
	// Not one at a time, which is the overhead of a call for every word and
	// gives the model no way to be consistent between two terms that share
	// one. Not all at once either, which is where a long answer turns into a
	// summary of itself.
	var terms []Term
	for _, en := range []string{"a term", "b term", "c term", "d term", "e term"} {
		terms = append(terms, Term{En: en})
	}
	g := &Glossary{Terms: terms}
	r, asks := answering(t, func(asked string) string {
		var out []string
		for _, line := range strings.Split(asked, "\n") {
			if strings.HasSuffix(line, " term") {
				out = append(out, line+" :: x")
			}
		}
		return strings.Join(out, "\n")
	})
	r.Size = 2

	filled, err := r.Fill(context.Background(), g, corpus.VI)
	if err != nil {
		t.Fatal(err)
	}
	if filled != 5 {
		t.Errorf("%d of 5 filled", filled)
	}
	if len(*asks) != 3 {
		t.Errorf("%d asks for 5 terms in batches of 2", len(*asks))
	}
}

func TestAnAlternativeComesBackAsANote(t *testing.T) {
	g := &Glossary{Terms: []Term{{En: "throughput"}}}
	r, _ := answering(t, func(string) string {
		return `throughput :: thông lượng :: also "băng thông", but that is bandwidth`
	})

	if _, err := r.Fill(context.Background(), g, corpus.VI); err != nil {
		t.Fatal(err)
	}
	if g.Terms[0].Vi != "thông lượng" {
		t.Errorf("the rendering is %q", g.Terms[0].Vi)
	}
	if !strings.Contains(g.Terms[0].Note, "băng thông") {
		t.Errorf("the alternative was lost: %q", g.Terms[0].Note)
	}
	if !strings.HasPrefix(g.Terms[0].Note, "vi:") {
		t.Errorf("the note does not say which language it is about: %q", g.Terms[0].Note)
	}
}

func TestTheAnswerIsReadThroughWhateverTheModelDressedItIn(t *testing.T) {
	// Every one of these came off a real fleet at some point. None of them is
	// worth asking forty terms again over.
	answer := "```\n" +
		"1. throughput :: thông lượng\n" +
		"- latency :: độ trễ\n" +
		"**cache** :: bộ nhớ đệm\n" +
		"\n" +
		"Here is the list you asked for.\n" +
		"```\n"
	got := Parse(answer)
	for en, want := range map[string]string{
		"throughput": "thông lượng",
		"latency":    "độ trễ",
		"cache":      "bộ nhớ đệm",
	} {
		if got[en].Text != want {
			t.Errorf("%s came out as %q", en, got[en].Text)
		}
	}
	if len(got) != 3 {
		t.Errorf("the sentence was read as an answer: %v", got)
	}
}

func TestAFullWidthColonIsNotASeparator(t *testing.T) {
	// Which is why the separator is two colons. A Chinese rendering with a
	// colon in it is ordinary punctuation, and a parser that split on one
	// would cut the rendering in half.
	got := Parse("read set :: 读集合：事务读过的项")
	if got["read set"].Text != "读集合：事务读过的项" {
		t.Errorf("the rendering came out as %q", got["read set"].Text)
	}
}

func TestALineWithNoSeparatorIsNotAnAnswer(t *testing.T) {
	got := Parse("I cannot translate these terms without more context.")
	if len(got) != 0 {
		t.Errorf("a refusal was read as %d renderings: %v", len(got), got)
	}
}

func TestSavingKeepsTheHeaderAndSortsTheTerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.yaml")
	const head = "# The controlled vocabulary.\n#\n# Reviewed by hand.\n"
	if err := os.WriteFile(path, []byte(head+"version: 1\nterms:\n  - en: throughput\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g.Terms = append(g.Terms, Term{En: "cache", Vi: "bộ nhớ đệm"})
	g.Version = 2
	if err := g.Save(path); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.HasPrefix(text, head) {
		t.Errorf("the header did not survive the write:\n%s", text)
	}
	if strings.Index(text, "cache") > strings.Index(text, "throughput") {
		t.Errorf("the terms were not sorted:\n%s", text)
	}
	if !strings.Contains(text, "  - en: cache") {
		t.Errorf("the file is not indented the way it is written by hand:\n%s", text)
	}

	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Version != 2 || len(again.Terms) != 2 {
		t.Errorf("what was written back reads as %+v", again)
	}
	if got, _ := again.Terms[0].Rendering(corpus.VI); got != "bộ nhớ đệm" {
		t.Errorf("the rendering came back as %q", got)
	}
}
