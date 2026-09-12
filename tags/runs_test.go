package tags

import (
	"strings"
	"testing"
)

func TestReadRuns(t *testing.T) {
	runs, err := ReadRuns(strings.NewReader("0001,0042\n\n0043,00FF\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("read %d runs, want 2", len(runs))
	}
	if runs[0].First != "0001" || runs[1].Last != "00FF" {
		t.Errorf("read %+v", runs)
	}
}

func TestReadRunsRefusesNonsense(t *testing.T) {
	for _, s := range []string{"0001\n", "0001,00GG\n", "00GG,0002\n", "0042,0001\n"} {
		if _, err := ReadRuns(strings.NewReader(s)); err == nil {
			t.Errorf("ReadRuns accepted %q", s)
		}
	}
}

func TestWriteRunsKeepsTheOrderTheyHappened(t *testing.T) {
	var b strings.Builder
	runs := []Run{{First: "0100", Last: "0120"}, {First: "0001", Last: "0042"}}
	if err := WriteRuns(&b, runs); err != nil {
		t.Fatal(err)
	}
	if want := "0100,0120\n0001,0042\n"; b.String() != want {
		t.Errorf("wrote %q, want %q", b.String(), want)
	}
}

// Two tags out of one run have an order the audit may insist on. Two out of
// different runs do not, because a section added next year takes a tag from
// the top of the register and sits wherever the author put it.
func TestTogether(t *testing.T) {
	runs := []Run{{First: "0001", Last: "0010"}, {First: "0011", Last: "0020"}}
	if !Together(runs, "0002", "0009") {
		t.Error("two tags from the first run are not together")
	}
	if Together(runs, "0009", "0012") {
		t.Error("tags from two runs are together")
	}
	if Together(runs, "0002", "00FF") {
		t.Error("a tag from no run is together with one from a run")
	}
}
