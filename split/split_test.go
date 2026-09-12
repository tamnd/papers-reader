package split

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/assemble"
)

// doc makes a document whose paragraphs are all on page one unless a test
// says otherwise.
func doc(texts ...string) *assemble.Document {
	d := &assemble.Document{First: 1, Last: 1}
	for _, t := range texts {
		d.Paragraphs = append(d.Paragraphs, assemble.Paragraph{Text: t, Page: 1, Pages: 1})
	}
	return d
}

func TestWhatComesBeforeTheFirstHeadingIsTheFrontMatter(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"Ada Lovelace, Grace Hopper",
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
	))
	if len(r.Sections) != 4 {
		t.Fatalf("split into %d sections, want 4: %v", len(r.Sections), names(r))
	}
	front := r.Sections[0]
	if front.Kind != KindFront {
		t.Errorf("the first section is %q, want %q", front.Kind, KindFront)
	}
	if front.Filename() != "00_front.md" {
		t.Errorf("the front matter is filed as %q", front.Filename())
	}
	if !strings.Contains(front.Body, "Ada Lovelace") {
		t.Errorf("the front matter reads %q", front.Body)
	}
}

func TestASectionIsFiledByItsPositionAndItsTitle(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"2 Background", prose,
		"3 Model Architecture", prose,
	))
	want := []string{"00_front.md", "01_introduction.md", "02_background.md", "03_model_architecture.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("the files are %v, want %v", got, want)
	}
}

func TestAPaperThatOpensWithItsFirstHeadingHasNoFrontFile(t *testing.T) {
	r := Split(doc(
		"1 Introduction", prose,
		"2 Background", prose,
		"3 Results", prose,
	))
	want := []string{"01_introduction.md", "02_background.md", "03_results.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("the files are %v, want %v", got, want)
	}
}

func TestASectionKeepsItsOwnNumberAndNotItsPosition(t *testing.T) {
	// The abstract is unnumbered and comes first, so section 1 of the paper
	// is the second file. The file is numbered by position and the front
	// matter records the number the paper printed.
	r := Split(doc(
		"Abstract", prose,
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
	))
	if len(r.Sections) != 4 {
		t.Fatalf("split into %d sections: %v", len(r.Sections), names(r))
	}
	s := r.Sections[1]
	if s.Number != "1" || s.Ordinal != 1 {
		t.Errorf("the introduction is section %q at ordinal %d, want 1 and 1", s.Number, s.Ordinal)
	}
	if r.Sections[0].Number != "" {
		t.Errorf("the abstract is numbered %q, want nothing", r.Sections[0].Number)
	}
}

func TestTheAbstractIsPartOfTheFrontMatter(t *testing.T) {
	// 00_front.md is the title, the authors and the abstract, whether or not
	// the paper headed the abstract as a section of its own.
	r := Split(doc(
		"A Paper About Something",
		"Abstract", prose,
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
	))
	want := []string{"00_front.md", "01_introduction.md", "02_method.md", "03_results.md"}
	if got := names(r); !equal(got, want) {
		t.Fatalf("the files are %v, want %v", got, want)
	}
	if !strings.Contains(r.Sections[0].Body, "Abstract") {
		t.Errorf("the abstract is not in the front matter: %q", r.Sections[0].Body)
	}
}

func TestAnUnnumberedSectionCountsOnFromTheOneBeforeIt(t *testing.T) {
	r := Split(doc(
		"1 Introduction", prose,
		"2 Method", prose,
		"3 Results", prose,
		"Acknowledgements", prose,
		"References", prose,
	))
	want := []string{"01_introduction.md", "02_method.md", "03_results.md", "04_acknowledgments.md", "05_references.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("the files are %v, want %v", got, want)
	}
}

func TestASubsectionStaysInsideItsSection(t *testing.T) {
	r := Split(doc(
		"1 Introduction", prose,
		"2 Background", prose,
		"3 Model Architecture", prose,
		"3.1 Encoder", prose,
		"3.2 Decoder", prose,
		"4 Results", prose,
	))
	if len(r.Sections) != 4 {
		t.Fatalf("split into %d sections, want 4: %v", len(r.Sections), names(r))
	}
	body := r.Sections[2].Body
	if !strings.Contains(body, "### 3.1 Encoder") || !strings.Contains(body, "### 3.2 Decoder") {
		t.Errorf("the subheadings are not set as headings: %q", body)
	}
}

func TestTheSectionsOwnHeadingIsNotInItsBody(t *testing.T) {
	r := Split(doc("1 Introduction", prose, "2 Method", prose, "3 Results", prose))
	if strings.Contains(r.Sections[0].Body, "1 Introduction") {
		t.Errorf("the heading is in the body as well as the front matter: %q", r.Sections[0].Body)
	}
	if r.Sections[0].Title != "Introduction" {
		t.Errorf("the section is titled %q", r.Sections[0].Title)
	}
}

func TestASectionRecordsThePagesItCameOff(t *testing.T) {
	d := &assemble.Document{First: 3, Last: 7}
	add := func(text string, page, pages int) {
		d.Paragraphs = append(d.Paragraphs, assemble.Paragraph{Text: text, Page: page, Pages: pages})
	}
	add("1 Introduction", 3, 1)
	add(prose, 3, 1)
	add(prose, 4, 2)
	add("2 Method", 6, 1)
	add(prose, 6, 1)
	add("3 Results", 7, 1)
	add(prose, 7, 1)

	r := Split(d)
	if len(r.Sections) != 3 {
		t.Fatalf("split into %d sections: %v", len(r.Sections), names(r))
	}
	if r.Sections[0].First != 3 || r.Sections[0].Last != 5 {
		t.Errorf("the introduction is on pages %d to %d, want 3 to 5", r.Sections[0].First, r.Sections[0].Last)
	}
	if r.Sections[2].First != 7 || r.Sections[2].Last != 7 {
		t.Errorf("the results are on pages %d to %d, want 7 to 7", r.Sections[2].First, r.Sections[2].Last)
	}
}

func TestAPaperWithNoHeadingsIsOneFileAndSaysSo(t *testing.T) {
	r := Split(doc(prose, prose, prose))
	if len(r.Sections) != 1 {
		t.Fatalf("split into %d sections, want 1", len(r.Sections))
	}
	if len(r.Notes) == 0 {
		t.Error("a paper read as one section was not reported")
	}
}

func TestAHeadingFoundByItsTypographyIsReported(t *testing.T) {
	// SUMMARY is one of the forty names and is not reported. The other one is
	// a heading only because of how it is set, and a person should look at it.
	r := Split(doc(prose, "THE SINGLE PROCESSOR APPROACH", prose, "SUMMARY", prose))
	if len(r.Notes) != 1 {
		t.Fatalf("reported %d headings found by typography, want 1: %v", len(r.Notes), r.Notes)
	}
	if !strings.Contains(r.Notes[0], "Single Processor") {
		t.Errorf("the report reads %q", r.Notes[0])
	}
}

func TestALongTitleIsCutAtAWord(t *testing.T) {
	got := Slug("Representation of the Encoding and Decoding Operations")
	if len(got) > 40 {
		t.Errorf("the slug %q is %d characters", got, len(got))
	}
	if strings.HasSuffix(got, "_") || strings.Contains(got, "__") {
		t.Errorf("the slug %q is not cut at a word", got)
	}
}

func TestASlugIsLowerCaseAsciiAndUnderscores(t *testing.T) {
	for title, want := range map[string]string{
		"Model Architecture":   "model_architecture",
		"Why It Works (Maybe)": "why_it_works_maybe",
		"  Results  ":          "results",
		"Lovász's Bound":       "lov_sz_s_bound",
	} {
		if got := Slug(title); got != want {
			t.Errorf("Slug(%q) is %q, want %q", title, got, want)
		}
	}
}

func TestAnEmptyDocumentSplitsIntoNothingUseful(t *testing.T) {
	r := Split(&assemble.Document{})
	if len(r.Sections) != 1 || r.Sections[0].Body != "" {
		t.Errorf("an empty document split into %d sections", len(r.Sections))
	}
}

func names(r *Result) []string {
	out := make([]string, len(r.Sections))
	for i, s := range r.Sections {
		out[i] = s.Filename()
	}
	return out
}

// A vision model writes the title of the paper as a level one heading, and it
// is the title and not the first section. The GAN paper came back that way and
// 00_front.md kept only the line arXiv prints down the side of page 1.
func TestThePapersOwnTitleIsNotASection(t *testing.T) {
	r := Titled(doc(
		"arXiv:1000.00000v1 [cs.XX] 1 Jan 2000",
		"# A Study of Something",
		"Alice Adams, Bob Brown",
		"# Abstract",
		"We did a thing and it worked.",
		"# 1 Introduction", prose,
		"# 2 Background", prose,
		"# 3 Results", prose,
	), "A Study of Something")
	if len(r.Sections) != 4 {
		t.Fatalf("split into %d sections, want 4: %v", len(r.Sections), names(r))
	}
	if got := r.Sections[0].Kind; got != KindFront {
		t.Errorf("the first section is %q, want front", got)
	}
	for _, want := range []string{"A Study of Something", "Alice Adams", "We did a thing"} {
		if !strings.Contains(r.Sections[0].Body, want) {
			t.Errorf("the front matter does not have %q in it:\n%s", want, r.Sections[0].Body)
		}
	}
	if got := r.Sections[1].Title; got != "Introduction" {
		t.Errorf("the second section is %q, want Introduction", got)
	}
}

// Compared the way a reader compares them, because the page breaks the title
// over two lines and prints the colon where the record does not.
func TestTheTitleIsMatchedThroughCaseAndPunctuation(t *testing.T) {
	r := Titled(doc(
		"# MAPPING AND REDUCING: A SIMPLE MODEL",
		"Alice Adams",
		"# 1 Introduction", prose,
		"# 2 Background", prose,
		"# 3 Results", prose,
	), "Mapping and Reducing, a Simple Model")
	if got := r.Sections[0].Kind; got != KindFront {
		t.Fatalf("the first section is %q, want front: %v", got, names(r))
	}
}

// Only the first heading is offered the comparison. A paper that prints its
// title again in the middle is printing a running head, and the section it
// heads is still a section.
func TestATitlePrintedAgainDoesNotSwallowASection(t *testing.T) {
	r := Titled(doc(
		"# A Study of Something",
		"Alice Adams",
		"# 1 Introduction", prose,
		"# A Study of Something",
		"# 2 Background", prose,
		"# 3 Results", prose,
	), "A Study of Something")
	want := []string{"00_front.md", "01_introduction.md", "02_a_study_of_something.md", "03_background.md", "04_results.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("split into %v, want %v", got, want)
	}
}

// A paper with no title on record keeps every heading, because a comparison
// against nothing that matched would eat the first section.
func TestAPaperWithNoTitleOnRecordKeepsItsFirstSection(t *testing.T) {
	r := Split(doc(
		"# Introduction", prose,
		"# Background", prose,
	))
	want := []string{"01_introduction.md", "02_background.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("split into %v, want %v", got, want)
	}
}

// A model that writes its headings in Markdown hands the splitter a
// subheading that already carries a marker, and the splitter used to put
// another one in front of it. The ResNet paper came back from gpt-5 with
// eleven headings reading "### ## Something".
func TestASubheadingGetsOneMarkerAndNotTwo(t *testing.T) {
	r := Titled(doc(
		"# 1 Introduction", prose,
		"## 1.1 Background", prose,
		"# 2 Method", prose,
		"# 3 Results", prose,
	), "")
	s := section(t, r, "01_introduction.md")
	if strings.Contains(s.Body, "### ##") {
		t.Errorf("the subheading is written twice over:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "### 1.1 Background") {
		t.Errorf("the subheading is not in the section:\n%s", s.Body)
	}
}

// The other notation a model writes a heading in, which PR 57 taught the
// splitter to read and which has the same problem.
func TestABoldSubheadingLosesItsEmphasisWhenItBecomesAHeading(t *testing.T) {
	r := Titled(doc(
		"# 1 Introduction", prose,
		"**1.1 Background**", prose,
		"# 2 Method", prose,
		"# 3 Results", prose,
	), "")
	s := section(t, r, "01_introduction.md")
	if strings.Contains(s.Body, "**") {
		t.Errorf("the emphasis is still on the heading:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "### 1.1 Background") {
		t.Errorf("the subheading is not in the section:\n%s", s.Body)
	}
}

// A paragraph that only looks like a heading keeps every character it came
// with, because taking a marker off something that is not a heading loses
// what the paper said.
func TestAParagraphThatIsNotAHeadingIsNotTouched(t *testing.T) {
	const emphasised = "*Deeper neural networks are more difficult to train.* We say so below."
	r := Titled(doc(
		"# 1 Introduction", emphasised,
		"# 2 Method", prose,
		"# 3 Results", prose,
	), "")
	s := section(t, r, "01_introduction.md")
	if !strings.Contains(s.Body, emphasised) {
		t.Errorf("the paragraph was rewritten:\n%s", s.Body)
	}
}

func section(t *testing.T, r *Result, filename string) Section {
	t.Helper()
	for _, s := range r.Sections {
		if s.Filename() == filename {
			return s
		}
	}
	t.Fatalf("there is no %s in %v", filename, names(r))
	return Section{}
}

// The title of a paper is in the front matter as title, and a file that also
// carried it as a level one heading put 00_front.md a level above every
// section it introduces. Rule T05 found the ResNet paper that way: a level
// three Abstract under a level one title.
func TestTheFrontMatterHasNoHeadingOverIt(t *testing.T) {
	r := Titled(doc(
		"arXiv:1000.00000v1 [cs.XX] 1 Jan 2000",
		"# A Study of Something",
		"Alice Adams, Bob Brown",
		"# Abstract",
		"We did a thing and it worked.",
		"# 1 Introduction", prose,
		"# 2 Method", prose,
		"# 3 Results", prose,
	), "A Study of Something")
	front := section(t, r, "00_front.md")
	if strings.Contains(front.Body, "#") {
		t.Errorf("the front matter carries a heading marker:\n%s", front.Body)
	}
	for _, want := range []string{"A Study of Something", "Alice Adams", "Abstract", "it worked"} {
		if !strings.Contains(front.Body, want) {
			t.Errorf("the front matter lost %q:\n%s", want, front.Body)
		}
	}
}

// The marker comes off the two headings the front matter is made of and off
// nothing else, so a subheading inside a section still gets one.
func TestOnlyTheFrontMattersOwnHeadingsGoBare(t *testing.T) {
	r := Titled(doc(
		"# A Study of Something",
		"# Abstract",
		"We did a thing and it worked.",
		"# 1 Introduction", prose,
		"## 1.1 Background", prose,
		"# 2 Method", prose,
		"# 3 Results", prose,
	), "A Study of Something")
	if front := section(t, r, "00_front.md"); strings.Contains(front.Body, "#") {
		t.Errorf("the front matter carries a heading marker:\n%s", front.Body)
	}
	intro := section(t, r, "01_introduction.md")
	if !strings.Contains(intro.Body, "### 1.1 Background") {
		t.Errorf("the subheading lost its marker too:\n%s", intro.Body)
	}
}
