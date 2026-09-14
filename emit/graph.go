package emit

import (
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/report"
)

// A Graph is site/graph.json: the citation graph as the app draws it.
//
// The same edges as reports/graph.md and built from the same place, because
// a picture of the corpus that disagreed with the report of it would be one
// of them lying. What differs is the shape: the report is a table meant to
// be read and this is a node list and an edge list meant to be laid out, so
// each node carries the year and the field the layout puts it at and the two
// degrees a hover needs.
type Graph struct {
	Version int         `json:"version"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

// A GraphNode is one paper at a point on the chart.
type GraphNode struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Year  int          `json:"year"`
	Field corpus.Field `json:"field"`
	// In is how many papers here cite it and Out is how many it cites. The
	// names are the graph's and not the catalogue's: this file is read by
	// the layout code, where in-degree and out-degree are the words.
	In  int `json:"in"`
	Out int `json:"out"`
	// Parsed says somebody has read this paper's bibliography. A node with
	// Out zero and Parsed false cites nothing here because nobody has
	// looked, which the chart draws differently from a paper that was read
	// and cites nothing.
	Parsed bool `json:"parsed"`
}

// A GraphEdge is one paper citing another.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// BuildGraph reads the corpus and builds the graph the app draws.
func BuildGraph(c *corpus.Corpus) (*Graph, error) {
	g, err := report.BuildGraph(c)
	if err != nil {
		return nil, err
	}
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	for _, p := range papers.Papers {
		titles[p.ID] = p.Title
	}

	out := &Graph{Version: Version, Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	for _, p := range g.Papers {
		out.Nodes = append(out.Nodes, GraphNode{
			ID: p.ID, Title: titles[p.ID], Year: p.Year, Field: p.Field,
			In: len(p.CitedBy), Out: len(p.Cites), Parsed: p.Parsed,
		})
	}
	for _, e := range g.Edges {
		out.Edges = append(out.Edges, GraphEdge{From: e.From, To: e.To})
	}
	return out, nil
}
