package audit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// Every listing in this file was written for the test. None of it is copied
// from a paper.

func TestC01FindsAFenceThatNeverCloses(t *testing.T) {
	res := result(t, onePaper(t, "The routine is this.\n\n```c\nint x = 1;\n"+pad), "C01")
	if !res.Failed() {
		t.Fatal("C01 passed a fence that never closed")
	}
	if res.Findings[0].Line != 3 {
		t.Errorf("C01 named line %d, want the line the fence opened on", res.Findings[0].Line)
	}
}

func TestC01PassesAFenceThatCloses(t *testing.T) {
	if res := result(t, onePaper(t, "```c\nint x = 1;\n```"+pad), "C01"); res.Failed() {
		t.Errorf("C01 reported a closed fence: %v", res.Findings)
	}
}

func TestC02WantsATagOnEveryFence(t *testing.T) {
	for _, c := range []struct {
		name  string
		body  string
		fails bool
	}{
		{"a known tag", "```c\nint x = 1;\n```" + pad, false},
		{"text, which is a tag and not a language", "```text\nIn    Out\n```" + pad, false},
		{"no tag at all", "```\nint x = 1;\n```" + pad, true},
		{"a tag nobody uses", "```algo\nint x = 1;\n```" + pad, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := result(t, onePaper(t, c.body), "C02")
			if res.Failed() != c.fails {
				t.Errorf("C02 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
		})
	}
}

// The wrapper bug seen from the inside. A reader that wraps an answer in a
// markdown fence and then transcribes a listing produces exactly this, and
// every fence rule but this one passes on it.
func TestC03FindsAFenceInsideAFence(t *testing.T) {
	res := result(t, onePaper(t, "~~~c\nint x = 1;\n```\nint y = 2;\n~~~"+pad), "C03")
	if !res.Failed() {
		t.Fatal("C03 passed a fence inside a fence")
	}
}

// A listing that is showing Markdown is the one honest reason for a fence
// inside a fence, and a paper about document formats prints one. It says so
// in its tag.
func TestC03LeavesAListingOfMarkdownAlone(t *testing.T) {
	if res := result(t, onePaper(t, "~~~text\n```c\nint x = 1;\n```\n~~~"+pad), "C03"); res.Failed() {
		t.Errorf("C03 reported a listing that says it is showing Markdown: %v", res.Findings)
	}
}

func TestC04FindsAFenceInsideMathematics(t *testing.T) {
	res := result(t, onePaper(t, "The bound is\n\n$$\nx = 1\n```\ny = 2\n$$"+pad), "C04")
	if !res.Failed() {
		t.Fatal("C04 passed a fence that opened inside a display")
	}
}

func TestC04PassesAFenceBesideMathematics(t *testing.T) {
	body := "The bound is $x = 1$ and the routine is\n\n```c\nint x = 1;\n```" + pad
	if res := result(t, onePaper(t, body), "C04"); res.Failed() {
		t.Errorf("C04 reported a fence that is next to mathematics and not inside it: %v", res.Findings)
	}
}

// A fence that swallowed the prose looks like a very long listing from the
// outside, and there is nothing else it can be checked against.
func TestC05FindsAListingThatSwallowedThePage(t *testing.T) {
	body := "```c\n" + strings.Repeat("int x = 1;\n", 200) + "```" + pad
	res := result(t, onePaper(t, body), "C05")
	if !res.Failed() {
		t.Fatal("C05 passed a listing of 200 lines")
	}
}

func TestC05PassesAListingOfOrdinaryLength(t *testing.T) {
	body := "```c\n" + strings.Repeat("int x = 1;\n", 20) + "```" + pad
	if res := result(t, onePaper(t, body), "C05"); res.Failed() {
		t.Errorf("C05 reported a listing of 20 lines: %v", res.Findings)
	}
}

func TestC06WantsAnAnchorOnANumberedListing(t *testing.T) {
	const listing = "```c\nint x = 1;\n```\n\n"
	for _, c := range []struct {
		name  string
		body  string
		fails bool
	}{
		{"a caption with its block", listing + "Algorithm 1: the routine. {#a-1970-paper-alg-1 .code tag=00B2}" + pad, false},
		{"a caption with no block at all", listing + "Algorithm 1: the routine." + pad, true},
		{"a caption filed under the wrong class", listing + "Listing 2: the routine. {#a-1970-paper-lst-2 .figure tag=00B3}" + pad, true},
		{"a listing nobody numbered", listing + "The routine above is the whole of it." + pad, false},
		{"a sentence that mentions one", listing + "The loop of Algorithm 1 runs until the queue is empty." + pad, false},
		{"a caption printed inside a fence", "```text\nAlgorithm 1: the routine.\n```" + pad, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := result(t, onePaper(t, c.body), "C06")
			if res.Failed() != c.fails {
				t.Errorf("C06 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
		})
	}
}

// C07 reads the fences with the code package rather than with the
// translator's own Protect, which is what makes it worth having beside L18.
func TestC07ComparesTheFencesOfATranslationWithItsEnglish(t *testing.T) {
	en := "The routine is this.\n\n```c\nint x = 1;  \n```\n"
	for _, c := range []struct {
		name  string
		tr    string
		fails bool
	}{
		{"the same listing", "Thủ tục là thế này.\n\n```c\nint x = 1;  \n```\n", false},
		{"the trailing spaces tidied", "Thủ tục là thế này.\n\n```c\nint x = 1;\n```\n", true},
		{"a word of the program translated", "Thủ tục là thế này.\n\n```c\nint y = 1;  \n```\n", true},
		{"the language tag changed", "Thủ tục là thế này.\n\n```cpp\nint x = 1;  \n```\n", true},
		{"the listing dropped", "Thủ tục là thế này.\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := result(t, pairOf(t, corpus.VI, en, c.tr), "C07")
			if res.Failed() != c.fails {
				t.Errorf("C07 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
		})
	}
}

// The rule the group was missing. Every fence rule passes on this page
// because there is no fence on it.
func TestC08FindsAListingWithNoFence(t *testing.T) {
	body := "Written out, the routine is this.\n\n" +
		"#include <math.h>\n" +
		"double f(double q, int z)\n" +
		"{\n" +
		"    double p = 1.0 - q;\n" +
		"    return p;\n" +
		"}\n" + pad
	res := result(t, onePaper(t, body), "C08")
	if !res.Failed() {
		t.Fatal("C08 passed a C routine written as prose")
	}
	if res.Findings[0].Line != 3 {
		t.Errorf("C08 named line %d, want the first line of the listing", res.Findings[0].Line)
	}
}

func TestC08LeavesAListingInAFenceAlone(t *testing.T) {
	body := "Written out, the routine is this.\n\n```c\n" +
		"#include <math.h>\n" +
		"double f(double q, int z)\n" +
		"{\n" +
		"    double p = 1.0 - q;\n" +
		"    return p;\n" +
		"}\n```" + pad
	if res := result(t, onePaper(t, body), "C08"); res.Failed() {
		t.Errorf("C08 reported a listing that is fenced: %v", res.Findings)
	}
}

// A reader given a program sometimes writes every line of it as its own
// paragraph. Floyd's Algorithm 97 came back that way and the rule saw ten
// runs of one line, so the listing went out unfenced and the translator then
// left the program lines in English.
func TestC08FindsAListingWrittenAsOneLineParagraphs(t *testing.T) {
	body := "The procedure is this.\n\n" +
		"procedure shortest path (m, n); value n; integer n; array m;\n\n" +
		"begin\n\n" +
		"integer i, j, k; real inf, s; inf := 1010;\n\n" +
		"for i := 1 step 1 until n do\n\n" +
		"if s < m[j, k] then m[j, k] := s\n\n" +
		"end shortest path\n" + pad
	res := result(t, onePaper(t, body), "C08")
	if !res.Failed() {
		t.Fatal("C08 passed an ALGOL procedure written a line to a paragraph")
	}
	if res.Findings[0].Line != 3 {
		t.Errorf("C08 named line %d, want the first line of the listing", res.Findings[0].Line)
	}
}

// Two blank lines is a gap between two things and not a listing with room in
// it, so the run stops there and neither half is long enough to report.
func TestC08StopsAtAGapOfTwoBlankLines(t *testing.T) {
	body := "The value of i := 1 is fixed throughout.\n\n\n" +
		"The value of j := 2 is fixed as well.\n\n\n" +
		"The value of k := 3 is what varies.\n" + pad
	if res := result(t, onePaper(t, body), "C08"); res.Failed() {
		t.Errorf("C08 joined paragraphs across a gap: %v", res.Findings)
	}
}

// The reason the rule wants three lines and two marks rather than one of
// either. Prose puts a brace in a citation, a semicolon at the end of a
// clause and a capitalised word at the head of a line, and none of those is
// a program.
func TestC08LeavesProseAlone(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{
			name: "ordinary paragraphs",
			body: "The first paragraph of the section, which is about the protocol.\n\nThe second paragraph, which is about the proof.\n\nThe third, which is about neither.",
		},
		{
			name: "a semicolon at the end of a clause",
			body: "The first claim holds for every honest node;\nthe second holds for the attacker as well.\nThe two together give the bound.",
		},
		{
			name: "a bibliography",
			body: "[1] A. Author, Title of the first work, Journal, 1970.\n[2] B. Author, Title of the second work, Journal, 1971.\n[3] C. Author, Title of the third work, Journal, 1972.",
		},
		{
			name: "a list",
			body: "- the first condition, which holds always\n- the second condition, which holds for honest nodes\n- the third condition, which is assumed",
		},
		{
			// Verse. The GPT-3 paper prints the poems its model
			// generated, and two of the nine lines of one of them end
			// in a semicolon, which is punctuation and not a
			// statement.
			name: "a verse punctuated with semicolons",
			body: "He sees shadows on the way, hears voices,\nhears the wind and the rustling of leaves;\nThrough an open glade\nHe sees a shape and the shape hears:\nIt waits as he waits,\nAs the voices wait;\nShadows on the way, voices in the wind.",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if res := result(t, onePaper(t, c.body+pad), "C08"); res.Failed() {
				t.Errorf("C08 reported prose: %v", res.Findings)
			}
		})
	}
}

// A paper with authors at four institutions prints each institution's
// addresses as a brace list, and a brace at the head of a line is one of the
// marks. Four of those with an affiliation line between each pair is an
// eight line listing as far as this rule can tell.
func TestC08LeavesTheMastheadAlone(t *testing.T) {
	front := "A Paper With Authors At Two Institutions\n\n" +
		"Ada Lovelace*, Grace Hopper†\n\n" +
		"A University*\n\n" +
		"{ada,grace}@a.example\n\n" +
		"Another University†\n\n" +
		"{turing,hoare}@b.example\n\n" +
		abstract
	rep := Run(in(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), front),
	}), false)
	if res := result(t, rep, "C08"); res.Failed() {
		t.Errorf("C08 reported a byline: %v", res.Findings)
	}
}

// Mathematics is full of braces and is not a program, and the rule does not
// look inside a span for exactly that reason.
func TestC08LeavesMathematicsAlone(t *testing.T) {
	body := "The value is\n\n$$\nq_z = \\begin{cases}\n1 & p \\leq q \\\\\n(q/p)^z & p > q\n\\end{cases}\n$$" + pad
	if res := result(t, onePaper(t, body), "C08"); res.Failed() {
		t.Errorf("C08 reported a display: %v", res.Findings)
	}
}

// The other half of a printed listing: the numbers it produced, in columns.
// Markdown collapses the spaces, so the reader of the corpus gets a ragged
// paragraph and cannot tell which number belongs to which heading.
func TestC09FindsColumnsLinedUpWithSpaces(t *testing.T) {
	body := "The results are these.\n\nz=0    P=1.0000000\nz=1    P=0.2045873\nz=2    P=0.0509779\n" + pad
	res := result(t, onePaper(t, body), "C09")
	if !res.Failed() {
		t.Fatal("C09 passed three lines of columns lined up with spaces")
	}
	if res.Findings[0].Line != 3 {
		t.Errorf("C09 named line %d, want the first line of the columns", res.Findings[0].Line)
	}
}

func TestC09LeavesAPipeTableAlone(t *testing.T) {
	body := "| z   | P         |\n| --- | --------- |\n| 0   | 1.0000000 |\n| 1   | 0.2045873 |" + pad
	if res := result(t, onePaper(t, body), "C09"); res.Failed() {
		t.Errorf("C09 reported a pipe table, which is the right answer and not the wrong one: %v", res.Findings)
	}
}

func TestC09LeavesAFencedTableAlone(t *testing.T) {
	body := "```text\nz=0    P=1.0000000\nz=1    P=0.2045873\n```" + pad
	if res := result(t, onePaper(t, body), "C09"); res.Failed() {
		t.Errorf("C09 reported a table that is already fenced: %v", res.Findings)
	}
}

// A column needs rows. One line with a wide gap in it is prose that was
// typeset with the gap, and seven of this rule's eight findings over the
// corpus were that: a GPT-3 figure caption with the number set in bold, and
// a run-in heading with its paragraph on the same line.
func TestC09LeavesOneLineWithAWideGapAlone(t *testing.T) {
	body := "Figure 4.1: GPT-3 Training Curves   We measure model performance during training on a validation split." + pad
	if res := result(t, onePaper(t, body), "C09"); res.Failed() {
		t.Errorf("C09 reported one line with a gap in it: %v", res.Findings)
	}
}

// A byline is names in three columns and an affiliation line is
// institutions in six, set that way because the page is two columns wide.
// This is the same exception rule L07 makes on the same lines.
func TestC09LeavesTheMastheadAlone(t *testing.T) {
	front := "A Paper With A Wide Byline\n\n" +
		"Ada Lovelace†    Grace Hopper‡    Edsger Dijkstra\n" +
		"Barbara Liskov†    Alan Turing‡    Tony Hoare\n\n" +
		"†A University    ‡Another University    A Third University\n\n" +
		abstract
	rep := Run(in(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), front),
	}), false)
	if res := result(t, rep, "C09"); res.Failed() {
		t.Errorf("C09 reported a byline: %v", res.Findings)
	}
}

// And the exception stops where the masthead does. A table of results
// further down the same front page is still a finding.
func TestC09StillReadsATableUnderTheAbstract(t *testing.T) {
	front := "A Paper With A Table On Its First Page\n\n" +
		"Ada Lovelace†    Grace Hopper‡\n\n" +
		abstract +
		"\nz=0    P=1.0000000\nz=1    P=0.2045873\nz=2    P=0.0509779\n"
	rep := Run(in(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), front),
	}), false)
	if res := result(t, rep, "C09"); !res.Failed() {
		t.Error("C09 passed a table of numbers under the abstract")
	}
}

func TestC09LeavesOrdinaryProseAlone(t *testing.T) {
	body := "A paragraph with one space between every pair of words, which is what prose is.\n\nAnd a second one." + pad
	if res := result(t, onePaper(t, body), "C09"); res.Failed() {
		t.Errorf("C09 reported prose: %v", res.Findings)
	}
}
