package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/prompt"
)

// translateCorpus writes the smallest corpus papers translate will look at:
// one paper, a front file with an abstract in it, a section and a
// bibliography. Nothing here is text from a paper.
func translateCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/papers.yaml", `papers:
  - id: a-1970-paper
    title: A Paper
    authors: [A. Author]
    year: 1970
    venue: A Journal
    field: theory
    status: listed
`)
	write("content/en/a-1970-paper/00_front.md", file("front", "Abstract\n\nThis paper is about a thing."))
	write("content/en/a-1970-paper/01_first.md", file("section", "The first paragraph."))
	write("content/en/a-1970-paper/02_references.md", file("references", "[1] A. Author. A Paper. A Journal, 1970."))

	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// file is one English content file, with the body's hash stamped in the way
// the splitter stamps it, so that the staleness check has something true to
// compare against.
func file(kind, body string) string {
	front := corpus.Front{
		Paper:         "a-1970-paper",
		Title:         "A Paper",
		Field:         corpus.Theory,
		Kind:          kind,
		Lang:          corpus.EN,
		ContentSHA256: corpus.ContentSHA([]byte(body)),
	}
	b, err := corpus.Render(front, []byte(body))
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestEveryEnglishFileIsPlannedForEveryLanguage(t *testing.T) {
	c := translateCorpus(t)
	papers, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI, corpus.ZH}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("%d jobs for three files in two languages", len(jobs))
	}
	for _, j := range jobs {
		if j.abstract == "" {
			t.Errorf("%s went up without the paper's abstract", j.name)
		}
	}
}

func TestTheBibliographyIsCopiedAndNotAsked(t *testing.T) {
	// Author names, the titles of cited works and venue names stand as
	// printed, which is rule L14, and a references file is nothing else.
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.front.Kind != "references" {
			continue
		}
		found = true
		if !j.copied() || j.chunks() != 0 {
			t.Errorf("the bibliography is planned as %d chunks", j.chunks())
		}
	}
	if !found {
		t.Error("the bibliography was left out of the plan altogether")
	}
}

func TestAFileWhoseEnglishHasNotMovedIsNotAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	// A translation that records the hash of the English as it stands.
	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        translatePrompt(t),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.name == "01_first.md" {
			t.Error("a translation that is already an answer to its English was planned again")
		}
	}
	again, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Errorf("-force planned %d of 3 files", len(again))
	}
}

// The prompt is half of what produced the file. A rule added to it is a rule
// the answers on disk were never held to, and the answer to that is to ask
// again, not to leave a corpus where some files followed the rule and some
// did not and nothing on disk says which.
func TestATranslationMadeWithAnOlderPromptIsAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        "0000000000000000000000000000000000000000000000000000000000000000",
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.name == "01_first.md" {
			found = true
		}
	}
	if !found {
		t.Error("a translation written to an older prompt was left as it was")
	}
}

// translatePrompt is the hash of the prompt this build carries, which is what
// a file has to record to count as current.
func translatePrompt(t *testing.T) string {
	t.Helper()
	p, err := prompt.Get(prompt.Translate)
	if err != nil {
		t.Fatal(err)
	}
	return p.SHA
}

func TestATranslationOfAnEnglishFileThatMovedIsPlannedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: corpus.ContentSHA([]byte("something else entirely")),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("%d of 3 files planned, and one of them is stale", len(jobs))
	}
}

func TestAHandEditedTranslationIsLeftAlone(t *testing.T) {
	// The edit is the version of record. A run that overwrote it would throw
	// away the review it came out of and leave no trace of having done so.
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: corpus.ContentSHA([]byte("something else entirely")),
		Edited:              true,
	}, "Đoạn thứ nhất, sửa bằng tay.")

	jobs, err := plan(c, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.name == "01_first.md" {
			t.Error("a hand edited translation was planned for overwriting")
		}
	}
}

// put writes one translated file into the corpus.
func put(t *testing.T, c *corpus.Corpus, name string, front corpus.Front, body string) {
	t.Helper()
	front.ContentSHA256 = corpus.ContentSHA([]byte(body))
	b, err := corpus.Render(front, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	dir := c.Content(front.Lang, front.Paper)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
