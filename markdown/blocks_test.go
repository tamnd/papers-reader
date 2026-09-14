package markdown

import (
	"strings"
	"testing"
)

// A listing has blank lines in it and they are part of the listing. This is
// the reason the splitter is a package and not a split on "\n\n", and it is
// the reason it is shared: the LaTeX, the EPUB and the reading app all have
// to cut a body in the same place or the block index means nothing.
func TestAFencedListingIsOneBlockHoweverManyBlankLinesItHas(t *testing.T) {
	const body = "Some prose.\n\n```\nread x\n\nwrite x\n```\n\nMore prose.\n"
	got := Blocks(body)
	if len(got) != 3 {
		t.Fatalf("%d blocks, want 3: %q", len(got), got)
	}
	if !strings.Contains(got[1], "write x") {
		t.Errorf("the listing was cut in half: %q", got[1])
	}
}

// A listing inside a list item is indented and so is the fence that closes
// it. Finding the open at column zero and the close anywhere reads the open
// as a paragraph and then swallows the rest of the file looking for a close
// it walked past, which is one block where there should be three.
func TestAnIndentedFenceIsStillAFence(t *testing.T) {
	const body = "1. Run it:\n\n   ```\n   read x\n\n   write x\n   ```\n\nThen read the output.\n"
	got := Blocks(body)
	if len(got) != 3 {
		t.Fatalf("%d blocks, want 3: %q", len(got), got)
	}
	if !strings.Contains(got[1], "write x") {
		t.Errorf("the listing was cut in half: %q", got[1])
	}
	if !strings.HasPrefix(got[2], "Then read") {
		t.Errorf("the prose after the listing was swallowed by it: %q", got[2])
	}
}

func TestTheSplitterDropsNothingAndInventsNothing(t *testing.T) {
	const body = "One.\n\n\n\nTwo.\n   \nThree.\n"
	got := Blocks(body)
	if len(got) != 3 {
		t.Fatalf("%d blocks, want 3: %q", len(got), got)
	}
}

// The attribute block goes both ways and papers tags writes both, so both
// have to come off.
func TestTheAttributeBlockComesOffEitherWayItIsWritten(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"$$x = 1$$\n{#p-1986-a-eq-1 .equation tag=00a1}", "$$x = 1$$"},
		{"Figure 2: A picture. {#p-1986-a-fig-2 .figure tag=00b2}", "Figure 2: A picture."},
	} {
		got, attr, ok := TakeAttr(c.in)
		if !ok {
			t.Errorf("%q kept its attribute block", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("%q came back as %q", c.in, got)
		}
		if attr.Anchor == "" || attr.Tag == "" {
			t.Errorf("%q gave up %+v", c.in, attr)
		}
	}
	// A block that is nothing but an attribute block is not a thing the
	// corpus writes, and turning it into an anchor on an empty paragraph
	// would be worse than leaving it alone.
	if _, _, ok := TakeAttr("{#p-1986-a-eq-1 .equation tag=00a1}"); ok {
		t.Error("an attribute block on its own was read as a labelled block")
	}
}

func TestWhatEachKindOfBlockIs(t *testing.T) {
	for _, c := range []struct {
		text string
		is   func(string) bool
		name string
		want bool
	}{
		{"## Method", IsHeading, "heading", true},
		{"Some prose.", IsHeading, "heading", false},
		{"$$x = 1$$", IsDisplay, "display", true},
		{"The value $x$ is one.", IsDisplay, "display", false},
		{"$$x$$ and $$y$$", IsDisplay, "display", false},
		{"- one\n- two", IsList, "list", true},
		{"1. one\n2. two", IsList, "list", true},
		{"- one\nand a sentence that carries on", IsList, "list", false},
		{"- one\n1. two", IsList, "list", false},
		{"| n | x |\n| --- | --- |\n| 1 | 2 |", IsTable, "table", true},
		{"| n | x |\n| 1 | 2 |", IsTable, "table", false},
		{"```go\nx := 1\n```", IsFenced, "fenced", true},
	} {
		if got := c.is(c.text); got != c.want {
			t.Errorf("%q is %s: %v, want %v", c.text, c.name, got, c.want)
		}
	}
}

func TestAHeadingGivesUpItsDepthAndItsText(t *testing.T) {
	depth, title, ok := Heading("#### A deep one\n\nand a line under it")
	if !ok || depth != 4 || title != "A deep one" {
		t.Errorf("the heading read as %d %q %v", depth, title, ok)
	}
}

// The equation number is the one the paper printed, and a number this
// toolchain invented would be worse than none, because every cross
// reference in the prose is to the paper's own numbering.
func TestTheEquationNumberIsTheOneThePaperPrinted(t *testing.T) {
	if !Tagged(`x = 1 \tag{3.2}`) {
		t.Error("a tagged equation read as untagged")
	}
	if got := Tag(`x = 1 \tag{(12)}`); got != "12" {
		t.Errorf("the tag is %q, want 12", got)
	}
	if got := Tag(`x = 1`); got != "" {
		t.Errorf("an untagged equation gave up %q", got)
	}
}

func TestAListGivesUpItsItemsWithoutTheMarkers(t *testing.T) {
	got := Items("- one\n-  two\n- three")
	if strings.Join(got, "|") != "one|two|three" {
		t.Errorf("the items are %v", got)
	}
	if !Ordered("1. one\n2. two") || Ordered("- one") {
		t.Error("the list kind read wrong")
	}
}

func TestATableGivesUpItsHeaderAndItsRows(t *testing.T) {
	head, rows := Rows("| n | x |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |")
	if strings.Join(head, "|") != "n|x" {
		t.Errorf("the header is %v", head)
	}
	if len(rows) != 2 || rows[1][1] != "4" {
		t.Errorf("the rows are %v", rows)
	}
}

// The caption prefix is read off the text and not looked up in a table of
// four languages, because in a translation the word is the translated word.
func TestACaptionGivesUpItsNumberInAnyLanguage(t *testing.T) {
	for _, c := range []struct{ text, want, rest string }{
		{"Figure 2: A network.", "2", "A network."},
		{"**Hình 2.** Một mạng.", "2", "Một mạng."},
		{"図 2: ネットワーク。", "2", "ネットワーク。"},
	} {
		number, rest := CaptionNumber(c.text, "2")
		if number != c.want || rest != c.rest {
			t.Errorf("%q gave up %q and %q", c.text, number, rest)
		}
	}
	// A caption whose number is not the one the figure is filed under is
	// left whole, because the two disagreeing means the prefix was never a
	// caption prefix.
	if number, rest := CaptionNumber("Figure 9: A network.", "2"); number != "" || !strings.HasPrefix(rest, "Figure 9") {
		t.Errorf("a mismatched number was cut off anyway: %q %q", number, rest)
	}
}

// A footnote definition has nowhere to go in any of the three renderers, so
// it is lifted out of the body wherever it fell.
func TestAFootnoteDefinitionIsLiftedOutOfTheBody(t *testing.T) {
	into := map[string]string{}
	body := Notes("Some prose [^1].\n\n[^1]: The note itself.\n\nMore prose.\n", into)
	if strings.Contains(body, "The note itself") {
		t.Errorf("the definition is still in the body: %q", body)
	}
	if into["1"] != "The note itself." {
		t.Errorf("the note came out as %q", into["1"])
	}
	if !strings.Contains(body, "[^1]") {
		t.Errorf("the marker went with it: %q", body)
	}
}
