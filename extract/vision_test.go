package extract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/papers-reader/poppler"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/render"
)

// a reader stands in for the fleet. It records what it was shown and answers
// from a script, one answer per attempt, so that a test can say what the
// second try comes back with.
type reader struct {
	says  []string
	fail  error
	asked []llm.Request
	seen  []string
}

func (r *reader) ask(_ context.Context, target string, req llm.Request) (Reply, error) {
	r.asked = append(r.asked, req)
	r.seen = append(r.seen, target)
	if r.fail != nil {
		return Reply{}, r.fail
	}
	text := ""
	if n := len(r.asked) - 1; n < len(r.says) {
		text = r.says[n]
	} else if len(r.says) > 0 {
		text = r.says[len(r.says)-1]
	}
	return Reply{
		Response: llm.Response{
			Text:    text,
			Usage:   llm.Usage{InputTokens: 1000, OutputTokens: 500},
			Elapsed: 90 * time.Second,
		},
		Model: "a-model",
	}, nil
}

// drawPage writes something where pdftoppm would. The bytes are not a PNG and
// nothing in this package looks at them, which is the honest shape of the
// test: what pdftoppm produces is pdftoppm's business.
func drawPage(_ context.Context, pdf string, page int, r poppler.Raster, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, fmt.Appendf(nil, "%s page %d at %d", pdf, page, r.DPI), 0o644)
}

func testVision(t *testing.T, r *reader) *Vision {
	t.Helper()
	return &Vision{
		Ask:    r.ask,
		Prompt: prompt.MustGet(prompt.OCR),
		Store:  render.Store{Dir: t.TempDir(), Draw: drawPage},
		PDF:    "codd-1970-relational.pdf",
		Paper:  "codd-1970-relational",
	}
}

func TestAPageIsRenderedAndRead(t *testing.T) {
	r := &reader{says: []string{"## 1. Relational Model and Normal Form\n\n377"}}
	v := testVision(t, r)

	got, err := v.Page(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK() || got.Attempts != 1 {
		t.Fatalf("the page came back %+v", got)
	}
	if got.Text != "## 1. Relational Model and Normal Form\n\n377" {
		t.Errorf("the page reads %q", got.Text)
	}
	if got.Model != "a-model" || got.Profile.DPI != render.Base || !got.Profile.Gray {
		t.Errorf("the page was read by %q at %s", got.Model, got.Profile)
	}
	if got.Usage.OutputTokens != 500 || got.Elapsed != 90*time.Second {
		t.Errorf("the page cost %+v in %s", got.Usage, got.Elapsed)
	}

	// The raster is on disk under the profile it was made at, which is what
	// makes a second run of the command free.
	if !v.Store.Has(got.Profile, 3) {
		t.Error("the page image was not kept")
	}
	// The prompt goes as the instructions, which is the part a provider can
	// cache across the pages of one paper, and the page as an image.
	req := r.asked[0]
	if !strings.Contains(req.Instructions, "Transcribe this page") {
		t.Error("the reading prompt was not sent as the instructions")
	}
	if len(req.Images) != 1 || req.Images[0].MediaType != "image/png" || req.Images[0].Detail != Detail {
		t.Errorf("the page went over as %+v", req.Images)
	}
	if r.seen[0] != "codd-1970-relational p3" {
		t.Errorf("the ledger would record this as %q", r.seen[0])
	}
}

// A refused page is asked again with more pixels in it, and the ladder is
// walked in order. This is the only retry in the toolchain and it is the whole
// reason the vision path is a loop and the other two are not.
func TestARefusedPageClimbsTheLadder(t *testing.T) {
	r := &reader{says: []string{
		"the page is $ unfinished",
		"still $ unfinished",
		"## 1. Relational Model and Normal Form",
	}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK() {
		t.Fatalf("the page was never accepted: %v", got.Faults)
	}
	if got.Attempts != 3 || got.Profile.DPI != render.Ladder[2] {
		t.Errorf("it took %d attempts and ended at %s", got.Attempts, got.Profile)
	}
	// Three asks, three rasters, and the cost of all three is on the page.
	for _, dpi := range render.Ladder {
		if !v.Store.Has(render.Profile{DPI: dpi, Gray: true}, 1) {
			t.Errorf("there is no raster at %d dpi", dpi)
		}
	}
	if got.Usage.OutputTokens != 1500 || got.Elapsed != 270*time.Second {
		t.Errorf("the page is recorded as costing %+v in %s", got.Usage, got.Elapsed)
	}
}

// A page refused at every rung is not an error. The fleet answered, the quota
// went, and what came back cannot be published: the caller reports it and
// carries on to the next page.
func TestAPageRefusedEverywhereComesBackWithItsFaults(t *testing.T) {
	r := &reader{says: []string{"I'm sorry, I cannot read this image."}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatalf("a refused page came back as an error: %v", err)
	}
	if got.OK() {
		t.Fatal("an apology was accepted as a page")
	}
	if got.Faults[0].Rule != A1 {
		t.Errorf("it was refused for %s", got.Faults[0].Rule)
	}
}

// A9 twice with the same words missing is a ladder that has stopped
// helping. The reader saw the paragraph and decided the page was over, and
// page 5 of the bitcoin paper spent two calls proving that at 400 and 600.
func TestTheLadderStopsOnTheSameMissingWordsTwice(t *testing.T) {
	first, _, ok := strings.Cut(layer, "\n\n")
	if !ok {
		t.Fatal("the fixture no longer has two paragraphs in it")
	}
	r := &reader{says: []string{first + "\n"}}
	v := testVision(t, r)
	checker := Checker{Layer: layerOf(1, layer)}
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK() {
		t.Fatal("a page with a paragraph missing was accepted")
	}
	if got.Attempts != 2 {
		t.Errorf("it asked %d times for the same missing words, want 2", got.Attempts)
	}
}

// Every other rule is about the answer alone, so two bad answers say nothing
// about the third and the ladder is walked the whole way.
func TestTheLadderStillClimbsForAnyOtherFault(t *testing.T) {
	r := &reader{says: []string{"I'm sorry, I cannot read this image."}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts != len(render.Ladder) {
		t.Errorf("it gave up after %d attempts", got.Attempts)
	}
}

// A host that will not answer is a different thing from a page that came back
// wrong, and only one of the two is worth trying at a higher resolution.
func TestAHostThatWillNotAnswerIsAnError(t *testing.T) {
	r := &reader{fail: errors.New("no route answered")}
	v := testVision(t, r)

	got, err := v.Page(context.Background(), 1)
	if err == nil {
		t.Fatal("a failed ask came back as a page")
	}
	if got.Attempts != 1 {
		t.Errorf("a failed ask was retried %d times", got.Attempts)
	}
}

// The wrapping comes off before the rules look at the page, so a good page in
// a code fence is accepted rather than costing two more asks.
func TestAWrappedPageIsAcceptedFirstTime(t *testing.T) {
	r := &reader{says: []string{"```markdown\n## 1. Introduction\n\n377\n```"}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK() || got.Attempts != 1 {
		t.Fatalf("a fenced page came back %+v", got)
	}
	if got.Text != "## 1. Introduction\n\n377" {
		t.Errorf("the page reads %q", got.Text)
	}
}

// The missing piece is named. A run that reached a model with no prompt would
// get pages nobody could account for, and the hash in the front matter is the
// only record of what was asked.
func TestVisionSaysWhatItIsMissing(t *testing.T) {
	good := func() *Vision {
		return &Vision{
			Ask:    (&reader{says: []string{"x"}}).ask,
			Prompt: prompt.MustGet(prompt.OCR),
			Store:  render.Store{Dir: t.TempDir(), Draw: drawPage},
			PDF:    "paper.pdf",
		}
	}
	for _, c := range []struct {
		what string
		make func() *Vision
		says string
	}{
		{"no model", func() *Vision { v := good(); v.Ask = nil; return v }, "model"},
		{"no prompt", func() *Vision { v := good(); v.Prompt = prompt.Prompt{}; return v }, "prompt"},
		{"no file", func() *Vision { v := good(); v.PDF = ""; return v }, "rasterise"},
		{"nowhere to put the images", func() *Vision { v := good(); v.Store.Dir = ""; return v }, "page images"},
	} {
		_, err := c.make().Page(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s came back as %v", c.what, err)
		}
	}
	if _, err := good().Page(context.Background(), 0); err == nil {
		t.Error("page zero was accepted")
	}
}

// Codd's note goes to Codd's pages and to nobody else's, and the hash of what
// was asked says which.
func TestTheNotedPaperIsAskedItsOwnQuestion(t *testing.T) {
	codd, err := prompt.Page("codd-1970-relational")
	if err != nil {
		t.Fatal(err)
	}
	other, err := prompt.Page("vaswani-2017-attention")
	if err != nil {
		t.Fatal(err)
	}
	if codd.SHA == other.SHA {
		t.Fatal("the noted paper and the rest are asked the same question")
	}

	r := &reader{says: []string{"377"}}
	v := testVision(t, r)
	v.Prompt = codd
	if _, err := v.Page(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.asked[0].Instructions, "Communications of the ACM") {
		t.Error("Codd's pages were read without Codd's note")
	}
}

// A table that comes back the same three times is written as a fence on the
// way out rather than thrown away. The page is otherwise fine and the cells
// are all there, and a fourth ask would buy the same answer again.
func TestATableThatNeverConvertsIsFencedAtTheTopOfTheLadder(t *testing.T) {
	table := "<table>\n" +
		"<tr><th>Run</th><th>Depth</th><th>Score</th></tr>\n" +
		"<tr><td rowspan=\"4\">(A)</td><td colspan=\"7\">3</td><td>27.3</td></tr>\n" +
		"</table>"
	r := &reader{says: []string{"### Results\n\n" + table}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK() {
		t.Fatalf("the page was thrown away for %v", got.Faults)
	}
	if got.Attempts != len(render.Ladder) {
		t.Errorf("it was fenced after %d attempts and should have climbed first", got.Attempts)
	}
	want := "### Results\n\n```text\nRun  Depth  Score\n(A)  3  27.3\n```"
	if got.Text != want {
		t.Errorf("the page reads:\n%s\nand should read:\n%s", got.Text, want)
	}
}

// The fence is the last resort for one thing only. A page the reader could
// not read is still a page nobody can publish, and writing its tables out
// differently does not change that.
func TestAFenceDoesNotRescueAPageThatIsWrongForOtherReasons(t *testing.T) {
	table := "<table>\n<tr><td rowspan=\"4\">(A)</td><td colspan=\"7\">3</td></tr>\n</table>"
	r := &reader{says: []string{"the page is $ unfinished\n\n" + table}}
	v := testVision(t, r)
	var checker Checker
	v.Check = checker.Check

	got, err := v.Page(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK() {
		t.Fatal("a page with an unclosed formula on it was accepted")
	}
	if strings.Contains(got.Text, "```text") {
		t.Error("the page that came back is not the one the reader gave")
	}
}
