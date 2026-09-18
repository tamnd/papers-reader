package audit

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// openSources is a source record for each paper in papersYAML, so that group
// T can be tested without every case also tripping S01.
const openSources = `sources:
  - id: vaswani-2017-attention
    access: open
    licence: arXiv non-exclusive licence to distribute
    pages: 15
  - id: codd-1970-relational
    access: open
    licence: all rights reserved
    pages: 11
`

// file writes one content file. The front matter is given as the lines
// between the fences so that a test can misspell a field or leave one out,
// and content_sha256 is filled in for the body unless the front matter sets
// it, because a test about section numbering should not have to hash
// anything.
func file(front, body string) string {
	if !strings.HasSuffix(front, "\n") {
		front += "\n"
	}
	if !strings.Contains(front, "content_sha256:") {
		front += "content_sha256: " + corpus.ContentSHA([]byte(body)) + "\n"
	}
	return "---\n" + front + "---\n\n" + body
}

// section is the front matter of an ordinary English section of the
// Transformer paper, which is the paper most of these tests use.
func section(kind string) string {
	return "paper: vaswani-2017-attention\ntitle: Attention Is All You Need\nkind: " + kind + "\nlang: en\n"
}

// abstract is a paragraph long enough to be one, for the tests that need a
// front file that satisfies T06 while they check something else.
const abstract = "The dominant sequence transduction models are based on complex recurrent or convolutional neural networks that include an encoder and a decoder, and the best of them connect the two through an attention mechanism, which is the part this paper keeps.\n"

func TestT01ReportsAFileThatDoesNotSplit(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": "no front matter at all\n",
	})
	res := result(t, Run(in, true), "T01")
	if len(res.Findings) != 1 {
		t.Fatalf("T01 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	// Every other rule has to skip the file rather than report its own
	// version of the same problem.
	for _, r := range Run(in, true).Results {
		if r.Rule.ID == "T01" || r.Rule.Group() != GroupStructure {
			continue
		}
		if r.Failed() {
			t.Errorf("%s also reported the unparseable file: %v", r.Rule.ID, r.Findings)
		}
	}
}

func TestT02ReportsAFieldNobodyReads(t *testing.T) {
	cases := []struct {
		name  string
		front string
		want  string
	}{
		{"a misspelled field", section("section") + "extration: native\n", "extration"},
		{"no paper", "title: Attention\nkind: section\nlang: en\n", "paper"},
		{"no title", "paper: vaswani-2017-attention\nkind: section\nlang: en\n", "title"},
		{"a kind nobody defined", section("chapter"), "chapter"},
		{"a language the corpus has no column for", "paper: vaswani-2017-attention\ntitle: A\nkind: section\nlang: de\n", "de"},
	}
	for _, tc := range cases {
		in := build(t, map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(tc.front, "a body long enough that T08 has nothing to say about it, which takes rather more words than it looks like it should.\n"),
		})
		res := result(t, Run(in, true), "T02")
		if !res.Failed() {
			t.Errorf("%s: T02 accepted it", tc.name)
			continue
		}
		if !strings.Contains(res.Findings[0].Message, tc.want) {
			t.Errorf("%s: T02 says %q, want it to name %q", tc.name, res.Findings[0].Message, tc.want)
		}
	}
}

func TestT02AcceptsTheFrontMatterTheToolchainWrites(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/sources.yaml": openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(
			section("front")+"authors:\n  - Ashish Vaswani\nyear: 2017\nvenue: NeurIPS\nfield: ai-ml\nsection_title: Front Matter\nextraction: native\nextraction_model: pdftotext\npdf_pages: \"1\"\nequations: 3\n", abstract),
	})
	if res := result(t, Run(in, true), "T02"); res.Failed() {
		t.Errorf("T02 refused front matter this program writes: %v", res.Findings)
	}
}

func TestT03NoticesABodyThatWasEdited(t *testing.T) {
	body := "a section of the paper, as extracted, long enough that no other rule has anything to say about it at all.\n"
	in := build(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(
			section("section")+"content_sha256: "+corpus.ContentSHA([]byte(body))+"\n",
			body+"and a sentence somebody added afterwards without rehashing.\n"),
	})
	res := result(t, Run(in, true), "T03")
	if len(res.Findings) != 1 {
		t.Fatalf("T03 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "content_sha256") {
		t.Errorf("T03 does not say which field disagrees: %s", res.Findings[0].Message)
	}
}

// The trailing newlines are not part of the body as far as the hash is
// concerned, because Render puts one there and a corpus where every file
// failed T03 over a newline would be a corpus nobody ran the audit on.
func TestT03IgnoresTrailingNewlines(t *testing.T) {
	body := "a section of the paper, as extracted, long enough that no other rule has anything to say about it at all."
	in := build(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(
			section("section")+"content_sha256: "+corpus.ContentSHA([]byte(body))+"\n", body+"\n\n\n"),
	})
	if res := result(t, Run(in, true), "T03"); res.Failed() {
		t.Errorf("T03 failed over trailing newlines: %v", res.Findings)
	}
}

func TestT04WantsTheSectionsContiguousFromZero(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		fails bool
	}{
		{"in order", []string{"00_front.md", "01_intro.md", "02_model.md"}, false},
		{"a hole in the middle", []string{"00_front.md", "01_intro.md", "03_model.md"}, true},
		{"no front file", []string{"01_intro.md", "02_model.md"}, true},
		{"the same number twice", []string{"00_front.md", "01_intro.md", "01_model.md"}, true},
		{"a name with no number", []string{"00_front.md", "notes.md"}, true},
	}
	for _, tc := range cases {
		files := map[string]string{"manifests/sources.yaml": openSources}
		for _, n := range tc.names {
			kind := "section"
			body := "a section body long enough that the length rules have nothing at all to say about it, which is two hundred characters and takes a surprising number of words to reach on purpose.\n"
			if strings.HasSuffix(n, "front.md") {
				kind, body = "front", abstract
			}
			files["content/en/vaswani-2017-attention/"+n] = file(section(kind), body)
		}
		res := result(t, Run(in(t, files), true), "T04")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T04 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// in is build under a shorter name, for the table tests that call it inside a
// loop and read better without the word "build" in the middle of a line.
func in(t *testing.T, files map[string]string) *Input {
	t.Helper()
	return build(t, files)
}

func TestT05WantsTheHeadingTreeWellFormed(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"one level at a time", "## Model\n\ntext\n\n### Encoder\n\ntext\n", false},
		{"back up several levels at once", "## Model\n\ntext\n\n#### Deep\n\ntext\n", true},
		{"down several levels at once is fine", "## Model\n\n### Encoder\n\n#### Layer\n\ntext\n\n## Training\n\ntext\n", false},
		{"a hash inside a code block", "## Model\n\n```sh\n#### not a heading\n```\n\ntext\n", false},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body+strings.Repeat("padding so the body is long enough. ", 8)+"\n"),
		}
		res := result(t, Run(in(t, files), true), "T05")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T05 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// T06 is the rule that tells a paper apart from a catalogue entry, and it is
// the one that caught three papers in this corpus whose front file was a
// cover page with no abstract behind it.
func TestT06WantsAnAbstractAndNotATitleBlock(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"an abstract", abstract, false},
		{"a cover page", "The Part-Time Parliament\n\nLeslie Lamport\n\nThis article appeared in ACM TOCS 16, 2.\n", true},
		{"a title and nothing else", "IP=PSPACE\n", true},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                        openSources,
			"content/en/vaswani-2017-attention/00_front.md": file(section("front"), tc.body),
		}
		res := result(t, Run(in(t, files), true), "T06")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T06 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
	if res := result(t, Run(in(t, map[string]string{"manifests/sources.yaml": openSources}), true), "T06"); !res.NotRun {
		t.Error("T06 passed a corpus with no content in it")
	}
}

// The message is read in a report by a person, so "1 word" and not "1 words".
func TestT06CountsInEnglish(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), "IP=PSPACE\n"),
	}
	res := result(t, Run(in(t, files), true), "T06")
	if !strings.Contains(res.Findings[0].Message, "is 1 word,") {
		t.Errorf("T06 says %q", res.Findings[0].Message)
	}
}

// The abstract is younger than the oldest papers here. McCarthy 1960 goes
// from the title straight into the introduction and there is no abstract in
// the PDF to find. Both it and Jacobson 1988 passed this rule while they
// were in the corpus as a front file holding the first pages whole, and
// failed it the day they were read in full and the introduction became a
// section of its own. A paper does not stop being a paper by being finished.
func TestT06LeavesAPaperWithABodyAlone(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), "Recursive Functions of Symbolic Expressions\n\nJohn McCarthy\n\nApril 1960\n"),
		"content/en/vaswani-2017-attention/01_intro.md": file(section("section"), strings.Repeat("A programming system called LISP has been developed for the IBM 704. ", 8)+"\n"),
	}
	if res := result(t, Run(in(t, files), true), "T06"); res.Failed() {
		t.Errorf("T06 refuses a paper that has no abstract to have: %v", res.Findings)
	}
}

// A language without spaces in it has words all the same. The Japanese
// front matter of the GAN paper failed this rule as a title block on the
// day it was written, with an abstract of eight sentences in it.
func TestT06CountsAParagraphWithNoSpacesInIt(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"a Japanese abstract", strings.Repeat("敵対的プロセスを介して生成モデルを推定する枠組みを提案する。", 3) + "\n", false},
		{"a Chinese abstract", strings.Repeat("我们提出了一个通过对抗过程估计生成模型的新框架。", 3) + "\n", false},
		{"a Japanese title", "生成的敵対ネットワーク\n", true},
		{"a Chinese title", "生成对抗网络\n", true},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                        openSources,
			"content/en/vaswani-2017-attention/00_front.md": file(section("front"), tc.body),
		}
		res := result(t, Run(in(t, files), true), "T06")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T06 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

func TestT07PutsTheBibliographyLast(t *testing.T) {
	long := strings.Repeat("a sentence of the paper. ", 12) + "\n"
	cases := []struct {
		name  string
		kinds map[string]string
		fails bool
	}{
		{"references last", map[string]string{"01_intro.md": "section", "02_references.md": "references"}, false},
		{"an appendix after them is allowed", map[string]string{"01_intro.md": "section", "02_references.md": "references", "03_appendix.md": "appendix"}, false},
		{"a section after them is not", map[string]string{"01_references.md": "references", "02_model.md": "section"}, true},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                        openSources,
			"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
		}
		for name, kind := range tc.kinds {
			files["content/en/vaswani-2017-attention/"+name] = file(section(kind), long)
		}
		res := result(t, Run(in(t, files), true), "T07")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T07 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

func TestT08AndT09OnLength(t *testing.T) {
	cases := []struct {
		name string
		body string
		rule string
	}{
		{"a stub", "Section 3.\n", "T08"},
		{"a file that swallowed the paper", strings.Repeat("x", maxBody+1) + "\n", "T09"},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body),
		}
		res := result(t, Run(in(t, files), false), tc.rule)
		if !res.Failed() {
			t.Errorf("%s: %s accepted it", tc.name, tc.rule)
		}
	}
}

// Section 3 of the GPT-3 paper is sixty three thousand characters and it is
// nineteen subsections of a results section that runs to twenty pages. The
// length is a symptom of a splitter that missed every heading, and a file
// with its subsections in it did not miss them.
func TestT09LeavesALongSectionWithSubsectionsAlone(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= minSubheads; i++ {
		fmt.Fprintf(&body, "### 3.%d A Subsection {#p-s3-%d .section tag=00%d}\n\n", i, i, i)
		body.WriteString(strings.Repeat("x", maxBody/2) + "\n\n")
	}
	files := map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), body.String()),
	}
	if res := result(t, Run(in(t, files), false), "T09"); res.Failed() {
		t.Errorf("T09 reported a long section that is split into subsections: %v", res.Findings)
	}
}

// The front file is the one file that is allowed to be short: it is a title
// block and an abstract, and T06 is what holds it to that.
func TestT08LeavesTheFrontFileAlone(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
	}
	if res := result(t, Run(in(t, files), false), "T08"); res.Failed() {
		t.Errorf("T08 objected to a front file: %v", res.Findings)
	}
}

func TestT10FindsPageFurniture(t *testing.T) {
	long := strings.Repeat("a sentence of the paper. ", 12)
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"clean", long + "\n", false},
		{"a bare folio", long + "\n\n12\n\n" + long + "\n", true},
		{"a folio in dashes", long + "\n\n- 12 -\n\n" + long + "\n", true},
		{"the word page and a number", long + "\n\nPage 12\n\n" + long + "\n", true},
		{"a year on its own is not a folio", long + "\n\n1998\n\n" + long + "\n", true},
		{"a numbered list item is not a folio", long + "\n\n1. the first point\n\n" + long + "\n", false},
		{"a display equation number is not a folio", long + "\n\n$$x = 1$$\n\n" + long + "\n", false},
		{"a one column table of bits is not fifteen folios", long + "\n\n| Table IV |\n| --- |\n| 0 |\n| 1 |\n| 1 |\n\n" + long + "\n", false},
		{"a folio under a table is still a folio", long + "\n\n| Table IV |\n| --- |\n| 0 |\n\n12\n\n" + long + "\n", true},
		{"a run of pipes with no divider is not a table", long + "\n\n| 12 |\n\n" + long + "\n", true},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body),
		}
		res := result(t, Run(in(t, files), true), "T10")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T10 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// S07 and S08 are the two licensing rules that read the content rather than
// the manifests, so they are tested here where the content helpers are.
func TestS07CapsWhatARestrictedPaperQuotes(t *testing.T) {
	restricted := `sources:
  - id: codd-1970-relational
    access: restricted
    landing: https://dl.acm.org/doi/10.1145/362384.362685
    pages: 11
`
	long := strings.Repeat("word ", 300)
	files := map[string]string{
		"manifests/sources.yaml":                        restricted,
		"content/en/codd-1970-relational/00_front.md":   file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: front\nlang: en\n", long),
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), long),
	}
	res := result(t, Run(in(t, files), true), "S07")
	if len(res.Findings) != 1 {
		t.Fatalf("S07 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].File, "codd") {
		t.Errorf("S07 counted a paper that is not restricted: %v", res.Findings[0])
	}
}

// S08 is the tripwire for a model that answered about a paper instead of
// reading one. Nothing else in the audit would notice: the text is
// well-formed, it parses, and it renders.
func TestS08RefusesMoreTextThanThePDFCouldHold(t *testing.T) {
	body := strings.Repeat("x", 11*PageChars+1)
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/codd-1970-relational/00_front.md":   file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: front\nlang: en\n", abstract),
		"content/en/codd-1970-relational/01_section.md": file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: section\nlang: en\n", body),
	}
	res := result(t, Run(in(t, files), true), "S08")
	if len(res.Findings) != 1 {
		t.Fatalf("S08 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "11 page") {
		t.Errorf("S08 does not say how long the PDF is: %s", res.Findings[0].Message)
	}
}

func TestS08AcceptsAPaperThatFits(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/codd-1970-relational/00_front.md":   file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: front\nlang: en\n", abstract),
		"content/en/codd-1970-relational/01_section.md": file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: section\nlang: en\n", strings.Repeat("x", 11*PageChars-len(abstract))),
	}
	if res := result(t, Run(in(t, files), true), "S08"); res.Failed() {
		t.Errorf("S08 refused a paper that fits: %v", res.Findings)
	}
}

// S09 is the rule that would have caught the two lecture slide decks the
// resolver fetched instead of the papers they were about.
func TestS09RefusesAPageWithAlmostNothingOnIt(t *testing.T) {
	files := map[string]string{
		"work/codd-1970-relational/pages/0001.txt": "A Relational Model\n\nE. F. Codd\n\nIBM Research",
		"work/codd-1970-relational/pages/0002.txt": "Outline\n\nMotivation\nThe model\nNormal form\nQuestions",
	}
	res := result(t, Run(build(t, files), true), "S09")
	if len(res.Findings) != 1 {
		t.Fatalf("S09 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "codd-1970-relational") {
		t.Errorf("S09 does not say which paper: %s", res.Findings[0].Message)
	}
	if !strings.Contains(res.Findings[0].Message, "2 pages") {
		t.Errorf("S09 does not say how much it read: %s", res.Findings[0].Message)
	}
}

// A page of a paper clears the floor several times over, and a blank verso
// in the middle of one does not drag it under, because the rule averages.
func TestS09AcceptsAPaperAndItsBlankVerso(t *testing.T) {
	files := map[string]string{
		"work/codd-1970-relational/pages/0001.txt": strings.Repeat("x", 3*SparsePage),
		"work/codd-1970-relational/pages/0002.txt": "",
	}
	if res := result(t, Run(build(t, files), true), "S09"); res.Failed() {
		t.Errorf("S09 refused a paper with one blank page: %v", res.Findings)
	}
}

// S11 is the hole nothing else can see. Cook's proof lost its last three
// pages to a reader that would not give up the mathematics on them, and
// every file around the hole is well formed.
func TestS11CountsThePagesThatNeverArrived(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/codd-1970-relational/00_front.md":   file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: front\nlang: en\npdf_pages: \"1\"\n", abstract),
		"content/en/codd-1970-relational/01_section.md": file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: section\nlang: en\npdf_pages: 2-8\n", abstract),
	}
	res := result(t, Run(in(t, files), false), "S11")
	if len(res.Findings) != 1 {
		t.Fatalf("S11 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "9 to 11") {
		t.Errorf("S11 does not name the pages: %s", res.Findings[0].Message)
	}
}

// A paper whose files cover every page is not reported, and a stub is never
// asked: the front matter and page one is what a stub is.
func TestS11AcceptsAWholePaperAndPassesOverAStub(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/codd-1970-relational/00_front.md":   file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: front\nlang: en\npdf_pages: 1-6\n", abstract),
		"content/en/codd-1970-relational/01_section.md": file("paper: codd-1970-relational\ntitle: A Relational Model\nkind: section\nlang: en\npdf_pages: 7-11\n", abstract),
		"content/en/vaswani-2017-attention/00_front.md": file("paper: vaswani-2017-attention\ntitle: Attention Is All You Need\nkind: front\nlang: en\npdf_pages: \"1\"\n", abstract),
	}
	if res := result(t, Run(in(t, files), false), "S11"); res.Failed() {
		t.Errorf("S11 reported a paper that is all here: %v", res.Findings)
	}
}

// S11 is soft. The last page of an offprint is the start of the next
// article in the issue and a scan carries the blank verso, and the corpus
// cannot tell either from a page a reader refused.
func TestS11IsSoft(t *testing.T) {
	for _, r := range Rules() {
		if r.ID == "S11" && r.Hard {
			t.Fatal("S11 is hard, and a page nobody should publish looks the same as a page that was refused")
		}
	}
}

// S10 is the rule for the fault that got past every other one: the front
// file of a restricted paper carrying a different paper's text, whole,
// because the pages that were read were not the pages the paper is on.
func TestS10CatchesAQuotationOffTheWrongPaper(t *testing.T) {
	body := "ALGORITHM 93\n\nGENERAL ORDER ARITHMETIC\n\nMillard H. Perstein\n\nThis procedure performs different order arithmetic operations, putting the result in the first parameter, and the order of the operation is given by the fourth.\n"
	files := map[string]string{
		"manifests/papers.yaml":                          restrictedPapers,
		"manifests/sources.yaml":                         restrictedSources,
		"content/en/floyd-1962-shortestpath/00_front.md": file(pinned("Algorithm 97: Shortest Path", "Robert W. Floyd"), body),
	}
	res := result(t, Run(in(t, files), true), "S10")
	if len(res.Findings) != 1 {
		t.Fatalf("S10 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "floyd-1962-shortestpath") {
		t.Errorf("S10 does not say which paper: %s", res.Findings[0].Message)
	}
}

// Naming an author is enough on its own. A title like "Go To Statement
// Considered Harmful" shares almost nothing with the abstract under it, and
// a rule that wanted both tests to pass would report the corpus for being
// well written.
func TestS10TakesAnAuthorAsProofEnough(t *testing.T) {
	body := "Dijkstra observes that the quality of programmers falls off with the density of the jumps in the programs they write, and asks that the construct be abolished.\n"
	files := map[string]string{
		"manifests/papers.yaml":                          restrictedPapers,
		"manifests/sources.yaml":                         restrictedSources,
		"content/en/floyd-1962-shortestpath/00_front.md": file(pinned("Algorithm 97: Shortest Path", "Edsger W. Dijkstra"), body),
	}
	res := result(t, Run(in(t, files), true), "S10")
	if res.NotRun {
		t.Fatal("S10 did not read the file at all")
	}
	if res.Failed() {
		t.Errorf("S10 refused a quotation that names its author: %v", res.Findings)
	}
}

// And the title on its own is enough the other way, which is the common
// case: a journal that sets the byline in a running head the reader dropped
// publishes an abstract with no author anywhere in it.
func TestS10TakesTheTitleWordsAsProofEnough(t *testing.T) {
	body := "The shortest path between every pair of points in a network is found by relaxing each link in turn against every point that could stand between its two ends.\n"
	files := map[string]string{
		"manifests/papers.yaml":                          restrictedPapers,
		"manifests/sources.yaml":                         restrictedSources,
		"content/en/floyd-1962-shortestpath/00_front.md": file(pinned("Algorithm 97: Shortest Path", "Robert W. Floyd"), body),
	}
	res := result(t, Run(in(t, files), true), "S10")
	if res.NotRun {
		t.Fatal("S10 did not read the file at all")
	}
	if res.Failed() {
		t.Errorf("S10 refused a quotation that is plainly the paper: %v", res.Findings)
	}
}

// An open paper publishes the whole of itself and is not a quotation of
// anything, so the rule has nothing to say about one.
func TestS10LeavesAnOpenPaperAlone(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), "A paragraph about something else entirely, long enough to be an abstract and sharing not one word with the title above it.\n"),
	}
	if res := result(t, Run(in(t, files), true), "S10"); !res.NotRun {
		t.Errorf("S10 read a paper nobody restricted: %v", res.Findings)
	}
}

// restrictedPapers and restrictedSources are the manifests the S10 tests
// share: one paper, restricted, off a scan of a whole journal department.
const restrictedPapers = `papers:
  - id: floyd-1962-shortestpath
    title: "Algorithm 97: Shortest Path"
    authors: [Robert W. Floyd]
    year: 1962
    field: algorithms
    status: listed
`

const restrictedSources = `sources:
  - id: floyd-1962-shortestpath
    access: restricted
    landing: https://dl.acm.org/doi/10.1145/367766.368168
    pages: 5
`

// pinned is the front matter of a restricted paper's one file, with the
// title and the author the rule reads.
func pinned(title, author string) string {
	return "paper: floyd-1962-shortestpath\ntitle: " + strconv.Quote(title) + "\nauthors:\n  - " + author + "\nkind: front\nlang: en\n"
}

// Nothing under work/ is committed, so in CI this rule has nothing to read
// and has to say so rather than pass.
func TestS09StandsDownWithNoExtractedPages(t *testing.T) {
	if res := result(t, Run(build(t, nil), true), "S09"); !res.NotRun {
		t.Errorf("S09 claims to have checked a corpus nobody extracted: %v", res.Findings)
	}
}

// The audit has to be able to say how many rules it is not running, so a
// group whose files do not exist yet reports that rather than passing.
func TestTheStructureGroupStandsDownOnAnEmptyCorpus(t *testing.T) {
	rep := Run(build(t, map[string]string{"manifests/sources.yaml": openSources}), false)
	for _, res := range rep.Results {
		if res.Rule.Group() != GroupStructure {
			continue
		}
		if !res.NotRun {
			t.Errorf("%s claims to have checked a corpus with no content: %v", res.Rule.ID, res.Findings)
		}
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0 words", 1: "1 word", 2: "2 words"} {
		if got := plural(n, "word"); got != want {
			t.Errorf("plural(%d) is %q, want %q", n, got, want)
		}
	}
}

func TestOrdinalOf(t *testing.T) {
	cases := map[string]int{
		"00_front.md":      0,
		"07_references.md": 7,
		"notes.md":         -1,
		"_leading.md":      -1,
		"xx_name.md":       -1,
	}
	for name, want := range cases {
		if got := ordinalOf(name); got != want {
			t.Errorf("ordinalOf(%q) is %d, want %d", name, got, want)
		}
	}
}

// The report is grouped by paper and language, so the same section number in
// two languages is not a clash.
func TestByPaperSeparatesTheLanguages(t *testing.T) {
	files := []*File{
		{Path: "content/en/a/00_front.md", Paper: "a", Lang: corpus.EN, Name: "00_front.md"},
		{Path: "content/vi/a/00_front.md", Paper: "a", Lang: corpus.VI, Name: "00_front.md"},
		{Path: "content/en/b/00_front.md", Paper: "b", Lang: corpus.EN, Name: "00_front.md"},
	}
	groups := byPaper(files)
	if len(groups) != 3 {
		t.Fatalf("byPaper made %d groups, want 3: %v", len(groups), keys(groups))
	}
	if got := fmt.Sprint(keys(groups)); got != "[a/en a/vi b/en]" {
		t.Errorf("the groups come back as %s, which is not a fixed order", got)
	}
}

func TestT11FindsRawHTML(t *testing.T) {
	long := strings.Repeat("a sentence of the paper. ", 12)
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"clean", long + "\n", false},
		{"a table", long + "\n\n<table>\n<tr><td>1</td></tr>\n</table>\n", true},
		{"a subscript", long + "\n\nthe term d<sub>k</sub> is the width.\n", true},
		{"a line break", long + "\n\nfirst line<br>second line\n", true},
		{"a tag in a fence is a listing", long + "\n\n```html\n<table>\n```\n", false},
		{"a tag in inline code is the word", long + "\n\nthe `<table>` element is a grid.\n", false},
		{"a tag in a display is mathematics", long + "\n\n$$\\langle a, b \\rangle < c$$\n", false},
		{"an email address is not a tag", long + "\n\nwrite to <someone@example.com> about it.\n", false},
		{"a comparison is not a tag", long + "\n\nwhenever n < k and k > 0 the bound holds.\n", false},
		{"a pipe table is not HTML", long + "\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n", false},
	}
	for _, tc := range cases {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body),
		}
		res := result(t, Run(in(t, files), true), "T11")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T11 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// One table is forty tags and one defect. A rule that reported all forty is a
// rule people learn to scroll past, so the file is reported once with the
// count on it.
func TestT11ReportsAFileOnceWithACount(t *testing.T) {
	body := strings.Repeat("a sentence of the paper. ", 12) +
		"\n\n<table>\n<tr><th>a</th><th>b</th></tr>\n<tr><td>1</td><td>2</td></tr>\n</table>\n"
	res := result(t, onePaper(t, body), "T11")
	if len(res.Findings) != 1 {
		t.Fatalf("T11 reported %d findings, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "14 tags") {
		t.Errorf("the message is %q, want the tag count in it", res.Findings[0].Message)
	}
}

// The corpus has no links in it, and a link in a body is always a model
// writing back a web address that the paper printed as prose. Reference 10
// of the MapReduce paper and the code footnote of the GAN introduction are
// the two that were found, and the second one was found in three languages.
func TestT12FindsAMarkdownLink(t *testing.T) {
	long := strings.Repeat("a sentence of the paper. ", 12)
	for _, tc := range []struct {
		name  string
		body  string
		fails bool
	}{
		{"clean", long + "\n", false},
		{"an address written as prose", long + "\n\nthe page is at http://example.org/x.\n", false},
		{"an address written as a link", long + "\n\nthe page is at [http://example.org/x](http://example.org/x).\n", true},
		{"a link with words for its text", long + "\n\nsee [the sort benchmark](http://example.org/x).\n", true},
		{"an image", long + "\n\n![Figure 1](figures/f1.png)\n", true},
		{"a citation into the corpus", long + "\n\nthe method of [[dean-2004-mapreduce]] is faster.\n", false},
		{"a numbered citation", long + "\n\nthe method of [3] is faster.\n", false},
		{"a link inside a fence is a listing", long + "\n\n```text\n[a](b)\n```\n", false},
		{"a link inside inline code is the markup itself", long + "\n\nthe form `[a](b)` is a link.\n", false},
	} {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body),
		}
		res := result(t, Run(in(t, files), true), "T12")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T12 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// A printed column breaks a word at a hyphen to make the line fit, and a
// reader that joins the two halves with a space instead of with nothing
// publishes a word that is not a word. "less than a mil- lisecond" is the
// one in the MapReduce cluster description.
func TestT13FindsAWordSplitAtALineBreakHyphen(t *testing.T) {
	long := strings.Repeat("a sentence of the paper. ", 12)
	for _, tc := range []struct {
		name  string
		body  string
		fails bool
	}{
		{"clean", long + "\n", false},
		{"a word the page broke", long + "\n\nthe round-trip time was under a mil- lisecond.\n", true},
		{"a word with its own hyphen", long + "\n\nthe round-trip time was short.\n", false},
		{"a hanging hyphen", long + "\n\nthe map- and reduce-side costs are equal.\n", false},
		{"a dash between two clauses", long + "\n\nthe cost is small - the gain is not.\n", false},
		{"a new sentence after a dash", long + "\n\nthe cost is small- The gain is not.\n", false},
		{"a single letter either side", long + "\n\nsend it by e- mail.\n", false},
		{"a minus sign in a formula", long + "\n\nthe bound is $a- b$ throughout.\n", false},
		{"a flag in inline code", long + "\n\nthe form `--keep- alive` is a flag.\n", false},
		{"a flag in a fence", long + "\n\n```text\ncc --keep- alive\n```\n", false},
	} {
		files := map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), tc.body),
		}
		res := result(t, Run(in(t, files), false), "T13")
		if res.Failed() != tc.fails {
			t.Errorf("%s: T13 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}
