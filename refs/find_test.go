package refs

import (
	"strings"
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

// The last page of a two column paper is where the reading order goes wrong.
// The heading and the first entry are at the foot of the left column and the
// rest carry on at the top of the right one, so a reader that takes the right
// column first files most of the bibliography as the end of the appendix.
func TestEntriesTheReadingOrderScatteredAreGatheredBack(t *testing.T) {
	d := doc(
		"APPENDIX",
		"[2] B. Boehm, Software and its impact, 1973.",
		"[3] W. Cammack, Improving the programming process, 1973.",
		"[4] D. Knuth, Structured programming, 1974.",
		"the complexity would be computed as follows.",
		"REFERENCES",
		"[1] C. Berge, Graphs and Hypergraphs, 1973.",
	)
	got := Bibliography(d)
	if len(got) != 4 {
		t.Fatalf("the bibliography has %d paragraphs, want 4: %v", len(got), got)
	}
	for i, want := range []string{"[1]", "[2]", "[3]", "[4]"} {
		if !strings.HasPrefix(got[i].Text, want) {
			t.Errorf("paragraph %d is %q, want it to start %s", i, got[i].Text, want)
		}
	}
}

// A numbered list in the body of a paper is common. One that happens to
// continue the bibliography's numbering from exactly where it stopped is not,
// and nothing short of that is moved.
func TestANumberedListInTheBodyIsNotGathered(t *testing.T) {
	d := doc(
		"the three cases are",
		"[1] the loop is never entered,",
		"[2] the loop runs once,",
		"REFERENCES",
		"[1] C. Berge, Graphs and Hypergraphs, 1973.",
		"[2] B. Boehm, Software and its impact, 1973.",
	)
	if got := Bibliography(d); len(got) != 2 {
		t.Errorf("the bibliography has %d paragraphs, want 2: %v", len(got), got)
	}
}

// The journals of the nineteen seventies print the author biographies straight
// after the references with no heading over them.
func TestAnAuthorBiographyEndsTheBibliography(t *testing.T) {
	d := doc(
		"REFERENCES",
		"[1] C. Berge, Graphs and Hypergraphs, 1973.",
		"Thomas J. McCabe was born in Central Falls, RI, on November 28, 1941.",
		"He has been employed since 1966 by the Department of Defense.",
	)
	got := Bibliography(d)
	if len(got) != 1 {
		t.Fatalf("the bibliography has %d paragraphs, want 1: %v", len(got), got)
	}
}

// A membership grade sits between the name and the phrase, and two authors
// sharing a paragraph are born in the plural.
func TestABiographyIsRecognisedWithAGradeOrTwoAuthors(t *testing.T) {
	for _, text := range []string{
		"Robert H. Dennard (M'65) was born in Terrell, Tex., in 1932.",
		"Alice Smith and Bob Jones were born in Leeds.",
	} {
		if !isBiography(text) {
			t.Errorf("isBiography(%q) = false, want true", text)
		}
	}
	// A reference is not a biography however the title reads.
	if isBiography("[4] D. E. Knuth, \"How the structured program was born,\" 1974.") {
		t.Error("a reference was read as a biography")
	}
}
