package refs

import "testing"

func links() map[string]string {
	return map[string]string{
		"2": "nkemelu-1991-slowindexes",
		"3": "ravensworth-1996-lastword",
		"5": "sandoval-1999-reconsidered",
	}
}

func TestACitationThatResolvedBecomesALink(t *testing.T) {
	got := Rewrite("The idea is older than it looks [2].", links())
	want := "The idea is older than it looks [[nkemelu-1991-slowindexes]]."
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestACitationThatDidNotResolveIsLeftAlone(t *testing.T) {
	// This is most of them, and it is the right answer: the reference is in
	// the paper's own reference list and the reader can go and read it
	// there.
	got := Rewrite("A point made elsewhere [7].", links())
	if got != "A point made elsewhere [7]." {
		t.Errorf("got %q", got)
	}
}

func TestAGroupOfCitationsIsRewrittenOneByOne(t *testing.T) {
	got := Rewrite("Several have said so [2, 7, 3].", links())
	want := "Several have said so [[nkemelu-1991-slowindexes]], [7], [[ravensworth-1996-lastword]]."
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestASpanOfCitationsIsOpenedOut(t *testing.T) {
	got := Rewrite("As in [2-5].", links())
	want := "As in [[nkemelu-1991-slowindexes]], [[ravensworth-1996-lastword]], [4], [[sandoval-1999-reconsidered]]."
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestAGroupWithNothingResolvedIsNotTouched(t *testing.T) {
	// Opening [7-9] out into [7], [8], [9] would change the author's text
	// for nothing and put churn in a diff nobody asked for.
	if got := Rewrite("As in [7-9].", links()); got != "As in [7-9]." {
		t.Errorf("got %q", got)
	}
}

func TestABracketInTheMathematicsIsNotACitation(t *testing.T) {
	body := "The interval is $[2, 3]$ and the reason is elsewhere [2]."
	want := "The interval is $[2, 3]$ and the reason is elsewhere [[nkemelu-1991-slowindexes]]."
	if got := Rewrite(body, links()); got != want {
		t.Errorf("got %q", got)
	}
}

func TestABracketInDisplayedMathematicsIsNotACitation(t *testing.T) {
	body := "Consider\n$$\nx \\in [2, 3]\n$$\nand see [3].\n"
	want := "Consider\n$$\nx \\in [2, 3]\n$$\nand see [[ravensworth-1996-lastword]].\n"
	if got := Rewrite(body, links()); got != want {
		t.Errorf("got %q", got)
	}
}

func TestABracketInACodeListingIsNotACitation(t *testing.T) {
	body := "See [2] for this:\n\n```go\nfmt.Println(a[2])\n```\n\nand [3] for the rest.\n"
	want := "See [[nkemelu-1991-slowindexes]] for this:\n\n```go\nfmt.Println(a[2])\n```\n\nand [[ravensworth-1996-lastword]] for the rest.\n"
	if got := Rewrite(body, links()); got != want {
		t.Errorf("got %q", got)
	}
}

func TestALinkThatIsAlreadyALinkIsLeftAlone(t *testing.T) {
	body := "See [[nkemelu-1991-slowindexes]] and [3]."
	want := "See [[nkemelu-1991-slowindexes]] and [[ravensworth-1996-lastword]]."
	if got := Rewrite(body, links()); got != want {
		t.Errorf("got %q", got)
	}
}

func TestNothingResolvedMeansNothingRewritten(t *testing.T) {
	body := "As in [2, 3] and [5]."
	if got := Rewrite(body, nil); got != body {
		t.Errorf("got %q", got)
	}
}

func TestAManifestRewritesFromItsOwnEntries(t *testing.T) {
	m := &Manifest{
		Paper: "somebody-2020-citing",
		Style: StyleBracket,
		Entries: []Entry{
			{Key: "1", Raw: "an entry"},
			{Key: "2", Raw: "another", ResolvesTo: "nkemelu-1991-slowindexes"},
		},
	}
	got := m.Rewrite("One [1] and another [2].")
	if got != "One [1] and another [[nkemelu-1991-slowindexes]]." {
		t.Errorf("got %q", got)
	}
}

// An array index is written the way a citation is, and a page of unfenced
// listing text is full of them. What tells them apart is the name against
// the bracket, so a bracket with one is left where it is.
func TestAnArrayIndexIsNotACitation(t *testing.T) {
	for _, body := range []string{
		"The loop body is F(A[2]),G(A[2]) for each row.",
		"It reads v[3] and writes w[5] on every pass.",
		"The second element is x)[2] after the call.",
	} {
		if got := Rewrite(body, links()); got != body {
			t.Errorf("Rewrite(%q) = %q, want it left alone", body, got)
		}
		if got := Citations(body); len(got) != 0 {
			t.Errorf("Citations(%q) = %v, want none", body, got)
		}
	}
}

// The character in front of a citation is part of the match and has to come
// back out of the rewrite unharmed, whatever it is.
func TestTheCharacterInFrontOfACitationIsKept(t *testing.T) {
	got := Rewrite("Two of them, [2] and [3], say so. (See [5].)", links())
	want := "Two of them, [[nkemelu-1991-slowindexes]] and [[ravensworth-1996-lastword]], say so. (See [[sandoval-1999-reconsidered]].)"
	if got != want {
		t.Errorf("got %q", got)
	}
}

// No bibliography numbers an entry zero. What is written that way is the
// unit interval, and both ends of it go, because 1 on its own is the other
// end of an interval and not a reference either.
func TestAGroupWithAZeroInItIsNotACitation(t *testing.T) {
	for _, body := range []string{
		"a value chosen from the interval [0, 1] at random",
		"the range [0-9] of the digits",
		"the empty index [0]",
	} {
		if got := Citations(body); len(got) != 0 {
			t.Errorf("Citations(%q) = %v, want none", body, got)
		}
	}
}
