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

// The shape a 1970 journal sets a numbered bibliography in: surnames first
// and in small capitals, the word between two authors in small capitals with
// them, the venue abbreviated to within an inch of its life, and the page
// range at the end with nothing to say that is what it is.
func TestANumberedBibliographyInTheOlderJournalStyle(t *testing.T) {
	r := parse(
		"1. ASHWORTH, P. K. A set-theoretic store for tuples. Proc. Summer Meeting of the",
		"Soc. for Machine Filing, Providence, R.I., July 1968, pp. 12-31.",
		"2. BRIGHTWELL, M. T., AND DUNNE, R. Q. On the composition of binary relations.",
		"J. Assoc. Comput. Mach. 15, 2 (Apr. 1968), 201-215.",
		"3. CHALMERS, W. E. Notation for a stored file of ordered pairs. Comm. ACM 12,",
		"9 (Sept. 1969), 501-507.",
	)
	if r.Style != StyleNumber {
		t.Fatalf("read the style as %s", r.Style)
	}
	if got := keys(r); got != "1 2 3" {
		t.Fatalf("the keys are %q", got)
	}
	second := r.Entries[1]
	if got := strings.Join(second.Authors, "; "); got != "M. T. BRIGHTWELL; R. Q. DUNNE" {
		t.Errorf("entry 2 has the authors %q", got)
	}
	if second.Title != "On the composition of binary relations" {
		t.Errorf("entry 2 has the title %q", second.Title)
	}
	if second.Year != 1968 || second.Pages != "201-215" {
		t.Errorf("entry 2 is dated %d at pages %q", second.Year, second.Pages)
	}
	if got := r.Entries[0].Pages; got != "12-31" {
		t.Errorf("entry 1 has the pages %q", got)
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

func TestABracketedWordIsNotAnEntryLabel(t *testing.T) {
	// The BERT paper's appendix repeats the masked language model examples,
	// and its bracketed tokens were read as the labels of a bibliography
	// seventeen entries long, none of which was a reference.
	r := parse(
		"The input uses the [CLS] token first.",
		"A word is replaced by the [MASK] token.",
		"The two sentences are divided by the [SEP] token.",
		"A fourth sentence with the [CLS] token again.",
	)

	if r.Style == StyleBracket {
		t.Fatalf("the style is %s and none of those brackets is a label", r.Style)
	}
	for _, e := range r.Entries {
		switch e.Key {
		case "CLS", "MASK", "SEP":
			t.Errorf("entry %q is a token of the paper's input and not a reference", e.Key)
		}
	}
}

func TestAnInitialsAndYearLabelIsStillAnEntryLabel(t *testing.T) {
	r := parse(
		"[Sha48] C. Shannon. A mathematical theory of nothing. 1948.",
		"[BL04] B. Lee and C. Lee. Another paper that does not exist. 2004.",
		"[AB+12] A. Bee, C. Dee, and E. Eff. A third one. 2012.",
	)

	if r.Style != StyleBracket {
		t.Fatalf("the style is %s and the labels are brackets", r.Style)
	}
	want := []string{"Sha48", "BL04", "AB+12"}
	if len(r.Entries) != len(want) {
		t.Fatalf("the parse found %d entries and the list has %d", len(r.Entries), len(want))
	}
	for i, key := range want {
		if r.Entries[i].Key != key {
			t.Errorf("entry %d is keyed %q and the paper keyed it %q", i+1, r.Entries[i].Key, key)
		}
	}
}

// A candidate label that is rejected must not take the separator the next
// one needs with it. The journal prints its own name and volume at the foot
// of every page, so an entry can end in "Not. 21, 7." and the "7." was
// eating the newline in front of the entry after it.
func TestALabelSurvivesARejectedOneInFrontOfIt(t *testing.T) {
	r := parse(
		"1. Aarons, P. On the first of the invented papers. Journal of Nothing 1, 1 (1970), 1-10. Published as Notices 21, 7.",
		"2. Beacham, Q. On the second of the invented papers. Journal of Nothing 1, 2 (1971), 11-20. Published as Notices 21, 7.",
		"3. Coleridge, R. On the third of the invented papers. Journal of Nothing 2, 1 (1972), 21-30.",
		"4. Danforth, S. On the fourth of the invented papers. Journal of Nothing 2, 2 (1973), 31-40.",
	)
	if len(r.Entries) != 4 {
		t.Fatalf("the parse found %d entries, want 4: %v", len(r.Entries), keys(r))
	}
	for i, want := range []string{"1", "2", "3", "4"} {
		if r.Entries[i].Key != want {
			t.Errorf("entry %d is keyed %q, want %q", i, r.Entries[i].Key, want)
		}
	}
}

// The page sets some of its labels tight against the entry and the reader
// transcribes what it sees, so an entry can begin "[7]J. Martin" with no
// space in it. Losing one of those costs the entry after it as well, since
// that one then fails the test that a label carries on the count.
func TestALabelWithNoSpaceAfterItIsStillALabel(t *testing.T) {
	r := parse(
		"[1] Aarons, P. On the first of the invented papers. Journal of Nothing 1, 1 (1970), 1-10.",
		"[2]Beacham, Q. On the second of the invented papers. Journal of Nothing 1, 2 (1971), 11-20.",
		"[3] Coleridge, R. On the third of the invented papers. Journal of Nothing 2, 1 (1972), 21-30.",
		"[4]\"On the fourth of the invented papers\", by S. Danforth. Journal of Nothing 2, 2 (1973), 31-40.",
	)
	if len(r.Entries) != 4 {
		t.Fatalf("the parse found %d entries, want 4: %v", len(r.Entries), keys(r))
	}
	for i, want := range []string{"1", "2", "3", "4"} {
		if r.Entries[i].Key != want {
			t.Errorf("entry %d is keyed %q, want %q", i, r.Entries[i].Key, want)
		}
	}
	if got := r.Entries[1].Raw; !strings.HasPrefix(got, "Beacham") {
		t.Errorf("entry 2 reads %q, want it to start at the author", got)
	}
}

// Without the separator there has to be something else saying the label is
// one, and that is the author after it. A number in brackets with a digit
// or a piece of punctuation against it is part of what it is written in.
func TestABracketRunningIntoSomethingElseIsNotALabel(t *testing.T) {
	r := parse(
		"[1] Aarons, P. On the first of the invented papers. Distributed over [2]-[3] of the series.",
		"[2] Beacham, Q. On the second of the invented papers. Journal of Nothing 1, 2 (1971), 11-20.",
	)
	if len(r.Entries) != 2 {
		t.Fatalf("the parse found %d entries, want 2: %v", len(r.Entries), keys(r))
	}
	if got := r.Entries[0].Raw; !strings.HasSuffix(got, "of the series.") {
		t.Errorf("entry 1 reads %q, want the whole of it", got)
	}
}

// A numbered list can lose the point after a number the same way a
// bracketed one loses the space, and it costs the entry after it too.
func TestANumberWithNoPointAfterItIsStillALabel(t *testing.T) {
	r := parse(
		"1. Aarons, P. On the first of the invented papers. Journal of Nothing 1, 1 (1970), 1-10.",
		"2. Beacham, Q. On the second of the invented papers. Journal of Nothing 1, 2 (1971), 11-20.",
		"3 Coleridge, R. On the third of the invented papers. Journal of Nothing 2, 1 (1972), 21-30.",
		"4. Danforth, S. On the fourth of the invented papers. Journal of Nothing 2, 2 (1973), 31-40.",
	)
	if len(r.Entries) != 4 {
		t.Fatalf("the parse found %d entries, want 4: %v", len(r.Entries), keys(r))
	}
	if got := r.Entries[2].Raw; !strings.HasPrefix(got, "Coleridge") {
		t.Errorf("entry 3 reads %q, want it to start at the author", got)
	}
}

// An ordinal in the middle of an entry is the shape a number with no point
// after it would otherwise be read as, and the venue of a conference paper
// is full of them.
func TestAnOrdinalInTheMiddleOfAnEntryIsNotALabel(t *testing.T) {
	r := parse(
		"1. Aarons, P. On the first of the invented papers. In Proceedings of the 2nd Symposium on Nothing (Springfield, 1970).",
		"2. Beacham, Q. On the second of the invented papers. In Proceedings of the 3rd Symposium on Nothing (Springfield, 1971).",
		"3. Coleridge, R. On the third of the invented papers. Journal of Nothing 2, 1 (1972), 21-30.",
	)
	if len(r.Entries) != 3 {
		t.Fatalf("the parse found %d entries, want 3: %v", len(r.Entries), keys(r))
	}
	if got := r.Entries[0].Raw; !strings.HasSuffix(got, "(Springfield, 1970).") {
		t.Errorf("entry 1 reads %q, want the whole of it", got)
	}
}
