package layout

import "testing"

// Every fixture in this package is written by hand. None of it is text from a
// paper, because a test file is committed to a public repository and the
// corpus commits no copyrighted text it did not extract under a licence.

func TestAPageKeepsTheOrderTheToolGaveIt(t *testing.T) {
	d := &Document{}
	d.add(1, 612, 792, Block{Kind: Text, Text: "left column, first"})
	d.add(1, 612, 792, Block{Kind: Text, Text: "left column, second"})
	d.add(1, 612, 792, Block{Kind: Text, Text: "right column, first"})
	d.sortPages()

	p, ok := d.Page(1)
	if !ok {
		t.Fatal("page one went missing")
	}
	want := []string{"left column, first", "left column, second", "right column, first"}
	if len(p.Blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d", len(p.Blocks), len(want))
	}
	for i, s := range want {
		if p.Blocks[i].Text != s {
			t.Errorf("block %d is %q, want %q", i, p.Blocks[i].Text, s)
		}
	}
}

func TestPagesComeBackInOrder(t *testing.T) {
	d := &Document{}
	d.add(3, 0, 0, Block{Kind: Text, Text: "three"})
	d.add(1, 0, 0, Block{Kind: Text, Text: "one"})
	d.add(2, 0, 0, Block{Kind: Text, Text: "two"})
	d.sortPages()

	if got, want := d.First(), 1; got != want {
		t.Errorf("first page is %d, want %d", got, want)
	}
	if got, want := d.Last(), 3; got != want {
		t.Errorf("last page is %d, want %d", got, want)
	}
	if got, want := len(d.Blocks()), 3; got != want {
		t.Fatalf("got %d blocks, want %d", got, want)
	}
	if d.Blocks()[0].Text != "one" {
		t.Errorf("reading order starts at %q, want the first page", d.Blocks()[0].Text)
	}
}

func TestABlankPageStaysAPage(t *testing.T) {
	d := &Document{}
	d.touch(1, 612, 792)
	d.touch(2, 612, 792)
	d.add(3, 612, 792, Block{Kind: Text, Text: "after the blank"})

	if got, want := len(d.Pages), 3; got != want {
		t.Fatalf("got %d pages, want %d: a blank verso is a real answer", got, want)
	}
	if got, want := d.Pages[2].Number, 3; got != want {
		t.Errorf("the third page is numbered %d, want %d", got, want)
	}
}

func TestTheSameNoteIsRecordedOnce(t *testing.T) {
	d := &Document{}
	for i := 0; i < 400; i++ {
		d.note("mineru block type %q is not one this reads", "whatsit")
	}
	if got, want := len(d.Notes), 1; got != want {
		t.Fatalf("got %d notes, want %d", got, want)
	}
}

func TestFiguresAreEveryPicture(t *testing.T) {
	d := &Document{}
	d.add(1, 0, 0, Block{Kind: Text, Text: "prose"})
	d.add(1, 0, 0, Block{Kind: Figure, Image: "one.png"})
	d.add(2, 0, 0, Block{Kind: Table, Rows: [][]string{{"a"}}})
	d.add(2, 0, 0, Block{Kind: Figure, Image: "two.png"})
	d.sortPages()

	figures := d.Figures()
	if got, want := len(figures), 2; got != want {
		t.Fatalf("got %d figures, want %d", got, want)
	}
	if figures[0].Image != "one.png" || figures[1].Image != "two.png" {
		t.Errorf("figures came back in the wrong order: %q then %q", figures[0].Image, figures[1].Image)
	}
}

func TestABoxKnowsWhetherItWasFilledIn(t *testing.T) {
	if !(Box{}).Empty() {
		t.Error("a zero box says it was filled in")
	}
	b := Box{X0: 10, Y0: 20, X1: 110, Y1: 70}
	if b.Empty() {
		t.Error("a real box says it is empty")
	}
	if got, want := b.Area(), 5000.0; got != want {
		t.Errorf("area is %v, want %v", got, want)
	}
	// A box the wrong way round has no area rather than a negative one,
	// because the page fraction cap divides by the page and a negative
	// fraction passes a cap it should fail.
	if got := (Box{X0: 110, Y0: 70, X1: 10, Y1: 20}).Area(); got != 0 {
		t.Errorf("a reversed box has area %v, want 0", got)
	}
}
