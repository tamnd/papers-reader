package poppler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The three refusals happen before anything is run, so they are the part of
// the rasteriser that can be checked on a machine with no poppler on it.
func TestToPNGRefusesWhatItCannotName(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		what string
		out  string
		page int
		r    Raster
	}{
		{"a destination that is not a png", filepath.Join(dir, "page.jpg"), 1, Raster{DPI: 300}},
		{"no resolution", filepath.Join(dir, "page.png"), 1, Raster{}},
		{"page zero", filepath.Join(dir, "page.png"), 0, Raster{DPI: 300}},
	} {
		err := ToPNG(context.Background(), filepath.Join(dir, "paper.pdf"), c.page, c.r, c.out)
		if err == nil {
			t.Errorf("%s was accepted", c.what)
		}
	}
	// None of the three got as far as making a directory to render into.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused render left %d things behind", len(entries))
	}
}

// A missing pdftoppm is named and explained rather than surfacing as an exec
// error, which is the rule for every program this package shells out to.
func TestToPNGSaysWhichProgramIsMissing(t *testing.T) {
	if Have("pdftoppm") {
		t.Skip("pdftoppm is installed on this machine")
	}
	dir := t.TempDir()
	err := ToPNG(context.Background(), filepath.Join(dir, "paper.pdf"), 1, Raster{DPI: 300}, filepath.Join(dir, "page.png"))
	if err == nil || !strings.Contains(err.Error(), "pdftoppm") {
		t.Errorf("a missing pdftoppm came back as %v", err)
	}
}

// A render that poppler refuses leaves nothing behind, because a zero byte
// PNG sitting where a page should be is a page every later run believes is
// already done.
func TestAFailedRenderLeavesNothingBehind(t *testing.T) {
	if !Have("pdftoppm") {
		t.Skip("pdftoppm is not installed here")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "pages", "0001.png")
	if err := ToPNG(context.Background(), filepath.Join(dir, "nothing.pdf"), 1, Raster{DPI: 300, Gray: true}, out); err == nil {
		t.Fatal("rendering a file that is not there succeeded")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a failed render wrote the page anyway")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "pages"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed render left %v behind", entries)
	}
}
