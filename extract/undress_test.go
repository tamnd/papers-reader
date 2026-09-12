package extract

import "testing"

// All the page text in this file is typed here. None of it is copied from a
// paper, the same as everywhere else in these tests.

func undressed(t *testing.T, pages map[int]string) map[int]string {
	t.Helper()
	return Undress(pages)
}

// bodies are lines that share no wording with each other.
//
// A fixture cannot tell its pages apart by numbering them, because fold
// replaces a run of digits with a single #, so "page 4" and "page 5" are the
// same line as far as this is concerned. That is what the folio needs and it
// is right for a running foot that reads "page 5 of 12" as well, but it does
// mean the pages of a test paper have to differ in their words.
var bodies = []string{
	"A graph of the program is drawn.",
	"Each decision in it adds an edge.",
	"The count is one more than the decisions.",
	"A subroutine is counted on its own.",
	"The bound holds for any entry point.",
	"Testing follows the independent paths.",
}

// tails is a second set of them, for a fixture that needs a line at the
// bottom of a page as well as one at the top. Reusing bodies for both ends
// would put every line on two pages of a six page paper, which is a third
// of the paper, which is furniture.
var tails = []string{
	"The argument goes on over the page.",
	"A worked example follows it.",
	"Two of these are shown below.",
	"The proof is left to the appendix.",
	"This is what the tool reports.",
	"A table closes the section.",
}

func wantPage(t *testing.T, got map[int]string, page int, want string) {
	t.Helper()
	if got[page] != want {
		t.Errorf("page %d is\n%q\nwant\n%q", page, got[page], want)
	}
}

// The bug this was written for. The reading prompt asks for the printed page
// number on a line of its own and promises it is taken out later, and the
// front matter of a paper went out with the number on the first line of it.
func TestThePrintedNumberComesOffTheTopOfThePage(t *testing.T) {
	got := undressed(t, map[int]string{
		1: "308\n\nJOURNAL OF THINGS, VOL. 2, NO. 4\n\n# A Measure\n\nThe opening paragraph.\n",
		2: "TRENT: A MEASURE\n\n309\n\nThe second page.\n",
		3: "310\n\nJOURNAL OF THINGS\n\nThe third page.\n",
	})
	wantPage(t, got, 1, "JOURNAL OF THINGS, VOL. 2, NO. 4\n\n# A Measure\n\nThe opening paragraph.\n")
	wantPage(t, got, 3, "JOURNAL OF THINGS\n\nThe third page.\n")
}

// The head and the number are both furniture and a page can print both, in
// either order, which is why the edge is two lines deep and not one.
func TestTheRunningHeadAndTheNumberBothComeOff(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		pages[i] = "TRENT: A MEASURE\n\n" + string(rune('0'+i)) + "\n\n" + bodies[i-1] + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, bodies[i-1]+"\n")
	}
}

func TestAFootIsStrippedAsWellAsAHead(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		pages[i] = bodies[i-1] + "\n\nTRENT: A MEASURE\n\n" + string(rune('0'+i)) + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, bodies[i-1]+"\n")
	}
}

// A journal that sets one head on the recto and another on the verso puts
// each of them on half the pages and neither on more than half, so the
// threshold is a third, the same as FindFurniture uses.
func TestBothHeadsOfAJournalThatAlternatesAreFurniture(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		head := "TRENT: A MEASURE"
		if i%2 == 0 {
			head = "JOURNAL OF THINGS, MARCH 1976"
		}
		pages[i] = head + "\n\n" + bodies[i-1] + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, bodies[i-1]+"\n")
	}
}

// The head is matched on its folded text, so a page number that moves and a
// scan that spells the head differently from one page to the next do not
// each look like a line of their own.
func TestTheHeadIsMatchedOnItsFoldedText(t *testing.T) {
	pages := map[int]string{
		1: "308  Trent: A Measure\n\nBody one.\n",
		2: "309   TRENT: A MEASURE\n\nBody two.\n",
		3: "310 trent: a measure.\n\nBody three.\n",
		4: "311  Trent: A Measure\n\nBody four.\n",
	}
	got := undressed(t, pages)
	for i := 1; i <= 4; i++ {
		if got[i] != "Body "+[]string{"", "one", "two", "three", "four"}[i]+".\n" {
			t.Errorf("page %d is %q", i, got[i])
		}
	}
}

// Two pages that share a line share it by coincidence as often as not, so a
// short paper keeps its header. The folio still goes, because a bare number
// on its own line is a page number whatever the paper is.
func TestAShortPaperKeepsItsHeaderAndLosesItsNumber(t *testing.T) {
	got := undressed(t, map[int]string{
		1: "7\n\nA SHARED LINE\n\nBody one.\n",
		2: "8\n\nA SHARED LINE\n\nBody two.\n",
	})
	wantPage(t, got, 1, "A SHARED LINE\n\nBody one.\n")
	wantPage(t, got, 2, "A SHARED LINE\n\nBody two.\n")
}

// The first page of a paper pulled out of a journal carries the volume, the
// number and the month, and that line is on that page alone. It is the one
// place the reader is told where the paper appeared, so it stays.
func TestTheCitationOnTheFirstPageIsNotFurniture(t *testing.T) {
	pages := map[int]string{
		1: "308\n\nJOURNAL OF THINGS, VOL. 2, NO. 4, DECEMBER 1976\n\n# A Measure\n",
	}
	for i := 2; i <= 6; i++ {
		pages[i] = "TRENT: A MEASURE\n\n" + bodies[i-1] + "\n"
	}
	got := undressed(t, pages)
	wantPage(t, got, 1, "JOURNAL OF THINGS, VOL. 2, NO. 4, DECEMBER 1976\n\n# A Measure\n")
}

// Nothing between the two edges is looked at. A bare number three lines
// deep is a numbered list or an equation tag, not a folio, and it stays.
func TestNothingInTheBodyIsTouched(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		pages[i] = "TRENT: A MEASURE\n\n" + bodies[i-1] + "\n\n5\n\n" + tails[i-1] + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, bodies[i-1]+"\n\n5\n\n"+tails[i-1]+"\n")
	}
}

// Peeling stops at the first line that is not furniture, so a paper that
// prints a number and nothing else loses the number and keeps the rest.
func TestPeelingStopsAtTheFirstLineThatIsNotFurniture(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		pages[i] = "1" + string(rune('0'+i)) + "\n\n" + bodies[i-1] + "\n\n" + tails[i-1] + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, bodies[i-1]+"\n\n"+tails[i-1]+"\n")
	}
}

// At most one running head comes off an edge. Two repeated lines stacked at
// the top of a page are far likelier to be a heading set over two lines than
// two running heads, and there are no coordinates here to settle it.
func TestOnlyOneRepeatedLineComesOffAnEdge(t *testing.T) {
	pages := map[int]string{}
	for i := 1; i <= 6; i++ {
		pages[i] = "TRENT: A MEASURE\n\nPart Two: The Measure Itself\n\n" + bodies[i-1] + "\n"
	}
	got := undressed(t, pages)
	for i := 1; i <= 6; i++ {
		wantPage(t, got, i, "Part Two: The Measure Itself\n\n"+bodies[i-1]+"\n")
	}
}

func TestAPageWithNothingOnItSurvives(t *testing.T) {
	got := undressed(t, map[int]string{
		1: "Body one.\n",
		2: "\n\n\n",
		3: "Body three.\n",
	})
	wantPage(t, got, 2, "")
}

func TestNoPagesIsNoPages(t *testing.T) {
	if got := Undress(nil); len(got) != 0 {
		t.Errorf("Undress(nil) is %v, want nothing", got)
	}
}

// Roman numerals are page numbers in the front matter of a thesis, which is
// the one place in the hundred they are used that way.
func TestARomanFolioIsAFolio(t *testing.T) {
	got := undressed(t, map[int]string{
		1: "iv\n\nBody one.\n",
		2: "v\n\nBody two.\n",
		3: "vi\n\nBody three.\n",
	})
	wantPage(t, got, 1, "Body one.\n")
	wantPage(t, got, 3, "Body three.\n")
}
