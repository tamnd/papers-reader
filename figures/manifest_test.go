package figures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func entry(paper, id string, page int) Figure {
	return Figure{
		Paper: paper, ID: id, Number: strings.TrimPrefix(id, "f"), Page: page,
		Box: Box{72, 400, 540, 600}, Caption: "Figure " + id + ": something",
		SHA256: strings.Repeat(id[1:], 32)[:64], Method: Vector,
		Fraction: 0.2, Width: 1200, Height: 800, Bytes: 90 << 10,
	}
}

func saved(t *testing.T, m *Manifest) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifests", "figures.yaml")
	if err := m.Save(path); err != nil {
		t.Fatalf("saving: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	return path, string(b)
}

// A corpus that has never cropped a figure has no manifest yet, and the
// first run has to have something to add to.
func TestAMissingManifestIsAnEmptyOne(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "figures.yaml"))
	if err != nil {
		t.Fatalf("loading a manifest that is not there: %v", err)
	}
	if len(m.Figures) != 0 {
		t.Fatalf("an empty corpus has %d figures", len(m.Figures))
	}
}

func TestAManifestThatDoesNotParseIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "figures.yaml")
	if err := os.WriteFile(path, []byte("figures: [oh dear\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a manifest that is not YAML loaded without complaint")
	}
}

func TestEveryFieldOfAnEntrySurvivesTheRoundTrip(t *testing.T) {
	want := entry("vaswani-2017-attention", "f01", 3)
	want.Method = Raster
	want.Scale = 0.75
	path, _ := saved(t, &Manifest{Figures: []Figure{want}})
	got, err := Load(path)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got.Figures) != 1 {
		t.Fatalf("%d figures came back, want 1", len(got.Figures))
	}
	if got.Figures[0] != want {
		t.Fatalf("came back as\n%+v\nwant\n%+v", got.Figures[0], want)
	}
}

// The header is the first thing anybody reading a diff of this file sees,
// and a run that dropped it would be a silent deletion in an unrelated pull
// request.
func TestTheHeaderIsWrittenEveryTime(t *testing.T) {
	_, text := saved(t, &Manifest{Figures: []Figure{entry("a-2020-one", "f01", 1)}})
	if !strings.HasPrefix(text, Header) {
		t.Fatalf("the file does not open with the header:\n%s", text[:min(len(text), 200)])
	}
}

// Sorted by paper and then by figure, so that a run which adds a paper adds
// one run of lines in one place and a run which redoes a paper rewrites that
// paper's lines and nothing else. An unsorted manifest makes every pull
// request look like it moved everything.
func TestTheManifestIsSorted(t *testing.T) {
	m := &Manifest{Figures: []Figure{
		entry("zhang-2021-last", "f02", 9),
		entry("abbot-2019-first", "f02", 4),
		entry("zhang-2021-last", "f01", 8),
		entry("abbot-2019-first", "f01", 2),
	}}
	path, _ := saved(t, m)
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, f := range got.Figures {
		order = append(order, f.Paper+"/"+f.ID)
	}
	want := "abbot-2019-first/f01 abbot-2019-first/f02 zhang-2021-last/f01 zhang-2021-last/f02"
	if strings.Join(order, " ") != want {
		t.Fatalf("order is %q, want %q", strings.Join(order, " "), want)
	}
}

func TestSavingCreatesTheManifestsDirectory(t *testing.T) {
	path, _ := saved(t, &Manifest{})
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("the directory was not created: %v", err)
	}
}

func TestTheManifestIsReadableByEverybody(t *testing.T) {
	path, _ := saved(t, &Manifest{Figures: []Figure{entry("a-2020-one", "f01", 1)}})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// CreateTemp opens at 0600, which would commit a file nobody else can
	// read into a public repository.
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode is %v, want 0644", got)
	}
}

func TestOfIsOnePapersFigures(t *testing.T) {
	m := &Manifest{Figures: []Figure{
		entry("a-2020-one", "f01", 1),
		entry("b-2021-two", "f01", 1),
		entry("a-2020-one", "f02", 3),
	}}
	got := m.Of("a-2020-one")
	if len(got) != 2 {
		t.Fatalf("Of returned %d figures, want 2", len(got))
	}
	for _, f := range got {
		if f.Paper != "a-2020-one" {
			t.Fatalf("Of returned a figure of %s", f.Paper)
		}
	}
}

func TestOfAPaperWithNoFiguresIsNothing(t *testing.T) {
	m := &Manifest{Figures: []Figure{entry("a-2020-one", "f01", 1)}}
	if got := m.Of("b-2021-two"); len(got) != 0 {
		t.Fatalf("Of returned %d figures for a paper that has none", len(got))
	}
}

// A second run of a paper is a correction: the figures it did not produce
// this time are figures that should no longer be there. Merging would leave
// the old ones behind with nothing on disk to match them, which is what F01
// fails on.
func TestReplaceDropsWhatThePaperNoLongerHas(t *testing.T) {
	m := &Manifest{Figures: []Figure{
		entry("a-2020-one", "f01", 1),
		entry("a-2020-one", "f02", 3),
		entry("b-2021-two", "f01", 1),
	}}
	m.Replace("a-2020-one", []Figure{entry("a-2020-one", "f01", 7)})
	if got := m.Of("a-2020-one"); len(got) != 1 || got[0].Page != 7 {
		t.Fatalf("after the re-run the paper has %+v", got)
	}
	if got := m.Of("b-2021-two"); len(got) != 1 {
		t.Fatalf("replacing one paper changed another: %+v", got)
	}
	if len(m.Figures) != 2 {
		t.Fatalf("the manifest holds %d entries, want 2", len(m.Figures))
	}
}

func TestReplacingAPaperThatWasNotThereAddsIt(t *testing.T) {
	m := &Manifest{}
	m.Replace("a-2020-one", []Figure{entry("a-2020-one", "f01", 1)})
	if len(m.Figures) != 1 {
		t.Fatalf("the manifest holds %d entries, want 1", len(m.Figures))
	}
}

// A paper that produced nothing this time has to lose what it had, or the
// entries outlive the files.
func TestReplacingWithNothingClearsThePaper(t *testing.T) {
	m := &Manifest{Figures: []Figure{
		entry("a-2020-one", "f01", 1),
		entry("b-2021-two", "f01", 1),
	}}
	m.Replace("a-2020-one", nil)
	if got := m.Of("a-2020-one"); len(got) != 0 {
		t.Fatalf("the paper still has %+v", got)
	}
	if got := m.Of("b-2021-two"); len(got) != 1 {
		t.Fatal("the other paper was dropped too")
	}
}

// The same diagram reprinted on two pages is one figure. Committing it twice
// would translate its caption twice and give a reader two names for one
// picture.
func TestTheSameBytesAreRecognised(t *testing.T) {
	first := entry("a-2020-one", "f01", 1)
	if got, ok := Has([]Figure{first}, first.SHA256); !ok || got.ID != "f01" {
		t.Fatalf("the hash already in the paper was not found: %+v %v", got, ok)
	}
	if _, ok := Has([]Figure{first}, SHA256([]byte("something else"))); ok {
		t.Fatal("a hash the paper does not have was found")
	}
	if _, ok := Has(nil, first.SHA256); ok {
		t.Fatal("a paper with no figures matched a hash")
	}
}

func TestTheNextIdCountsFromOne(t *testing.T) {
	if got := Next(nil); got != "f01" {
		t.Fatalf("the first figure is %q, want f01", got)
	}
	got := Next([]Figure{entry("a-2020-one", "f01", 1), entry("a-2020-one", "f02", 2)})
	if got != "f03" {
		t.Fatalf("the third figure is %q, want f03", got)
	}
}
