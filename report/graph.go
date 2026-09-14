package report

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/refs"
)

// A Graph is the citations that run between papers of this corpus.
//
// It is built from the resolves_to field of the parsed bibliographies and
// not from the [[id]] links in the bodies, although the two say the same
// thing. The links are written into the bodies from these manifests by
// refs.Rewrite, so reading them back would be reading this data through a
// copy of itself, and audit rule R01 already checks that the copy agrees.
// The manifests are also the only place that knows about an edge whose
// paper has no body yet, which is most of the corpus.
//
// Self loops are dropped, because a paper citing its own earlier version is
// rule R05's finding, and a paper cited twice by one bibliography counts
// once. Both match how rule R06 walks the same edges, so a cycle the audit
// reports and a cycle this draws are the same cycle.
type Graph struct {
	// Papers is every paper in papers.yaml, in id order, whether or not
	// anything cites it. A paper with no edges at either end is the useful
	// row here: it is the one the corpus has not connected to anything.
	Papers []GraphPaper
	// Edges is every citation between two papers of the corpus, sorted by
	// the citing paper and then by the cited one.
	Edges []Edge
	// Entries is how many bibliography entries were read to find them and
	// Resolved is how many named a paper the corpus holds. The gap is the
	// honest shape of the thing: a hundred papers cite thousands of others
	// and hold a few dozen of them between them.
	Entries  int
	Resolved int
	// Parsed is how many papers have a bibliography manifest, out of how
	// many there are. An edge can only be found in a paper that has one, so
	// this is the graph's own coverage and belongs beside it.
	Parsed int
	// Wanted is papers outside the corpus that several papers in it cite,
	// most cited first. It is the reading list: audit rule R07 reports the
	// same thing as a finding and this says how strong each case is.
	Wanted []Wanted
}

// An Edge is one paper citing another, both of them in the corpus.
type Edge struct{ From, To string }

// A GraphPaper is one paper and its two degrees.
type GraphPaper struct {
	ID    string
	Field corpus.Field
	Year  int
	// Cites is the papers of the corpus this one cites, in id order, and
	// CitedBy is the papers of the corpus that cite it.
	Cites   []string
	CitedBy []string
	// Parsed says whether this paper has a bibliography manifest. A paper
	// without one cites nothing here because nobody has read its
	// references yet, which is a different thing from citing nothing.
	Parsed bool
}

// Degree is how many edges touch a paper at either end.
func (p GraphPaper) Degree() int { return len(p.Cites) + len(p.CitedBy) }

// A Wanted is a paper outside the corpus that papers inside it cite.
type Wanted struct {
	// Key is the title the citing papers printed, lowercased, which is the
	// only handle there is on a paper that has no id here.
	Title string
	// By is the papers of the corpus that cite it, in id order.
	By []string
}

// wantedFloor is how many citing papers make an outside paper worth naming.
//
// Two. Rule R07 uses three and reports a paper that ought to be in the
// corpus; this is a report rather than a finding, so it can afford to show
// the near misses, and with a hundred papers the counts are small enough
// that the difference between two and three is a handful of rows.
const wantedFloor = 2

// BuildGraph reads every bibliography the corpus has parsed and draws the
// edges between papers it holds.
func BuildGraph(c *corpus.Corpus) (*Graph, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}

	g := &Graph{}
	rows := map[string]*GraphPaper{}
	known := map[string]bool{}
	for _, p := range papers.Papers {
		rows[p.ID] = &GraphPaper{ID: p.ID, Field: p.Field, Year: p.Year}
		known[p.ID] = true
	}

	outside := map[string][]string{}
	for _, p := range papers.Papers {
		m, err := refs.Load(c.Refs(p.ID))
		if os.IsNotExist(err) {
			// Not an error. The milestone that parses bibliographies is
			// still working through the corpus, and a graph that refused to
			// draw until every one was done would never draw at all.
			continue
		}
		if err != nil {
			return nil, err
		}
		rows[p.ID].Parsed = true
		g.Parsed++
		g.Entries += len(m.Entries)

		seen := map[string]bool{}
		for _, e := range m.Entries {
			if e.ResolvesTo == "" {
				if t := strings.ToLower(strings.TrimSpace(e.Title)); t != "" {
					outside[t] = append(outside[t], p.ID)
				}
				continue
			}
			g.Resolved++
			// A self loop is rule R05's finding and a repeat is one edge.
			if e.ResolvesTo == p.ID || seen[e.ResolvesTo] || !known[e.ResolvesTo] {
				continue
			}
			seen[e.ResolvesTo] = true
			g.Edges = append(g.Edges, Edge{From: p.ID, To: e.ResolvesTo})
			rows[p.ID].Cites = append(rows[p.ID].Cites, e.ResolvesTo)
			rows[e.ResolvesTo].CitedBy = append(rows[e.ResolvesTo].CitedBy, p.ID)
		}
	}

	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	for _, p := range papers.Papers {
		row := rows[p.ID]
		sort.Strings(row.Cites)
		sort.Strings(row.CitedBy)
		g.Papers = append(g.Papers, *row)
	}
	sort.Slice(g.Papers, func(i, j int) bool { return g.Papers[i].ID < g.Papers[j].ID })

	for title, by := range outside {
		by = unique(by)
		if len(by) < wantedFloor {
			continue
		}
		g.Wanted = append(g.Wanted, Wanted{Title: title, By: by})
	}
	sort.Slice(g.Wanted, func(i, j int) bool {
		if len(g.Wanted[i].By) != len(g.Wanted[j].By) {
			return len(g.Wanted[i].By) > len(g.Wanted[j].By)
		}
		return g.Wanted[i].Title < g.Wanted[j].Title
	})
	return g, nil
}

// unique sorts a list of ids and drops the repeats. One paper citing
// another twice is one paper.
func unique(ids []string) []string {
	sort.Strings(ids)
	out := ids[:0:0]
	for i, id := range ids {
		if i == 0 || id != ids[i-1] {
			out = append(out, id)
		}
	}
	return out
}

// Summary is the one line a run prints.
func (g *Graph) Summary() string {
	return fmt.Sprintf("%d papers, %d with a bibliography read, %d edges between them out of %d references",
		len(g.Papers), g.Parsed, len(g.Edges), g.Entries)
}

// Markdown is reports/graph.md.
func (g *Graph) Markdown() string {
	var b strings.Builder
	b.WriteString("# The citation graph\n\nWhich papers of this corpus cite which others of it.\n\n")
	b.WriteString("An edge is a bibliography entry of one paper here that was resolved to another paper here. Almost every reference in the corpus points somewhere else, which is what a hundred papers spread over eighty years looks like, so the edge count is small next to the reference count and that is the right shape rather than a shortfall.\n\n")
	fmt.Fprintf(&b, "%s.\n\n", g.Summary())

	if g.Parsed < len(g.Papers) {
		fmt.Fprintf(&b, "%d papers have no bibliography read yet. A paper with none cites nothing here, which is not the same as citing nothing.\n\n",
			len(g.Papers)-g.Parsed)
	}

	b.WriteString("## Edges\n\n")
	if len(g.Edges) == 0 {
		b.WriteString("No paper here cites another one yet.\n")
	} else {
		b.WriteString("| cites | cited |\n| --- | --- |\n")
		for _, e := range g.Edges {
			fmt.Fprintf(&b, "| %s | %s |\n", e.From, e.To)
		}
	}

	b.WriteString("\n## Most cited\n\nThe papers of the corpus that other papers of the corpus cite, most cited first.\n\n")
	cited := make([]GraphPaper, 0, len(g.Papers))
	for _, p := range g.Papers {
		if len(p.CitedBy) > 0 {
			cited = append(cited, p)
		}
	}
	sort.SliceStable(cited, func(i, j int) bool { return len(cited[i].CitedBy) > len(cited[j].CitedBy) })
	if len(cited) == 0 {
		b.WriteString("None yet.\n")
	} else {
		b.WriteString("| paper | field | year | cited by |\n| --- | --- | --: | --- |\n")
		for _, p := range cited {
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", p.ID, p.Field, p.Year, strings.Join(p.CitedBy, ", "))
		}
	}

	if len(g.Wanted) > 0 {
		fmt.Fprintf(&b, "\n## Cited from outside\n\nPapers this corpus does not hold that %d or more papers in it cite. This is the reading list, and audit rule R07 reports the strongest of these as a finding.\n\n", wantedFloor)
		b.WriteString("| title as printed | cited by |\n| --- | --- |\n")
		for _, w := range g.Wanted {
			fmt.Fprintf(&b, "| %s | %s |\n", w.Title, strings.Join(w.By, ", "))
		}
	}

	var alone []string
	for _, p := range g.Papers {
		if p.Parsed && p.Degree() == 0 {
			alone = append(alone, p.ID)
		}
	}
	if len(alone) > 0 {
		fmt.Fprintf(&b, "\n## Connected to nothing\n\nThe %d papers whose bibliography has been read and that neither cite nor are cited by anything else here. A paper stays in this list until the corpus grows around it.\n\n", len(alone))
		for _, id := range alone {
			fmt.Fprintf(&b, "- %s\n", id)
		}
	}
	return b.String()
}
