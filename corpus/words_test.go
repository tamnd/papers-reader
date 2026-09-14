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
	}{{VI, 500}, {ZH, 562}, {JA, 875}} {
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
	}{{VI, 1.86}, {ZH, 1.95}, {JA, 3.13}} {
		if at := int(250 * c.ratio); at > Limit(250, c.Lang) {
			t.Errorf("the widest %s translation of a 250 word passage counts %d and the limit is %d",
				c.Lang, at, Limit(250, c.Lang))
		}
	}
}

// A file that has grown a whole section it was never given still has to be
// caught, which is the only reason the limit is a limit.
//
// Twice the median translation, and not a multiple of the English, because
// the English number is the wrong yardstick for what this is guarding. A
// doubled Japanese file is twice what Japanese normally runs to, and
// Japanese normally runs to nearly two and a half times the English, so a
// ceiling written as four abstracts is nearly no ceiling at all there while
// being a tight one in Vietnamese. The medians are from the doc comment on
// Limit and move with it.
func TestACeilingIsStillACeiling(t *testing.T) {
	if Limit(250, EN) >= 250*2 {
		t.Errorf("the en limit is %d, which is two abstracts", Limit(250, EN))
	}
	for _, c := range []struct {
		Lang
		median float64
	}{{VI, 1.44}, {ZH, 1.66}, {JA, 2.40}} {
		grown := int(250 * c.median * 2)
		if got := Limit(250, c.Lang); got >= grown {
			t.Errorf("the %s limit is %d, and a %s file of twice the usual length is %d, so the limit would not catch it",
				c.Lang, got, c.Lang, grown)
		}
	}
}
