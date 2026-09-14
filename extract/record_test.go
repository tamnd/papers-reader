package extract

import (
	"strings"
	"testing"
	"time"
)

func TestRecordRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := Record{
		Path:  "layout",
		Tool:  "mineru 2.1.0",
		First: 1,
		Last:  15,
		When:  time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
	}
	if err := want.Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("wrote a record and read back nothing")
	}
	if got.Path != want.Path || got.Tool != want.Tool {
		t.Errorf("read back %s by %q, want %s by %q", got.Path, got.Tool, want.Path, want.Tool)
	}
	if got.First != 1 || got.Last != 15 {
		t.Errorf("read back pages %d to %d, want 1 to 15", got.First, got.Last)
	}
	if !got.When.Equal(want.When) {
		t.Errorf("read back %s, want %s", got.When, want.When)
	}
}

func TestMissingRecordIsNotAnError(t *testing.T) {
	got, err := ReadRecord(t.TempDir())
	if err != nil {
		t.Fatalf("a work directory with no record is the state every paper extracted before this existed is in: %v", err)
	}
	if got != nil {
		t.Errorf("read %v out of an empty directory", got)
	}
}

// A resumed run does the pages the first run did not. The record has to end
// up covering both halves, because it is what the front matter's page range
// is written from.
func TestASecondRunWidensTheRange(t *testing.T) {
	dir := t.TempDir()
	first := Record{Path: "native", Tool: "pdftotext version 24.02.0", First: 1, Last: 8}
	if err := first.Write(dir); err != nil {
		t.Fatal(err)
	}
	second := Record{Path: "native", Tool: "pdftotext version 24.02.0", First: 9, Last: 12}
	if err := second.Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.First != 1 || got.Last != 12 {
		t.Errorf("after two runs the record covers %d to %d, want 1 to 12", got.First, got.Last)
	}
}

// A whole paper read again by a different path is that path's text now, and
// the range starts over: every page the old path wrote has been overwritten.
func TestANewPathOverTheWholePaperReplacesTheRecord(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "native", First: 1, Last: 8}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := (Record{Path: "layout", Tool: "docling 2.0.0", First: 1, Last: 8}).Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "layout" || got.First != 1 || got.Last != 8 {
		t.Errorf("got %s over %d to %d, want layout over 1 to 8", got.Path, got.First, got.Last)
	}
}

// Some of a paper read again by a different path is refused, because there
// is no record that is true of it. This is the Bitcoin paper: nine pages
// read by a model, page 5 read again with pdftotext, and the record left
// saying native over one page while eight of the nine were still the
// model's. Split then stamped extraction: native on all fourteen files.
func TestAPartialReReadOnAnotherPathIsRefused(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "vision", Tool: "olmOCR", First: 1, Last: 9}).Write(dir); err != nil {
		t.Fatal(err)
	}
	err := (Record{Path: "native", Tool: "pdftotext version 26.09.0", First: 5, Last: 5}).Write(dir)
	if err == nil {
		t.Fatal("Write accepted a one page re-read on another path over a nine page paper")
	}
	for _, want := range []string{"1 to 9", "vision", "5 to 5", "native", "re-extract"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say %q: %v", want, err)
		}
	}
	// And the record on disk is the one that is still true, because a
	// refusal that had already overwritten the file would be the same bug
	// with a message on top of it.
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "vision" || got.First != 1 || got.Last != 9 {
		t.Errorf("after the refusal the record is %s over %d to %d, want vision over 1 to 9", got.Path, got.First, got.Last)
	}
}

// A record written before first_page was always filled in reads as starting
// at page one, so a re-read of the whole paper still covers it.
func TestAMissingFirstPageReadsAsPageOne(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "native", Last: 4}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := (Record{Path: "vision", Tool: "olmOCR", First: 1, Last: 4}).Write(dir); err != nil {
		t.Errorf("Write refused a re-read of the whole paper: %v", err)
	}
}

// A different PDF is a different document, and the pages the old one had
// read are not pages of it.
func TestANewPDFReplacesTheRange(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "vision", Source: "aaaa", First: 1, Last: 18}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := (Record{Path: "vision", Source: "bbbb", First: 1, Last: 3}).Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.First != 1 || got.Last != 3 {
		t.Errorf("after the PDF was replaced the record covers %d to %d, want 1 to 3", got.First, got.Last)
	}
}

func TestARecordFromBeforeTheSourceFieldStillWidens(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "native", First: 1, Last: 8}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := (Record{Path: "native", Source: "aaaa", First: 9, Last: 12}).Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.First != 1 || got.Last != 12 {
		t.Errorf("the record covers %d to %d, want 1 to 12", got.First, got.Last)
	}
}

func TestReplacedNeedsBothSides(t *testing.T) {
	for _, c := range []struct {
		why  string
		old  *Record
		sha  string
		want bool
	}{
		{"nothing was read before", nil, "aaaa", false},
		{"the record does not say which file", &Record{}, "aaaa", false},
		{"the caller does not know the hash", &Record{Source: "aaaa"}, "", false},
		{"the same file", &Record{Source: "aaaa"}, "aaaa", false},
		{"a different file", &Record{Source: "aaaa"}, "bbbb", true},
	} {
		if got := Replaced(c.old, c.sha); got != c.want {
			t.Errorf("%s: Replaced gave %v, want %v", c.why, got, c.want)
		}
	}
}
