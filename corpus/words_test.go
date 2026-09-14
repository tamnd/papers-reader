package corpus

import "testing"

// A word count that reads a language with no spaces in it has to agree with
// itself on a passage that mixes the two, which every translated page does:
// the mathematics and the names in it stay in the Latin alphabet.
func TestCountingWordsInAMixedParagraph(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"", 0},
		{"one", 1},
		{"  spaced   out  ", 2},
		{"生成対抗", 4},
		{"モデル$G$と", 5},
		{"the model $G$ 生成", 5},
		{"和 Bengio 一起", 4},
	} {
		if n := Words(tc.text); n != tc.want {
			t.Errorf("Words(%q) is %d, want %d", tc.text, n, tc.want)
		}
	}
}
