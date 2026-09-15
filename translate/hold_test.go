package translate

import (
	"strings"
	"testing"
)

func TestHoldTakesEveryListingOutAndUnholdPutsThemBack(t *testing.T) {
	source := "A paragraph.\n\n```go\nfunc main() {}\n```\n\nAnother paragraph.\n\n```text\nName   Metric\n```\n\nThe last paragraph."
	masked, held := Hold(source)
	if len(held) != 2 {
		t.Fatalf("%d listings were held back, want 2", len(held))
	}
	if strings.Contains(masked, "```") {
		t.Errorf("a fence was left in the question:\n%s", masked)
	}
	for _, want := range []string{"[[listing-1]]", "[[listing-2]]"} {
		if !strings.Contains(masked, want) {
			t.Errorf("%s is not in the question:\n%s", want, masked)
		}
	}
	if got := Unhold(masked, held); got != source {
		t.Errorf("the passage did not come back as it was written:\n%s", got)
	}
}

// The chunker keeps a listing inside a list item indented, and the indent is
// part of the listing rather than part of the line the marker goes on.
func TestAnIndentedListingComesBackWithItsIndent(t *testing.T) {
	source := "- An item:\n\n  ```text\n  a line\n  ```\n\n- Another item."
	masked, held := Hold(source)
	if len(held) != 1 {
		t.Fatalf("%d listings were held back, want 1", len(held))
	}
	if got := Unhold(masked, held); got != source {
		t.Errorf("the passage did not come back as it was written:\n%s", got)
	}
}

func TestAPassageWithNoListingIsLeftAlone(t *testing.T) {
	source := "A paragraph with `an inline span` in it and $x = 1$ as well."
	masked, held := Hold(source)
	if held != nil {
		t.Errorf("%d listings were held back in a passage with none", len(held))
	}
	if masked != source {
		t.Errorf("the passage was rewritten:\n%s", masked)
	}
}

// A marker is a protected span, so an answer that dropped one is refused
// before anything is put back.
func TestAMarkerIsAProtectedSpan(t *testing.T) {
	masked, _ := Hold("A paragraph.\n\n```text\na line\n```")
	bad := Compare(masked, "Một đoạn văn.")
	if len(bad) == 0 {
		t.Fatal("an answer that dropped the marker was accepted")
	}
	if !strings.Contains(bad[0].String(), "listing-1") {
		t.Errorf("the difference does not name the marker: %s", bad[0])
	}
}

func TestHoldingSeesAPassageThatIsAlreadySpelledThatWay(t *testing.T) {
	if Holding("A paragraph about [[knuth-1974-structured]] and nothing else.") {
		t.Error("a citation was read as a marker")
	}
	if !Holding("A paragraph that says [[listing-2]] for some reason.") {
		t.Error("a passage written the way a marker is written was not noticed")
	}
}
