package audit

import (
	"fmt"
	"strings"
	"testing"
)

// onePaper builds a corpus of one paper whose one section carries the given
// body, and runs the audit over it. Group M is per span and per file, so a
// case is a body and nothing else.
func onePaper(t *testing.T, body string) *Report {
	t.Helper()
	return Run(in(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), body),
	}), false)
}

// pad is what a case adds so that the length rules have nothing to say about
// it. Group M is about the mathematics and a finding from T08 in the middle
// of one of these tests would be a distraction.
const pad = "\n\nThe surrounding paragraph of the section, which is here so that the file is long enough for the rest of the audit to leave it alone entirely.\n"

func TestM01FindsAnUnclosedDelimiter(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"closed", "the value $x$ is fixed." + pad, false},
		{"an inline span left open", "the value $x is fixed." + pad, true},
		{"a display left open", "$$\nx = 1\n" + pad, true},
		{"an escaped dollar is not a delimiter", `it costs \$5 and no more.` + pad, false},
		{"no mathematics at all", "a section with no formulae in it." + pad, false},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M01")
		if res.Failed() != tc.fails {
			t.Errorf("%s: M01 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// The line a finding points at is the line the delimiter was opened on. The
// end of the file is where the problem shows up and never where it is.
func TestM01PointsAtTheOpeningDelimiter(t *testing.T) {
	body := "first line.\n\nthe value $x is fixed.\n\nthird line.\n\nfourth line, and a good deal more text after it so that the file is long enough.\n"
	res := result(t, onePaper(t, body), "M01")
	if len(res.Findings) != 1 {
		t.Fatalf("M01 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if res.Findings[0].Line != 3 {
		t.Errorf("M01 points at line %d, want 3", res.Findings[0].Line)
	}
}

// The corpus writes one spelling of the number sets, so a reader moving
// between four papers does not have to work out that one paper's bold R is
// another paper's blackboard R.
func TestM02WantsOneSpellingOfTheNumberSets(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"blackboard", `a point $x \in \mathbb{R}^d$ of the space.` + pad, false},
		{"bold", `a point $x \in \mathbf{R}^d$ of the space.` + pad, true},
		{"roman", `a point $x \in \mathrm{Q}$ of the space.` + pad, true},
		{"the bare Unicode letter", "a point $x \\in ℝ^d$ of the space." + pad, true},
		{"a bold letter that is not a number set", `the matrix $\mathbf{A}$ is square.` + pad, false},
		{"the letter in the prose", "the real numbers R are not mathematics here." + pad, false},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M02")
		if res.Failed() != tc.fails {
			t.Errorf("%s: M02 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

func TestM03FindsAGlyphThatNeverBecameItsTeX(t *testing.T) {
	res := result(t, onePaper(t, "the rate $α$ is chosen so that $x$ converges."+pad), "M03")
	if !res.Failed() {
		t.Fatal("M03 let a bare Greek letter through")
	}
	if !strings.Contains(res.Findings[0].Message, "1 character") {
		t.Errorf("M03 says %q", res.Findings[0].Message)
	}
	if res := result(t, onePaper(t, `the rate $\alpha$ is chosen so that $x$ converges.`+pad), "M03"); res.Failed() {
		t.Errorf("M03 objected to a letter that is already written out: %v", res.Findings)
	}
}

// A sigma with a subscript is a sum and a sigma without one is the letter,
// and the only thing that tells them apart is the shape. The repair refuses
// to guess and the rule reports the refusal rather than swallowing it.
func TestM03ReportsWhatTheRepairRefusedToGuess(t *testing.T) {
	res := result(t, onePaper(t, "the sum $Σ_i x_i$ converges."+pad), "M03")
	if !res.Failed() {
		t.Fatal("M03 swallowed a refusal")
	}
	if !strings.Contains(res.Findings[0].Message, "will not guess") {
		t.Errorf("M03 says %q", res.Findings[0].Message)
	}
}

func TestM04AsksTheRendererThatWillSetIt(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"a formula KaTeX reads", `the sum $\sum_{i=1}^n x_i$ converges.` + pad, false},
		{"a command nobody defined", `the sum $\summ_{i=1}^n x_i$ converges.` + pad, true},
		{"a brace left open", `the sum $\frac{1{2}$ is not one.` + pad, true},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M04")
		if res.NotRun {
			t.Fatalf("%s: M04 did not run", tc.name)
		}
		if res.Failed() != tc.fails {
			t.Errorf("%s: M04 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// A marker is allowed during extraction on a page errata.yaml records as
// damaged. It is allowed nowhere in what gets published.
func TestM05RefusesAnIllegibleMarkerInThePublishedCorpus(t *testing.T) {
	for _, marker := range []string{"[?]", "[illegible]", "⟨illegible⟩"} {
		res := result(t, onePaper(t, "the constant is "+marker+" in the original."+pad), "M05")
		if !res.Failed() {
			t.Errorf("M05 published a page marked %s", marker)
		}
	}
	if res := result(t, onePaper(t, "the constant is 3 in the original."+pad), "M05"); res.Failed() {
		t.Errorf("M05 found a marker in a clean page: %v", res.Findings)
	}
}

// M06 is the rule for a section that lost its display mathematics: the
// formulae came out as prose, nothing is malformed, and every other rule in
// the group passes.
func TestM06FindsASectionThatLostItsDisplays(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
	}
	dense := "$$\nx = 1\n$$\n\n$$\ny = 2\n$$\n\n$$\nz = 3\n$$\n\n$$\nw = 4\n$$" + pad
	for i := 1; i <= 6; i++ {
		body := dense
		if i == 6 {
			body = "a section of the same paper with no display in it at all." + pad
		}
		files[fmt.Sprintf("content/en/vaswani-2017-attention/%02d_section.md", i)] =
			file(section("section")+fmt.Sprintf("pdf_pages: \"%d\"\n", i+1), body)
	}
	res := result(t, Run(in(t, files), false), "M06")
	if len(res.Findings) != 1 {
		t.Fatalf("M06 found %d, want 1: %v", len(res.Findings), res.Findings)
	}
	if !strings.HasSuffix(res.Findings[0].File, "06_section.md") {
		t.Errorf("M06 named %s, want the section with no displays", res.Findings[0].File)
	}
}

// One paper's worth of sections is not enough to have an opinion about the
// next one, and a rule that guessed from three would report the first long
// section of every paper in the corpus.
func TestM06StandsDownOnTooFewSections(t *testing.T) {
	files := map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
	}
	for i := 1; i <= 3; i++ {
		files[fmt.Sprintf("content/en/vaswani-2017-attention/%02d_section.md", i)] =
			file(section("section")+fmt.Sprintf("pdf_pages: \"%d\"\n", i+1), "$$\nx = 1\n$$"+pad)
	}
	if res := result(t, Run(in(t, files), false), "M06"); !res.NotRun {
		t.Errorf("M06 had an opinion about three sections: %v", res.Findings)
	}
}

func TestM07FindsABracketOnTheWrongSideOfADollar(t *testing.T) {
	res := result(t, onePaper(t, `the model (see $\S 3)$ uses attention.`+pad), "M07")
	if !res.Failed() {
		t.Fatal("M07 let the prose's bracket close inside the mathematics")
	}
	if res := result(t, onePaper(t, `the model (see $\S 3$) uses attention.`+pad), "M07"); res.Failed() {
		t.Errorf("M07 objected to a bracket that closes where it opened: %v", res.Findings)
	}
}

// A matrix that came out of the text layer as a pair of scripts renders as a
// superscript over a subscript: the rows are there, the brackets are not,
// and it looks enough like mathematics that a reader skimming will not stop.
func TestM08FindsAFlattenedMatrix(t *testing.T) {
	res := result(t, onePaper(t, "the matrix $A^{a b}_{c d}$ is square."+pad), "M08")
	if !res.Failed() {
		t.Fatal("M08 let a flattened matrix through")
	}
	if res := result(t, onePaper(t, `the matrix $\begin{pmatrix} a & b \\ c & d \end{pmatrix}$ is square.`+pad), "M08"); res.Failed() {
		t.Errorf("M08 objected to a real matrix: %v", res.Findings)
	}
}

func TestM09FindsTwoMarksOfTheSameKindOnOneBase(t *testing.T) {
	res := result(t, onePaper(t, "the term $x_i_j$ of the sum."+pad), "M09")
	if !res.Failed() {
		t.Fatal("M09 let a double subscript through")
	}
	if res := result(t, onePaper(t, "the term $x_i^j$ of the sum."+pad), "M09"); res.Failed() {
		t.Errorf("M09 objected to one script of each kind: %v", res.Findings)
	}
}

// The smallest difference in the group and the largest in meaning: a
// relation with the stroke lost says the opposite of what the paper said.
func TestM10FindsASignThatLostItsStroke(t *testing.T) {
	res := result(t, onePaper(t, `the element $x \in / S$ is outside it.`+pad), "M10")
	if !res.Failed() {
		t.Fatal("M10 let an inverted relation through")
	}
	if !strings.Contains(res.Findings[0].Message, "opposite") {
		t.Errorf("M10 does not say what is at stake: %s", res.Findings[0].Message)
	}
	if res := result(t, onePaper(t, `the element $x \notin S$ is outside it.`+pad), "M10"); res.Failed() {
		t.Errorf("M10 objected to a sign that was negated properly: %v", res.Findings)
	}
}

// A rule that can be evaded by writing the same mathematics another way is
// not a rule, so the corpus has one spelling of the delimiters.
func TestM11WantsDollarsAndNotTheLaTeXDelimiters(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"dollars", "the value $x$ is fixed." + pad, false},
		{"an inline pair", `the value \(x\) is fixed.` + pad, true},
		{"a display pair", `the value \[x = 1\] is fixed.` + pad, true},
		{"a spacing command inside the mathematics", `the value $x \[ y$ is fixed.` + pad, false},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M11")
		if res.Failed() != tc.fails {
			t.Errorf("%s: M11 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

func TestM12WantsTheFormulaTightAgainstItsDollars(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"tight", "the value $x$ is fixed." + pad, false},
		{"a space after the opening dollar", "the value $ x$ is fixed." + pad, true},
		{"a space before the closing dollar", "the value $x $ is fixed." + pad, true},
		{"a display set on its own lines", "the value\n\n$$\nx = 1\n$$\n\nis fixed." + pad, false},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M12")
		if res.Failed() != tc.fails {
			t.Errorf("%s: M12 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// A shell listing full of $PATH and $HOME opens a math span on every other
// line, and by the end of the file every rule in this group is reading a
// listing as mathematics.
func TestM13FindsADollarInAFencedCodeBlock(t *testing.T) {
	body := "the script reads\n\n```sh\nexport PATH=$HOME/bin:$PATH\n```\n\nand then runs." + pad
	res := result(t, onePaper(t, body), "M13")
	if !res.Failed() {
		t.Fatal("M13 let a shell variable open a math span")
	}
	if res.Findings[0].Line < 3 || res.Findings[0].Line > 5 {
		t.Errorf("M13 points at line %d, want the line inside the fence", res.Findings[0].Line)
	}
	clean := "the script reads\n\n```sh\nexport PATH=/usr/bin\n```\n\nand the value $x$ is fixed." + pad
	if res := result(t, onePaper(t, clean), "M13"); res.Failed() {
		t.Errorf("M13 objected to mathematics outside the fence: %v", res.Findings)
	}
}

// The group has to be able to say it did not run. A corpus with content in
// it and no mathematics is the normal state of a corpus extracted natively,
// and the span rules passing on it would be a claim nobody checked.
func TestTheSpanRulesStandDownOnACorpusWithNoMathematics(t *testing.T) {
	rep := onePaper(t, "a section of the paper with no formulae in it whatsoever."+pad)
	spanRules := map[string]bool{"M02": true, "M04": true, "M06": true, "M07": true, "M09": true, "M10": true, "M12": true}
	for _, res := range rep.Results {
		if res.Rule.Group() != GroupMathematics {
			continue
		}
		if spanRules[res.Rule.ID] != res.NotRun {
			t.Errorf("%s reports not-run=%v on a corpus with no mathematics", res.Rule.ID, res.NotRun)
		}
	}
}

func TestPageSpan(t *testing.T) {
	cases := map[string]int{
		"":       0,
		"1":      1,
		"3-7":    5,
		" 3 - 7": 5,
		"7-3":    0,
		"x":      0,
		"3-x":    0,
	}
	for s, want := range cases {
		if got := pageSpan(s); got != want {
			t.Errorf("pageSpan(%q) is %d, want %d", s, got, want)
		}
	}
}

func TestCodeRanges(t *testing.T) {
	body := "one\n```\ntwo\n```\nthree\n~~~\nfour\n"
	got := codeRanges(body)
	want := [][2]int{{2, 4}, {6, 8}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("codeRanges is %v, want %v", got, want)
	}
}

// M14 is the rule for the mathematics that was not mangled but deleted. It
// is the only rule in the group that reads a file with no math span in it
// and has something to say about it.
func TestM14FindsAPaperWhoseFormulasWereFlattened(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{
			"a paper with no mathematics in it",
			"a section about naming things, which has no formulae in it at all." + pad,
			false,
		},
		{
			"a paper whose mathematics is marked up",
			"we divide by $\\sqrt{d}$ where $d \\in \\mathbb{N}$ and the sum $\\sum_i x_i$ is bounded." + pad,
			false,
		},
		{
			// What pdftotext leaves behind: the radical, the set sign and the
			// sum are all still on the page, and not one of them is in a span.
			"a paper whose mathematics was flattened into the prose",
			"we divide by √ d where d ∈ N and the sum ∑ i x i is bounded." + pad,
			true,
		},
		{
			// One glyph in a sentence about notation. A rule that failed here
			// would be a rule people turned off.
			"a single sign in a sentence about notation",
			"the paper writes the empty set as ∅ throughout, and nowhere else." + pad,
			false,
		},
		{
			"two signs are still not enough to be sure",
			"the paper writes ∅ for the empty set and ≤ for the ordering." + pad,
			false,
		},
		{
			// The characters prose has a use for. A kernel is three by three,
			// an error bar is plus or minus, and the authors are separated by
			// middle dots. None of that is mathematics going missing.
			"the characters prose uses are not counted",
			"the 3 × 3 kernel gave 92.1 ± 0.4 on the set · and ran in one hour." + pad,
			false,
		},
		{
			// A Greek letter is a word in a sentence as often as it is a
			// variable in a formula.
			"greek letters are not counted",
			"the alpha release used the α parameter, the β release the θ one." + pad,
			false,
		},
	}
	for _, tc := range cases {
		res := result(t, onePaper(t, tc.body), "M14")
		if res.Failed() != tc.fails {
			t.Errorf("%s: M14 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// Arithmetic in a code block is code. A paper that prints a program is not a
// paper whose formulas were flattened, and blanking the fences first is what
// keeps the two apart.
func TestM14DoesNotReadACodeBlockAsLostMathematics(t *testing.T) {
	body := "the routine is short:\n\n```\nif x ≤ y && y ≠ z && z ≥ x {\n  return x\n}\n```\n" + pad
	if res := result(t, onePaper(t, body), "M14"); res.Failed() {
		t.Errorf("M14 read a code block as lost mathematics: %v", res.Findings)
	}
}

// One finding per paper and not one per file. A paper keeps its mathematics
// in the sections that need it, and a finding per section would be a page of
// findings about one thing.
func TestM14IsOneFindingForThePaperAndPointsAtTheFirstSign(t *testing.T) {
	res := result(t, Run(in(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), "the bound is ≤ n and the limit is ∞ as ∑ grows."+pad),
		"content/en/vaswani-2017-attention/02_section.md": file(section("section"), "the same again, with ≥ m and ∈ S and ∏ over it."+pad),
	}), false), "M14")
	if len(res.Findings) != 1 {
		t.Fatalf("M14 found %d, want the one paper: %v", len(res.Findings), res.Findings)
	}
	if !strings.HasSuffix(res.Findings[0].File, "01_section.md") {
		t.Errorf("M14 points at %s, want the first section it saw a sign in", res.Findings[0].File)
	}
	if !strings.Contains(res.Findings[0].Message, "6 mathematical characters") {
		t.Errorf("M14 counted the whole paper as %q", res.Findings[0].Message)
	}
}
