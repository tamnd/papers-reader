package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/report"
)

// Two papers, one citing the other, so an edge has somewhere to go and the
// catalogue has a field with more than one thing in it.
const papersYAML = `papers:
  - id: rumelhart-1986-backprop
    title: Learning Representations by Back-Propagating Errors
    authors: [David E. Rumelhart, Geoffrey E. Hinton]
    year: 1986
    venue: Nature
    field: ai-ml
    number: 42
    difficulty: 3
    core_idea: backpropagation
    status: listed
  - id: hochreiter-1997-lstm
    title: Long Short-Term Memory
    authors: [Sepp Hochreiter, Jürgen Schmidhuber]
    year: 1997
    venue: Neural Computation
    field: ai-ml
    prerequisites: [rumelhart-1986-backprop]
    status: listed
`

const sourcesYAML = `sources:
  - id: rumelhart-1986-backprop
    access: open
    licence: CC-BY-4.0
    url: https://example.org/backprop.pdf
    landing: https://example.org/backprop
    text_layer: native
  - id: hochreiter-1997-lstm
    access: restricted
    url: https://example.org/lstm.pdf
    landing: https://example.org/lstm
    text_layer: native
`

const collectionsYAML = `collections:
  - id: the-path
    title: The path to large language models
    description: How the field got here.
    order: manual
    members: [rumelhart-1986-backprop, hochreiter-1997-lstm]
`

// A glossary covering every term in one language and none in another, so
// that one language is a draft and one is not.
const glossaryYAML = `version: 3
terms:
  - en: gradient
    vi: độ dốc
    zh: 梯度
  - en: layer
    vi: lớp
    zh: 层
`

// front is one 00_front.md, with the fields the emitter counts off.
func front(paper, title string, l corpus.Lang, figures, equations int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\npaper: %s\ntitle: %s\nkind: front\nlang: %s\n", paper, title, l)
	if figures > 0 {
		b.WriteString("figures:\n")
		for i := 1; i <= figures; i++ {
			fmt.Fprintf(&b, "  - figures/%s/f%02d.png\n", paper, i)
		}
	}
	if equations > 0 {
		fmt.Fprintf(&b, "equations: %d\n", equations)
	}
	fmt.Fprintf(&b, "---\n\n# %s\n\nAn abstract.\n", title)
	return b.String()
}

// section is one body file, which is what makes a paper full rather than a
// stub.
func section(paper string, l corpus.Lang) string {
	return fmt.Sprintf("---\npaper: %s\ntitle: A section\nkind: section\nlang: %s\n"+
		"section: \"1\"\nsection_title: A section\n---\n\nSome prose.\n", paper, l)
}

func corpusOf(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{
		"manifests/papers.yaml":      papersYAML,
		"manifests/sources.yaml":     sourcesYAML,
		"manifests/collections.yaml": collectionsYAML,
		"manifests/glossary.yaml":    glossaryYAML,
	}
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

// whole is a corpus with both papers extracted, one of them translated, and
// a bibliography that draws the edge between them.
func whole(t *testing.T) *corpus.Corpus {
	t.Helper()
	return corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations by Back-Propagating Errors", corpus.EN, 2, 3),
		"content/en/rumelhart-1986-backprop/01_method.md": section("rumelhart-1986-backprop", corpus.EN),
		"content/vi/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Học biểu diễn bằng lan truyền ngược sai số", corpus.VI, 2, 3),
		"content/vi/rumelhart-1986-backprop/01_method.md": section("rumelhart-1986-backprop", corpus.VI),
		"content/en/hochreiter-1997-lstm/00_front.md": front(
			"hochreiter-1997-lstm", "Long Short-Term Memory", corpus.EN, 0, 0),
		"manifests/refs/hochreiter-1997-lstm.yaml": `paper: hochreiter-1997-lstm
style: numeric
entries:
  - key: "1"
    raw: Rumelhart, Hinton and Williams. Learning representations. Nature, 1986.
    title: Learning Representations by Back-Propagating Errors
    resolves_to: rumelhart-1986-backprop
  - key: "2"
    raw: Somebody else. A paper this corpus does not hold. 1990.
    title: A Paper This Corpus Does Not Hold
`,
	})
}

func index(t *testing.T, c *corpus.Corpus) *Index {
	t.Helper()
	ix, err := BuildIndex(c)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func paper(t *testing.T, ix *Index, id string) IndexPaper {
	t.Helper()
	for _, p := range ix.Papers {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("there is no %s in the index", id)
	return IndexPaper{}
}

func TestTheCatalogueCountsAPaperOffItsFiles(t *testing.T) {
	p := paper(t, index(t, whole(t)), "rumelhart-1986-backprop")
	if p.Sections != 2 {
		t.Errorf("%d sections, want 2", p.Sections)
	}
	if p.Figures != 2 || p.Equations != 3 {
		t.Errorf("%d figures and %d equations, want 2 and 3", p.Figures, p.Equations)
	}
	if p.Number != 42 || p.Venue != "Nature" || p.CoreIdea != "backpropagation" {
		t.Errorf("the manifest did not come through: %+v", p)
	}
	if p.Landing != "https://example.org/backprop" {
		t.Errorf("the landing page is %q", p.Landing)
	}
}

// The status of a paper in a language is read off what is on disk and not
// off the manifest, because the manifest says what somebody intends and the
// disk says what a reader can actually be shown.
func TestStatusIsPerLanguageAndAbsentWhereThereIsNothing(t *testing.T) {
	ix := index(t, whole(t))
	back := paper(t, ix, "rumelhart-1986-backprop")
	if back.Status[corpus.EN] != report.Full || back.Status[corpus.VI] != report.Full {
		t.Errorf("the status is %v", back.Status)
	}
	if _, ok := back.Status[corpus.JA]; ok {
		t.Error("a language the paper has nothing in is in the status map")
	}
	// One file and it is the front matter, which is a stub. That is what a
	// restricted paper is allowed and it is not a shortfall.
	if got := paper(t, ix, "hochreiter-1997-lstm").Status[corpus.EN]; got != report.Stub {
		t.Errorf("the restricted paper is %q, want stub", got)
	}
}

// The title a translator wrote is worth carrying, and the same title in
// English is not: a catalogue that printed the English twice under two
// language headings would be saying something untrue about the second one.
func TestATranslatedTitleIsCarriedAndAnUntranslatedOneIsNot(t *testing.T) {
	c := whole(t)
	p := paper(t, index(t, c), "rumelhart-1986-backprop")
	if p.Titles[corpus.VI] != "Học biểu diễn bằng lan truyền ngược sai số" {
		t.Errorf("the Vietnamese title is %q", p.Titles[corpus.VI])
	}

	same := front("rumelhart-1986-backprop", "Learning Representations by Back-Propagating Errors", corpus.VI, 0, 0)
	path := filepath.Join(c.Content(corpus.VI, "rumelhart-1986-backprop"), "00_front.md")
	if err := os.WriteFile(path, []byte(same), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := paper(t, index(t, c), "rumelhart-1986-backprop").Titles; len(got) != 0 {
		t.Errorf("a title left in English was carried as a translation: %v", got)
	}
}

func TestACollectionIsResolvedToIdentifiers(t *testing.T) {
	ix := index(t, whole(t))
	if len(ix.Collections) != 1 {
		t.Fatalf("the reading lists are %+v", ix.Collections)
	}
	got := ix.Collections[0]
	if got.ID != "the-path" || len(got.Members) != 2 || got.Members[0] != "rumelhart-1986-backprop" {
		t.Errorf("the reading list is %+v", got)
	}
}

// A language whose glossary is under the floor is still emitted, marked a
// draft. Hiding it would be the same corpus with less of it visible.
func TestALanguageUnderTheFloorIsADraft(t *testing.T) {
	ix := index(t, whole(t))
	draft := map[corpus.Lang]bool{}
	for _, l := range ix.DraftLangs {
		draft[l] = true
	}
	if !draft[corpus.JA] {
		t.Errorf("Japanese has no renderings at all and is not a draft: %v", ix.DraftLangs)
	}
	if draft[corpus.VI] || draft[corpus.ZH] {
		t.Errorf("a language the glossary covers is a draft: %v", ix.DraftLangs)
	}
	if draft[corpus.EN] {
		t.Error("English is a draft")
	}
}

// The degrees in the catalogue and the edges in the graph are counted from
// one place, so a catalogue that says a paper is cited and a graph with no
// edge into it is not a state this can reach.
func TestTheCatalogueAndTheGraphAgreeAboutTheEdges(t *testing.T) {
	c := whole(t)
	ix := index(t, c)
	g, err := BuildGraph(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 1 || g.Edges[0] != (GraphEdge{From: "hochreiter-1997-lstm", To: "rumelhart-1986-backprop"}) {
		t.Fatalf("the edges are %+v", g.Edges)
	}
	if got := paper(t, ix, "rumelhart-1986-backprop"); got.CitedBy != 1 || got.CitesInCorpus != 0 {
		t.Errorf("the cited paper is cited by %d and cites %d", got.CitedBy, got.CitesInCorpus)
	}
	if got := paper(t, ix, "hochreiter-1997-lstm"); got.CitedBy != 0 || got.CitesInCorpus != 1 {
		t.Errorf("the citing paper is cited by %d and cites %d", got.CitedBy, got.CitesInCorpus)
	}
	// Both entries of the bibliography count as references. Only one of
	// them is an edge, and the difference is the point of the two numbers.
	if got := paper(t, ix, "hochreiter-1997-lstm").Refs; got != 2 {
		t.Errorf("%d references, want 2", got)
	}
}

// A node whose bibliography nobody has read cites nothing here, which the
// chart has to be able to tell from a paper that was read and cites nothing.
func TestAGraphNodeSaysWhetherAnybodyReadItsBibliography(t *testing.T) {
	g, err := BuildGraph(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range g.Nodes {
		want := n.ID == "hochreiter-1997-lstm"
		if n.Parsed != want {
			t.Errorf("%s: parsed is %v, want %v", n.ID, n.Parsed, want)
		}
	}
}

func TestABuildWritesTheTwoDocuments(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := site.Write(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.json", "graph.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if v.Version != Version {
			t.Errorf("%s carries version %d, want %d", name, v.Version, Version)
		}
		if b[len(b)-1] != '\n' {
			t.Errorf("%s does not end in a newline", name)
		}
	}
}

// An empty corpus emits empty lists and not nulls. A reader that has to
// check for null before iterating is a reader that will forget once.
func TestAnEmptyCorpusEmitsListsAndNotNulls(t *testing.T) {
	site, err := Build(corpusOf(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	files, err := site.Files()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		var v map[string]any
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for key, got := range v {
			if got == nil {
				t.Errorf("%s: %s is null", name, key)
			}
		}
	}
}

// Nothing in an emit says where it was built. The corpus repository is
// public and a path or a host name in a committed file is a leak, and the
// same rule holds for anything a build could be published with.
func TestTheEmitNamesNoHostsAndNoLocalPaths(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	files, err := site.Files()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		for _, bad := range []string{"server1", "server2", "server3", "/Users/", "/home/", "/tmp/"} {
			if strings.Contains(string(body), bad) {
				t.Errorf("%s names %q", name, bad)
			}
		}
	}
}
