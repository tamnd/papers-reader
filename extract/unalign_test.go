package extract

import "testing"

// The line this was written for, set the way the Scheme memo sets it: a
// report number against the left margin and a date against the right.
func TestALineSetToTheMarginsIsBrokenAtTheGap(t *testing.T) {
	got := Unalign("ARTIFICIAL INTELLIGENCE LABORATORY\n\nMemo No. 349                      December 1975\n\nby the authors\n")
	want := "ARTIFICIAL INTELLIGENCE LABORATORY\n\nMemo No. 349\n\nDecember 1975\n\nby the authors\n"
	if got != want {
		t.Errorf("Unalign gave\n%q\nwant\n%q", got, want)
	}
}

// The fields have to be paragraphs. Putting them on consecutive lines would
// let Markdown join them straight back into the line this started with.
func TestTheFieldsAreParagraphsAndNotLines(t *testing.T) {
	got := Unalign("Memo No. 349                      December 1975\n")
	if got != "Memo No. 349\n\nDecember 1975\n" {
		t.Errorf("Unalign gave %q", got)
	}
}

// A line of three fields is broken into three.
func TestEveryFieldOnTheLineGetsItsOwnParagraph(t *testing.T) {
	got := Unalign("Volume 3          Number 4          October 1974\n")
	if got != "Volume 3\n\nNumber 4\n\nOctober 1974\n" {
		t.Errorf("Unalign gave %q", got)
	}
}

// A run-in heading is a heading and the sentence after it on one line, and
// the gap between them is the width of a word space or three. Breaking that
// one gave the splitter a heading, which it made into a section, and the
// Attention paper published an acknowledgements section of one sentence and
// renumbered its references behind it.
func TestARunInHeadingIsNotALineSetToTheMargins(t *testing.T) {
	line := "Acknowledgements   We are grateful to the reviewers for their comments.\n"
	if got := Unalign(line); got != line {
		t.Errorf("Unalign broke a run-in heading: %q", got)
	}
}

// Two lined up lines in a row are a table. Breaking each row into its cells
// would finish destroying the grid rather than save it.
func TestARunOfLinedUpLinesIsLeftForTheTableRules(t *testing.T) {
	table := "q=0.10        z=5\nq=0.15        z=8\nq=0.20        z=11\n"
	if got := Unalign(table); got != table {
		t.Errorf("Unalign broke up a table:\n%s", got)
	}
}

// A pipe table is padded with exactly this white space and is the right
// answer rather than the wrong one.
func TestAPipeTableIsLeftAlone(t *testing.T) {
	row := "| q        | z       |\n"
	if got := Unalign(row); got != row {
		t.Errorf("Unalign rewrote a pipe table row: %q", got)
	}
}

func TestSpacesInsideAFenceAreLeftAlone(t *testing.T) {
	code := "```fortran\n      NTOT = NCHARS        + NWORDS\n```\n"
	if got := Unalign(code); got != code {
		t.Errorf("Unalign rewrote program text:\n%s", got)
	}
}

// An aligned environment is built out of runs of spaces, so a display is the
// one place where the spacing is the author's own and not a printer's.
func TestSpacesInsideADisplayAreLeftAlone(t *testing.T) {
	math := "$$\n\\begin{aligned}\na        &= b \\\\\nc        &= d\n\\end{aligned}\n$$\n"
	if got := Unalign(math); got != math {
		t.Errorf("Unalign rewrote mathematics:\n%s", got)
	}
}

// A $$ inside a listing is program text and opens no display, so a line
// after it is still an ordinary line.
func TestADollarPairInsideAFenceOpensNothing(t *testing.T) {
	in := "```bash\necho $$\n```\n\nMemo No. 349                      December 1975\n"
	want := "```bash\necho $$\n```\n\nMemo No. 349\n\nDecember 1975\n"
	if got := Unalign(in); got != want {
		t.Errorf("Unalign gave\n%q\nwant\n%q", got, want)
	}
}

// Two spaces are a typing habit and not a column, so ordinary prose comes
// back exactly as it went in.
func TestOrdinaryProseIsNotTouched(t *testing.T) {
	prose := "The measure is one more than the number of decisions.  It is\nindependent of the size of the program.\n"
	if got := Unalign(prose); got != prose {
		t.Errorf("Unalign rewrote prose:\n%s", got)
	}
}
