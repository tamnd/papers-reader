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

// cover is a title block of the kind a paper puts on a page of its own: a
// title, an author and a citation, and no paragraph anywhere near long
// enough to be an abstract. Three of the first eight papers in this corpus
// open with one.
const cover = "Notes on the Analytical Engine\n\nAda Lovelace\n\nThis article appeared in the Scientific Memoirs, volume 3.\n"

// A restricted paper publishes one file, and the cap is applied to whatever
// goes in it.
func TestRestrictKeepsOneFileAndCapsIt(t *testing.T) {
	fs := Restrict(files(t), 20)
	if len(fs) != 1 {
		t.Fatalf("Restrict left %d files, want 1", len(fs))
	}
	if n := len(strings.Fields(string(fs[0].Body))); n > 20 {
		t.Errorf("the published file runs to %d words, want at most 20", n)
	}
	if fs[0].Front.ContentSHA256 != corpus.ContentSHA(fs[0].Body) {
		t.Error("Restrict cut the body and left the hash of what it cut")
	}
}

// The abstract is not always in the front block. A paper that opens with a
// cover sheet used to publish the cover sheet, which is a catalogue entry
// rather than a paper, and it is the whole of what the corpus would ever say
// about a paper it may not redistribute.
func TestRestrictFetchesTheAbstractFromPastTheCoverSheet(t *testing.T) {
	fs := Restrict(Files(base(), Split(doc(
		cover,
		"Abstract", prose,
		"1 Introduction", prose,
	))), AbstractWords)
	body := string(fs[0].Body)
	if !strings.Contains(body, "Notes on the Analytical Engine") {
		t.Error("the title did not survive")
	}
	if longestParagraph(body) < AbstractParagraph {
		t.Errorf("the published file is still a cover sheet:\n%s", body)
	}
}

// A front block that already carries an abstract is not given a second one
// off the next page.
func TestRestrictLeavesAFrontBlockThatAlreadyHasAnAbstract(t *testing.T) {
	fs := Restrict(Files(base(), Split(doc(
		"Notes on the Analytical Engine",
		prose,
		"1 Introduction", "A paragraph of the introduction that is quite distinctive and nothing at all like the abstract above it, so that it can be recognised.",
		"2 Method", prose,
		"3 Results", prose,
	))), AbstractWords)
	if strings.Contains(string(fs[0].Body), "quite distinctive") {
		t.Errorf("Restrict took a paragraph it did not need:\n%s", fs[0].Body)
	}
}

// The page range is a claim about where the published text came from, so it
// has to cover the page the abstract was taken off.
func TestRestrictRecordsThePagesThePublishedTextCameFrom(t *testing.T) {
	fs := Restrict([]File{
		{Name: "00_front.md", Front: corpus.Front{PDFPages: "1"}, Body: []byte(cover)},
		{Name: "01_intro.md", Front: corpus.Front{PDFPages: "2-4"}, Body: []byte(prose + "\n")},
	}, AbstractWords)
	if got := fs[0].Front.PDFPages; got != "1-4" {
		t.Errorf("pdf_pages is %q, want 1-4", got)
	}
}

// A paper with nothing long enough anywhere is published as it stands, and
// audit rule T06 is what says so. Silently writing nothing would lose the
// title as well.
func TestRestrictPublishesAShortPaperAsItStands(t *testing.T) {
	fs := Restrict([]File{
		{Name: "00_front.md", Front: corpus.Front{PDFPages: "1"}, Body: []byte(cover)},
	}, AbstractWords)
	if !strings.Contains(string(fs[0].Body), "Ada Lovelace") {
		t.Errorf("the title block was dropped:\n%s", fs[0].Body)
	}
}

// A title, a byline and a citation are separate things a reader looks for.
// The first cut of this joined the first 250 words into one line and the
// Paxos front matter came out as a paragraph of run-on prose.
func TestAbstractKeepsTheParagraphsItFits(t *testing.T) {
	body := strings.Join([]string{
		"Notes on the Analytical Engine",
		"Ada Lovelace",
		"This article appeared in the Scientific Memoirs, volume 3.",
		prose,
	}, "\n\n")
	got := Abstract(body, 30)
	if strings.Count(got, "\n\n") < 2 {
		t.Errorf("the paragraphs were run together:\n%s", got)
	}
	if !strings.HasPrefix(got, "Notes on the Analytical Engine\n\nAda Lovelace\n\n") {
		t.Errorf("the title block did not survive as a title block:\n%s", got)
	}
	if n := len(strings.Fields(got)); n > 30 {
		t.Errorf("the cut runs to %d words, want at most 30", n)
	}
}

// A paragraph that does not fit is cut where a sentence ends, and dropped
// when nothing that fits ends one.
func TestAbstractCutsAParagraphAtASentence(t *testing.T) {
	body := "One two. Three four. Five six."
	if got := Abstract(body, 5); got != "One two. Three four." {
		t.Errorf("Abstract cut to %q, want %q", got, "One two. Three four.")
	}
	long := "a b c d e f g h i j"
	if got := Abstract("Short one.\n\n"+long, 4); got != "Short one." {
		t.Errorf("Abstract kept half a clause: %q", got)
	}
}

// A block that is short enough is not touched at all.
func TestAbstractLeavesAShortBlockAlone(t *testing.T) {
	body := "Notes on the Analytical Engine\n\nAda Lovelace\n"
	if got := Abstract(body, AbstractWords); got != strings.TrimRight(body, "\n") {
		t.Errorf("Abstract rewrote a block it did not need to cut: %q", got)
	}
}

func TestSpanPages(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"1", "2-4", "1-4"},
		{"2-4", "1", "1-4"},
		{"1", "", "1"},
		{"", "2-4", "2-4"},
		{"", "", ""},
		{"3", "3", "3"},
	}
	for _, tc := range cases {
		if got := spanPages(tc.a, tc.b); got != tc.want {
			t.Errorf("spanPages(%q, %q) is %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// Accept is what makes a hand correction a first class thing rather than a
// file in a broken state. Before it, the same mismatch that protected an edit
// from the splitter failed audit rule T03, so a corrected corpus could never
// be a green one.
func TestAcceptRestampsAHandEdit(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "02_method.md")
	edit(t, path, "a correction somebody made by hand")

	names, err := Accept(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "02_method.md" {
		t.Fatalf("accepted %v, want the one file that was edited", names)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		t.Fatal(err)
	}
	if !front.Edited {
		t.Error("the file does not say it was edited")
	}
	if front.ContentSHA256 != corpus.ContentSHA(body) {
		t.Error("the hash was not restamped over the correction")
	}
	if !strings.Contains(string(body), "by hand") {
		t.Error("the correction itself was lost")
	}
}

// A file nobody has touched is left as it is. Marking one of those as edited
// would put a fence round a file the splitter should keep updating.
func TestAcceptLeavesUntouchedFilesAlone(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	names, err := Accept(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("accepted %v, want nothing", names)
	}
}

// The point of writing it down. Once a correction is accepted the hash agrees
// with the body again, so the old signal has gone, and the splitter has to
// protect the file by what it says instead.
func TestAnAcceptedEditIsStillNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "02_method.md")
	edit(t, path, "a correction somebody made by hand")
	if _, err := Accept(dir); err != nil {
		t.Fatal(err)
	}

	fs := files(t)
	fs[2].Body = []byte("what the next extraction run would have written\n")
	fs[2].Front.ContentSHA256 = corpus.ContentSHA(fs[2].Body)
	r, err := Write(dir, fs, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Kept) != 1 || r.Kept[0] != "02_method.md" {
		t.Fatalf("the run kept %v, want the accepted file", r.Kept)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "by hand") {
		t.Error("the accepted correction was overwritten")
	}
}

// --force still wins. It is the escape hatch for a correction somebody wants
// to throw away, and an accepted edit must not be harder to get rid of than
// an unaccepted one.
func TestForceOverwritesAnAcceptedEdit(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "02_method.md")
	edit(t, path, "a correction somebody made by hand")
	if _, err := Accept(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir, files(t), true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "by hand") {
		t.Error("--force left the hand edit in place")
	}
}

// Accepting twice is accepting once. The second run finds a file whose hash
// already matches its body and has nothing to do.
func TestAcceptIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, files(t), false); err != nil {
		t.Fatal(err)
	}
	edit(t, filepath.Join(dir, "02_method.md"), "a correction somebody made by hand")
	if _, err := Accept(dir); err != nil {
		t.Fatal(err)
	}
	names, err := Accept(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("the second run accepted %v, want nothing", names)
	}
}
