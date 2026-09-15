package report

import (
	"strings"
	"testing"
)

// entry is one reference in a bibliography, with as much or as little of the
// fields as a test needs.
type entry struct {
	title, resolvesTo, doi, arxiv string
}

// refsFile is one manifests/refs/<id>.yaml written out of entries.
func refsFile(paper string, entries ...entry) string {
	var b strings.Builder
	b.WriteString("paper: " + paper + "\nstyle: numeric\nentries:\n")
	for i, e := range entries {
		b.WriteString("  - key: \"" + string(rune('1'+i)) + "\"\n")
		b.WriteString("    raw: " + e.title + ".\n")
		b.WriteString("    title: " + e.title + "\n")
		if e.doi != "" {
			b.WriteString("    doi: " + e.doi + "\n")
		}
		if e.arxiv != "" {
			b.WriteString("    arxiv: \"" + e.arxiv + "\"\n")
		}
		b.WriteString("    resolves_to: " + e.resolvesTo + "\n")
	}
	return b.String()
}

func suggested(t *testing.T, min int, files map[string]string) []Suggestion {
	t.Helper()
	all := map[string]string{"manifests/papers.yaml": graphPapers}
	for k, v := range files {
		all[k] = v
	}
	out, err := Suggest(corpusOf(t, all), min)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func only(t *testing.T, all []Suggestion) Suggestion {
	t.Helper()
	if len(all) != 1 {
		t.Fatalf("there are %d suggestions, want 1: %+v", len(all), all)
	}
	return all[0]
}

func TestAWorkTwoPapersCiteIsSuggestedAtTwoAndNotAtThree(t *testing.T) {
	files := map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Finding Structure in Time"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "Finding Structure in Time"}),
	}
	got := only(t, suggested(t, 2, files))
	if got.Title != "Finding Structure in Time" {
		t.Errorf("the suggestion is %q", got.Title)
	}
	if strings.Join(got.By, ",") != "hochreiter-1997-lstm,rumelhart-1986-backprop" {
		t.Errorf("it is cited by %v", got.By)
	}
	if all := suggested(t, 3, files); len(all) != 0 {
		t.Errorf("two citations cleared a floor of three: %+v", all)
	}
}

// One paper citing the same work twice is one paper, which is the difference
// between a reading list and a count of lines in a bibliography.
func TestOnePaperCitingAWorkTwiceCountsOnce(t *testing.T) {
	all := suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Finding Structure in Time"},
			entry{title: "Finding Structure in Time"}),
	})
	if len(all) != 0 {
		t.Errorf("one paper made a work worth adding: %+v", all)
	}
}

// Punctuation is not a different paper. The ACM style and the IEEE style
// print the same title with a colon in one and a comma in the other.
func TestTheSameTitlePunctuatedTwoWaysIsOneWork(t *testing.T) {
	got := only(t, suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Adam, A Method for Stochastic Optimization"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "Adam - A Method for Stochastic Optimization"}),
	}))
	if len(got.By) != 2 {
		t.Errorf("the two readings did not join up: %+v", got)
	}
}

// A title one bibliography printed in full and another cut short is two
// groups by title, and the identifier is what says they are one paper.
func TestTwoTitlesWithOneIdentifierAreOneWork(t *testing.T) {
	got := only(t, suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Neural Machine Translation by Jointly Learning to Align and Translate", arxiv: "1409.0473"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "Neural Machine Translation", arxiv: "1409.0473"}),
	}))
	if len(got.By) != 2 {
		t.Errorf("the two titles did not join up: %+v", got)
	}
	// The longer reading, because the shorter one is a bibliography that ran
	// out of room and not a different paper.
	if got.Title != "Neural Machine Translation by Jointly Learning to Align and Translate" {
		t.Errorf("the suggestion is titled %q", got.Title)
	}
	if got.Command() != "papers add -arxiv 1409.0473" {
		t.Errorf("the line to run is %q", got.Command())
	}
}

// A work with no identifier anywhere has nothing to run, and saying so is
// better than printing a command that will not work.
func TestAWorkWithNoIdentifierHasNoCommand(t *testing.T) {
	got := only(t, suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Finding Structure in Time"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "Finding Structure in Time"}),
	}))
	if got.Command() != "" {
		t.Errorf("it offered %q to run", got.Command())
	}
	if !strings.Contains(SuggestMarkdown([]Suggestion{got}, 2), "looked up by hand") {
		t.Error("the report does not say the work has to be looked up by hand")
	}
}

// A DOI is the other identifier, and a reference that carries one is worth a
// papers add -doi.
func TestAWorkWithADOIIsAddedByDOI(t *testing.T) {
	got := only(t, suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "A Method for the Construction of Minimum Redundancy Codes", doi: "10.1109/JRPROC.1952.273898"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "A Method for the Construction of Minimum Redundancy Codes"}),
	}))
	if got.Command() != "papers add -doi 10.1109/JRPROC.1952.273898" {
		t.Errorf("the line to run is %q", got.Command())
	}
}

// A reference that resolved is a paper the corpus already holds, and a
// reading list that told somebody to add it would be wrong twice over.
func TestAPaperTheCorpusHoldsIsNeverSuggested(t *testing.T) {
	all := suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "A Relational Model of Data for Large Shared Data Banks", resolvesTo: "codd-1970-relational"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "A Relational Model of Data for Large Shared Data Banks", resolvesTo: "codd-1970-relational"}),
	})
	if len(all) != 0 {
		t.Errorf("a paper in the corpus was suggested: %+v", all)
	}
}

// The resolver missing an edge is rule R01's finding. Suggesting a paper
// three lines up the same manifest would be this command's own bug, so the
// manifest is checked as well as the resolution.
func TestAPaperTheCorpusHoldsIsNotSuggestedEvenWhenTheEdgeWasMissed(t *testing.T) {
	all := suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "A Relational Model of Data for Large Shared Data Banks"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "A Relational Model of Data for Large Shared Data Banks"}),
	})
	if len(all) != 0 {
		t.Errorf("a paper in the corpus was suggested: %+v", all)
	}
}

// Most cited first, because that is the order somebody would work down the
// list in.
func TestTheListIsMostCitedFirst(t *testing.T) {
	all := suggested(t, 2, map[string]string{
		"manifests/refs/hochreiter-1997-lstm.yaml": refsFile("hochreiter-1997-lstm",
			entry{title: "Finding Structure in Time"}, entry{title: "Serial Order"}),
		"manifests/refs/rumelhart-1986-backprop.yaml": refsFile("rumelhart-1986-backprop",
			entry{title: "Finding Structure in Time"}, entry{title: "Serial Order"}),
		"manifests/refs/codd-1970-relational.yaml": refsFile("codd-1970-relational",
			entry{title: "Finding Structure in Time"}),
	})
	if len(all) != 2 {
		t.Fatalf("there are %d suggestions, want 2", len(all))
	}
	if all[0].Title != "Finding Structure in Time" {
		t.Errorf("the list starts with %q", all[0].Title)
	}
}

// A paper whose bibliography nobody has read yet cites nothing here, which
// is a different thing from citing nothing.
func TestAPaperWithNoBibliographyIsNotAnError(t *testing.T) {
	if all := suggested(t, 1, nil); len(all) != 0 {
		t.Errorf("a corpus with no parsed references suggested %+v", all)
	}
}

// The report is what gets committed, so it has to say what the list is and
// what it is not.
func TestTheReportSaysACandidateIsNotADecision(t *testing.T) {
	md := SuggestMarkdown([]Suggestion{{Title: "Finding Structure in Time", By: []string{"a-1990-b", "c-1991-d"}}}, 2)
	for _, want := range []string{"# Worth adding", "candidate and not a decision", "Cited by 2: a-1990-b, c-1991-d."} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not contain %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "—") {
		t.Error("the report has an em dash in it")
	}
}

func TestAnEmptyListStillWritesAReport(t *testing.T) {
	md := SuggestMarkdown(nil, 3)
	if !strings.Contains(md, "Nothing outside the corpus is cited by at least 3") {
		t.Errorf("the report of an empty list reads:\n%s", md)
	}
}
