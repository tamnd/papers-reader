package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

const papersYAML = `papers:
  - id: vaswani-2017-attention
    title: Attention Is All You Need
    authors: [Ashish Vaswani]
    year: 2017
    field: ai-ml
    status: listed
  - id: codd-1970-relational
    title: A Relational Model of Data for Large Shared Data Banks
    authors: [E. F. Codd]
    year: 1970
    field: databases
    status: listed
`

// front is a content file with nothing in the body, because ReadPages reads
// the front matter and the body is somebody else's business.
func front(paper, pages, path, model string) string {
	return "---\npaper: " + paper + "\nkind: section\nlang: en\npdf_pages: \"" + pages +
		"\"\nextraction: " + path + "\nextraction_model: " + model + "\n---\n\nA line of the paper.\n"
}

// corpusOf writes a corpus and opens it. Files are given as paths relative
// to the root, so a test reads as a description of what is on disk.
func corpusOf(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{"manifests/papers.yaml": papersYAML}
	for k, v := range files {
		all[k] = v
	}
	for path, body := range all {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func read(t *testing.T, c *corpus.Corpus) []Read {
	t.Helper()
	out, err := ReadPages(c)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAPageIsCountedOnceHoweverManySectionsCameOffIt(t *testing.T) {
	// Section 2 ends halfway down page 4 and section 3 starts under it.
	// That is one page, read once, and a count of five would be this report
	// claiming work nobody did.
	c := corpusOf(t, map[string]string{
		"content/en/vaswani-2017-attention/01_intro.md":     front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
		"content/en/vaswani-2017-attention/02_attention.md": front("vaswani-2017-attention", "4-5", "native", "pdftotext 25"),
	})
	got := read(t, c)
	if len(got) != 1 {
		t.Fatalf("%d paths, want the one: %+v", len(got), got)
	}
	if got[0].Pages != 4 || got[0].Papers != 1 {
		t.Errorf("%+v, want 4 pages of 1 paper", got[0])
	}
}

func TestThePathsComeInTheOrderTheCorpusTrustsThem(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/codd-1970-relational/01_model.md":       front("codd-1970-relational", "1-3", "ocr", "a vision model"),
		"content/en/vaswani-2017-attention/01_intro.md":     front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
		"content/en/vaswani-2017-attention/02_attention.md": front("vaswani-2017-attention", "5", "layout", "a layout model"),
	})
	got := read(t, c)
	if len(got) != 3 {
		t.Fatalf("%d paths, want 3: %+v", len(got), got)
	}
	for i, want := range []string{"native", "layout", "ocr"} {
		if got[i].Path != want {
			t.Errorf("path %d is %s, want %s", i, got[i].Path, want)
		}
	}
	if got[2].Pages != 3 || got[2].Models[0] != "a vision model" {
		t.Errorf("the ocr row is %+v", got[2])
	}
}

func TestATranslationIsNotASecondReadingOfThePage(t *testing.T) {
	// The Vietnamese was translated from the English and no model went near
	// the PDF to make it. Counting it here would double every page of every
	// paper that has been translated.
	c := corpusOf(t, map[string]string{
		"content/en/vaswani-2017-attention/01_intro.md": front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
		"content/vi/vaswani-2017-attention/01_intro.md": front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
	})
	if got := read(t, c); len(got) != 1 || got[0].Pages != 3 {
		t.Errorf("%+v, want 3 pages counted once", got)
	}
}

func TestAFileThatDoesNotSayHowItWasReadIsNotCounted(t *testing.T) {
	// A front matter with no extraction field is a file nobody can say was
	// read by a model or by pdftotext, and a report that guesses is worse
	// than one that leaves it out. The audit is what objects to it.
	c := corpusOf(t, map[string]string{
		"content/en/codd-1970-relational/01_model.md": "---\npaper: codd-1970-relational\nkind: section\nlang: en\npdf_pages: \"1-3\"\n---\n\nA line.\n",
	})
	if got := read(t, c); len(got) != 0 {
		t.Errorf("%+v, want nothing counted", got)
	}
}

func TestAPaperWithNoContentCountsNothingAndIsNotAnError(t *testing.T) {
	// Which is every restricted paper until the front matter is split, and
	// every paper at all before extraction has run.
	if got := read(t, corpusOf(t, nil)); len(got) != 0 {
		t.Errorf("%+v, want nothing counted", got)
	}
}

func TestThePagesReadAreInTheReport(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/vaswani-2017-attention/01_intro.md": front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
	})
	u := BuildUsage(nil, UsageOptions{Stages: []string{"extract"}, Reads: read(t, c)})
	md := u.Markdown()
	if !strings.Contains(md, "| native | 1 | 3 | pdftotext 25 |") {
		t.Errorf("the pages read are not in the report:\n%s", md)
	}
}

func TestAPageSpanIsReadOrCountsNothing(t *testing.T) {
	for _, c := range []struct {
		s           string
		first, last int
	}{
		{"7", 7, 7},
		{"2-5", 2, 5},
		{"2 - 5", 2, 5},
		{"", 0, 0},
		{"front", 0, 0},
		{"0", 0, 0},
		// A range that runs backwards is a field somebody wrote by hand and
		// got wrong. The page it starts on is the one thing in it that can
		// still be believed.
		{"4-2", 4, 4},
		{"-3", 0, 0},
	} {
		first, last := span(c.s)
		if first != c.first || last != c.last {
			t.Errorf("span(%q) is %d-%d, want %d-%d", c.s, first, last, c.first, c.last)
		}
	}
}
