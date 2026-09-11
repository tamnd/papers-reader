package extract

import (
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

// A paper read again by a different path is that path's text now, and the
// range starts over: the pages the old path wrote have been overwritten.
func TestANewPathReplacesTheRecord(t *testing.T) {
	dir := t.TempDir()
	if err := (Record{Path: "native", First: 1, Last: 8}).Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := (Record{Path: "layout", Tool: "docling 2.0.0", First: 3, Last: 5}).Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "layout" || got.First != 3 || got.Last != 5 {
		t.Errorf("got %s over %d to %d, want layout over 3 to 5", got.Path, got.First, got.Last)
	}
}
