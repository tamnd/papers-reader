package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

// minimal is the smallest index.json that is a valid one: the required
// fields, with nothing in the lists. An empty corpus has to emit something
// the app can render, and a build of one is the first thing anybody does.
const minimal = `{
  "version": 1,
  "generated": "2026-09-14T12:00:00Z",
  "langs": ["en", "vi", "zh", "ja"],
  "draft_langs": [],
  "fields": [],
  "collections": [],
  "papers": []
}`

// with returns minimal with one paper in it, built from a map so a test can
// change one field and leave the rest alone.
func with(t *testing.T, paper map[string]any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(minimal), &doc); err != nil {
		t.Fatal(err)
	}
	full := map[string]any{
		"id": "cook-1971-np", "title": "The Complexity of Theorem-Proving Procedures",
		"authors": []any{"Stephen A. Cook"}, "year": 1971, "field": "theory",
		"access": "open", "status": map[string]any{"en": "full"},
		"sections": 4, "figures": 0, "equations": 2, "refs": 12,
		"cited_by": 3, "cites_in_corpus": 0,
	}
	for k, v := range paper {
		full[k] = v
	}
	doc["papers"] = []any{full}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func bad(t *testing.T, doc string, body []byte) []string {
	t.Helper()
	why, err := Validate(doc, body)
	if err != nil {
		t.Fatal(err)
	}
	return why
}

func TestTheSmallestValidCatalogueValidates(t *testing.T) {
	if why := bad(t, Index, []byte(minimal)); len(why) != 0 {
		t.Errorf("an empty catalogue does not validate: %v", why)
	}
}

func TestAWholePaperValidates(t *testing.T) {
	if why := bad(t, Index, with(t, nil)); len(why) != 0 {
		t.Errorf("a paper does not validate: %v", why)
	}
}

// The identifier shape is the one thing every other file keys off, so it is
// pinned rather than left as any string.
func TestAnIdentifierOfTheWrongShapeIsRefused(t *testing.T) {
	for _, id := range []string{"Cook-1971-np", "cook-71-np", "cook-1971", "cook 1971 np", ""} {
		if why := bad(t, Index, with(t, map[string]any{"id": id})); len(why) == 0 {
			t.Errorf("%q was accepted as an identifier", id)
		}
	}
}

// A field the schema does not know is a typo or a shape change nobody
// updated the schema for, and either way the app has no page for it.
func TestAFieldOutsideTheTwelveIsRefused(t *testing.T) {
	if why := bad(t, Index, with(t, map[string]any{"field": "quantum"})); len(why) == 0 {
		t.Error("a subject field the corpus does not have was accepted")
	}
}

// Additional properties are refused everywhere, which is what turns a
// forgotten schema update into a failing audit rather than into a field the
// app silently never reads.
func TestAFieldTheSchemaDoesNotKnowIsRefused(t *testing.T) {
	why := bad(t, Index, with(t, map[string]any{"citations": 4000}))
	if len(why) == 0 {
		t.Fatal("an unknown property was accepted")
	}
	if !strings.Contains(strings.Join(why, " "), "citations") {
		t.Errorf("the complaint does not name the property: %v", why)
	}
}

// A landing page is where the paper is published. A link straight to a PDF
// would be the corpus pointing at a file rather than at a publisher, and
// the one thing this project never does is hand out the PDF.
func TestALandingPageMustBeAnHTTPURL(t *testing.T) {
	if why := bad(t, Index, with(t, map[string]any{"landing": "example.org/paper"})); len(why) == 0 {
		t.Error("a landing page that is not a URL was accepted")
	}
	if why := bad(t, Index, with(t, map[string]any{"landing": "https://example.org/paper"})); len(why) != 0 {
		t.Errorf("a landing page was refused: %v", why)
	}
}

func TestAStatusOutsideTheThreeStatesIsRefused(t *testing.T) {
	why := bad(t, Index, with(t, map[string]any{"status": map[string]any{"en": "partial"}}))
	if len(why) == 0 {
		t.Error("a state the corpus does not have was accepted")
	}
	why = bad(t, Index, with(t, map[string]any{"status": map[string]any{"de": "full"}}))
	if len(why) == 0 {
		t.Error("a language the corpus does not have was accepted as a status key")
	}
}

// The timestamp is asserted and not merely annotated, because a format
// keyword that is only an annotation is a schema that documents a rule
// nobody enforces.
func TestAGeneratedStampThatIsNotATimeIsRefused(t *testing.T) {
	body := strings.Replace(minimal, "2026-09-14T12:00:00Z", "last Tuesday", 1)
	if why := bad(t, Index, []byte(body)); len(why) == 0 {
		t.Error("a timestamp that is not one was accepted")
	}
}

func TestAGraphValidates(t *testing.T) {
	const g = `{
  "version": 1,
  "nodes": [
    {"id": "cook-1971-np", "title": "The Complexity of Theorem-Proving Procedures",
     "year": 1971, "field": "theory", "in": 1, "out": 0, "parsed": false},
    {"id": "karp-1972-reducibility", "title": "Reducibility Among Combinatorial Problems",
     "year": 1972, "field": "theory", "in": 0, "out": 1, "parsed": true}
  ],
  "edges": [{"from": "karp-1972-reducibility", "to": "cook-1971-np"}]
}`
	if why := bad(t, Graph, []byte(g)); len(why) != 0 {
		t.Errorf("a graph does not validate: %v", why)
	}
}

// A node without parsed set would be a node the chart cannot draw honestly,
// because false and absent would look the same and mean different things.
func TestAGraphNodeMustSayWhetherItWasParsed(t *testing.T) {
	const g = `{"version": 1, "nodes": [
  {"id": "cook-1971-np", "title": "A Paper", "year": 1971, "field": "theory", "in": 0, "out": 0}
], "edges": []}`
	if why := bad(t, Graph, []byte(g)); len(why) == 0 {
		t.Error("a node with no parsed flag was accepted")
	}
}

// The catalogue and the graph are different shapes and the schema knows
// which is which. Validating one against the other is how a mixed-up
// argument at a call site is caught.
func TestTheCatalogueIsNotAGraph(t *testing.T) {
	if why := bad(t, Graph, []byte(minimal)); len(why) == 0 {
		t.Error("the catalogue validated as a graph")
	}
}

func TestAnUnknownDocumentIsAnError(t *testing.T) {
	if _, err := Validate("search-vi.json", []byte(`{}`)); err == nil {
		t.Error("a document the schema does not cover was validated anyway")
	}
}

// Every complaint says where in the document it is, so an audit finding
// points somebody at a line rather than at a file.
func TestEveryComplaintNamesWhereItIs(t *testing.T) {
	why := bad(t, Index, with(t, map[string]any{"year": 1600, "field": "quantum"}))
	if len(why) < 2 {
		t.Fatalf("two things are wrong and the complaints are %v", why)
	}
	for _, line := range why {
		if !strings.HasPrefix(line, "/papers/0/") {
			t.Errorf("a complaint does not say where it is: %q", line)
		}
	}
}
