package code

import "testing"

// The journals set assignment with a left arrow when they had the sort. The
// McCabe paper prints one procedure in two halves, the first assigning with a
// colon and the second with an arrow, and only the first half was fenced.
func TestAnArrowIsAnAssignment(t *testing.T) {
	for _, line := range []string{"    H ← J - 1", "F←0;", "x ⟵ y"} {
		if !Statement(line) {
			t.Errorf("Statement(%q) = false, want true", line)
		}
	}
	// An arrow in prose is a direction, not an assignment, and there is no
	// name in front of it.
	if Statement("← back to the previous section") {
		t.Error("an arrow with nothing in front of it was read as an assignment")
	}
}

// A listing printed with one keyword to a line has very little else on it.
func TestABareAlgolKeywordIsAMark(t *testing.T) {
	for _, line := range []string{"        THEN", "    ELSE", "REPEAT", "    GOTO 990"} {
		if !Statement(line) {
			t.Errorf("Statement(%q) = false, want true", line)
		}
	}
	// In lower case they are English, and in capitals at the head of a line
	// IF and FOR are how a paper heads a section about them.
	for _, line := range []string{"then the bound holds", "IF STATEMENTS", "FOR LOOPS"} {
		if Statement(line) {
			t.Errorf("Statement(%q) = true, want false", line)
		}
	}
}

func TestALowerCaseBeginAloneIsAMark(t *testing.T) {
	for _, line := range []string{"begin", "    end", "        end;", "end;"} {
		if !Statement(line) {
			t.Errorf("Statement(%q) = false, want true", line)
		}
	}
	for _, line := range []string{
		"begin by reading the goto function",
		"    end of the second pass",
		"we begin;",
	} {
		if Statement(line) {
			t.Errorf("Statement(%q) = true, want false", line)
		}
	}
}

func TestACaptionOverAListingIsRecognised(t *testing.T) {
	for _, line := range []string{
		"Algorithm 4. Construction of a deterministic finite automaton.",
		"    Listing 2: the parser",
		"**Algorithm 1.2.**",
	} {
		if !Caption(line) {
			t.Errorf("Caption(%q) = false, want true", line)
		}
	}
	for _, line := range []string{
		"Figure 3. The goto function.",
		"Algorithm 4 builds the automaton in one pass",
		"begin",
	} {
		if Caption(line) {
			t.Errorf("Caption(%q) = true, want false", line)
		}
	}
}
