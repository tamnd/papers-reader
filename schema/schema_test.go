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
	if _, err := Validate("tags.json", []byte(`{}`)); err == nil {
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

// smallestPage is the least a page can be and still be one: a paper with
// front matter, no sections and no bibliography, which is what a restricted
// paper emits.
const smallestPage = `{
  "version": 1,
  "id": "cook-1971-np",
  "lang": "en",
  "provenance": {"small_model": false, "gateway": false},
  "front": {"title": "The Complexity of Theorem-Proving Procedures", "authors": ["Stephen A. Cook"], "blocks": []},
  "sections": [],
  "refs": []
}`

// page returns smallestPage with one block in one section, built from a map
// so a test can change one field and leave the rest alone.
func page(t *testing.T, block map[string]any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(smallestPage), &doc); err != nil {
		t.Fatal(err)
	}
	full := map[string]any{"kind": "p", "i": 0, "html": "<p>Some prose.</p>"}
	for k, v := range block {
		full[k] = v
	}
	doc["sections"] = []any{map[string]any{
		"anchor": "cook-1971-np-s1", "title": "A section", "level": 2,
		"kind": "section", "blocks": []any{full},
	}}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A page is validated by the path it is written to, because a caller
// walking a build hands over every file it finds without working out what
// each one is.
func TestAPageIsRecognisedByItsPath(t *testing.T) {
	for _, path := range []string{"p/cook-1971-np/en.json", "p/rumelhart-1986-backprop/vi.json"} {
		kind, ok := Kind(path)
		if !ok || kind != Page {
			t.Errorf("%s is %q %v", path, kind, ok)
		}
	}
	for _, path := range []string{"p/Cook_1971/en.json", "p/cook-1971-np/en.md", "p/cook-1971-np.json", "index.html"} {
		if _, ok := Kind(path); ok {
			t.Errorf("%s was read as a document of the site", path)
		}
	}
}

func TestTheSmallestValidPageValidates(t *testing.T) {
	if why := bad(t, "p/cook-1971-np/en.json", []byte(smallestPage)); len(why) != 0 {
		t.Errorf("a page with nothing in it does not validate: %v", why)
	}
	if why := bad(t, "p/cook-1971-np/en.json", page(t, nil)); len(why) != 0 {
		t.Errorf("a page with a paragraph in it does not validate: %v", why)
	}
}

// The block index is the alignment key for the side by side view, so the
// field is required on every block and nothing else in the file matters as
// much.
func TestABlockMustBeNumberedAndMustSayWhatItIs(t *testing.T) {
	for _, block := range []string{`{"kind": "p", "html": "<p>x</p>"}`, `{"i": 0, "html": "<p>x</p>"}`} {
		var doc map[string]any
		if err := json.Unmarshal([]byte(smallestPage), &doc); err != nil {
			t.Fatal(err)
		}
		var b any
		if err := json.Unmarshal([]byte(block), &b); err != nil {
			t.Fatal(err)
		}
		doc["sections"] = []any{map[string]any{
			"anchor": "cook-1971-np-s1", "title": "A section", "level": 2,
			"kind": "section", "blocks": []any{b},
		}}
		body, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if why := bad(t, "p/cook-1971-np/en.json", body); len(why) == 0 {
			t.Errorf("the block %s was accepted", block)
		}
	}
}

// A kind the app does not know is a block the app would not draw, so the
// list is closed rather than left as any string.
func TestTheBlockKindsAreTheOnesTheAppDraws(t *testing.T) {
	for _, kind := range []string{"p", "heading", "list", "math", "figure", "code", "table"} {
		if why := bad(t, "p/cook-1971-np/en.json", page(t, map[string]any{"kind": kind})); len(why) != 0 {
			t.Errorf("a %s block does not validate: %v", kind, why)
		}
	}
	if why := bad(t, "p/cook-1971-np/en.json", page(t, map[string]any{"kind": "blockquote"})); len(why) == 0 {
		t.Error("a kind the app cannot draw was accepted")
	}
}

// A figure path is pinned because it is the one string in a page that the
// browser turns into a request. Anything that could climb out of the build
// is refused here rather than in the app.
func TestAFigurePathIsPinnedToTheBuild(t *testing.T) {
	ok := map[string]any{"kind": "figure", "src": "figures/cook-1971-np/fig-1.png", "w": 760, "h": 480}
	if why := bad(t, "p/cook-1971-np/en.json", page(t, ok)); len(why) != 0 {
		t.Errorf("a figure in the build does not validate: %v", why)
	}
	for _, src := range []string{"../../etc/passwd", "/figures/x/f.png", "https://example.org/f.png", "figures/x.png"} {
		block := map[string]any{"kind": "figure", "src": src}
		if why := bad(t, "p/cook-1971-np/en.json", page(t, block)); len(why) == 0 {
			t.Errorf("a figure at %q was accepted", src)
		}
	}
}

// Nothing the emitter does not write may appear in a page, because a field
// the schema does not know is a field the generated TypeScript does not
// have and the app would silently ignore.
func TestAPageTakesNoFieldTheEmitterDoesNotWrite(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(smallestPage), &doc); err != nil {
		t.Fatal(err)
	}
	doc["abstract"] = "Something the emitter stopped writing."
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if why := bad(t, "p/cook-1971-np/en.json", b); len(why) == 0 {
		t.Error("a page with a field nobody writes was accepted")
	}
}

// A page is not a catalogue and a catalogue is not a page, and the schema
// knows which is which, so a mixed-up argument at a call site is caught.
func TestAPageIsNotACatalogue(t *testing.T) {
	if why := bad(t, "p/cook-1971-np/en.json", []byte(minimal)); len(why) == 0 {
		t.Error("the catalogue validated as a page")
	}
	if why := bad(t, Index, []byte(smallestPage)); len(why) == 0 {
		t.Error("a page validated as the catalogue")
	}
}

// smallestSearch is the least a search index can be and still be one: a
// language with nothing indexed in it yet, which is what a corpus that has
// not been translated into that language emits.
const smallestSearch = `{
  "version": 1,
  "lang": "vi",
  "posts": [],
  "terms": {},
  "symbols": {}
}`

// searchWith returns smallestSearch with one post in it and one term
// pointing at it, built from maps so a test can change one field.
func searchWith(t *testing.T, post map[string]any, terms map[string]any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(smallestSearch), &doc); err != nil {
		t.Fatal(err)
	}
	full := map[string]any{
		"p": "cook-1971-np", "s": "cook-1971-np-s1", "i": 3,
		"t": "2. The theorem", "k": "p", "x": "Every language accepted by a machine.",
	}
	for k, v := range post {
		full[k] = v
	}
	doc["posts"] = []any{full}
	if terms == nil {
		terms = map[string]any{"machine": []any{0}}
	}
	doc["terms"] = terms
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestASearchIndexIsRecognisedByItsPath(t *testing.T) {
	for _, path := range []string{"search-en.json", "search-ja.json"} {
		kind, ok := Kind(path)
		if !ok || kind != Search {
			t.Errorf("%s is %q %v", path, kind, ok)
		}
	}
	for _, path := range []string{"search.json", "search-en.js", "search-english.json", "p/search-en.json"} {
		if _, ok := Kind(path); ok {
			t.Errorf("%s was read as a document of the site", path)
		}
	}
}

func TestTheSmallestValidSearchIndexValidates(t *testing.T) {
	if why := bad(t, "search-vi.json", []byte(smallestSearch)); len(why) != 0 {
		t.Errorf("an empty index does not validate: %v", why)
	}
	if why := bad(t, "search-vi.json", searchWith(t, nil, nil)); len(why) != 0 {
		t.Errorf("an index with one post in it does not validate: %v", why)
	}
}

// A post has to be able to draw a result line on its own, so the paper, the
// block, the kind and the snippet are all required. The section and the
// title are not, because a block of the front page belongs to no section.
func TestAPostMustBeAbleToDrawAResultLine(t *testing.T) {
	if why := bad(t, "search-vi.json", searchWith(t, map[string]any{"s": nil, "t": nil}, nil)); len(why) == 0 {
		t.Error("a post with a null section was accepted")
	}
	for _, field := range []string{"p", "i", "k", "x"} {
		var doc map[string]any
		if err := json.Unmarshal(searchWith(t, nil, nil), &doc); err != nil {
			t.Fatal(err)
		}
		post := doc["posts"].([]any)[0].(map[string]any)
		delete(post, field)
		b, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if why := bad(t, "search-vi.json", b); len(why) == 0 {
			t.Errorf("a post with no %s was accepted", field)
		}
	}
}

// A posting is a position in the posts list, so a negative one is a bug in
// the emitter and a repeated one is a term filed twice against one block.
func TestAPostingListIsPositionsAndNotRepeats(t *testing.T) {
	for _, posts := range []any{[]any{-1}, []any{0, 0}, []any{}, "0", []any{"machine"}} {
		body := searchWith(t, nil, map[string]any{"machine": posts})
		if why := bad(t, "search-vi.json", body); len(why) == 0 {
			t.Errorf("the posting list %v was accepted", posts)
		}
	}
}

// The kinds a post can carry are the kinds a block can be, from the one
// definition, so the two cannot drift apart.
func TestAPostCarriesABlockKind(t *testing.T) {
	if why := bad(t, "search-vi.json", searchWith(t, map[string]any{"k": "math"}, nil)); len(why) != 0 {
		t.Errorf("a formula post does not validate: %v", why)
	}
	if why := bad(t, "search-vi.json", searchWith(t, map[string]any{"k": "blockquote"}, nil)); len(why) == 0 {
		t.Error("a post of a kind the app cannot draw was accepted")
	}
}

func TestASearchIndexIsNotAPage(t *testing.T) {
	if why := bad(t, "search-vi.json", []byte(smallestPage)); len(why) == 0 {
		t.Error("a page validated as a search index")
	}
	if why := bad(t, "p/cook-1971-np/en.json", []byte(smallestSearch)); len(why) == 0 {
		t.Error("a search index validated as a page")
	}
}
