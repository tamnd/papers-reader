package split

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

func base() corpus.Front {
	return corpus.Front{
		Paper:      "lovelace-1843-notes",
		Title:      "Notes on the Analytical Engine",
		Authors:    []string{"Ada Lovelace"},
		Year:       1843,
		Lang:       corpus.EN,
		Extraction: "native",
	}
}

func files(t *testing.T) []File {
	t.Helper()
	r := Split(doc(
		"Notes on the Analytical Engine",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
	))
	return Files(base(), r)
}

func TestAFileCarriesTheSectionItHolds(t *testing.T) {
	fs := files(t)
	if len(fs) != 4 {
		t.Fatalf("made %d files, want 4", len(fs))
	}
	f := fs[2]
	if f.Name != "02_method.md" {
		t.Errorf("the second section is filed as %q", f.Name)
	}
	if f.Front.Section != "2" || f.Front.SectionTitle != "Method" {
		t.Errorf("the front matter says section %q %q", f.Front.Section, f.Front.SectionTitle)
	}
	if f.Front.Kind != KindSection {
		t.Errorf("the kind is %q, want %q", f.Front.Kind, KindSection)
	}
	if f.Front.Paper != "lovelace-1843-notes" || f.Front.Extraction != "native" {
		t.Errorf("the paper's own fields did not survive: %+v", f.Front)
	}
}

func TestTheHashInTheFrontMatterIsOfTheBody(t *testing.T) {
	f := files(t)[1]
	if f.Front.ContentSHA256 != corpus.ContentSHA(f.Body) {
		t.Errorf("content_sha256 is %q, want the hash of the body", f.Front.ContentSHA256)
	}
	b, err := f.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.ContentSHA(body) != front.ContentSHA256 {
		t.Error("a file this package wrote does not agree with itself about its own hash")
	}
}

func TestThePagesASectionCameOffAreRecorded(t *testing.T) {
	r := &Result{Sections: []Section{
		{Ordinal: 1, Title: "Introduction", Kind: KindSection, First: 3, Last: 6, Body: "a."},
		{Ordinal: 2, Title: "Method", Kind: KindSection, First: 7, Last: 7, Body: "b."},
		{Ordinal: 3, Title: "Results", Kind: KindSection, Body: "c."},
	}}
	fs := Files(base(), r)
	for i, want := range []string{"3-6", "7", ""} {
		if got := fs[i].Front.PDFPages; got != want {
			t.Errorf("section %d records pdf_pages %q, want %q", i+1, got, want)
		}
	}
}

func TestAFirstRunCreatesEveryFile(t *testing.T) {
	dir := t.TempDir()
	r, err := Write(dir, files(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Created) != 4 || len(r.Updated) != 0 || len(r.Kept) != 0 {
		t.Errorf("the first run created %v, updated %v, kept %v", r.Created, r.Updated, r.Kept)
	}
	if _, err := os.Stat(filepath.Join(dir, "01_introduction.md")); err != nil {
		t.Error(err)
	}
}

func TestASecondRunOverTheSameTextChangesNothing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	r, err := Write(dir, files(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Unchanged) != 4 {
		t.Errorf("the second run left %d files alone, want 4: created %v updated %v", len(r.Unchanged), r.Created, r.Updated)
	}
}

func TestAHandEditIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "02_method.md")
	edit(t, path, "a correction somebody made by hand")

	fs := files(t)
	fs[2].Body = []byte("what the next extraction run would have written\n")
	fs[2].Front.ContentSHA256 = corpus.ContentSHA(fs[2].Body)
	r, err := Write(dir, fs, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Kept) != 1 || r.Kept[0] != "02_method.md" {
		t.Fatalf("the run kept %v, want the file somebody edited", r.Kept)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "by hand") {
		t.Error("the hand edit was overwritten")
	}
}

func TestForceOverwritesTheHandEdit(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "02_method.md")
	edit(t, path, "a correction somebody made by hand")

	r, err := Write(dir, files(t), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Updated) != 1 || len(r.Kept) != 0 {
		t.Errorf("force updated %v and kept %v", r.Updated, r.Kept)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "by hand") {
		t.Error("force did not overwrite the hand edit")
	}
}

func TestAFileFromAnOlderSplitIsReportedAndNotDeleted(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "09_appendix.md")
	if err := os.WriteFile(old, []byte("from a split that read the paper differently\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Write(dir, files(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Stale) != 1 || r.Stale[0] != "09_appendix.md" {
		t.Errorf("the run reported %v as stale", r.Stale)
	}
	if _, err := os.Stat(old); err != nil {
		t.Error("a stale file was deleted rather than reported")
	}
}

func TestAFileWithNoFrontMatterIsSomebodysWork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "01_introduction.md")
	if err := os.WriteFile(path, []byte("notes somebody started by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Write(dir, files(t), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Kept) != 1 || r.Kept[0] != "01_introduction.md" {
		t.Errorf("the run kept %v, want the file it did not write", r.Kept)
	}
}

// edit rewrites the body of a content file the way a person would: the front
// matter is left as it was, which is what makes the hash disagree.
func edit(t *testing.T, path, body string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	front, _, err := corpus.ParseFront(b)
	if err != nil {
		t.Fatal(err)
	}
	out, err := corpus.Render(front, []byte(body+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
