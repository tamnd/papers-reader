package report

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// sourcesYAML matches the two papers in papersYAML. The first is open and
// has a text layer, the second is restricted and does not.
const sourcesYAML = `sources:
  - id: vaswani-2017-attention
    access: open
    licence: CC-BY-4.0
    url: https://example.org/a.pdf
    text_layer: native
  - id: codd-1970-relational
    access: restricted
    licence: all-rights-reserved
    url: https://example.org/b.pdf
    text_layer: none
`

func coverage(t *testing.T, files map[string]string) *Coverage {
	t.Helper()
	cov, err := BuildCoverage(corpusOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return cov
}

// paper is one row out of the report, by id.
func paper(t *testing.T, cov *Coverage, id string) CoveragePaper {
	t.Helper()
	for _, p := range cov.Papers {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("there is no %s in %+v", id, cov.Papers)
	return CoveragePaper{}
}

func TestARestrictedPaperWithItsAbstractIsDone(t *testing.T) {
	// The whole of what the licence allows is the front matter and an
	// abstract. Counting that as a shortfall would be a report that can
	// never reach the top of its own scale, and it would put work on the
	// list that nobody is allowed to do.
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":                      sourcesYAML,
		"content/en/codd-1970-relational/00_front.md": front("codd-1970-relational", "1", "native", "pdftotext 25"),
	})
	codd := paper(t, cov, "codd-1970-relational")
	if codd.State != Stub {
		t.Errorf("the state is %s, want stub", codd.State)
	}
	if codd.Why != "" {
		t.Errorf("a restricted paper with its abstract is waiting on %q", codd.Why)
	}
	if !strings.Contains(cov.Markdown(), "| databases | 1 | 0 | 1 | 0 | 100% |") {
		t.Errorf("the field is not counted as done:\n%s", cov.Markdown())
	}
}

func TestAnOpenPaperWithOnlyItsFrontMatterIsNotDone(t *testing.T) {
	// Same files on disk as the restricted one, and a different answer,
	// because the licence is what says whether there is more to come.
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":                        sourcesYAML,
		"pdf/vaswani-2017-attention.pdf":                "%PDF-1.4\n",
		"content/en/vaswani-2017-attention/00_front.md": front("vaswani-2017-attention", "1", "native", "pdftotext 25"),
	})
	p := paper(t, cov, "vaswani-2017-attention")
	if p.State != Stub {
		t.Errorf("the state is %s, want stub", p.State)
	}
	if p.Why != "waiting on extraction, which needs no model" {
		t.Errorf("it is waiting on %q", p.Why)
	}
}

func TestAPaperNobodyRecordedASourceForPublishesNothing(t *testing.T) {
	// No sources manifest at all, which is the corpus on the day it is
	// started. Nothing may be published from a paper nobody has checked the
	// licence of, so that is the row and not a path or a tool.
	cov := coverage(t, nil)
	p := paper(t, cov, "vaswani-2017-attention")
	if p.Why != "nothing is known about what may be published from it" {
		t.Errorf("it is waiting on %q", p.Why)
	}
	if p.State != None || cov.Total.None != 2 {
		t.Errorf("%+v and %d none in total", p, cov.Total.None)
	}
}

func TestAPaperThatIsNotFetchedIsWaitingOnTheFetchAndNotOnAModel(t *testing.T) {
	// There is no PDF, so naming the tool that would read it would send
	// somebody to run the wrong command.
	cov := coverage(t, map[string]string{"manifests/sources.yaml": sourcesYAML})
	if got := paper(t, cov, "vaswani-2017-attention").Why; got != "not fetched yet" {
		t.Errorf("it is waiting on %q", got)
	}
}

func TestThePathAPaperIsWaitingOnComesFromItsTextLayer(t *testing.T) {
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":         sourcesYAML,
		"pdf/vaswani-2017-attention.pdf": "%PDF-1.4\n",
		"pdf/codd-1970-relational.pdf":   "%PDF-1.4\n",
	})
	// A paper with no text layer at all is a scan, and a scan is read by a
	// vision model. Restricted or not: the licence decides what may be
	// published, and the text layer decides what would have to read it.
	if got := paper(t, cov, "codd-1970-relational").Why; got != "waiting on a vision model" {
		t.Errorf("the scan is waiting on %q", got)
	}
	if got := paper(t, cov, "vaswani-2017-attention").Why; got != "waiting on extraction, which needs no model" {
		t.Errorf("the native paper is waiting on %q", got)
	}
	// The reasons are counted, because the number of papers behind one
	// missing tool is the number that decides whether to go and get it.
	if len(cov.Waiting) != 2 {
		t.Fatalf("%d reasons, want 2: %+v", len(cov.Waiting), cov.Waiting)
	}
	if !strings.Contains(cov.Markdown(), "| waiting on a vision model | 1 |") {
		t.Errorf("the reasons are not in the report:\n%s", cov.Markdown())
	}
}

func TestAPaperWithSectionsIsFull(t *testing.T) {
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":                            sourcesYAML,
		"content/en/vaswani-2017-attention/00_front.md":     front("vaswani-2017-attention", "1", "native", "pdftotext 25"),
		"content/en/vaswani-2017-attention/01_intro.md":     front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
		"content/en/vaswani-2017-attention/02_attention.md": front("vaswani-2017-attention", "5", "native", "pdftotext 25"),
	})
	p := paper(t, cov, "vaswani-2017-attention")
	if p.State != Full || p.Sections != 3 || p.Why != "" {
		t.Errorf("%+v, want a full paper of 3 sections waiting on nothing", p)
	}
	if cov.Total.Full != 1 {
		t.Errorf("%d full, want the one", cov.Total.Full)
	}
}

func TestATranslationOfAnEnglishThatMovedOnIsStale(t *testing.T) {
	// The one number in the language table that means somebody has work to
	// do rather than work to start. The Vietnamese records the hash of the
	// English it was made from, and the English has been re-extracted since.
	english := "---\npaper: vaswani-2017-attention\nkind: section\nlang: en\ncontent_sha256: aaaa\n---\n\nThe new English.\n"
	fresh := "---\npaper: vaswani-2017-attention\nkind: section\nlang: vi\nsource_content_sha256: aaaa\n---\n\nBan dich.\n"
	old := "---\npaper: vaswani-2017-attention\nkind: section\nlang: vi\nsource_content_sha256: bbbb\n---\n\nBan dich cu.\n"
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":                        sourcesYAML,
		"content/en/vaswani-2017-attention/01_intro.md": english,
		"content/en/vaswani-2017-attention/02_next.md":  english,
		"content/vi/vaswani-2017-attention/01_intro.md": fresh,
		"content/vi/vaswani-2017-attention/02_next.md":  old,
	})
	vi := lang(t, cov, corpus.VI)
	if vi.Papers != 1 || vi.Files != 2 {
		t.Errorf("%+v, want 2 files of 1 paper", vi)
	}
	if vi.Stale != 1 {
		t.Errorf("%d stale, want the one whose English moved on", vi.Stale)
	}
	if !strings.Contains(cov.Markdown(), "| vi | 1 | 2 | 1 |") {
		t.Errorf("the language table is:\n%s", cov.Markdown())
	}
}

func TestATranslationThatDoesNotSayWhatItCameFromIsNotCalledStale(t *testing.T) {
	// It is not stale and it is not fresh either. Nobody can say, and a
	// report that guesses either way is worse than the audit rule that
	// objects to the missing field.
	cov := coverage(t, map[string]string{
		"manifests/sources.yaml":                        sourcesYAML,
		"content/en/vaswani-2017-attention/01_intro.md": front("vaswani-2017-attention", "2-4", "native", "pdftotext 25"),
		"content/ja/vaswani-2017-attention/01_intro.md": "---\npaper: vaswani-2017-attention\nkind: section\nlang: ja\n---\n\nHonyaku.\n",
	})
	ja := lang(t, cov, corpus.JA)
	if ja.Files != 1 || ja.Stale != 0 {
		t.Errorf("%+v, want one file nobody can call stale", ja)
	}
}

func TestEveryLanguageIsARowEvenTheOnesNobodyHasStarted(t *testing.T) {
	cov := coverage(t, nil)
	if len(cov.Langs) != len(corpus.Langs) {
		t.Fatalf("%d languages, want %d", len(cov.Langs), len(corpus.Langs))
	}
	for i, want := range corpus.Langs {
		if cov.Langs[i].Lang != want {
			t.Errorf("language %d is %s, want %s", i, cov.Langs[i].Lang, want)
		}
	}
}

func TestTheFieldsComeInCatalogueOrderAndEmptyOnesAreLeftOut(t *testing.T) {
	// The catalogue is ordered so that a reader can follow it, and a report
	// that re-sorts it alphabetically loses that. A field with no papers in
	// this corpus is not a row, because a row of zeroes over zero is not a
	// fact about anything.
	cov := coverage(t, map[string]string{"manifests/sources.yaml": sourcesYAML})
	if len(cov.Fields) != 2 {
		t.Fatalf("%d fields, want the 2 with papers in them: %+v", len(cov.Fields), cov.Fields)
	}
	first, second := -1, -1
	for i, f := range corpus.Fields {
		switch f {
		case cov.Fields[0].Field:
			first = i
		case cov.Fields[1].Field:
			second = i
		}
	}
	if first < 0 || second < 0 || first > second {
		t.Errorf("the fields are %s then %s, which is not catalogue order", cov.Fields[0].Field, cov.Fields[1].Field)
	}
}

func TestAnEmptyCorpusIsZeroPerCentAndNotACrash(t *testing.T) {
	cov := coverage(t, nil)
	if cov.Total.Done() != 0 {
		t.Errorf("an empty corpus is %v done", cov.Total.Done())
	}
	// Two papers in the manifest and nothing published of either, so both
	// are none and the summary still adds up to the manifest.
	if got := cov.Summary(); got != "2 papers: 0 full, 0 stub, 2 none" {
		t.Errorf("the summary is %q", got)
	}
	md := cov.Markdown()
	if !strings.Contains(md, "| **all** | 2 | 0 | 0 | 2 | 0% |") {
		t.Errorf("the total row is:\n%s", md)
	}
	if !strings.Contains(md, "## What is left") {
		t.Errorf("nothing is done and nothing is listed as left:\n%s", md)
	}
}

func TestAPerCentIsAWholeNumberOfPapers(t *testing.T) {
	for _, c := range []struct {
		f CoverageField
		s string
	}{
		{CoverageField{}, "0%"},
		{CoverageField{Papers: 100, Full: 41}, "41%"},
		{CoverageField{Papers: 100, Full: 40, Stub: 1}, "41%"},
		{CoverageField{Papers: 3, Full: 1}, "33%"},
		{CoverageField{Papers: 3, Full: 2}, "67%"},
		{CoverageField{Papers: 7, Full: 7}, "100%"},
	} {
		if got := percent(c.f.Done()); got != c.s {
			t.Errorf("%+v is %s done, want %s", c.f, got, c.s)
		}
	}
}

func lang(t *testing.T, cov *Coverage, want corpus.Lang) CoverageLang {
	t.Helper()
	for _, l := range cov.Langs {
		if l.Lang == want {
			return l
		}
	}
	t.Fatalf("there is no %s in %+v", want, cov.Langs)
	return CoverageLang{}
}
