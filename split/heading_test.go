package split

import "testing"

// prose is a paragraph long enough that nothing mistakes it for a heading. No
// paper is quoted anywhere in these tests: every fixture is typeset here.
const prose = "This is a paragraph of the body text of a paper, long enough that " +
	"no detector reads it as a heading and plain enough that it says nothing " +
	"about anything. It runs past two hundred characters so that the last of " +
	"the three detectors has something to stand on."

func TestANumberedPaperIsDetectedAsNumbered(t *testing.T) {
	s, hs := Headings([]string{
		"1 Introduction", prose,
		"2 Background", prose,
		"3 Model Architecture", prose,
		"3.1 Encoder", prose,
		"4 Results", prose,
	})
	if s != SchemeArabic {
		t.Fatalf("the paper numbers its sections %q, want arabic", s)
	}
	if len(hs) != 5 {
		t.Fatalf("found %d headings, want 5: %v", len(hs), titles(hs))
	}
	if hs[3].Level != 2 || hs[3].Number != "3.1" {
		t.Errorf("3.1 Encoder came back at level %d numbered %q", hs[3].Level, hs[3].Number)
	}
	if hs[2].Title != "Model Architecture" {
		t.Errorf("the third heading is titled %q", hs[2].Title)
	}
}

func TestALineInsideAParagraphIsNotASection(t *testing.T) {
	// The reason the numbering is detected once and then required. Every one
	// of these lines starts with a digit and none of them is a heading.
	s, hs := Headings([]string{
		"1 Introduction", prose,
		"2 Background", prose,
		"3 Results", prose,
		"7 of the 12 runs converged",
		"1997 was the year the first of these was published",
		"4 Conclusion", prose,
	})
	if s != SchemeArabic {
		t.Fatalf("the paper numbers its sections %q, want arabic", s)
	}
	want := []string{"Introduction", "Background", "Results", "Conclusion"}
	if got := titles(hs); !equal(got, want) {
		t.Errorf("found %v, want %v", got, want)
	}
}

func TestASubsectionOfASectionNotReachedYetIsNotAHeading(t *testing.T) {
	// "3.2 is the rule" printed inside section 1 is a cross-reference.
	_, hs := Headings([]string{
		"1 Introduction", prose,
		"3.2 is where this is proved", prose,
		"2 Background", prose,
		"3 Results", prose,
	})
	for _, h := range hs {
		if h.Number == "3.2" {
			t.Errorf("a cross-reference to 3.2 was read as a heading")
		}
	}
}

func TestRomanNumeralsAreASchemeToo(t *testing.T) {
	s, hs := Headings([]string{
		"I. INTRODUCTION", prose,
		"II. RELATED WORK", prose,
		"III. THE ALGORITHM", prose,
		"IV. RESULTS", prose,
	})
	if s != SchemeRoman {
		t.Fatalf("the paper numbers its sections %q, want roman", s)
	}
	if len(hs) != 4 {
		t.Fatalf("found %d headings, want 4: %v", len(hs), titles(hs))
	}
	if hs[3].Number != "IV" {
		t.Errorf("the fourth section is numbered %q, want the numeral the paper printed", hs[3].Number)
	}
}

func TestTheSectionSignIsAScheme(t *testing.T) {
	s, hs := Headings([]string{
		"§1 The Setting", prose,
		"§2 The Consistency Condition", prose,
		"§3 Anomalous Behaviour", prose,
	})
	if s != SchemeSign {
		t.Fatalf("the paper numbers its sections %q, want the section sign", s)
	}
	if len(hs) != 3 {
		t.Errorf("found %d headings, want 3: %v", len(hs), titles(hs))
	}
}

func TestTwoNumberedLinesAreNotAScheme(t *testing.T) {
	// A paper with a numbered list in it and no numbered sections.
	if s := DetectScheme([]string{prose, "1 first of the two conditions", "2 second of them", prose}); s != SchemeNone {
		t.Errorf("two numbered lines were read as the scheme %q", s)
	}
}

func TestAnUnnumberedNamedHeadingIsFoundOnANumberedPaper(t *testing.T) {
	_, hs := Headings([]string{
		"Abstract", prose,
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
		"Acknowledgements", prose,
		"References", prose,
	})
	want := []string{"Abstract", "Introduction", "Method", "Results", "Acknowledgments", "References"}
	if got := titles(hs); !equal(got, want) {
		t.Fatalf("found %v, want %v", got, want)
	}
	if hs[4].How != Named {
		t.Errorf("the acknowledgements were found by %q, want the name", hs[4].How)
	}
}

func TestTheNamesAreFiledUnderOneSpelling(t *testing.T) {
	// A reader moving between two papers should not have to notice that one
	// wrote ACKNOWLEDGMENT and the other Acknowledgements.
	for _, printed := range []string{"ACKNOWLEDGMENT", "Acknowledgements", "acknowledgments"} {
		got, ok := named(printed)
		if !ok || got != "Acknowledgments" {
			t.Errorf("%q was filed as %q, %v", printed, got, ok)
		}
	}
}

func TestAHeadingSetInCapitalsIsFoundOnAPaperThatNumbersNothing(t *testing.T) {
	s, hs := Headings([]string{
		prose,
		"THE VALIDITY OF THE SINGLE PROCESSOR APPROACH", prose,
		"SUMMARY", prose,
	})
	if s != SchemeNone {
		t.Fatalf("the paper numbers its sections %q, want nothing", s)
	}
	if len(hs) != 2 {
		t.Fatalf("found %d headings, want 2: %v", len(hs), titles(hs))
	}
	if hs[0].How != Typographic {
		t.Errorf("the first heading was found by %q, want its typography", hs[0].How)
	}
	if hs[0].Title != "The Validity of the Single Processor Approach" {
		t.Errorf("the heading is titled %q", hs[0].Title)
	}
}

func TestTypographyIsNotTrustedOnAPaperThatNumbersItsSections(t *testing.T) {
	// A line of capitals in a numbered paper is a caption, a table header or
	// an emphasised phrase far more often than it is a section the paper
	// forgot to number.
	_, hs := Headings([]string{
		"1 Introduction", prose,
		"2 Background", prose,
		"NOTE THAT THIS HOLDS ONLY FOR THE SYMMETRIC CASE", prose,
		"3 Results", prose,
	})
	for _, h := range hs {
		if h.How == Typographic {
			t.Errorf("a line of capitals was read as a heading: %q", h.Title)
		}
	}
}

func TestTheBibliographyIsItsOwnKind(t *testing.T) {
	_, hs := Headings([]string{
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
		"Bibliography", prose,
	})
	last := hs[len(hs)-1]
	if last.Kind != KindReferences {
		t.Errorf("the bibliography came back as %q, want %q", last.Kind, KindReferences)
	}
}

func TestEverythingAfterTheFirstAppendixIsAnAppendix(t *testing.T) {
	_, hs := Headings([]string{
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
		"Appendix A: The Proof", prose,
		"Appendix B: The Tables", prose,
	})
	if len(hs) != 5 {
		t.Fatalf("found %d headings, want 5: %v", len(hs), titles(hs))
	}
	for _, h := range hs[3:] {
		if h.Kind != KindAppendix {
			t.Errorf("%q came back as %q, want %q", h.Title, h.Kind, KindAppendix)
		}
	}
	for _, h := range hs[:3] {
		if h.Kind != KindSection {
			t.Errorf("%q came back as %q, want %q", h.Title, h.Kind, KindSection)
		}
	}
}

func TestARomanNumeralThatIsNotOneIsNotASection(t *testing.T) {
	for _, s := range []string{"", "MCM", "Hello", "C", "LXXX"} {
		if n := romanValue(s); n != 0 {
			t.Errorf("romanValue(%q) is %d, want 0: no paper has that many sections", s, n)
		}
	}
	for s, want := range map[string]int{"I": 1, "IV": 4, "IX": 9, "XIV": 14, "XL": 40} {
		if n := romanValue(s); n != want {
			t.Errorf("romanValue(%q) is %d, want %d", s, n, want)
		}
	}
}

func titles(hs []Heading) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Title
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The layout path writes a heading as an ATX heading, because a layout model
// does not have to guess at its headings the way the native path does.

func TestAMarkedHeadingIsFoundWithoutANumberingScheme(t *testing.T) {
	paragraphs := []string{
		"# Introduction",
		"A paragraph of invented prose that runs on for a while.",
		"# The Method",
		"Another paragraph of invented prose.",
	}
	s, hs := Headings(paragraphs)
	if s != SchemeNone {
		t.Errorf("scheme is %q, want none", s)
	}
	if len(hs) != 2 {
		t.Fatalf("found %d headings, want 2: %+v", len(hs), hs)
	}
	for _, h := range hs {
		if h.How != Marked {
			t.Errorf("%q was found by %q, want %q", h.Title, h.How, Marked)
		}
	}
	if got, want := hs[0].Title, "Introduction"; got != want {
		t.Errorf("the first heading is %q, want %q", got, want)
	}
}

func TestAMarkedHeadingKeepsThePaperSOwnNumber(t *testing.T) {
	paragraphs := []string{
		"# 1 Introduction",
		"A paragraph of invented prose.",
		"# 2 Background",
		"Another paragraph.",
		"## 2.1 Notation",
		"A third paragraph.",
		"# 3 The Method",
		"A fourth paragraph.",
	}
	s, hs := Headings(paragraphs)
	if s != SchemeArabic {
		t.Fatalf("scheme is %q, want %q", s, SchemeArabic)
	}
	if len(hs) != 4 {
		t.Fatalf("found %d headings, want 4: %+v", len(hs), hs)
	}
	want := []struct {
		number string
		level  int
		title  string
	}{
		{"1", 1, "Introduction"},
		{"2", 1, "Background"},
		{"2.1", 2, "Notation"},
		{"3", 1, "The Method"},
	}
	for i, w := range want {
		if hs[i].Number != w.number || hs[i].Level != w.level || hs[i].Title != w.title {
			t.Errorf("heading %d is %+v, want %s %d %q", i, hs[i], w.number, w.level, w.title)
		}
		if hs[i].How != Marked {
			t.Errorf("heading %d was found by %q, want %q", i, hs[i].How, Marked)
		}
	}
}

func TestTheNumberingWinsOverTheLevelTheModelGave(t *testing.T) {
	// A model that labelled 3.2 a top level heading is wrong about the
	// structure of the paper in a way the number settles.
	paragraphs := []string{
		"# 1 Introduction",
		"A paragraph.",
		"# 2 Background",
		"A paragraph.",
		"# 3 The Method",
		"A paragraph.",
		"# 3.1 A Detail",
		"A paragraph.",
	}
	_, hs := Headings(paragraphs)
	if len(hs) != 4 {
		t.Fatalf("found %d headings, want 4: %+v", len(hs), hs)
	}
	if got, want := hs[3].Level, 2; got != want {
		t.Errorf("3.1 came out at level %d, want %d", got, want)
	}
}

func TestAMarkedHeadingThatBreaksTheNumberingIsStillAHeading(t *testing.T) {
	// The acknowledgements are unnumbered in a paper that numbers
	// everything else, and the numbered pass skips them by design.
	paragraphs := []string{
		"# 1 Introduction",
		"A paragraph.",
		"# 2 Background",
		"A paragraph.",
		"# 3 The Method",
		"A paragraph.",
		"# Acknowledgements",
		"A paragraph.",
		"# References",
		"A paragraph.",
	}
	_, hs := Headings(paragraphs)
	if len(hs) != 5 {
		t.Fatalf("found %d headings, want 5: %+v", len(hs), hs)
	}
	if got, want := hs[3].Title, "Acknowledgments"; got != want {
		t.Errorf("the heading is filed as %q, want %q", got, want)
	}
	if got, want := hs[4].Kind, KindReferences; got != want {
		t.Errorf("the references are kind %q, want %q", got, want)
	}
}

func TestAHashInProseIsNotAHeading(t *testing.T) {
	for _, text := range []string{
		"#define MAX 32",
		"# 12 is the issue number and this paragraph runs on\nacross two lines of prose.",
		"####### seven hashes are not a heading",
		"#",
	} {
		if _, _, ok := atxParts(text); ok {
			t.Errorf("%q was read as a heading", text)
		}
	}
}

func TestAMarkedHeadingIsTitleCasedWhenThePaperShouted(t *testing.T) {
	_, hs := Headings([]string{"# THE CONSISTENCY CONDITION", "A paragraph."})
	if len(hs) != 1 {
		t.Fatalf("found %d headings, want 1", len(hs))
	}
	if got, want := hs[0].Title, "The Consistency Condition"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
