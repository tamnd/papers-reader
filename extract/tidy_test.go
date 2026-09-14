package extract

import "testing"

func TestTidyTakesOffTheWrapping(t *testing.T) {
	for _, c := range []struct {
		what string
		in   string
		want string
	}{
		{
			"a fence round the whole answer",
			"```\nThe relational view of data.\n```",
			"The relational view of data.",
		},
		{
			"a fence that says the answer is markdown",
			"```markdown\n## 1. Introduction\n\nThis paper is concerned with relations.\n```",
			"## 1. Introduction\n\nThis paper is concerned with relations.",
		},
		{
			"a line of introduction",
			"Here is the transcription of the page:\n\n## 1. Introduction",
			"## 1. Introduction",
		},
		{
			"an introduction and a fence",
			"Sure! Here is the page:\n\n```md\n## 1. Introduction\n```",
			"## 1. Introduction",
		},
		{
			"a closing courtesy",
			"## 1. Introduction\n\nLet me know if you would like the next page.",
			"## 1. Introduction",
		},
		{
			"a bare label",
			"Transcription:\n\n377",
			"377",
		},
		{
			"nothing to take off",
			"## 1. Introduction\n\nThis paper is concerned with relations.",
			"## 1. Introduction\n\nThis paper is concerned with relations.",
		},
		{
			"windows line endings",
			"## 1. Introduction\r\n\r\nRelations.\r\n",
			"## 1. Introduction\n\nRelations.",
		},
	} {
		if got := Tidy(c.in); got != c.want {
			t.Errorf("%s came out as %q, want %q", c.what, got, c.want)
		}
	}
}

// The page's own listing is the thing this must never eat. A page of the
// ALGOL 60 report is a fence from its first line to its last, and a reader
// that treated that as packaging would publish an empty page.
func TestTidyLeavesThePagesOwnCodeAlone(t *testing.T) {
	page := "```algol\nbegin\n  integer i;\nend\n```"
	if got := Tidy(page); got != page {
		t.Errorf("a page that is one listing came out as %q", got)
	}

	// A wrapper with a listing inside it: the outer fence goes, the inner one
	// stays, and the fences still balance afterwards.
	wrapped := "```markdown\nThe procedure is\n\n```algol\nbegin end\n```\n\nas printed.\n```"
	want := "The procedure is\n\n```algol\nbegin end\n```\n\nas printed."
	if got := Tidy(wrapped); got != want {
		t.Errorf("a wrapped page with a listing in it came out as %q", got)
	}
}

// An odd number of fences is a page with a fence problem, and rule A7 should
// see it as it is. Taking the first one off would make an unclosed fence look
// like a closed one and let a broken page through.
func TestTidyLeavesAnOddNumberOfFencesAlone(t *testing.T) {
	page := "```\nbegin\n```\nand then\n```\nend"
	if got := Tidy(page); got != page {
		t.Errorf("a page with three fences came out as %q", got)
	}
}

// The preamble rule has to be narrower than it looks, because a paper writes
// these sentences too. The colon and the line of its own are what tell them
// apart.
func TestTidyDoesNotEatASentenceOfThePaper(t *testing.T) {
	for _, page := range []string{
		"Here is a result which is used repeatedly in what follows. Let $R$ be a relation.",
		"The following is proved in Section 3:\n\nEvery relation in normal form has a primary key.",
		"Below is the line at which the two curves meet, and above it they diverge.",
		"Let me know the degree of the relation and I will tell you the arity.",
	} {
		if got := Tidy(page); got != page {
			t.Errorf("%q was trimmed to %q", page, got)
		}
	}
}

// A page that is nothing but wrapping comes out empty, and empty is what rule
// A1 refuses. Tidy is not allowed to have an opinion about that; it just has
// to leave the page in a state where the rule can see it.
func TestAPageThatIsAllWrappingComesOutEmpty(t *testing.T) {
	if got := Tidy("```\n```"); got != "" {
		t.Errorf("an empty fence came out as %q", got)
	}
	var c Checker
	if faults := c.Check(1, Tidy("```\n```")); len(faults) == 0 || faults[0].Rule != A1 {
		t.Errorf("an empty page was not refused by A1: %v", faults)
	}
}

// A refusal is not packaging and must survive, because A1 is the rule that
// tells a page nobody read from a page that came back short.
func TestTidyKeepsARefusalWhereTheRulesCanSeeIt(t *testing.T) {
	text := Tidy("I'm sorry, I cannot transcribe this image.")
	var c Checker
	faults := c.Check(1, text)
	if len(faults) == 0 || faults[0].Rule != A1 {
		t.Errorf("an apology came through as %q with faults %v", text, faults)
	}
}

// The offer at the foot of a page, which the last two pages of the MapReduce
// paper came back with and which took the printed page number into the
// corpus with it: the folio was the line above the offer, so it was no longer
// the last line of the page and the furniture pass left it alone.
func TestTidyTakesOffAnOfferThatIsNotAddressedToAnybody(t *testing.T) {
	for _, c := range []struct {
		what string
		in   string
		want string
	}{
		{
			"an offer with no me in it",
			"## A Word Frequency\n\n149\n\nWould you like a concise explanation of how this example works?",
			"## A Word Frequency\n\n149",
		},
		{
			"two courtesies, one under the other",
			"308\n\nWould you like the next page?\nI hope this helps.",
			"308",
		},
		{
			"an offer to do more work",
			"## 1. Introduction\n\nDo you want me to transcribe page 2 as well?",
			"## 1. Introduction",
		},
	} {
		if got := Tidy(c.in); got != c.want {
			t.Errorf("%s: Tidy gave %q, want %q", c.what, got, c.want)
		}
	}
}

// A page of a paper that ends on one of these sentences keeps it. The test
// above works because the sentence is the whole of the last line, and a
// paper writes it in the middle of a paragraph.
func TestTidyKeepsAnOfferAPaperPrinted(t *testing.T) {
	const page = "The system then asks the user: would you like to see the next ten results, or refine the query?"
	if got := Tidy(page); got != page {
		t.Errorf("a sentence of the paper was trimmed to %q", got)
	}
}
