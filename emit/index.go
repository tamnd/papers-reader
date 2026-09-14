package emit

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/refs"
	"github.com/tamnd/papers-reader/report"
)

// Version is the shape of the emitted JSON, and it is the contract between
// this package and the reading app.
//
// It goes up by one whenever a field is added, removed or changed meaning,
// and the schema and the app move in the same commit. An app that finds a
// version it does not know refuses to render rather than guessing, because a
// reader shown a page built from a shape the app half understands has no way
// to tell that anything is wrong.
const Version = 1

// An Index is site/index.json: everything a catalogue page needs about every
// paper, and nothing a paper page would need.
//
// It is one file and not one per field because it is read on the first
// request of every visit and it is small. A hundred papers of this shape is
// around fifty kilobytes and a tenth of that gzipped, which is cheaper to
// fetch once than a field at a time is to fetch twice.
type Index struct {
	Version   int    `json:"version"`
	Generated string `json:"generated"`
	// Langs is every language the corpus can be read in and DraftLangs is
	// the subset whose glossary is under the floor. A draft language is
	// still offered, with the page saying what it is. Hiding it would be
	// the same corpus with less of it visible and no more of it true.
	Langs       []corpus.Lang `json:"langs"`
	DraftLangs  []corpus.Lang `json:"draft_langs"`
	Fields      []IndexField  `json:"fields"`
	Collections []IndexList   `json:"collections"`
	Papers      []IndexPaper  `json:"papers"`
}

// An IndexField is one subject field and how many papers are in it.
type IndexField struct {
	ID    corpus.Field `json:"id"`
	Title string       `json:"title"`
	Count int          `json:"count"`
}

// An IndexList is one reading list with its members resolved to ids, in the
// order the list says to read them.
//
// Resolved here rather than in the app because the two words a collection
// may hold instead of a list, "all" and "all-with-number", and the
// topological order that sorts by prerequisites, are corpus rules. An app
// that reimplemented them would be a second place for them to be wrong.
type IndexList struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Members     []string `json:"members"`
}

// An IndexPaper is one paper as the catalogue knows it.
type IndexPaper struct {
	ID string `json:"id"`
	// Number is the position in the seed list of a hundred, and is absent
	// on every paper added afterwards.
	Number int          `json:"number,omitempty"`
	Title  string       `json:"title"`
	Titles Titles       `json:"titles,omitempty"`
	Author []string     `json:"authors"`
	Year   int          `json:"year"`
	Venue  string       `json:"venue,omitempty"`
	Field  corpus.Field `json:"field"`
	// CoreIdea is the one thing the paper is remembered for, which is the
	// line a catalogue card is read for.
	CoreIdea   string        `json:"core_idea,omitempty"`
	Difficulty int           `json:"difficulty,omitempty"`
	Access     corpus.Access `json:"access"`
	// Landing is the publisher's page for the paper and not a PDF. It is
	// where a reader goes for the paper itself, and it is the only outward
	// link the site makes on a paper's behalf.
	Landing string `json:"landing,omitempty"`
	// Status is how much of the paper exists in each language: full, stub
	// or none. A language the paper has nothing in is absent from the map
	// rather than present and "none", so the app can offer what is there
	// without filtering.
	Status map[corpus.Lang]report.State `json:"status"`
	// Sections, Figures, Equations and Refs are counted off the English,
	// because the audit holds every translation to the same structure and
	// counting them per language would be four copies of one number.
	Sections  int `json:"sections"`
	Figures   int `json:"figures"`
	Equations int `json:"equations"`
	Refs      int `json:"refs"`
	// CitedBy and CitesInCorpus are this paper's two degrees in the
	// citation graph, carried here so the catalogue can sort by influence
	// without loading the graph.
	CitedBy       int      `json:"cited_by"`
	CitesInCorpus int      `json:"cites_in_corpus"`
	Prerequisites []string `json:"prerequisites,omitempty"`
}

// Titles is the title as each translation printed it.
//
// Keyed by language rather than one field per language, because the corpus
// gains languages and a shape with vi, zh and ja written into it gains a
// field and a version every time it does. The English is in Title above and
// is not repeated here.
type Titles map[corpus.Lang]string

// BuildIndex reads the corpus and builds the catalogue.
func BuildIndex(c *corpus.Corpus) (*Index, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	sources, err := c.LoadSources()
	if err != nil {
		return nil, err
	}
	collections, err := c.LoadCollections()
	if err != nil {
		return nil, err
	}
	g, err := glossary.Load(c.GlossaryManifest())
	if err != nil {
		return nil, err
	}
	graph, err := report.BuildGraph(c)
	if err != nil {
		return nil, err
	}
	degree := map[string]report.GraphPaper{}
	for _, p := range graph.Papers {
		degree[p.ID] = p
	}

	ix := &Index{
		Version: Version,
		// Second precision and UTC. A site built twice from one commit
		// should differ in this field and in nothing else, and a timestamp
		// carrying a machine's offset would say where it was built.
		Generated:   time.Now().UTC().Format(time.RFC3339),
		Langs:       append([]corpus.Lang(nil), corpus.Langs...),
		DraftLangs:  []corpus.Lang{},
		Fields:      []IndexField{},
		Collections: []IndexList{},
		Papers:      []IndexPaper{},
	}
	for _, l := range corpus.Langs {
		if l.Translated() && g.Under(l) {
			ix.DraftLangs = append(ix.DraftLangs, l)
		}
	}

	counts := map[corpus.Field]int{}
	for _, p := range papers.Papers {
		row, err := indexPaper(c, p, sources, degree[p.ID])
		if err != nil {
			return nil, err
		}
		ix.Papers = append(ix.Papers, row)
		counts[p.Field]++
	}
	sort.Slice(ix.Papers, func(i, j int) bool { return ix.Papers[i].ID < ix.Papers[j].ID })

	for _, f := range corpus.Fields {
		if counts[f] == 0 {
			continue
		}
		ix.Fields = append(ix.Fields, IndexField{ID: f, Title: f.Title(), Count: counts[f]})
	}
	for _, col := range collections.Collections {
		members, err := col.Resolve(papers)
		if err != nil {
			return nil, err
		}
		ix.Collections = append(ix.Collections, IndexList{
			ID: col.ID, Title: col.Title, Description: col.Description, Members: members,
		})
	}
	return ix, nil
}

// indexPaper counts one paper off the files it has.
func indexPaper(c *corpus.Corpus, p corpus.Paper, sources *corpus.Sources, node report.GraphPaper) (IndexPaper, error) {
	row := IndexPaper{
		ID: p.ID, Number: p.Number, Title: p.Title, Author: p.Authors,
		Year: p.Year, Venue: p.Venue, Field: p.Field, CoreIdea: p.CoreIdea,
		Difficulty: p.Difficulty, Access: corpus.AccessUnknown,
		Prerequisites: p.Prerequisites,
		Status:        map[corpus.Lang]report.State{},
		CitedBy:       len(node.CitedBy), CitesInCorpus: len(node.Cites),
	}
	if rec, ok := sources.ByID(p.ID); ok {
		if rec.Access != "" {
			row.Access = rec.Access
		}
		row.Landing = rec.Landing
	}
	if row.Author == nil {
		row.Author = []string{}
	}

	for _, l := range corpus.Langs {
		found, err := report.Files(c, l, p.ID)
		if err != nil {
			return row, err
		}
		if len(found) == 0 {
			continue
		}
		row.Status[l] = report.StateOf(found)
		if l == corpus.EN {
			row.Sections = len(found)
		}
		title, err := printedTitle(found)
		if err != nil {
			return row, err
		}
		if l.Translated() && title != "" && title != p.Title {
			if row.Titles == nil {
				row.Titles = Titles{}
			}
			row.Titles[l] = title
		}
		if l != corpus.EN {
			continue
		}
		figures, equations, err := countItems(found)
		if err != nil {
			return row, err
		}
		row.Figures, row.Equations = figures, equations
	}

	m, err := refs.Load(c.Refs(p.ID))
	if err != nil && !os.IsNotExist(err) {
		return row, err
	}
	if m != nil {
		row.Refs = len(m.Entries)
	}
	return row, nil
}

// printedTitle is the title the front matter of a paper carries in one
// language, which for a translation is the title the translator wrote.
func printedTitle(found []string) (string, error) {
	for _, path := range found {
		if !strings.HasPrefix(filepath.Base(path), "00_") {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		front, _, err := corpus.ParseFront(b)
		if err != nil {
			return "", err
		}
		return front.Title, nil
	}
	return "", nil
}

// countItems adds up the figures and displayed equations of a paper from
// what the front matter of each of its files records.
//
// A figure is counted once however many files name it, because a figure
// referred to from two sections is one picture.
func countItems(found []string) (figures, equations int, err error) {
	seen := map[string]bool{}
	for _, path := range found {
		b, err := os.ReadFile(path)
		if err != nil {
			return 0, 0, err
		}
		front, _, err := corpus.ParseFront(b)
		if err != nil {
			return 0, 0, err
		}
		for _, f := range front.Figures {
			if !seen[f] {
				seen[f] = true
				figures++
			}
		}
		equations += front.Equations
	}
	return figures, equations, nil
}
