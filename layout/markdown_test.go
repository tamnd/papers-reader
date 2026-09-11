package layout

import (
	"strings"
	"testing"
)

func TestAHeadingKeepsItsNumber(t *testing.T) {
	b := Block{Kind: Heading, Text: "3.1  Gated units", Level: 2}
	if got, want := b.Markdown(), "## 3.1 Gated units"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAHeadingLevelIsClamped(t *testing.T) {
	for _, tt := range []struct {
		level int
		want  string
	}{
		{0, "# a section"},
		{1, "# a section"},
		{6, "###### a section"},
		{9, "###### a section"},
	} {
		b := Block{Kind: Heading, Text: "a section", Level: tt.level}
		if got := b.Markdown(); got != tt.want {
			t.Errorf("level %d gave %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestADisplayEquationIsWrappedOnce(t *testing.T) {
	for _, in := range []string{
		`x = y + z`,
		`$$x = y + z$$`,
		`\[x = y + z\]`,
	} {
		b := Block{Kind: Equation, Text: in}
		want := "$$\nx = y + z\n$$"
		if got := b.Markdown(); got != want {
			t.Errorf("%q gave %q, want %q", in, got, want)
		}
	}
}

func TestTeXIsNeverReflowed(t *testing.T) {
	tex := `\begin{bmatrix} a & b \\ c & d \end{bmatrix}`
	b := Block{Kind: Equation, Text: tex}
	if got, want := b.Markdown(), "$$\n"+tex+"\n$$"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCodeKeepsEverySpace(t *testing.T) {
	body := "def f(x):\n    if x > 0:\n        return x\n    return 0"
	b := Block{Kind: Code, Text: body, Lang: "python"}
	want := "```python\n" + body + "\n```"
	if got := b.Markdown(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCodeWithNoLanguageSaysText(t *testing.T) {
	b := Block{Kind: Code, Text: "for i in 1..n"}
	if got := b.Markdown(); !strings.HasPrefix(got, "```text\n") {
		t.Errorf("got %q, want a text fence", got)
	}
}

func TestAFenceInsideAListingLengthensTheFence(t *testing.T) {
	b := Block{Kind: Code, Text: "```\nnested\n```", Lang: "markdown"}
	got := b.Markdown()
	if !strings.HasPrefix(got, "````markdown\n") || !strings.HasSuffix(got, "\n````") {
		t.Errorf("got %q, want a four backtick fence", got)
	}
}

func TestATableIsMarkdownAndNotAPicture(t *testing.T) {
	b := Block{
		Kind:    Table,
		Caption: "Table 2: made up numbers",
		Rows: [][]string{
			{"model", "score"},
			{"first", "1.0"},
			{"second", "2.0"},
		},
	}
	want := strings.Join([]string{
		"*Table 2: made up numbers*",
		"",
		"| model | score |",
		"| --- | --- |",
		"| first | 1.0 |",
		"| second | 2.0 |",
	}, "\n")
	if got := b.Markdown(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAShortRowIsPaddedToTheWidestOne(t *testing.T) {
	b := Block{Kind: Table, Rows: [][]string{{"a", "b", "c"}, {"one"}}}
	want := "| a | b | c |\n| --- | --- | --- |\n| one |  |  |"
	if got := b.Markdown(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAPipeInACellIsNotAColumn(t *testing.T) {
	b := Block{Kind: Table, Rows: [][]string{{"command"}, {"ls | wc"}}}
	if got := b.Markdown(); !strings.Contains(got, `| ls \| wc |`) {
		t.Errorf("got %q, want the pipe escaped", got)
	}
}

func TestATableWithNoRowsKeepsOnlyItsCaption(t *testing.T) {
	b := Block{Kind: Table, Caption: "Table 1: results", Image: "table_1.png"}
	if got, want := b.Markdown(), "*Table 1: results*"; got != want {
		t.Errorf("got %q, want %q: a table is never committed as a picture", got, want)
	}
}

func TestAFigureIsAPictureAndACaption(t *testing.T) {
	b := Block{Kind: Figure, Image: "images/fig1.png", Caption: "Figure 1: the shape of the thing"}
	want := "![Figure 1: the shape of the thing](images/fig1.png)\n\n*Figure 1: the shape of the thing*"
	if got := b.Markdown(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAFigureWithNoCaptionStillHasAltText(t *testing.T) {
	b := Block{Kind: Figure, Image: "images/fig2.png"}
	if got, want := b.Markdown(), "![figure](images/fig2.png)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFurnitureIsDropped(t *testing.T) {
	b := Block{Kind: Furniture, Text: "Journal of Invented Results, vol 4"}
	if got := b.Markdown(); got != "" {
		t.Errorf("got %q, want the running head dropped", got)
	}
}

func TestAPageReadsLikeThePageStore(t *testing.T) {
	p := Page{Number: 1, Blocks: []Block{
		{Kind: Furniture, Text: "a running head"},
		{Kind: Heading, Text: "1 Introduction", Level: 1},
		{Kind: Text, Text: "A first paragraph of\ninvented prose."},
		{Kind: Equation, Text: "e = m c^2"},
	}}
	want := "# 1 Introduction\n\nA first paragraph of invented prose.\n\n$$\ne = m c^2\n$$\n"
	if got := p.Text(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAPageOfNothingButFurnitureIsEmpty(t *testing.T) {
	p := Page{Number: 9, Blocks: []Block{{Kind: Furniture, Text: "9"}}}
	if got := p.Text(); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}
