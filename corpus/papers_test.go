package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testCorpus writes a small corpus to a temporary directory and opens it. The
// papers in it are real papers, because a fixture that uses made up ids hides
// every mistake that only shows up on the shapes ids really take.
func testCorpus(t *testing.T, papers, collections, sources string) *Corpus {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if body == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(root, "manifests", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("papers.yaml", papers)
	write("collections.yaml", collections)
	write("sources.yaml", sources)
	c, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const threePapers = `papers:
  - id: turing-1936-computable
    title: On Computable Numbers, with an Application to the Entscheidungsproblem
    authors: [Alan M. Turing]
    year: 1936
    venue: Proc. London Math. Soc.
    field: theory
    number: 1
    expect: public-domain
    difficulty: 4
    status: listed
  - id: sutskever-2014-seq2seq
    title: Sequence to Sequence Learning with Neural Networks
    authors: [Ilya Sutskever, Oriol Vinyals, Quoc V. Le]
    year: 2014
    venue: NIPS
    field: ai-ml
    number: 89
    arxiv: 1409.3215
    expect: open
    difficulty: 3
    status: listed
  - id: vaswani-2017-attention
    title: Attention Is All You Need
    authors: [Ashish Vaswani, Noam Shazeer]
    year: 2017
    venue: NIPS
    field: ai-ml
    number: 90
    prerequisites: [sutskever-2014-seq2seq]
    arxiv: 1706.03762
    expect: open
    difficulty: 3
    status: listed
`

func TestLoadPapers(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Papers) != 3 {
		t.Fatalf("loaded %d papers, want 3", len(m.Papers))
	}
	p, ok := m.ByID("vaswani-2017-attention")
	if !ok {
		t.Fatal("vaswani-2017-attention is missing")
	}
	if p.Year != 2017 || p.Field != AIML || p.Number != 90 || p.ArXiv != "1706.03762" {
		t.Errorf("got %+v", *p)
	}
	if _, ok := m.ByID("nobody-1999-nothing"); ok {
		t.Error("ByID invented a paper")
	}
}

func TestPapersFieldAndCounts(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(m.Field(AIML)); got != 2 {
		t.Errorf("ai-ml has %d papers, want 2", got)
	}
	if got := len(m.Field(Graphics)); got != 0 {
		t.Errorf("graphics has %d papers, want none", got)
	}
	counts := m.Counts()
	if counts[Theory] != 1 || counts[AIML] != 2 {
		t.Errorf("counts are %v", counts)
	}
}

// Numbered sorts by the seed list position, which is not the order the
// manifest happens to be in and not the order ids sort in.
func TestPapersNumbered(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, p := range m.Numbered() {
		got = append(got, p.ID)
	}
	want := "turing-1936-computable sutskever-2014-seq2seq vaswani-2017-attention"
	if strings.Join(got, " ") != want {
		t.Errorf("Numbered is %v", got)
	}
}

// A prerequisite comes before the paper that needs it, whatever order the
// caller asked in.
func TestReadingOrder(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadingOrder([]string{"vaswani-2017-attention", "sutskever-2014-seq2seq"})
	if err != nil {
		t.Fatal(err)
	}
	want := "sutskever-2014-seq2seq vaswani-2017-attention"
	if strings.Join(got, " ") != want {
		t.Errorf("ReadingOrder is %v, want %s", got, want)
	}
}

// Ordering a list does not add to it. Asking for one paper gets one paper
// back, even though it has a prerequisite, because a reading list is what
// somebody chose to read.
func TestReadingOrderAddsNothing(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadingOrder([]string{"vaswani-2017-attention"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "vaswani-2017-attention" {
		t.Errorf("ReadingOrder is %v, want the one paper it was given", got)
	}
}

func TestReadingOrderReportsACycle(t *testing.T) {
	cyclic := `papers:
  - id: alpha-2001-one
    title: One
    authors: [A Alpha]
    year: 2001
    field: theory
    prerequisites: [beta-2002-two]
    status: listed
  - id: beta-2002-two
    title: Two
    authors: [B Beta]
    year: 2002
    field: theory
    prerequisites: [alpha-2001-one]
    status: listed
`
	c := testCorpus(t, cyclic, "", "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.ReadingOrder([]string{"alpha-2001-one", "beta-2002-two"})
	if err == nil {
		t.Fatal("a prerequisite cycle was accepted")
	}
	if !strings.Contains(err.Error(), "alpha-2001-one") {
		t.Errorf("the error does not name the papers in the cycle: %v", err)
	}
}

func TestOpenRejectsSomethingElse(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("an empty directory was opened as a corpus")
	}
}

func TestOpenPrefersItsArgument(t *testing.T) {
	t.Setenv(EnvRoot, filepath.Join(t.TempDir(), "not-here"))
	c := testCorpus(t, threePapers, "", "")
	if _, err := c.LoadPapers(); err != nil {
		t.Fatalf("the environment beat the argument: %v", err)
	}
}

func TestFindRootWalksUp(t *testing.T) {
	c := testCorpus(t, threePapers, "", "")
	deep := filepath.Join(c.Root, "content", "en", "vaswani-2017-attention")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(deep)
	if err != nil {
		t.Fatal(err)
	}
	if got != c.Root {
		t.Errorf("FindRoot is %q, want %q", got, c.Root)
	}
}
