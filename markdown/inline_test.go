package markdown

import "testing"

// A four digit number in brackets is the year of an author and year
// citation. The Paxos paper writes "by Lampson [1996]" and its
// bibliography has no entry 1996.
func TestNumCiteLeavesAYearAlone(t *testing.T) {
	for _, s := range []string{"by Lampson [1996]", "by De Prisco et al. [1997]", "[1988, 1990]"} {
		if m := NumCite.FindString(s); m != "" {
			t.Errorf("%q was read as the citation %q", s, m)
		}
	}
}

// And a label is still a label, alone, in a list and as a range.
func TestNumCiteStillFindsALabel(t *testing.T) {
	for s, want := range map[string]string{
		"as shown in [3]":    "[3]",
		"see [3, 7]":         "[3, 7]",
		"see [2-5] for more": "[2-5]",
		"entry [147] of it":  "[147]",
	} {
		if m := NumCite.FindString(s); m != want {
			t.Errorf("%q gave %q and not %q", s, m, want)
		}
	}
}

func TestReplaceCitesRewritesTheWholeCitation(t *testing.T) {
	got := ReplaceCites("As in [2] and [3, 7-9].", func(inner string) string {
		return "<" + inner + ">"
	})
	want := "As in <2> and <3, 7-9>."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A bracket with a name against it is an array index, and a page of
// unfenced listing text is full of them.
func TestReplaceCitesLeavesAnArrayIndexAlone(t *testing.T) {
	for _, s := range []string{
		"The loop body is F(A[2]),G(A[2]) for each row.",
		"It reads v[3] and writes w[5] on every pass.",
		"The second element is x)[2] after the call.",
		"An index of an index, a[1][2], twice over.",
	} {
		if got := ReplaceCites(s, func(string) string { return "LINKED" }); got != s {
			t.Errorf("ReplaceCites(%q) = %q, want it left alone", s, got)
		}
		if got := Cites(s); len(got) != 0 {
			t.Errorf("Cites(%q) = %v, want none", s, got)
		}
	}
}

// No reference list numbers an entry zero, so a group with one in it is an
// interval and the whole group stays as it is.
func TestReplaceCitesLeavesAGroupWithAZeroInIt(t *testing.T) {
	for _, s := range []string{
		"a value chosen from the interval [0, 1] at random",
		"the range [0-9] of the digits",
		"the empty index [0]",
		"a probability in [00, 10] of the trials",
	} {
		if got := ReplaceCites(s, func(string) string { return "LINKED" }); got != s {
			t.Errorf("ReplaceCites(%q) = %q, want it left alone", s, got)
		}
	}
}

func TestCitesReadsTheGroupsInOrder(t *testing.T) {
	got := Cites("First [2], then [3, 7-9], and an index a[4] that is not one.")
	want := []string{"2", "3, 7-9"}
	if len(got) != len(want) {
		t.Fatalf("Cites() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group %d is %q, want %q", i, got[i], want[i])
		}
	}
}
