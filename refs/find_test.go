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

func TestALongMarkedHeadingStillEndsTheBibliography(t *testing.T) {
	// A paper that heads its appendix with its own title, which is what BERT
	// does and what ran the parse on through the appendix: the heading is a
	// hundred and two characters long and the cap is sixty.
	long := "## Appendix for “A Very Long Title Repeated in Full at the Head of the Appendix of This Paper”"
	d := doc("the body of the paper.", "## References", "[1] an entry.", long, "the appendix.")

	section := Bibliography(d)
	if len(section) != 1 || section[0].Text != "[1] an entry." {
		t.Errorf("the bibliography is %v and the appendix is not part of it", got(section))
	}
}

func TestALongUnmarkedParagraphIsStillNotAHeading(t *testing.T) {
	// The cap is what stops a reference from being read as a heading, and a
	// paragraph with no marker on it is still held to it.
	entry := "[2] R. Q. Appendix and T. Author. 1994. A paper whose first author is unfortunately named. In Proceedings of Somewhere, pages 1 to 12."
	d := doc("the body of the paper.", "References", "[1] an entry.", entry)

	section := Bibliography(d)
	if len(section) != 2 {
		t.Errorf("the bibliography is %v and both entries belong to it", got(section))
	}
}
