package work

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/queue"
	"github.com/tamnd/llm/route"
	"github.com/tamnd/papers-reader/corpus"
)

// testCorpus is the smallest corpus that opens: one real paper, because a
// fixture with a made up id hides every mistake that only shows up on the
// shapes ids really take.
func testCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	const manifest = `papers:
  - id: codd-1970-relational
    title: A Relational Model of Data for Large Shared Data Banks
    authors: [E. F. Codd]
    year: 1970
    venue: Communications of the ACM
    field: databases
    number: 1
`
	if err := os.WriteFile(filepath.Join(root, "manifests", "papers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheQueueLivesUnderWorkAndNotInTheCorpus(t *testing.T) {
	c := testCorpus(t)

	q, err := Queue(c)
	if err != nil {
		t.Fatal(err)
	}
	if want := c.Work("queue"); q.Root != want {
		t.Errorf("the queue is at %s, want %s", q.Root, want)
	}
	// work/ is gitignored in the corpus repository. A queue anywhere else is
	// a list of what a model was asked, committed by accident.
	if _, err := os.Stat(filepath.Join(c.Root, "work", "queue")); err != nil {
		t.Error(err)
	}
}

func TestAnEmptyStageIsOnTheBoard(t *testing.T) {
	// A stage with no work left and a stage nobody has started are different
	// things, and a board that shows only the stages that have files in them
	// cannot tell them apart.
	c := testCorpus(t)

	q, err := Queue(c)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := q.Stages()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[queue.Stage]bool, len(stages))
	for _, s := range stages {
		found[s] = true
	}
	for _, want := range Stages {
		if !found[want] {
			t.Errorf("%s is not on the board", want)
		}
	}
}

func TestTheSameWorkTwiceIsOneJob(t *testing.T) {
	c := testCorpus(t)
	q, err := Queue(c)
	if err != nil {
		t.Fatal(err)
	}
	job := queue.New(Extract, "codd-1970-relational/p012", "input-hash", "prompt-hash")

	for i := range 2 {
		added, err := q.Add(job)
		if err != nil {
			t.Fatal(err)
		}
		if want := i == 0; added != want {
			t.Errorf("adding it the %d time reported %v, want %v", i+1, added, want)
		}
	}
	stats, err := q.Stats(Extract)
	if err != nil {
		t.Fatal(err)
	}
	if got := stats.Counts[queue.Pending]; got != 1 {
		t.Errorf("%d jobs pending, want 1", got)
	}
}

// The reader on the machine with the graphics card is an OCR model at rank
// 5, so it wins every pick there is. Asked to translate, it answered in a
// mixture of Vietnamese and Russian, and the translator's own checks passed
// it because the formulas and the citations all survived.
func TestAStageOnlyGetsTheRoutesThatWillServeIt(t *testing.T) {
	reader := route.Route{
		Name: "reader", Kind: route.KindDirect, BaseURL: "http://127.0.0.1:1/v1",
		Model: "olmOCR", Rank: 5, Vision: true, Jobs: []string{"extract"},
	}
	general := route.Route{
		Name: "pool", Kind: route.KindPool, BaseURL: "http://127.0.0.1:2/v1",
		Model: "gpt-5", Rank: 10, Vision: true,
	}
	registry := route.Registry{Routes: []route.Route{reader, general}}
	for _, c := range []struct {
		stage queue.Stage
		want  []string
	}{
		{"extract", []string{"reader", "pool"}},
		{"translate", []string{"pool"}},
	} {
		var got []string
		for _, r := range Able(registry, c.stage).Routes {
			got = append(got, r.Name)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s = %v, want %v", c.stage, got, c.want)
		}
	}
}

// A fleet with routes in it that all name other stages is a route file to
// edit, and saying "no routes configured" over a table with two in it sends
// somebody to look in the wrong place.
func TestAFleetSaysWhichFilterEmptiedIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.json")
	registry := route.Registry{Routes: []route.Route{{
		Name: "reader", Kind: route.KindDirect, BaseURL: "http://127.0.0.1:1/v1",
		Model: "olmOCR", Rank: 5, Vision: true, Jobs: []string{"extract"},
	}}}
	if err := registry.Write(path); err != nil {
		t.Fatal(err)
	}
	_, _, err := Fleet("translate", path, false, nil)
	if err == nil || !strings.Contains(err.Error(), "no route will do translate") {
		t.Errorf("error = %v, want it to name the stage nothing will serve", err)
	}
}

func TestThisApplicationNamesItself(t *testing.T) {
	// The name is the config directory, the environment variable prefix and
	// the prompt cache key on a proxy that more than one project shares. Two
	// projects that share a cache prefix read each other's answers.
	Configure()
	if got := llm.App(); got != App {
		t.Errorf("the library thinks this is %q, want %q", got, App)
	}
	if got := llm.EnvName("ROUTES"); got != "PAPERS_ROUTES" {
		t.Errorf("the route file variable is %q, want PAPERS_ROUTES", got)
	}
}
