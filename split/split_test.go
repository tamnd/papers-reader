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

// Only the first heading is offered the comparison, so a title printed again
// in the middle of the paper is still a cut when the sections are made. It
// has nothing under it, because the section it appears to head has a heading
// of its own on the next line, and a repeat of the paper's own name with
// nothing under it is a running head. It goes, and the section it was
// printed over is still a section.
func TestATitlePrintedAgainDoesNotSwallowASection(t *testing.T) {
	r := Titled(doc(
		"# A Study of Something",
		"Alice Adams",
		"# 1 Introduction", prose,
		"# A Study of Something",
		"# 2 Background", prose,
		"# 3 Results", prose,
	), "A Study of Something")
	want := []string{"00_front.md", "01_introduction.md", "02_background.md", "03_results.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("split into %v, want %v", got, want)
	}
	for _, s := range r.Sections[1:] {
		if strings.Contains(s.Body, "A Study of Something") {
			t.Errorf("the running head was published in %s", s.Title)
		}
	}
}

// A heading with nothing under it is a heading of what comes after it, not of
// what came before, so it goes to the top of the next section and not the
// bottom of the last one. An appendix tacked onto the end of a conclusion
// reads as part of the conclusion.
func TestAHeadingWithNothingUnderItGoesToTheSectionBelow(t *testing.T) {
	r := Split(doc(
		"# Introduction", prose,
		"# Conclusion", prose,
		"# Appendix",
		"# Proof of the Theorem", prose,
	))
	want := []string{"01_introduction.md", "02_conclusion.md", "03_proof_of_the_theorem.md"}
	if got := names(r); !equal(got, want) {
		t.Fatalf("split into %v, want %v", got, want)
	}
	last := r.Sections[len(r.Sections)-1]
	if !strings.HasPrefix(last.Body, "### Appendix\n\n") {
		t.Errorf("the proof opens %q, want the appendix heading over it", first(last.Body))
	}
	if strings.Contains(r.Sections[1].Body, "Appendix") {
		t.Errorf("the appendix heading was left on the end of the conclusion")
	}
}

// A run of them all lands in the same place, in the order the paper printed
// them. Sketchpad's appendix G lists the features of TX-2 and the reader set
// every item in the list as a heading of its own.
func TestARunOfEmptyHeadingsKeepsItsOrder(t *testing.T) {
	r := Split(doc(
		"# Introduction", prose,
		"# Sixty four index registers",
		"# Deferred addressing",
		"# Magnetic tape", prose,
	))
	want := []string{"01_introduction.md", "02_magnetic_tape.md"}
	if got := names(r); !equal(got, want) {
		t.Fatalf("split into %v, want %v", got, want)
	}
	body := r.Sections[1].Body
	at, to := strings.Index(body, "Sixty four"), strings.Index(body, "Deferred")
	if at < 0 || to < 0 || at > to {
		t.Errorf("the two headings are at %d and %d in %q", at, to, first(body))
	}
}

// The numbers the files take are given out after the fold, because a section
// that has gone takes its number with it and a run of files numbered 1, 3, 4
// is rule T04's hole in the numbering.
func TestTheFoldDoesNotLeaveAHoleInTheNumbering(t *testing.T) {
	r := Split(doc(
		"# Introduction", prose,
		"# Method",
		"# Magnetic tape", prose,
		"# Conclusion", prose,
	))
	want := []string{"01_introduction.md", "02_magnetic_tape.md", "03_conclusion.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("split into %v, want %v", got, want)
	}
}

// The last heading in the paper has nothing to fold into, and dropping it
// would lose the only record that the paper has a heading there.
func TestAnEmptyHeadingAtTheEndStaysWhereItIs(t *testing.T) {
	r := Split(doc(
		"# Introduction", prose,
		"# Conclusion", prose,
		"# Acknowledgments",
	))
	want := []string{"01_introduction.md", "02_conclusion.md", "03_acknowledgments.md"}
	if got := names(r); !equal(got, want) {
		t.Errorf("split into %v, want %v", got, want)
	}
}

// first is the opening of a body, for an error message that has to fit on a
// line.
func first(body string) string {
	if line, _, ok := strings.Cut(body, "\n"); ok {
		return line
	}
	return body
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

// Paxos printed its folio on six pages out of thirty, which is too few for
// the extractor's furniture detector to learn, so the numbers arrived in the
// body as paragraphs of their own and rule T10 refused the file.
func TestABarePageNumberIsNotPartOfTheSection(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"9",
		prose,
		"2 Method", prose,
		"- 11 -",
		prose,
		"3 Results", prose,
	))
	for _, name := range []string{"01_introduction.md", "02_method.md"} {
		s := section(t, r, name)
		for _, line := range strings.Split(s.Body, "\n") {
			if Folio.MatchString(line) && strings.TrimSpace(line) != "" {
				t.Errorf("%s keeps the page number %q:\n%s", name, strings.TrimSpace(line), s.Body)
			}
		}
		if !strings.Contains(s.Body, "This is a paragraph") {
			t.Errorf("%s lost its prose:\n%s", name, s.Body)
		}
	}
}

// A heading that reads as a number is a heading and stays. Shannon numbers
// his appendices and nothing else, so appendix 5 is a section called 5.
func TestAHeadingThatIsANumberIsNotAPageNumber(t *testing.T) {
	r := Split(doc(
		"A Paper About Something",
		"1 Introduction", prose,
		"1.1 A Subsection", prose,
		"2 Method", prose,
		"3 Results", prose,
	))
	intro := section(t, r, "01_introduction.md")
	if !strings.Contains(intro.Body, "### 1.1 A Subsection") {
		t.Errorf("the subheading went with the page numbers:\n%s", intro.Body)
	}
}

// The extractor read the running head and the folio as one paragraph of two
// lines, because they print on the same band of the page. Three sections of
// Paxos carried a page number that nothing in the toolchain could take out.
func TestARunningHeadCarryingAPageNumberIsNotPartOfTheSection(t *testing.T) {
	d := doc(
		"# 1. Ballots", prose,
		"The Part-Time Parliament ·\n11",
		prose,
		"# 2. Quorums", prose,
		"# 3. Decrees", prose,
	)
	r := Split(d)
	body := section(t, r, "01_ballots.md").Body
	if strings.Contains(body, "11") || strings.Contains(body, "Part-Time Parliament") {
		t.Errorf("the running head and its page number are still in the section:\n%s", body)
	}
	if !strings.Contains(body, prose) {
		t.Errorf("the prose around it is gone:\n%s", body)
	}
}

// A paragraph of real prose that swept a folio up keeps the prose. Only the
// number goes, because the rest of it is what the paper says.
func TestAParagraphOfProseThatSweptUpAFolioKeepsTheProse(t *testing.T) {
	d := doc(
		"# 1. Ballots", prose+"\n7",
		"# 2. Quorums", prose,
		"# 3. Decrees", prose,
	)
	body := section(t, Split(d), "01_ballots.md").Body
	if !strings.Contains(body, prose) {
		t.Errorf("the prose is gone:\n%s", body)
	}
	for _, l := range strings.Split(body, "\n") {
		if Folio.MatchString(l) {
			t.Errorf("%q is still a page number on a line of its own", l)
		}
	}
}

// A one column table of numbers is not a run of page numbers, and a table
// with its rows taken out no longer says what the paper said.
func TestARowOfATableIsNotAPageNumber(t *testing.T) {
	rows := "| Packets |\n| --- |\n| 400 |\n| 512 |"
	d := doc("# 1. Performance", rows, "# 2. Results", prose, "# 3. Conclusion", prose)
	body := section(t, Split(d), "01_performance.md").Body
	if !strings.Contains(body, "400") || !strings.Contains(body, "512") {
		t.Errorf("the table lost a row:\n%s", body)
	}
}

// A journal prints its own name over the paper and a reader transcribes it
// the way it transcribes everything else set large, so the AlphaGo paper
// opens with the word Article and only then gives the title. The masthead
// is not section one, and the title under it is still the title.
func TestAMastheadOverTheTitleIsStillFrontMatter(t *testing.T) {
	d := doc(
		"# ARTICLE",
		"doi:10.1000/nature00000",
		"# Mastering the game of Go",
		"David Silver, Aja Huang",
		"# 1 Introduction",
		"The game of Go has long been viewed as the hardest classic game.",
		"# 2 Supervised learning of policy networks",
		"The first stage of the training pipeline.",
		"# 3 Reinforcement learning of value networks",
		"The final stage of the training pipeline.",
	)
	r := Titled(d, "Mastering the game of Go")
	if len(r.Sections) != 4 {
		t.Fatalf("got %d sections, want the front matter and three sections", len(r.Sections))
	}
	if r.Sections[0].Kind != KindFront {
		t.Errorf("the first section is %s, want the front matter", r.Sections[0].Kind)
	}
	for _, want := range []string{"ARTICLE", "Mastering the game of Go", "David Silver"} {
		if !strings.Contains(r.Sections[0].Body, want) {
			t.Errorf("the front matter does not carry %q:\n%s", want, r.Sections[0].Body)
		}
	}
	if r.Sections[1].Title != "Introduction" {
		t.Errorf("the first real section is %q, want the introduction", r.Sections[1].Title)
	}
}

// A paper whose own section one is named after the paper keeps it. The
// search over the mastheads stops at the first heading with a number,
// because a masthead does not have one.
func TestANumberedSectionIsNotSwallowedByTheTitleSearch(t *testing.T) {
	d := doc(
		"The Part Time Parliament",
		"Leslie Lamport",
		"# 1 The Part Time Parliament",
		"Recent archaeological discoveries on the island of Paxos.",
		"# 2 The Single Decree Synod",
		"The Paxons first devised a protocol for choosing one decree.",
		"# 3 The Multi Decree Parliament",
		"The parliamentary protocol chooses a sequence of decrees.",
	)
	r := Titled(d, "The Part Time Parliament")
	if len(r.Sections) != 4 {
		t.Fatalf("got %d sections, want the front matter and three sections", len(r.Sections))
	}
	if r.Sections[1].Number != "1" {
		t.Errorf("section one came out as %q, want it kept", r.Sections[1].Number)
	}
}

// A paper the reader gave no headings at all has no appendices in it. With
// no last heading of the body to start after, the lettered appendix scan
// used to begin at the first paragraph and read the RSA paper's own title,
// "A Method for Obtaining Digital Signatures", as appendix A.
func TestAPaperWithNoHeadingsHasNoAppendices(t *testing.T) {
	d := doc(
		"A Method for Obtaining Digital Signatures and Public-Key Cryptosystems",
		"R.L. Rivest, A. Shamir, and L. Adleman",
		"An encryption method is presented with a novel property.",
		"B is another paragraph that opens with a capital and a space.",
		"C is a third one, which is what made a chain of three.",
	)
	r := Titled(d, "A Method for Obtaining Digital Signatures and Public-Key Cryptosystems")
	if len(r.Sections) != 1 {
		t.Fatalf("got %d sections, want the front matter on its own", len(r.Sections))
	}
	if r.Sections[0].Kind != KindFront {
		t.Errorf("the only section is %s, want the front matter", r.Sections[0].Kind)
	}
}
