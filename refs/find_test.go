package refs

import (
	"testing"

	"github.com/tamnd/papers-reader/assemble"
)

// Every fixture in this package is invented. A bibliography is somebody's
// work like the rest of the paper is, and a test file is the last place to
// put one, so the references below name papers that do not exist and were
// written to have the shape of the real thing and none of its text.

func doc(texts ...string) *assemble.Document {
	d := &assemble.Document{First: 1, Last: 1}
	for _, t := range texts {
		d.Paragraphs = append(d.Paragraphs, assemble.Paragraph{Text: t, Page: 1, Pages: 1})
	}
	return d
}

func got(ps []assemble.Paragraph) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Text
	}
	return out
}

func TestTheBibliographyIsFoundByItsHeading(t *testing.T) {
	for _, heading := range []string{
		"References", "REFERENCES", "Bibliography", "Literature Cited",
		"Works Cited", "6 References", "6. References", "§7 Bibliography",
		"VII. References",
	} {
		d := doc("the body of the paper.", heading, "[1] an entry.")
		section := Bibliography(d)
		if len(section) != 1 || section[0].Text != "[1] an entry." {
			t.Errorf("%q found %v", heading, got(section))
		}
	}
}

func TestAPaperWithNoBibliographyHasNone(t *testing.T) {
	if section := Bibliography(doc("the body of the paper.")); section != nil {
		t.Errorf("found %v", got(section))
	}
	if section := Bibliography(nil); section != nil {
		t.Errorf("found %v in nothing at all", got(section))
	}
}

func TestTheWordReferencesInTheProseIsNotTheHeading(t *testing.T) {
	// The introduction of a paper very often says "see the references at the
	// end", and taking the first match would file the whole paper as a
	// bibliography.
	d := doc(
		"We survey the field and give the references in full at the end of this paper, which is where a reader should start.",
		"References",
		"[1] an entry.",
	)
	section := Bibliography(d)
	if len(section) != 1 {
		t.Fatalf("found %v", got(section))
	}
}

func TestTheAppendixAfterTheBibliographyIsNotPartOfIt(t *testing.T) {
	d := doc(
		"References",
		"[1] an entry.",
		"[2] another entry.",
		"Appendix A: The Proof",
		"The proof runs to four pages and nobody reads it.",
	)
	section := Bibliography(d)
	if len(section) != 2 {
		t.Fatalf("found %v", got(section))
	}
}

func TestTheAcknowledgementsAfterTheBibliographyAreNotPartOfIt(t *testing.T) {
	d := doc(
		"References",
		"[1] an entry.",
		"Acknowledgements",
		"We thank the reviewers.",
	)
	if section := Bibliography(d); len(section) != 1 {
		t.Fatalf("found %v", got(section))
	}
}

func TestAHeadingWithNothingUnderItIsNotABibliography(t *testing.T) {
	if section := Bibliography(doc("the body.", "References")); section != nil {
		t.Errorf("found %v", got(section))
	}
}

func TestALongParagraphIsNeverAHeading(t *testing.T) {
	long := "References to the literature are collected at the end of the paper in the usual way and are numbered in order of first appearance."
	if section := Bibliography(doc(long, "[1] an entry.")); section != nil {
		t.Errorf("found %v", got(section))
	}
}
