package code

import "testing"

// Every fixture here is typeset in the test. None of it is copied from a
// paper, because a test file is committed to a public repository and a
// listing out of somebody's paper is not ours to put there.

func TestAFenceWithATagIsOneBlock(t *testing.T) {
	body := "Before.\n\n```c\nint main(void) { return 0; }\n```\n\nAfter."
	blocks, unclosed := Blocks(body)
	if unclosed != nil {
		t.Fatalf("a closed fence came back unclosed: %+v", unclosed)
	}
	if len(blocks) != 1 {
		t.Fatalf("found %d blocks, want 1: %+v", len(blocks), blocks)
	}
	b := blocks[0]
	if b.Lang != "c" {
		t.Errorf("the tag is %q, want c", b.Lang)
	}
	if b.Text != "int main(void) { return 0; }" {
		t.Errorf("the text is %q", b.Text)
	}
	if b.Line != 3 || b.End != 5 {
		t.Errorf("the block runs from line %d to %d, want 3 to 5", b.Line, b.End)
	}
	if !b.Closed() {
		t.Error("the block says it is not closed")
	}
	if b.Lines() != 1 {
		t.Errorf("the block is %d lines, want 1", b.Lines())
	}
}

func TestAFenceWithNoTagIsStillABlock(t *testing.T) {
	blocks, _ := Blocks("```\nsomething\n```")
	if len(blocks) != 1 || blocks[0].Lang != "" {
		t.Fatalf("found %+v, want one block with no tag", blocks)
	}
}

// The tag is the first word. A fence carrying an attribute after the language
// is a fence in that language and not a fence in a language with a space in
// its name.
func TestTheTagIsTheFirstWord(t *testing.T) {
	blocks, _ := Blocks("```c linenos=true\nint x;\n```")
	if len(blocks) != 1 || blocks[0].Lang != "c" {
		t.Fatalf("found %+v, want one block tagged c", blocks)
	}
}

func TestTheTagIsLowerCased(t *testing.T) {
	blocks, _ := Blocks("```ALGOL\nbegin end\n```")
	if len(blocks) != 1 || blocks[0].Lang != "algol" {
		t.Fatalf("found %+v, want one block tagged algol", blocks)
	}
}

func TestAnUnclosedFenceRunsToTheEnd(t *testing.T) {
	body := "Before.\n\n```c\nint x;\nint y;"
	blocks, unclosed := Blocks(body)
	if unclosed == nil {
		t.Fatal("an unclosed fence came back closed")
	}
	if unclosed.Line != 3 {
		t.Errorf("the fence opened on line %d, want 3", unclosed.Line)
	}
	if unclosed.End != 0 {
		t.Errorf("the fence closed on line %d, want 0", unclosed.End)
	}
	if unclosed.Text != "int x;\nint y;" {
		t.Errorf("the text is %q", unclosed.Text)
	}
	if len(blocks) != 1 {
		t.Errorf("found %d blocks, want the unclosed one in the slice too", len(blocks))
	}
}

// A tilde fence is not closed by a backtick fence and the other way round,
// which is the only way to write a listing that shows what a fence looks
// like.
func TestAFenceClosesOnItsOwnMark(t *testing.T) {
	body := "~~~text\n```\nnot a fence here\n```\n~~~"
	blocks, unclosed := Blocks(body)
	if unclosed != nil {
		t.Fatalf("a closed fence came back unclosed: %+v", unclosed)
	}
	if len(blocks) != 1 {
		t.Fatalf("found %d blocks, want 1: %+v", len(blocks), blocks)
	}
	if blocks[0].Text != "```\nnot a fence here\n```" {
		t.Errorf("the text is %q", blocks[0].Text)
	}
}

// CommonMark's rule: a longer run opens a block that a shorter run does not
// close, which is how a listing with three backticks in it is written.
func TestALongerFenceIsNotClosedByAShorterOne(t *testing.T) {
	body := "````text\n```\nstill inside\n```\n````"
	blocks, unclosed := Blocks(body)
	if unclosed != nil {
		t.Fatalf("a closed fence came back unclosed: %+v", unclosed)
	}
	if len(blocks) != 1 || blocks[0].Text != "```\nstill inside\n```" {
		t.Fatalf("found %+v, want one block holding both inner fences", blocks)
	}
}

// Two blocks in a file are two blocks, and the second one starts after the
// first one ends rather than inside it.
func TestTwoBlocksAreTwoBlocks(t *testing.T) {
	body := "```c\nint x;\n```\n\nProse between them.\n\n```python\nx = 1\n```"
	blocks, _ := Blocks(body)
	if len(blocks) != 2 {
		t.Fatalf("found %d blocks, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Lang != "c" || blocks[1].Lang != "python" {
		t.Errorf("the tags are %q and %q", blocks[0].Lang, blocks[1].Lang)
	}
}

// Inline code is not a fence. Two backticks around a word is the commonest
// thing in the corpus after prose and reading it as a fence would put every
// paragraph after it inside a code block.
func TestInlineCodeIsNotAFence(t *testing.T) {
	if blocks, _ := Blocks("The function `main` returns ``int``."); len(blocks) != 0 {
		t.Errorf("found %+v, want nothing", blocks)
	}
}

// Markdown allows three spaces before a fence and no more. Four spaces is an
// indented code block, whose contents are literal, so a run of backticks
// there is text.
func TestAFenceIndentedPastThreeSpacesIsNotAFence(t *testing.T) {
	if blocks, _ := Blocks("    ```c\n    int x;\n    ```"); len(blocks) != 0 {
		t.Errorf("found %+v, want nothing", blocks)
	}
	if blocks, _ := Blocks("   ```c\n   int x;\n   ```"); len(blocks) != 1 {
		t.Errorf("found %+v, want one block", blocks)
	}
}

// Trailing spaces inside a fence are part of the program. This is what makes
// code a protected kind and it is the whole of rule C07.
func TestTrailingSpacesInsideAFenceAreKept(t *testing.T) {
	blocks, _ := Blocks("```fortran\n      X = 1   \n```")
	if len(blocks) != 1 {
		t.Fatalf("found %d blocks, want 1", len(blocks))
	}
	if blocks[0].Text != "      X = 1   " {
		t.Errorf("the text is %q, want the spaces on both ends kept", blocks[0].Text)
	}
}

func TestInsideMarksTheFenceAndItsContents(t *testing.T) {
	body := "Prose.\n\n```c\nint x;\n```\n\nMore prose."
	got := Inside(body)
	for _, c := range []struct {
		line int
		want bool
	}{{1, false}, {2, false}, {3, true}, {4, true}, {5, true}, {6, false}, {7, false}} {
		if got[c.line] != c.want {
			t.Errorf("line %d is inside=%v, want %v", c.line, got[c.line], c.want)
		}
	}
}

func TestInsideIsFalseForEveryLineOfAPlainPage(t *testing.T) {
	for i, in := range Inside("One paragraph.\n\nAnother one.") {
		if in {
			t.Errorf("line %d of a page with no fence on it is marked as code", i)
		}
	}
}

func TestAnEmptyBodyHasNoBlocks(t *testing.T) {
	blocks, unclosed := Blocks("")
	if len(blocks) != 0 || unclosed != nil {
		t.Errorf("found %+v and %+v, want nothing", blocks, unclosed)
	}
}

func TestStatementIsTheHalfOfAMarkThatIsNotPunctuation(t *testing.T) {
	for _, c := range []struct {
		line       string
		mark, stmt bool
	}{
		{"}", true, true},
		{"#include <math.h>", true, true},
		{"// the probability of catching up", true, true},
		{"int i = 0;", true, true},
		{"hears the wind and the rustling of leaves;", true, false},
		{"the shadows wait,", false, false},
	} {
		if got := Mark(c.line); got != c.mark {
			t.Errorf("Mark(%q) is %v, want %v", c.line, got, c.mark)
		}
		if got := Statement(c.line); got != c.stmt {
			t.Errorf("Statement(%q) is %v, want %v", c.line, got, c.stmt)
		}
	}
}
