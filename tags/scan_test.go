package tags

import (
	"strings"
	"testing"
)

// found is the items a body scans into, as "class:key" strings, which is
// short enough to write a whole expectation on one line.
func found(body string) []string {
	var out []string
	for _, it := range Scan(body) {
		out = append(out, it.Class+":"+it.Key)
	}
	return out
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("scanned %v, want %v", got, want)
	}
}

// The anchor comes from the number the paper printed, so section 3.2 is
// s3-2 whatever line of whatever file it lands on. That is what makes a
// link written today survive the paper being read again next year.
func TestScanReadsSectionNumbers(t *testing.T) {
	body := strings.Join([]string{
		"# 3 Model Architecture",
		"",
		"Most competitive models have an encoder-decoder structure.",
		"",
		"### 3.2.1 Scaled Dot-Product Attention",
		"",
		"We call our particular attention Scaled Dot-Product Attention.",
		"",
		"## Acknowledgements",
	}, "\n")
	same(t, found(body), []string{"section:s3", "section:s3-2-1", "section:s-acknowledgements"})
}

// A heading is anchored where the paper's number is, and the block goes at
// the end of the heading line.
func TestScanPutsASectionBlockOnTheHeadingLine(t *testing.T) {
	items := Scan("## 2 Background\n\nSome prose.\n")
	if len(items) != 1 {
		t.Fatalf("scanned %d items, want 1", len(items))
	}
	if items[0].Line != 0 || items[0].Col != len("## 2 Background") {
		t.Errorf("the block goes at line %d column %d", items[0].Line, items[0].Col)
	}
	got, err := Apply("## 2 Background\n\nSome prose.\n", items, []string{"{#p-s2 .section tag=0001}"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "## 2 Background {#p-s2 .section tag=0001}"; !strings.HasPrefix(got, want) {
		t.Errorf("wrote %q", got)
	}
}

// A figure takes its number from the caption when there is one, because that
// is the number the prose says when it says "see Figure 2". The file name is
// the fallback, because papers figures names a crop after what it cropped.
func TestScanNumbersAFigureFromItsCaption(t *testing.T) {
	body := strings.Join([]string{
		"![Two attention layers.](../../../figures/p/f07.png)",
		"",
		"*Figure 2: (left) Scaled Dot-Product Attention.*",
		"",
		"![An unlabelled diagram.](../../../figures/p/f04.png)",
		"",
		"Some prose that is not a caption.",
	}, "\n")
	same(t, found(body), []string{"figure:fig-2", "figure:fig-4"})
}

// The block for an image goes on the line after it, not at the end of it,
// because the end of it is inside the image syntax.
func TestScanPutsAFigureBlockOnItsOwnLine(t *testing.T) {
	body := "![A diagram.](../../../figures/p/f01.png)\n\n*Figure 1: A diagram.*\n"
	items := Scan(body)
	if len(items) != 1 || items[0].Col != -1 || items[0].Line != 0 {
		t.Fatalf("scanned %+v", items)
	}
	got, err := Apply(body, items, []string{"{#p-fig-1 .figure tag=000A}"})
	if err != nil {
		t.Fatal(err)
	}
	want := "![A diagram.](../../../figures/p/f01.png)\n{#p-fig-1 .figure tag=000A}\n\n*Figure 1: A diagram.*\n"
	if got != want {
		t.Errorf("wrote\n%q\nwant\n%q", got, want)
	}
}

// A caption with no image is still the thing the prose refers to. The corpus
// is full of them until the figures have been cropped out.
func TestScanAnchorsABareCaption(t *testing.T) {
	same(t, found("Figure 1: The Transformer, model architecture.\n"), []string{"figure:fig-1"})
	same(t, found("Table 3: Variations on the Transformer architecture.\n"), []string{"table:tab-3"})
	same(t, found("Algorithm 2: Proof of work.\n"), []string{"code:alg-2"})
}

func TestScanAnchorsNumberedStatements(t *testing.T) {
	body := strings.Join([]string{
		"**Theorem 1** The consensus algorithm is safe.",
		"",
		"Lemma 3.2. Every proposal is chosen at most once.",
		"",
		"Definition 4: A quorum is a majority of acceptors.",
		"",
		"Theorem without a number is prose and not a statement.",
	}, "\n")
	same(t, found(body), []string{"statement:thm-1", "statement:lem-3-2", "statement:def-4"})
}

// A display the paper numbered can be referred to as "(3)" and gets an
// anchor. One it did not number is a step in a derivation and gets none.
func TestScanAnchorsANumberedDisplay(t *testing.T) {
	body := strings.Join([]string{
		"$$",
		`\mathrm{Attention}(Q, K, V) = V \tag{1}`,
		"$$",
		"",
		"$$ e = mc^2 $$",
		"",
		"$$",
		`x + y = z \quad (4)`,
		"$$",
	}, "\n")
	same(t, found(body), []string{"equation:eq-1", "equation:eq-4"})
}

// A dollar sign inside a fenced code block is a shell prompt.
func TestScanIgnoresAFencedBlock(t *testing.T) {
	body := strings.Join([]string{
		"```sh",
		"$$",
		"# 1 Not a heading",
		"$$",
		"```",
		"",
		"# 1 A heading",
	}, "\n")
	same(t, found(body), []string{"section:s1"})
}

// An item that already carries a tag is recognised as carrying it, so the
// assigner leaves it alone and rule G05 does not report it.
func TestScanSeesTheTagAnItemAlreadyHas(t *testing.T) {
	body := strings.Join([]string{
		"## 3.2 Attention {#p-s3-2 .section tag=0A3F}",
		"",
		"![A diagram.](../../../figures/p/f02.png)",
		"{#p-fig-2 .figure tag=0A41}",
		"",
		"*Figure 2: Attention.*",
		"",
		"## 3.3 Position",
	}, "\n")
	items := Scan(body)
	if len(items) != 3 {
		t.Fatalf("scanned %d items, want 3: %+v", len(items), items)
	}
	if items[0].Tag != "0A3F" || items[0].Anchor != "p-s3-2" {
		t.Errorf("the heading came back as %+v", items[0])
	}
	if items[1].Tag != "0A41" {
		t.Errorf("the figure came back as %+v", items[1])
	}
	if items[2].Tag != "" {
		t.Errorf("the untagged heading came back as %+v", items[2])
	}
}

// Apply works from the last item back, so inserting a line above does not
// move the lines the earlier items were found on.
func TestApplyWritesEveryBlockInOnePass(t *testing.T) {
	body := strings.Join([]string{
		"# 1 One",
		"",
		"![A.](../../../figures/p/f01.png)",
		"",
		"# 2 Two",
	}, "\n")
	items := Scan(body)
	blocks := []string{"{#p-s1 .section tag=0001}", "{#p-fig-1 .figure tag=0002}", "{#p-s2 .section tag=0003}"}
	got, err := Apply(body, items, blocks)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if !strings.Contains(got, b) {
			t.Errorf("%s is missing from\n%s", b, got)
		}
	}
	again := Scan(got)
	if len(again) != 3 {
		t.Fatalf("the written body scans into %d items", len(again))
	}
	for i, it := range again {
		if it.Tag == "" {
			t.Errorf("item %d came back untagged: %+v", i, it)
		}
	}
}

func TestApplyRefusesMismatchedBlocks(t *testing.T) {
	if _, err := Apply("# 1 One\n", Scan("# 1 One\n"), nil); err == nil {
		t.Error("Apply accepted no blocks for one item")
	}
}

// A file's own section has no heading line in the body, because papers split
// lifted it into the front matter. Its key is still the number the paper
// printed, so a link to s3 resolves whichever place the heading ended up.
func TestSectionKey(t *testing.T) {
	for _, c := range []struct{ section, kind, want string }{
		{"3", "section", "s3"},
		{"3.2", "section", "s3-2"},
		{"A", "appendix", "sa"},
		{"", "front", "s-front"},
		{"", "references", "s-references"},
		{"", "section", ""},
		{"", "", ""},
	} {
		if got := SectionKey(c.section, c.kind); got != c.want {
			t.Errorf("SectionKey(%q, %q) is %q, want %q", c.section, c.kind, got, c.want)
		}
	}
}

func TestSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Acknowledgements", "acknowledgements"},
		{"Appendix A: Proofs", "appendix-a-proofs"},
		{"  Related Work  ", "related-work"},
		{"???", ""},
	} {
		if got := slug(c.in); got != c.want {
			t.Errorf("slug(%q) is %q, want %q", c.in, got, c.want)
		}
	}
}
