package split

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/assemble"
)

// Every fixture here is typeset in the test. None of it is copied from a
// paper, because a test file is committed to a public repository and a page
// of somebody's paper is not ours to put there.

func marks(ns ...int) map[int]bool {
	m := map[int]bool{}
	for _, n := range ns {
		m[n] = true
	}
	return m
}

func TestEndnotesWritesTheNotesAsDefinitions(t *testing.T) {
	body := strings.Join([]string{
		"1 The first of the invented notes, about the first section.",
		"2. The second of them, about the second section.",
		"3)The third, set tight against its number the way a raised one comes back.",
	}, "\n\n")
	got, n := Endnotes(body, marks(2))
	if n != 3 {
		t.Fatalf("Endnotes wrote %d definitions, want 3: %q", n, got)
	}
	want := strings.Join([]string{
		"[^1]: The first of the invented notes, about the first section.",
		"[^2]: The second of them, about the second section.",
		"[^3]: The third, set tight against its number the way a raised one comes back.",
	}, "\n\n") + "\n"
	if got != want {
		t.Errorf("Endnotes() = %q, want %q", got, want)
	}
}

// The run is the notes and what follows it is prose. A paper can print
// three notes and then carry on with the rest of a page the splitter put in
// the same section.
func TestEndnotesStopsWhereTheCountingStops(t *testing.T) {
	body := strings.Join([]string{
		"1 The first of the invented notes.",
		"2 The second of them.",
		"The rest of the page, which is not a note and carries no number.",
		"3 A paragraph that opens with a number after the run has ended.",
	}, "\n\n")
	got, n := Endnotes(body, marks(1))
	if n != 2 {
		t.Fatalf("Endnotes wrote %d definitions, want 2: %q", n, got)
	}
	if !strings.Contains(got, "\n\nThe rest of the page, which is not a note") {
		t.Errorf("the prose after the run was rewritten: %q", got)
	}
	if !strings.Contains(got, "\n\n3 A paragraph that opens") {
		t.Errorf("a paragraph past the end of the run was rewritten: %q", got)
	}
}

// Two papers in the corpus print a notes section that nothing refers to.
// Lifting those into definitions would empty the section and move its
// contents to the foot of the paper, which fixes nothing.
func TestEndnotesLeavesASectionNothingRefersTo(t *testing.T) {
	body := "1. The first of the invented notes.\n\n2. The second of them.\n"
	if got, n := Endnotes(body, nil); n != 0 || got != body {
		t.Errorf("Endnotes() = %q, %d, want it left alone", got, n)
	}
}

// The run has to start at one and count up, which is what keeps a list of
// steps or of design principles from being read as notes.
func TestEndnotesNeedsTheCountToStartAtOne(t *testing.T) {
	body := "2. The second of the invented notes.\n\n3. The third of them.\n"
	if got, n := Endnotes(body, marks(2, 3)); n != 0 || got != body {
		t.Errorf("Endnotes() = %q, %d, want it left alone", got, n)
	}
}

// A digit after the number is not a separator, so a paragraph that opens
// with a year is not note 197.
func TestAYearAtTheHeadOfAParagraphIsNotANoteNumber(t *testing.T) {
	body := "1975 was the year the invented paper appeared.\n"
	if got, n := Endnotes(body, marks(1)); n != 0 || got != body {
		t.Errorf("Endnotes() = %q, %d, want it left alone", got, n)
	}
}

func TestMarkersReadsTheProseOfTheWholePaper(t *testing.T) {
	ps := []assemble.Paragraph{
		{Text: "The first section, which marks a note.[^1]"},
		{Text: "The second, which marks two more.[^2] And again.[^13]"},
		{Text: "The third, which marks none."},
	}
	got := Markers(ps)
	for _, n := range []int{1, 2, 13} {
		if !got[n] {
			t.Errorf("marker %d was not found in %v", n, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("Markers() = %v, want three of them", got)
	}
}

// The whole of it, through the splitter: a paper with a marker in its prose
// and a notes section at the back comes out with the two joined up.
func TestASplitJoinsAMarkerToItsNote(t *testing.T) {
	d := &assemble.Document{Paragraphs: []assemble.Paragraph{
		{Text: "1. Introduction"},
		{Text: "The invented paper opens by marking a note.[^1]"},
		{Text: "Notes"},
		{Text: "1 The note the introduction marks."},
		{Text: "2 A second note that nothing marks."},
	}}
	r := Split(d)
	var notes *Section
	for i := range r.Sections {
		if r.Sections[i].Title == "Notes" {
			notes = &r.Sections[i]
		}
	}
	if notes == nil {
		t.Fatalf("the split found no notes section: %v", r.Sections)
	}
	if !strings.Contains(notes.Body, "[^1]: The note the introduction marks.") {
		t.Errorf("the notes section reads %q", notes.Body)
	}
	if !strings.Contains(notes.Body, "[^2]: A second note that nothing marks.") {
		t.Errorf("the second note was left behind: %q", notes.Body)
	}
}
