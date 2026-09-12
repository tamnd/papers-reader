package grid

import "testing"

func TestATableIsWrittenWithARuleUnderTheHeader(t *testing.T) {
	got := Pipe([][]string{
		{"Machine", "Year"},
		{"EDSAC", "1949"},
		{"Atlas", "1962"},
	})
	want := "| Machine | Year |\n| --- | --- |\n| EDSAC | 1949 |\n| Atlas | 1962 |"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestAShortRowIsPaddedToTheWidthOfTheTable(t *testing.T) {
	// A renderer keeps the header's column count and quietly drops anything
	// past it, so the row that has to be padded is the short one and the
	// table has to be as wide as its widest row.
	got := Pipe([][]string{
		{"Machine", "Year"},
		{"EDSAC", "1949", "Cambridge"},
		{"Atlas"},
	})
	want := "| Machine | Year |  |\n| --- | --- | --- |\n| EDSAC | 1949 | Cambridge |\n| Atlas |  |  |"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestATableWithNoCellsIsNotWritten(t *testing.T) {
	if got := Pipe(nil); got != "" {
		t.Errorf("wrote %q for no rows", got)
	}
	if got := Pipe([][]string{{}, {}}); got != "" {
		t.Errorf("wrote %q for rows with no cells", got)
	}
}

func TestAPipeInACellDoesNotBecomeAColumn(t *testing.T) {
	got := Pipe([][]string{{"shell", "meaning"}, {"a | b", "run a, feed b"}})
	want := "| shell | meaning |\n| --- | --- |\n| a \\| b | run a, feed b |"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestAFenceWithNoLanguageIsTaggedText(t *testing.T) {
	got := Fence("", "one\ntwo")
	want := "```text\none\ntwo\n```"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestAFenceKeepsTheLanguageItWasGiven(t *testing.T) {
	got := Fence("go", "package main")
	want := "```go\npackage main\n```"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestAListingHoldingAFenceGetsALongerOne(t *testing.T) {
	got := Fence("", "open with ``` and close with ```")
	want := "````text\nopen with ``` and close with ```\n````"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

func TestAnEmptyBlockIsNotFenced(t *testing.T) {
	// Two fence lines with nothing between them is a block a reader stops at
	// and a rule counts, and it says nothing.
	if got := Fence("go", "  \n\n "); got != "" {
		t.Errorf("wrote %q for an empty block", got)
	}
}

func TestTheBlankLinesAroundABlockAreNotPartOfIt(t *testing.T) {
	got := Fence("text", "\n\nfirst\n\nlast\n\n")
	want := "```text\nfirst\n\nlast\n```"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}
