package extract

import (
	"strings"
	"testing"
)

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

// Page 10 of the MapReduce paper came back with the relay's own button on
// the end of it. That is not a courtesy and it is not the page, and it cost
// the page its running head, because the furniture pass takes one head off
// an edge and the button took its turn.
func TestTheRelaysOwnInterfaceComesOffTheFootOfThePage(t *testing.T) {
	const page = "One of our most significant uses of MapReduce to date has been a\ncomplete rewrite of the production indexing system.\n\n146\n\n Give feedback\n"
	got := Tidy(page)
	if strings.Contains(got, "Give feedback") {
		t.Errorf("the button is still on the page:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "146") {
		t.Errorf("the folio is no longer the last line:\n%s", got)
	}
}

// Matched whole and not as a prefix, because these are two words and a paper
// about interfaces will one day write them.
func TestASentenceThatStartsLikeTheInterfaceStays(t *testing.T) {
	const page = "A paragraph about what the system does.\n\nGive feedback to the user before the request completes.\n"
	if got := Tidy(page); !strings.Contains(got, "Give feedback to the user") {
		t.Errorf("a sentence of the paper came off:\n%s", got)
	}
}

// Two pages of the MapReduce paper, and the number in each is the page
// number the proceedings printed. Both were published with the number on
// them, because the question underneath it was the last line of the page and
// the folio was the second last.
func TestTheRelayAsksItsOwnQuestionsAtTheFootOfThePage(t *testing.T) {
	for _, page := range []string{
		"The reduce function is passed all per-document term vectors.\n\n138\n\n Is this conversation helpful so far?\n",
		"The Map invocations are distributed across multiple machines.\n\n139\n\n Do you like this personality?\n",
	} {
		got := strings.TrimSpace(Tidy(page))
		if strings.Contains(got, "?") {
			t.Errorf("the question is still on the page:\n%s", got)
		}
		if !strings.HasSuffix(got, "9") && !strings.HasSuffix(got, "8") {
			t.Errorf("the folio is no longer the last line:\n%s", got)
		}
	}
}

// The first line of a page is the running head, and the furniture pass
// counts first lines across the paper to find it. A progress line standing
// where the head should be is a head the pass never sees.
func TestTheRelaysProgressLineComesOffTheHeadOfThePage(t *testing.T) {
	const page = "Worked for 9s\n\nFigure 1: Execution overview\n\nInverted Index: The map function parses each document.\n"
	got := Tidy(page)
	if strings.Contains(got, "Worked for") {
		t.Errorf("the progress line is still on the page:\n%s", got)
	}
	if !strings.HasPrefix(got, "Figure 1:") {
		t.Errorf("the page does not start where it should:\n%s", got)
	}
}

// A progress line and an announcement are two lines above the page, and
// taking one off has to leave none.
func TestAProgressLineAndAnAnnouncementBothComeOff(t *testing.T) {
	const page = "Thought for 12 seconds\nHere is the transcription of the page:\n\nThe computation takes a set of input key/value pairs.\n"
	if got := Tidy(page); !strings.HasPrefix(got, "The computation takes") {
		t.Errorf("the page does not start where it should:\n%s", got)
	}
}

// A sentence of a paper that begins the way the progress line does. The
// match is the whole line and ends on a duration, so a paper that says how
// long something took keeps saying it.
func TestASentenceThatStartsLikeTheProgressLineStays(t *testing.T) {
	const page = "Worked for 9s on the first shard, the machine then failed and the task was\nreassigned to another worker.\n"
	if got := Tidy(page); !strings.HasPrefix(got, "Worked for 9s on the first shard") {
		t.Errorf("a sentence of the paper came off:\n%s", got)
	}
}

// The relay draws its buttons side by side and the tool picks them up with
// nothing between them. The re-read of page 13 of the MapReduce paper came
// back ending "Give feedbackDo you like this personality?".
func TestTwoButtonsRunTogetherAreStillButtons(t *testing.T) {
	const page = "[18] Jim Wyllie. Spsort: How to sort a terabyte quickly.\n\n Give feedbackDo you like this personality?\n"
	got := strings.TrimSpace(Tidy(page))
	if !strings.HasSuffix(got, "terabyte quickly.") {
		t.Errorf("the buttons are still on the page:\n%s", got)
	}
}

// The whole of the line has to be chrome. A sentence that ends on one of the
// phrases has words in front of it and keeps them.
func TestALineThatIsOnlyPartlyChromeStays(t *testing.T) {
	const page = "A paragraph about what the system does.\n\nReviewers give feedback\n"
	if got := Tidy(page); !strings.Contains(got, "Reviewers give feedback") {
		t.Errorf("a sentence of the paper came off:\n%s", got)
	}
}
