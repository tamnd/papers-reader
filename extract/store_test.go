package extract

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAPageIsWrittenAndReadBack(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "pages")}
	if s.Has(7) {
		t.Fatal("a page was there before anything was written")
	}
	if err := s.Write(7, "the text of page seven"); err != nil {
		t.Fatal(err)
	}
	if !s.Has(7) {
		t.Fatal("the page was written and is not there")
	}
	got, err := s.Read(7)
	if err != nil {
		t.Fatal(err)
	}
	// The newline is added, because a text file without one is a file every
	// other tool complains about.
	if got != "the text of page seven\n" {
		t.Errorf("read %q", got)
	}
}

func TestThePageFilesSortTheWayThePagesRun(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, p := range []int{9, 10, 100, 1} {
		if err := s.Write(p, "a page"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"0001.txt", "0009.txt", "0010.txt", "0100.txt"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("the directory reads %v", names)
	}
}

func TestTheStoreSaysWhichPagesAreStillToDo(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, p := range []int{1, 2, 5} {
		if err := s.Write(p, "a page"); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := s.Pages()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pages, []int{1, 2, 5}) {
		t.Errorf("the store holds %v", pages)
	}
	if got := s.Missing(1, 6); !reflect.DeepEqual(got, []int{3, 4, 6}) {
		t.Errorf("the work left is %v", got)
	}
}

func TestABlankPageCountsAsDone(t *testing.T) {
	// A blank verso is a real answer. Asking for it again on every run is
	// asking for it forever.
	s := Store{Dir: t.TempDir()}
	if err := s.Write(3, ""); err != nil {
		t.Fatal(err)
	}
	if got := s.Missing(1, 3); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("the work left is %v, and page 3 is done", got)
	}
}

func TestAStoreWithNoDirectoryYetHoldsNoPages(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "nothing", "here")}
	pages, err := s.Pages()
	if err != nil {
		t.Fatalf("a store that has not been written to gave an error: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("it holds %v", pages)
	}
}

func TestNothingIsLeftBehindByAWrite(t *testing.T) {
	// The page is written to a temporary file and renamed. A run that is
	// killed leaves a temporary file and no half written page file, which is
	// the one thing that must not happen: the next run would see the page as
	// done and never look at it again.
	s := Store{Dir: t.TempDir()}
	if err := s.Write(1, "a page"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "0001.txt" {
		t.Errorf("the directory holds %d entries", len(entries))
	}
}

func TestThePageMapIsLearnedFromTheFoliosThePagesPrint(t *testing.T) {
	pages := []Page{
		{Number: 1, Printed: "483"},
		{Number: 2, Printed: "484"},
		{Number: 3, Printed: "485"},
	}
	m := LearnMap(pages)
	if !m.Known || m.Offset != 482 {
		t.Fatalf("learned offset %d, known %v", m.Offset, m.Known)
	}
	if got, ok := m.Printed(2); !ok || got != 484 {
		t.Errorf("page 2 should print %d", got)
	}
}

func TestOneMisreadFolioDoesNotMoveTheMap(t *testing.T) {
	// A scan misreads a folio now and then, and a mean would put the whole
	// paper's offset out and refuse every page of it.
	pages := []Page{
		{Number: 1, Printed: "483"},
		{Number: 2, Printed: "4844"},
		{Number: 3, Printed: "485"},
		{Number: 4, Printed: "486"},
	}
	if m := LearnMap(pages); m.Offset != 482 {
		t.Errorf("learned offset %d", m.Offset)
	}
}

func TestAPaperThatPrintsNoNumbersHasNoMap(t *testing.T) {
	pages := []Page{{Number: 1}, {Number: 2}, {Number: 3, Printed: "iv"}}
	m := LearnMap(pages)
	if m.Known {
		t.Errorf("learned an offset of %d from nothing", m.Offset)
	}
	if _, ok := m.Printed(1); ok {
		t.Error("a map that knows nothing answered anyway")
	}
}

func TestAPageKnowsWhatNumberItPrinted(t *testing.T) {
	if n, ok := (Page{Printed: "[12]"}).PrintedNumber(); !ok || n != 12 {
		t.Errorf("read %d from a bracketed folio", n)
	}
	if _, ok := (Page{Printed: "xiv"}).PrintedNumber(); ok {
		t.Error("a roman numeral was read as an offset")
	}
	if _, ok := (Page{}).PrintedNumber(); ok {
		t.Error("a page that printed nothing gave a number")
	}
}
