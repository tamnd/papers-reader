package audit

import (
	"fmt"
	"strings"
	"testing"
)

// Every bibliography in this file is invented, apart from the two records
// that are already in the test corpus above. A reference list is somebody's
// work like the rest of a paper is, and a test file is the last place to put
// one.

// front is a content file with the header the corpus writes.
func front(paper, kind, body string) string {
	return "---\npaper: " + paper + "\nkind: " + kind + "\nlang: en\n---\n\n" + body + "\n"
}

func TestGroupRIsQuietWithNoBibliographies(t *testing.T) {
	rep := Run(build(t, nil), false)
	for _, id := range []string{"R01", "R02", "R03", "R04", "R05", "R06", "R07", "R08"} {
		res := result(t, rep, id)
		if !res.NotRun {
			t.Errorf("%s claims to have run on a corpus with no bibliographies", id)
		}
	}
}

// R01 is the rule that keeps the reading app from offering a page that is
// not there.
func TestR01FindsALinkToNoPaper(t *testing.T) {
	in := build(t, map[string]string{
		"content/en/vaswani-2017-attention/02_model.md": front("vaswani-2017-attention", "section",
			"The idea goes back to [[codd-1970-relational]] and to [[nkemelu-1991-slowindexes]]."),
	})
	res := result(t, Run(in, true), "R01")
	if len(res.Findings) != 1 {
		t.Fatalf("R01 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "nkemelu-1991-slowindexes") {
		t.Errorf("R01 named the wrong link: %s", res.Findings[0].Message)
	}
}

// R02 is the rule that catches an entry lost in extraction. The paper still
// says "as shown in [7]" and there is no seventh reference to show.
func TestR02FindsACitationWithNoEntry(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    resolves_to: ""
  - key: "2"
    raw: B. Oyelaran. Indexes that are slower still. Slow Things Quarterly, 1994.
    resolves_to: ""
`,
		"content/en/vaswani-2017-attention/02_model.md": front("vaswani-2017-attention", "section",
			"This follows [1] and [2], and the bound is due to [7]."),
	})
	res := result(t, Run(in, true), "R02")
	if len(res.Findings) != 1 {
		t.Fatalf("R02 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "[7]") {
		t.Errorf("R02 named the wrong citation: %s", res.Findings[0].Message)
	}
}

// A citation inside a listing is an array index, and R02 reads a body the
// same way the rewriter does so that it does not complain about one.
func TestR02LeavesCodeAndMathematicsAlone(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    resolves_to: ""
`,
		"content/en/vaswani-2017-attention/02_model.md": front("vaswani-2017-attention", "section",
			"As in [1]:\n\n```go\nsum += weight[31]\n```\n\nand the interval $x \\in [0, 1]$ is closed."),
	})
	if res := result(t, Run(in, true), "R02"); res.Failed() {
		t.Errorf("R02 read a listing as a citation: %v", res.Findings)
	}
}

// R03 is the rule the whole design rests on. The parser is allowed to be
// wrong about a reference because the page renders what the paper printed,
// and an entry with no raw text has nothing left to render.
func TestR03FindsAnEntryThatLostItsText(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    resolves_to: ""
  - key: "2"
    raw: "   "
    title: Indexes that are slower still
    resolves_to: ""
`,
	})
	res := result(t, Run(in, true), "R03")
	if len(res.Findings) != 1 {
		t.Fatalf("R03 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "entry 2") {
		t.Errorf("R03 named the wrong entry: %s", res.Findings[0].Message)
	}
	if res.Findings[0].File != "manifests/refs/vaswani-2017-attention.yaml" {
		t.Errorf("R03 does not point at the manifest: %s", res.Findings[0].File)
	}
}

// R04 runs the matcher again over what it decided last time. It is soft
// because a title corrected in papers.yaml can move the score honestly.
func TestR04ReRunsTheMatcher(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    title: A theory of slow indexes
    authors: [A. Nkemelu]
    year: 1991
    resolves_to: codd-1970-relational
`,
	})
	res := result(t, Run(in, false), "R04")
	if len(res.Findings) != 1 {
		t.Fatalf("R04 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if res.Rule.Hard {
		t.Error("R04 is hard, and a corrected title would fail a build")
	}
}

// An identifier is not a similarity score. Re-running the matcher over an
// entry that resolved by its arXiv id would fail every one of them.
func TestR04SkipsWhatResolvedByIdentifier(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: E. F. Codd. Some paper or other. arXiv:1234.56789, 1970.
    title: Some paper or other
    arxiv: "1234.56789"
    year: 1970
    resolves_to: codd-1970-relational
`,
	})
	if res := result(t, Run(in, false), "R04"); res.Failed() {
		t.Errorf("R04 second-guessed an identifier: %v", res.Findings)
	}
}

// A paper's own bibliography never names the paper, and a self loop would
// break the citation graph before the cycle rules got to look at it.
func TestR05FindsTheSelfLoop(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    resolves_to: vaswani-2017-attention
`,
	})
	res := result(t, Run(in, true), "R05")
	if len(res.Findings) != 1 {
		t.Fatalf("R05 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
}

const twoEntries = `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.
    resolves_to: ""
  - key: "2"
    raw: B. Oyelaran. Indexes that are slower still. Slow Things Quarterly, 1994.
    resolves_to: ""
`

// R08 is the rule that stops a reference list being rebuilt out of the
// parsed fields. Such a list reads well and is subtly wrong about a hundred
// entries at once, and nobody proofreads a bibliography.
func TestR08WantsThePrintedText(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": twoEntries,
		"content/en/vaswani-2017-attention/09_references.md": front("vaswani-2017-attention", "references",
			"[1] A. Nkemelu. A theory of slow indexes. Journal of Slow Things,\n1991.\n\n[2] Oyelaran, B. Slower indexes. 1994."),
	})
	res := result(t, Run(in, true), "R08")
	if len(res.Findings) != 1 {
		t.Fatalf("R08 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "1 of the 2") {
		t.Errorf("R08 counted wrong: %s", res.Findings[0].Message)
	}
}

func TestR08PassesOnThePaperSOwnList(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": twoEntries,
		"content/en/vaswani-2017-attention/09_references.md": front("vaswani-2017-attention", "references",
			"[1] A. Nkemelu. A theory of slow indexes. Journal of Slow\nThings, 1991.\n\n[2] B. Oyelaran. Indexes that are slower still. Slow Things\nQuarterly, 1994."),
	})
	res := result(t, Run(in, true), "R08")
	if res.Failed() || res.NotRun {
		t.Errorf("R08 objected to the paper's own list: %v", res.Findings)
	}
}

// The reference section is found by its kind and not by its name, because
// the number in front of the name is the paper's own section number and is
// not the same twice.
func TestR08FindsTheSectionByKind(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": twoEntries,
		"content/en/vaswani-2017-attention/03_bibliografia.md": front("vaswani-2017-attention", "references",
			"[1] A. Nkemelu. A theory of slow indexes. Journal of Slow Things, 1991.\n\n[2] B. Oyelaran. Indexes that are slower still. Slow Things Quarterly, 1994."),
	})
	if res := result(t, Run(in, true), "R08"); res.NotRun {
		t.Error("R08 did not find a reference section under an unexpected name")
	}
}

// R06 is the check on the graph as a whole. Two papers 47 years apart cannot
// both cite the other, and the usual cause is the matcher joining two
// different works with similar titles.
func TestR06FindsALoopAcrossTheYears(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: E. F. Codd. A relational model of data. CACM, 1970.
    resolves_to: codd-1970-relational
`,
		"manifests/refs/codd-1970-relational.yaml": `paper: codd-1970-relational
style: bracket
entries:
  - key: "1"
    raw: A. Vaswani. Attention is all you need. NeurIPS, 2017.
    resolves_to: vaswani-2017-attention
`,
	})
	res := result(t, Run(in, false), "R06")
	if len(res.Findings) != 1 {
		t.Fatalf("R06 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "47 years apart") {
		t.Errorf("R06 measured the gap wrong: %s", res.Findings[0].Message)
	}
}

// A conference paper and the journal version of it a year later cite one
// another's drafts, and that is not a mistake.
func TestR06LeavesARevisionAlone(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/papers.yaml": `papers:
  - id: nkemelu-1991-indexes
    title: A Theory of Slow Indexes
    authors: [A. Nkemelu]
    year: 1991
    field: databases
    status: listed
  - id: nkemelu-1992-indexes
    title: A Theory of Slow Indexes, Revised
    authors: [A. Nkemelu]
    year: 1992
    field: databases
    status: listed
`,
		"manifests/refs/nkemelu-1991-indexes.yaml": `paper: nkemelu-1991-indexes
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes, revised. In preparation, 1992.
    resolves_to: nkemelu-1992-indexes
`,
		"manifests/refs/nkemelu-1992-indexes.yaml": `paper: nkemelu-1992-indexes
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Slow Things Quarterly, 1991.
    resolves_to: nkemelu-1991-indexes
`,
	})
	if res := result(t, Run(in, false), "R06"); res.Failed() {
		t.Errorf("R06 read a revision as a mistake: %v", res.Findings)
	}
}

// A chain that goes forward and stops is what a citation graph looks like.
func TestR06PassesOnAGraphThatOnlyGoesBackwards(t *testing.T) {
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": `paper: vaswani-2017-attention
style: bracket
entries:
  - key: "1"
    raw: E. F. Codd. A relational model of data. CACM, 1970.
    resolves_to: codd-1970-relational
`,
	})
	if res := result(t, Run(in, false), "R06"); res.Failed() || res.NotRun {
		t.Errorf("R06 objected to an ordinary citation: %v %v", res.NotRun, res.Findings)
	}
}

// R07 is the reading list telling the people who keep it what it is missing.
func TestR07NamesAWorkThreePapersCite(t *testing.T) {
	cites := func(paper, doi string) string {
		return "paper: " + paper + `
style: bracket
entries:
  - key: "1"
    raw: A. Nkemelu. A theory of slow indexes. Slow Things Quarterly, 1991.
    title: A Theory of Slow Indexes
    doi: ` + doi + `
    resolves_to: ""
`
	}
	in := build(t, map[string]string{
		"manifests/papers.yaml": papersYAML + `  - id: oyelaran-1994-slower
    title: Indexes That Are Slower Still
    authors: [B. Oyelaran]
    year: 1994
    field: databases
    status: listed
`,
		"manifests/refs/vaswani-2017-attention.yaml": cites("vaswani-2017-attention", "10.1000/slow"),
		"manifests/refs/codd-1970-relational.yaml":   cites("codd-1970-relational", "10.1000/SLOW"),
		"manifests/refs/oyelaran-1994-slower.yaml":   cites("oyelaran-1994-slower", "10.1000/slow"),
	})
	res := result(t, Run(in, false), "R07")
	if len(res.Findings) != 1 {
		t.Fatalf("R07 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.Contains(res.Findings[0].Message, "A Theory of Slow Indexes") {
		t.Errorf("R07 named the wrong work: %s", res.Findings[0].Message)
	}
}

// Two papers citing the same work is what a field is, and a reference the
// parser could read no title from is not evidence of anything.
func TestR07IsQuietAboutTwoAndAboutNothing(t *testing.T) {
	cites := func(paper, title string) string {
		return "paper: " + paper + `
style: bracket
entries:
  - key: "1"
    raw: A reference the parser could not read.
    title: ` + title + `
    resolves_to: ""
`
	}
	in := build(t, map[string]string{
		"manifests/refs/vaswani-2017-attention.yaml": cites("vaswani-2017-attention", "A Theory of Slow Indexes"),
		"manifests/refs/codd-1970-relational.yaml":   cites("codd-1970-relational", "A Theory of Slow Indexes"),
	})
	if res := result(t, Run(in, false), "R07"); res.Failed() {
		t.Errorf("R07 reported a work only two papers cite: %v", res.Findings)
	}

	short := build(t, map[string]string{
		"manifests/papers.yaml": papersYAML + `  - id: oyelaran-1994-slower
    title: Indexes That Are Slower Still
    authors: [B. Oyelaran]
    year: 1994
    field: databases
    status: listed
`,
		"manifests/refs/vaswani-2017-attention.yaml": cites("vaswani-2017-attention", "Notes"),
		"manifests/refs/codd-1970-relational.yaml":   cites("codd-1970-relational", "Notes"),
		"manifests/refs/oyelaran-1994-slower.yaml":   cites("oyelaran-1994-slower", "Notes"),
	})
	if res := result(t, Run(short, false), "R07"); res.Failed() {
		t.Errorf("R07 grouped references by a title too short to tell them apart: %v", res.Findings)
	}
}

func TestCycles(t *testing.T) {
	got := cycles(map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
		"d": {"a"},
		"e": {"f"},
		"f": {"e"},
	})
	want := [][]string{{"a", "b", "c"}, {"e", "f"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("cycles found %v, want %v", got, want)
	}
}
