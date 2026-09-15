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
