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

func TestACeilingTravelsIntoTheLanguageItIsChecking(t *testing.T) {
	// The English number stands in English, and grows by enough that an
	// ordinary translation of a passage at the ceiling is still under it.
	if got := Limit(250, EN); got != 250 {
		t.Errorf("Limit(250, en) = %d, want 250", got)
	}
	for _, c := range []struct {
		Lang
		want int
	}{{VI, 437}, {ZH, 500}, {JA, 750}} {
		if got := Limit(250, c.Lang); got != c.want {
			t.Errorf("Limit(250, %s) = %d, want %d", c.Lang, got, c.want)
		}
	}
	// The measured stretch of the corpus, at its largest, has to fit. These
	// are the ratios in the doc comment: if a later corpus stretches further
	// than this the numbers are wrong and not the corpus.
	for _, c := range []struct {
		Lang
		ratio float64
	}{{VI, 1.58}, {ZH, 1.85}, {JA, 2.81}} {
		if at := int(250 * c.ratio); at > Limit(250, c.Lang) {
			t.Errorf("the widest %s translation of a 250 word passage counts %d and the limit is %d",
				c.Lang, at, Limit(250, c.Lang))
		}
	}
}

// A file that has grown a whole section it was never given still has to be
// caught, which is the only reason the limit is a limit.
func TestACeilingIsStillACeiling(t *testing.T) {
	for _, l := range []Lang{EN, VI, ZH, JA} {
		if Limit(250, l) >= 250*4 {
			t.Errorf("the %s limit is %d, which is four abstracts", l, Limit(250, l))
		}
	}
}
