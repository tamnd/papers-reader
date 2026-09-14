package report

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Three papers, so that an edge has somewhere to go and a third paper can
// sit outside the edges and be reported as connected to nothing.
const graphPapers = `papers:
  - id: rumelhart-1986-backprop
    title: Learning Representations by Back-Propagating Errors
    authors: [David E. Rumelhart]
    year: 1986
    field: ai-ml
    status: listed
  - id: hochreiter-1997-lstm
    title: Long Short-Term Memory
    authors: [Sepp Hochreiter]
    year: 1997
    field: ai-ml
    status: listed
  - id: codd-1970-relational
    title: A Relational Model of Data for Large Shared Data Banks
    authors: [E. F. Codd]
    year: 1970
    field: databases
    status: listed
`

// bibliography is one manifests/refs/<id>.yaml. Entries are given as pairs
// of a title and the id it resolved to, with an empty id for a reference to
// a paper the corpus does not hold.
func bibliography(paper string, entries ...[2]string) string {
	var b strings.Builder
	b.WriteString("paper: " + paper + "\nstyle: numeric\nentries:\n")
	for i, e := range entries {
		b.WriteString(fmt.Sprintf("  - key: %q\n", strconv.Itoa(i+1)))
		b.WriteString("    raw: " + e[0] + ".\n")
		b.WriteString("    title: " + e[0] + "\n")
		b.WriteString("    resolves_to: " + e[1] + "\n")
	}
	return b.String()
}

func graph(t *testing.T, files map[string]string) *Graph {
	t.Helper()
	all := map[string]string{"manifests/papers.yaml": graphPapers}
	for k, v := range files {
		all[k] = v
	}
	g, err := BuildGraph(corpusOf(t, all))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func node(t *testing.T, g *Graph, id string) GraphPaper {
	t.Helper()
	for _, p := range g.Papers {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("there is no %s in the graph", id)
	return GraphPaper{}
}

func TestAResolvedReferenceIsAnEdgeAtBothEnds(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": bibliography("hochreiter-1997-lstm",
			[2]string{"Learning Representations by Back-Propagating Errors", "rumelhart-1986-backprop"},
			[2]string{"A Theory Of Something Else", ""}),
	})
	if len(g.Edges) != 1 || g.Edges[0] != (Edge{From: "hochreiter-1997-lstm", To: "rumelhart-1986-backprop"}) {
		t.Fatalf("the edges are %+v", g.Edges)
	}
	if got := node(t, g, "hochreiter-1997-lstm").Cites; len(got) != 1 || got[0] != "rumelhart-1986-backprop" {
		t.Errorf("the citing paper cites %v", got)
	}
	if got := node(t, g, "rumelhart-1986-backprop").CitedBy; len(got) != 1 || got[0] != "hochreiter-1997-lstm" {
		t.Errorf("the cited paper is cited by %v", got)
	}
	if g.Entries != 2 || g.Resolved != 1 {
		t.Errorf("%d entries and %d resolved, want 2 and 1", g.Entries, g.Resolved)
	}
}

// One bibliography naming the same paper twice is one edge. A long paper
// cites its predecessor in the introduction and again in the related work,
// and the graph is about whether the edge is there and not about how often
// it was written.
func TestAPaperCitedTwiceByOneBibliographyIsOneEdge(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": bibliography("hochreiter-1997-lstm",
			[2]string{"Learning Representations", "rumelhart-1986-backprop"},
			[2]string{"Learning Representations, reprinted", "rumelhart-1986-backprop"}),
	})
	if len(g.Edges) != 1 {
		t.Errorf("the edges are %+v", g.Edges)
	}
	// Both entries still count as resolved, because that number is about
	// the bibliography and not about the graph.
	if g.Resolved != 2 {
		t.Errorf("%d entries resolved, want 2", g.Resolved)
	}
}

// A paper resolving to itself is rule R05's finding. Drawing it as an edge
// would put a self loop in the graph and make the paper look connected.
func TestASelfCitationIsNotAnEdge(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": bibliography("hochreiter-1997-lstm",
			[2]string{"Long Short-Term Memory", "hochreiter-1997-lstm"}),
	})
	if len(g.Edges) != 0 {
		t.Errorf("a self citation was drawn: %+v", g.Edges)
	}
	if node(t, g, "hochreiter-1997-lstm").Degree() != 0 {
		t.Error("a self citation gave the paper a degree")
	}
}

// A paper whose bibliography nobody has read yet cites nothing here, and
// that is not the same as citing nothing. The report says which it is.
func TestAPaperWithNoBibliographyIsNotCountedAsConnectedToNothing(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": bibliography("hochreiter-1997-lstm",
			[2]string{"Learning Representations", "rumelhart-1986-backprop"}),
	})
	if node(t, g, "codd-1970-relational").Parsed {
		t.Error("a paper with no manifest is marked parsed")
	}
	if g.Parsed != 1 {
		t.Errorf("%d bibliographies read, want 1", g.Parsed)
	}
	md := g.Markdown()
	if strings.Contains(md, "codd-1970-relational") {
		t.Errorf("a paper nobody has read the references of is listed as connected to nothing:\n%s", md)
	}
	if !strings.Contains(md, "2 papers have no bibliography read yet") {
		t.Errorf("the report does not say how much of the corpus it has read:\n%s", md)
	}
}

func TestAPaperWithABibliographyAndNoEdgesIsConnectedToNothing(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/codd-1970-relational.yaml": bibliography("codd-1970-relational",
			[2]string{"A Paper This Corpus Does Not Hold", ""}),
	})
	md := g.Markdown()
	if !strings.Contains(md, "## Connected to nothing") || !strings.Contains(md, "- codd-1970-relational") {
		t.Errorf("the paper is not reported as connected to nothing:\n%s", md)
	}
}

// The reading list. A paper outside the corpus that one paper cites is an
// ordinary reference; one that several cite is a gap.
func TestAPaperSeveralOfThemCiteAndTheCorpusDoesNotHoldIsOnTheReadingList(t *testing.T) {
	g := graph(t, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": bibliography("hochreiter-1997-lstm",
			[2]string{"Gradient Flow In Recurrent Nets", ""},
			[2]string{"A Paper Only This One Cites", ""}),
		"manifests/refs/codd-1970-relational.yaml": bibliography("codd-1970-relational",
			[2]string{"Gradient flow in recurrent nets", ""}),
	})
	if len(g.Wanted) != 1 {
		t.Fatalf("the reading list is %+v", g.Wanted)
	}
	// The two papers spelled the title with different capitals, which is
	// the ordinary case, so the match is made on the lowercase of it.
	if g.Wanted[0].Title != "gradient flow in recurrent nets" {
		t.Errorf("the title is %q", g.Wanted[0].Title)
	}
	if got := strings.Join(g.Wanted[0].By, ", "); got != "codd-1970-relational, hochreiter-1997-lstm" {
		t.Errorf("cited by %s", got)
	}
}

// An empty corpus is a report that says so rather than an error or a page
// of empty tables with no words round them.
func TestAGraphWithNoEdgesSaysSo(t *testing.T) {
	g := graph(t, nil)
	md := g.Markdown()
	if !strings.Contains(md, "No paper here cites another one yet.") {
		t.Errorf("the empty report reads badly:\n%s", md)
	}
	if !strings.Contains(g.Summary(), "3 papers, 0 with a bibliography read, 0 edges") {
		t.Errorf("the summary is %q", g.Summary())
	}
}
