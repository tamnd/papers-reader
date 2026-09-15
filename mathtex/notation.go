package mathtex

import "strings"

// Notation is the characters that only turn up when a page is doing
// mathematics: the operators, the relations, the set signs and the
// quantifiers.
//
// No Greek and no Latin. A Greek letter is a name as often as it is a
// variable, and the Paxos paper calls its five priests A, B, Γ, ∆ and E,
// which is prose and not an equation. What is on this list cannot be read
// as anything but mathematics wherever it appears.
const Notation = "√∛∈∉∋∌∞∑∏∫∮∂∇∀∃∄⊆⊄⊂⊇⊃⊕⊗⊙∧∨¬≈≅≡≢≪≫≤≥≠≜≔∝∅∪∩⌈⌉⌊⌋∥⟨⟩ℵ"

// Enough is how many of those characters make a page's mathematics certain.
//
// One is a glyph that wandered into a sentence. Three is a page that was
// doing mathematics in the paper and is not doing any in the file.
const Enough = 3

// Signs counts the notation in a body, with the listings blanked first.
//
// A fenced block full of arithmetic is code, and the shell prompt in it is
// not this paper's mathematics going missing.
func Signs(body string) int {
	n := 0
	for _, c := range BlankFences(body) {
		if strings.ContainsRune(Notation, c) {
			n++
		}
	}
	return n
}

// FirstSign is where the notation starts, as a line number counting from
// one, and zero where there is none. It is what a report points a person at.
func FirstSign(body string) int {
	for n, line := range strings.Split(BlankFences(body), "\n") {
		if strings.ContainsAny(line, Notation) {
			return n + 1
		}
	}
	return 0
}
