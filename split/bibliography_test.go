package split

import (
	"strings"
	"testing"
)

// Three entries typeset here rather than taken off any paper, like every
// other fixture in this package.
const (
	entry1 = "[1] A. Adams and B. Brown. A paper about one thing. *A Journal*, 1:1-10, 1970."
	entry2 = "[2] C. Clark. A paper about another thing. In *Proceedings of a Conference*, pages 20-30, 1975."
	entry3 = "[3] D. Davis and E. Evans. A third paper. *Another Journal*, 3(2):40-50, 1980."
)

// supplied says whether the split put a heading over a bibliography that had
// none, which is the thing every test here is about.
func supplied(r *Result) bool {
	for _, n := range r.Notes {
		if strings.Contains(n, "bibliography") {
			return true
		}
	}
	return false
}

// The page with REFERENCES printed on it is one of the pages the Razborov
// extraction never got back, so the document runs from the last sentence of
// the conclusion straight into the entries and the whole bibliography was
// filed as the second half of the conclusion.
func TestABibliographyWithNoHeadingGetsOne(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Conclusion", prose,
		entry1, entry2, entry3,
	))
	s := section(t, r, "04_references.md")
	if s.Kind != KindReferences {
		t.Errorf("the section is kind %q, want %q", s.Kind, KindReferences)
	}
	for _, e := range []string{entry1, entry2, entry3} {
		if !strings.Contains(s.Body, e) {
			t.Errorf("the entry is not in the section:\n%s", s.Body)
		}
	}
	if s := section(t, r, "03_conclusion.md"); strings.Contains(s.Body, entry1) {
		t.Errorf("the conclusion kept the bibliography:\n%s", s.Body)
	}
	if !supplied(r) {
		t.Errorf("the split said nothing about it: %v", r.Notes)
	}
}

// The heading is supplied where the entries begin and nowhere else, so the
// pages of the section are the pages the entries came off.
func TestTheSuppliedHeadingIsOnThePageTheEntriesAreOn(t *testing.T) {
	d := doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Conclusion", prose,
		entry1, entry2, entry3,
	)
	for i := len(d.Paragraphs) - 3; i < len(d.Paragraphs); i++ {
		d.Paragraphs[i].Page = 7
	}
	s := section(t, Split(d), "04_references.md")
	if s.First != 7 || s.Last != 7 {
		t.Errorf("the references are pages %d to %d, want 7 to 7", s.First, s.Last)
	}
}

// A paper that heads its own bibliography is left alone, whatever it calls
// it. Otherwise the heading goes in under the heading the paper already has
// and the paper gets an empty section.
func TestAHeadedBibliographyIsLeftAlone(t *testing.T) {
	for _, head := range []string{"References", "REFERENCES", "# Bibliography", "4 References", "Literature Cited"} {
		r := Split(doc(
			"A Paper About Something",
			"1 Introduction", prose,
			"2 Method", prose,
			"3 Conclusion", prose,
			head, entry1, entry2, entry3,
		))
		if supplied(r) {
			t.Errorf("%q had a heading supplied over it: %v", head, r.Notes)
		}
	}
}

// A numbered list is not a bibliography. The Jacobson paper ends with seven
// numbered notes and every one of them starts with a number counting up, so
// the entries have to be labelled the way a bibliography labels them.
func TestANumberedListIsNotABibliography(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Notes", prose,
		"1. The first note, which says something about 1970 and the first section.",
		"2. The second note, which says something about 1975 and the second.",
		"3. The third note, which says something about 1980 and the third.",
	))
	if supplied(r) {
		t.Errorf("a numbered list was read as a bibliography: %v", r.Notes)
	}
}

// A count that does not carry on is not a bibliography either.
func TestLabelsThatDoNotCountUpAreNotABibliography(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Conclusion", prose,
		entry3, entry2, entry1,
	))
	if supplied(r) {
		t.Errorf("labels counting down were read as a bibliography: %v", r.Notes)
	}
}

// A labelled line with no year in it is a list of something else.
func TestLabelledLinesWithNoYearAreNotABibliography(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Conclusion", prose,
		"[1] the first of three cases the proof goes through",
		"[2] the second of three cases the proof goes through",
		"[3] the third of three cases the proof goes through",
	))
	if supplied(r) {
		t.Errorf("a labelled list of cases was read as a bibliography: %v", r.Notes)
	}
}

// The heading goes in unmarked. Written as an ATX heading it was the only one
// in a document that had none, which changed how every other heading in the
// paper was read: the Razborov corollary that is set on a line of its own
// stopped being a heading and the paper came back a section short.
func TestSupplyingTheHeadingLeavesTheOtherHeadingsAlone(t *testing.T) {
	texts := []string{
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Conclusion", prose,
	}
	before := names(Split(doc(texts...)))
	after := names(Split(doc(append(texts, entry1, entry2, entry3)...)))
	if len(after) != len(before)+1 {
		t.Fatalf("the split went from %v to %v", before, after)
	}
	if !equal(before, after[:len(before)]) {
		t.Errorf("the sections changed: %v became %v", before, after)
	}
}
