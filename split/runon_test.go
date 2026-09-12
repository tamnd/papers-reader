package split

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/assemble"
)

// para is one paragraph off one page, which is what almost all of them are.
func para(text string) assemble.Paragraph {
	return assemble.Paragraph{Text: text, Page: 1, Pages: 1}
}

func texts(ps []assemble.Paragraph) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Text
	}
	return out
}

func TestUnrunSeparatesAHeadingFromItsSection(t *testing.T) {
	in := []assemble.Paragraph{para("1.1. Introduction\n" +
		"This paper is concerned with the application of elementary relation theory to systems which provide shared access to large banks of formatted data.")}
	got := texts(Unrun(in))
	want := []string{
		"1.1. Introduction",
		"This paper is concerned with the application of elementary relation theory to systems which provide shared access to large banks of formatted data.",
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Unrun gave %q, want %q", got, want)
	}
}

func TestUnrunKeepsTheHeadingOnThePageItWasPrintedOn(t *testing.T) {
	// A paragraph that ran across a page break carries the span, and the
	// heading at the top of it did not run anywhere.
	in := []assemble.Paragraph{{
		Text: "2 Background\n" +
			"The goal of reducing sequential computation also forms the foundation of a number of models that have been proposed over the last few years.",
		Page:  4,
		Pages: 2,
	}}
	got := Unrun(in)
	if len(got) != 2 {
		t.Fatalf("Unrun gave %d paragraphs, want 2", len(got))
	}
	if got[0].Page != 4 || got[0].Pages != 1 {
		t.Errorf("the heading is on page %d over %d pages, want page 4 over 1", got[0].Page, got[0].Pages)
	}
	if got[1].Page != 4 || got[1].Pages != 2 {
		t.Errorf("the section is on page %d over %d pages, want page 4 over 2", got[1].Page, got[1].Pages)
	}
}

func TestUnrunLeavesEverythingElseAlone(t *testing.T) {
	prose := "The provision of data description tables in recently developed information systems represents a major advance toward the goal of data independence."
	cases := []struct {
		name string
		in   string
	}{
		{"a paragraph on one line", prose},
		{"a heading on a line of its own", "1.1. Introduction"},
		{
			"a list item whose second line carries the sentence on",
			"1. Ordering dependence\nand the indexing dependence that follows from it, both of which are discussed below in their turn.",
		},
		{
			"a list item with a short second line",
			"1. Ordering dependence\nAnd indexing dependence too.",
		},
		{
			"a first line that is not numbered",
			"Ordering dependence\n" + prose,
		},
		{
			"a first line that is a sentence",
			"This is not a heading at all, whatever else it is.\n" + prose,
		},
		{
			"a first line too long to be a heading",
			strings.Repeat("1. word ", 20) + "\n" + prose,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Unrun([]assemble.Paragraph{para(c.in)})
			if len(got) != 1 || got[0].Text != c.in {
				t.Errorf("Unrun gave %q, want it left alone", texts(got))
			}
		})
	}
}

func TestUnrunSeparatesAHeadingTheExtractorMarked(t *testing.T) {
	// The layout path writes an ATX heading and the model it ran under may or
	// may not have left a blank line under it either.
	in := []assemble.Paragraph{para("## 3 Model Architecture\n" +
		"Most competitive neural sequence transduction models have an encoder decoder structure, and the Transformer follows this overall architecture.")}
	got := texts(Unrun(in))
	if len(got) != 2 || got[0] != "## 3 Model Architecture" {
		t.Errorf("Unrun gave %q, want the heading on its own", got)
	}
}

// The defect this was written for, end to end. A paper whose reader left a
// blank line under one heading and not under the next three lost its
// numbering scheme altogether, because the chain needs three in sequence.
func TestAPaperWhoseHeadingsRanIntoTheirSectionsStillNumbers(t *testing.T) {
	prose := "This section says something at sufficient length that nobody could mistake it for the second line of a list item, which is the whole point of it."
	d := &assemble.Document{Paragraphs: []assemble.Paragraph{
		para("A Relational Model of Data for Large Shared Data Banks"),
		para("An abstract that runs on for a while about data independence and the several kinds of dependency that the paper is going to set out in order."),
		para("1. Relational Model and Normal Form"),
		para("1.1. Introduction\n" + prose),
		para("1.2. Data Dependencies in Present Systems\n" + prose),
		para("1.3. A Relational View of Data\n" + prose),
		para("2. Redundancy and Consistency\n" + prose),
	}}
	r := Split(d)
	if r.Scheme != SchemeArabic {
		t.Fatalf("the scheme is %q, want arabic", r.Scheme)
	}
	var got []string
	for _, s := range r.Sections {
		got = append(got, s.Number+" "+s.Title)
	}
	want := []string{" Front Matter", "1 Relational Model and Normal Form", "2 Redundancy and Consistency"}
	if len(got) != len(want) {
		t.Fatalf("the sections are %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section %d is %q, want %q", i, got[i], want[i])
		}
	}
	for _, n := range r.Notes {
		if strings.Contains(n, "found by how it is set") {
			t.Errorf("a heading was found by typography: %q", n)
		}
	}
}

// The subheadings have to survive the separation as well, in the body of the
// section they belong to.
func TestASeparatedSubheadingIsStillInItsSection(t *testing.T) {
	prose := "This section says something at sufficient length that nobody could mistake it for the second line of a list item, which is the whole point of it."
	d := &assemble.Document{Paragraphs: []assemble.Paragraph{
		para("1. Relational Model and Normal Form"),
		para("1.1. Introduction\n" + prose),
		para("1.2. Data Dependencies\n" + prose),
		para("1.3. A Relational View\n" + prose),
	}}
	r := Split(d)
	if len(r.Sections) != 1 {
		t.Fatalf("the paper split into %d sections, want 1", len(r.Sections))
	}
	body := r.Sections[0].Body
	for _, want := range []string{"Introduction", "Data Dependencies", "A Relational View"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body has lost %q:\n%s", want, body)
		}
	}
}
