package extract

import "testing"

// Every fixture here is typeset in the test. None of it is copied from a
// paper, for the same reason the fixtures next door are not.

func TestAnInventedLinkWithACaptionUnderItLosesTheLink(t *testing.T) {
	in := "The chain is built as follows.\n\n![A chain of blocks](../images/chain.png)\nFigure 1: Blocks in a chain.\n\nEach block names the one before it."
	want := "The chain is built as follows.\n\nFigure 1: Blocks in a chain.\n\nEach block names the one before it."
	if got := Unlink(in); got != want {
		t.Errorf("Unlink() = %q, want %q", got, want)
	}
}

// A caption printed above the picture is as common as one printed below it,
// and the page says the same thing either way.
func TestALinkWithACaptionOverItLosesTheLink(t *testing.T) {
	in := "Figure 2: The two attentions.\n![The two attentions side by side](fig2.png)\n\nThe left one is cheaper."
	want := "Figure 2: The two attentions.\n\nThe left one is cheaper."
	if got := Unlink(in); got != want {
		t.Errorf("Unlink() = %q, want %q", got, want)
	}
}

func TestALinkAloneInWhiteSpaceBecomesAFigureLine(t *testing.T) {
	in := "The protocol runs in rounds.\n\n![Diagram of the protocol](../images/protocol.png)\n\nEach round costs one message."
	want := "The protocol runs in rounds.\n\nFigure.\n\nEach round costs one message."
	if got := Unlink(in); got != want {
		t.Errorf("Unlink() = %q, want %q", got, want)
	}
}

// A page that opens or closes on a picture has nothing above or below it, and
// the edge of the page is not a caption.
func TestALinkAtTheEdgeOfThePageBecomesAFigureLine(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{
			name: "at the top",
			in:   "![A plate](plate.png)\n\nThe plate is discussed in Section 4.",
			want: "Figure.\n\nThe plate is discussed in Section 4.",
		},
		{
			name: "at the bottom",
			in:   "The plate is discussed in Section 4.\n\n![A plate](plate.png)",
			want: "The plate is discussed in Section 4.\n\nFigure.",
		},
		{
			name: "on its own",
			in:   "![A plate](plate.png)",
			want: "Figure.",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Unlink(c.in); got != c.want {
				t.Errorf("Unlink(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Two links in a row are two pictures with no caption between them, not one
// picture captioned by another.
func TestTwoLinksInARowBothBecomeFigureLines(t *testing.T) {
	in := "![Before pruning](before.png)\n![After pruning](after.png)"
	want := "Figure.\nFigure."
	if got := Unlink(in); got != want {
		t.Errorf("Unlink() = %q, want %q", got, want)
	}
}

// An image written inside a sentence is not a figure a reader invented in
// place of a caption, and taking it out would take the sentence apart.
func TestAnImageInTheMiddleOfALineIsLeftAlone(t *testing.T) {
	in := "The symbol ![dagger](dagger.png) marks the second author."
	if got := Unlink(in); got != in {
		t.Errorf("Unlink(%q) = %q, want it unchanged", in, got)
	}
}

// The three flow graphs on page 6 of McCabe's paper. The picture is not on
// disk and never will be, and the name and the number beside it are on the
// page and have to stay.
func TestAnImageBesideALabelAndAFormulaIsTakenOut(t *testing.T) {
	in := "G1: ![Graph G1](../images/graph_G1.png) $v = 6$\nG2: ![Graph G2](../images/graph_G2.png) $v = 6$"
	want := "G1: $v = 6$\nG2: $v = 6$"
	if got := Unlink(in); got != want {
		t.Errorf("Unlink() = %q, want %q", got, want)
	}
}

func TestAnImageAtTheEndOfACaptionIsTakenOut(t *testing.T) {
	in := "Fig. 3. The lattice of subgroups. ![Figure 3](f03.png)"
	want := "Fig. 3. The lattice of subgroups."
	if got := Unlink(in); got != want {
		t.Errorf("Unlink(%q) = %q, want %q", in, got, want)
	}
}

// Program text is transcribed verbatim, so a listing that shows how to write
// an image link shows how to write one.
func TestProgramTextKeepsItsLinks(t *testing.T) {
	in := "```markdown\n![alt](file.png)\n```"
	if got := Unlink(in); got != in {
		t.Errorf("Unlink(%q) = %q, want it unchanged", in, got)
	}
}

func TestAPageWithNoPicturesOnItComesBackUnchanged(t *testing.T) {
	in := "A block chain is a chain of blocks.\n\nEach block names the one before it."
	if got := Unlink(in); got != in {
		t.Errorf("Unlink(%q) = %q, want it unchanged", in, got)
	}
}

// A link is not an image link unless it opens with the bang, and an ordinary
// Markdown link on a line of its own is a reference and stays.
func TestAnOrdinaryLinkIsNotAnImage(t *testing.T) {
	in := "See also:\n\n[The original announcement](https://example.org/announce)\n\nfor the dates."
	if got := Unlink(in); got != in {
		t.Errorf("Unlink(%q) = %q, want it unchanged", in, got)
	}
}

func TestTidyUnwrapsAndUnlinksInOnePass(t *testing.T) {
	in := "Here is the transcription of the page:\n\n```markdown\nThe chain is built as follows.\n\n![A chain of blocks](../images/chain.png)\n\nEach block names the one before it.\n```"
	want := "The chain is built as follows.\n\nFigure.\n\nEach block names the one before it."
	if got := Tidy(in); got != want {
		t.Errorf("Tidy() = %q, want %q", got, want)
	}
}
