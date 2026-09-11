package refs

import (
	"strings"
	"testing"
)

func parse(texts ...string) *Result { return Parse(doc(texts...).Paragraphs) }

func keys(r *Result) string { return strings.Join(r.Keys(), " ") }

func TestABracketedBibliographyIsCutAtItsLabels(t *testing.T) {
	r := parse(
		`[1] A. Nkemelu, "A theory of slow indexes," Journal of Made Up Results, pages 1-12, 1991.`,
		`[2] B. Oyelaran and C. Fairweather, "Indexes that are slower still," In Proceedings of Nowhere, 1994.`,
		`[3] D. Ravensworth, "The last word on indexes," Technical Report 4, 1996.`,
	)
	if r.Style != StyleBracket {
		t.Fatalf("read the style as %s", r.Style)
	}
	if got := keys(r); got != "1 2 3" {
		t.Errorf("the keys are %q", got)
	}
	if got := r.Entries[0].Title; got != "A theory of slow indexes" {
		t.Errorf("entry 1 has the title %q", got)
	}
}

func TestAnEntryBrokenAcrossParagraphsIsPutBackTogether(t *testing.T) {
	// This is the usual shape. The hanging indent of a reference list looks
	// like a paragraph break to the extractor, so an entry arrives in
	// pieces and only the first piece carries the label.
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. Journal of Made Up",
		"Results, pages 1-12, 1991.",
		"[2] B. Oyelaran. Indexes that are slower still. In Proceedings of",
		"Nowhere, 1994.",
		"[3] D. Ravensworth. The last word on indexes. Technical Report 4, 1996.",
	)
	if got := keys(r); got != "1 2 3" {
		t.Fatalf("the keys are %q", got)
	}
	want := "A. Nkemelu. A theory of slow indexes. Journal of Made Up Results, pages 1-12, 1991."
	if got := r.Entries[0].Raw; got != want {
		t.Errorf("entry 1 reads\n%q\nwant\n%q", got, want)
	}
}

func TestSeveralEntriesInOneParagraphAreCutApart(t *testing.T) {
	// The other shape, from a single column paper: the extractor runs the
	// whole reference list into a handful of paragraphs and the labels are
	// in the middle of them.
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. 1991. [2] B. Oyelaran. Indexes that are slower still. 1994.",
		"[3] D. Ravensworth. The last word on indexes. 1996. [4] E. Sandoval. Indexes reconsidered. 1999.",
	)
	if got := keys(r); got != "1 2 3 4" {
		t.Fatalf("the keys are %q", got)
	}
	if got := r.Entries[1].Raw; got != "B. Oyelaran. Indexes that are slower still. 1994." {
		t.Errorf("entry 2 reads %q", got)
	}
}

func TestAWordBrokenAcrossTheParagraphBreakIsHealed(t *testing.T) {
	r := parse(
		"[1] A. Nkemelu. A theory of slow index-",
		"ing. Journal of Made Up Results, 1991.",
		"[2] B. Oyelaran. Something else entirely. 1994.",
		"[3] D. Ravensworth. A third thing. 1996.",
	)
	if got := r.Entries[0].Title; got != "A theory of slow indexing" {
		t.Errorf("the title reads %q", got)
	}
}

func TestANumberedBibliographyIsCutAtItsLabels(t *testing.T) {
	r := parse(
		"1. A. Nkemelu. A theory of slow indexes. Journal of Made Up Results, 1991.",
		"2. B. Oyelaran. Indexes that are slower still. In Proceedings of Nowhere, 1994.",
		"3. D. Ravensworth. The last word on indexes. Technical Report 4, 1996.",
	)
	if r.Style != StyleNumber {
		t.Fatalf("read the style as %s", r.Style)
	}
	if got := keys(r); got != "1 2 3" {
		t.Errorf("the keys are %q", got)
	}
}

func TestAnAuthorYearBibliographyIsCutAtTheAuthors(t *testing.T) {
	r := parse(
		"Nkemelu, A. (1991). A theory of slow indexes. Journal of Made Up Results, 3, 1-12.",
		"Oyelaran, B. and Fairweather, C. (1994). Indexes that are slower still. Proceedings of Nowhere.",
		"Ravensworth, D. (1996). The last word on indexes. Technical Report 4.",
	)
	if r.Style != StyleAuthorYear {
		t.Fatalf("read the style as %s", r.Style)
	}
	if got := keys(r); got != "Nkemelu 1991 Oyelaran 1994 Ravensworth 1996" {
		t.Errorf("the keys are %q", got)
	}
	e := r.Entries[0]
	if e.Title != "A theory of slow indexes" || e.Year != 1991 {
		t.Errorf("entry 1 parsed as %+v", e)
	}
	if len(e.Authors) != 1 || e.Authors[0] != "A. Nkemelu" {
		t.Errorf("the authors are %v", e.Authors)
	}
	// The authors are part of the entry in this style and the label is not
	// in front of it, so raw has to start where the paper starts.
	if !strings.HasPrefix(e.Raw, "Nkemelu, A. (1991).") {
		t.Errorf("raw reads %q", e.Raw)
	}
}

func TestANumberInTheMiddleOfAnEntryIsNotALabel(t *testing.T) {
	// Every part of this is a number followed by a full stop and a space,
	// and none of them carries on the count.
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. In Proc. 1980 Symposium on Indexing, vol. 2. Pages 122-133, April 1991.",
		"[2] B. Oyelaran. Indexes that are slower still. 1994.",
		"[3] D. Ravensworth. The last word on indexes. 1996.",
	)
	if got := keys(r); got != "1 2 3" {
		t.Errorf("the keys are %q", got)
	}
}

func TestALabelThatDoesNotCarryOnTheCountIsNotALabel(t *testing.T) {
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. 1991.",
		"[2] B. Oyelaran. See also [1] and the discussion there. 1994.",
		"[3] D. Ravensworth. The last word on indexes. 1996.",
	)
	if got := keys(r); got != "1 2 3" {
		t.Fatalf("the keys are %q", got)
	}
	if !strings.Contains(r.Entries[1].Raw, "[1] and the discussion") {
		t.Errorf("entry 2 lost its own text: %q", r.Entries[1].Raw)
	}
}

func TestAnEntryTheExtractorLostIsReportedAsAGap(t *testing.T) {
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. 1991.",
		"[2] B. Oyelaran. Indexes that are slower still. 1994.",
		"[4] E. Sandoval. Indexes reconsidered. 1999.",
		"[5] F. Turnbull. Indexes at last. 2001.",
	)
	if got := keys(r); got != "1 2 4 5" {
		t.Fatalf("the keys are %q", got)
	}
	if len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "skips 3") {
		t.Errorf("the notes are %v", r.Notes)
	}
}

func TestABibliographyWithNoLabelsIsReadOnePerParagraph(t *testing.T) {
	r := parse(
		"A. Nkemelu. A theory of slow indexes. Journal of Made Up Results, 1991.",
		"B. Oyelaran. Indexes that are slower still. In Proceedings of Nowhere, 1994.",
		"D. Ravensworth. The last word on indexes. Technical Report 4, 1996.",
	)
	if r.Style != StyleHanging {
		t.Fatalf("read the style as %s", r.Style)
	}
	if got := keys(r); got != "1 2 3" {
		t.Errorf("the keys are %q", got)
	}
	if len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "no labels") {
		t.Errorf("the notes are %v", r.Notes)
	}
}

func TestTwoLabelsAreNotEnoughToBeAStyle(t *testing.T) {
	// Two is a coincidence. Reading these as a bracketed bibliography would
	// throw away the third entry, which has no label at all.
	r := parse(
		"[1] A. Nkemelu. A theory of slow indexes. 1991.",
		"[2] B. Oyelaran. Indexes that are slower still. 1994.",
	)
	if r.Style != StyleHanging {
		t.Errorf("read the style as %s with %d entries", r.Style, len(r.Entries))
	}
}

func TestTheTextBeforeTheFirstEntryIsDropped(t *testing.T) {
	// A running head the extractor did not catch, sitting between the
	// heading and the first reference.
	r := parse(
		"Nkemelu and Oyelaran 14",
		"[1] A. Nkemelu. A theory of slow indexes. 1991.",
		"[2] B. Oyelaran. Indexes that are slower still. 1994.",
		"[3] D. Ravensworth. The last word on indexes. 1996.",
	)
	if got := keys(r); got != "1 2 3" {
		t.Fatalf("the keys are %q", got)
	}
	if strings.Contains(r.Entries[0].Raw, "Nkemelu and Oyelaran 14") {
		t.Errorf("entry 1 kept the running head: %q", r.Entries[0].Raw)
	}
}

func TestAnAlphabeticBracketKeyIsKept(t *testing.T) {
	r := parse(
		"[Nke91] A. Nkemelu. A theory of slow indexes. 1991.",
		"[Oye94] B. Oyelaran. Indexes that are slower still. 1994.",
		"[Rav96] D. Ravensworth. The last word on indexes. 1996.",
	)
	if got := keys(r); got != "Nke91 Oye94 Rav96" {
		t.Errorf("the keys are %q", got)
	}
}

func TestRawIsAlwaysKept(t *testing.T) {
	// Rule R03. Whatever the field parse makes of an entry, the entry as
	// printed survives, because that is what the page renders.
	r := parse(
		"[1] ??? --- nothing here parses at all",
		"[2] B. Oyelaran. Indexes that are slower still. 1994.",
		"[3] D. Ravensworth. The last word on indexes. 1996.",
	)
	for _, e := range r.Entries {
		if strings.TrimSpace(e.Raw) == "" {
			t.Errorf("entry %s has no raw text", e.Key)
		}
	}
	if r.Entries[0].Raw != "??? --- nothing here parses at all" {
		t.Errorf("entry 1 reads %q", r.Entries[0].Raw)
	}
}

func TestAnEmptyBibliographyParsesIntoNothing(t *testing.T) {
	r := Parse(nil)
	if len(r.Entries) != 0 || len(r.Notes) != 0 {
		t.Errorf("got %d entries and %v", len(r.Entries), r.Notes)
	}
}
