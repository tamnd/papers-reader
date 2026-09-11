package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEachToolIsRecognisedByWhatItWrote(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want Tool
	}{
		{"mineru middle", mineruMiddleFixture, MinerU},
		{"mineru content list", mineruListFixture, MinerU},
		{"marker", markerFixture, Marker},
		{"docling", doclingFixture, Docling},
	} {
		got, err := Detect([]byte(tt.in))
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s was read as %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSomethingElseIsNotATool(t *testing.T) {
	for _, in := range []string{
		`{"hello": "world"}`,
		`not json at all`,
		``,
	} {
		if got, err := Detect([]byte(in)); err == nil {
			t.Errorf("%q was read as %q, want an error", in, got)
		}
	}
}

func TestParsingAsTheWrongToolIsAnError(t *testing.T) {
	if _, err := ParseAs("olmocr", []byte(markerFixture)); err == nil {
		t.Fatal("a tool this does not read was accepted")
	}
}

func TestOpenDirPrefersTheFileWithBoxes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "invented_content_list.json"), mineruListFixture)
	write(t, filepath.Join(dir, "invented_middle.json"), mineruMiddleFixture)

	d, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The middle file has boxes and the content list does not, and the
	// figure stage needs them, so the middle file is the one to read.
	if got, want := len(d.Figures()), 1; got != want {
		t.Fatalf("got %d figures, want %d", got, want)
	}
	if d.Figures()[0].Box.Empty() {
		t.Error("the content list was read in preference to the middle file")
	}
}

func TestOpenDirSniffsAFileItDoesNotKnowTheNameOf(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "whatever-they-called-it.json"), markerFixture)

	d, err := OpenDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != Marker {
		t.Errorf("tool is %q, want %q", d.Tool, Marker)
	}
}

func TestOpenDirSaysSoWhenThereIsNothingToRead(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "notes.json"), `{"hello": "world"}`)
	if _, err := OpenDir(dir); err == nil {
		t.Fatal("a directory with nothing in it was accepted")
	}
}

func TestDoneSeesFinishedWork(t *testing.T) {
	dir := t.TempDir()
	if Done(dir) {
		t.Error("an empty directory says the work is finished")
	}
	write(t, filepath.Join(dir, "invented_middle.json"), mineruMiddleFixture)
	if !Done(dir) {
		t.Error("a finished directory says the work is not done")
	}
}

func TestDoneLooksInTheSubdirectoryTheToolMade(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "invented")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(sub, "invented.json"), doclingFixture)
	if !Done(dir) {
		t.Error("output in the subdirectory the tool made was not found")
	}
}

func TestOpenReportsTheFileThatWouldNotRead(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "broken.json")
	write(t, name, `{"hello": "world"}`)
	_, err := Open(name)
	if err == nil {
		t.Fatal("a file that is not a tool's output was accepted")
	}
	if got := err.Error(); !strings.Contains(got, "broken.json") {
		t.Errorf("the error is %q, want the file named in it", got)
	}
}

func TestTheToolsAreNamedInAnError(t *testing.T) {
	if got, want := names(), "mineru, marker or docling"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func write(t *testing.T, name, body string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
