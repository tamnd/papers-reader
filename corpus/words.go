package corpus

import "unicode"

// Words is how long a passage is, in a way that survives a language that
// does not put spaces between its words.
//
// strings.Fields counts spaces, and Chinese and Japanese have none. The
// Japanese abstract of the GAN paper is a hundred and eighty characters of
// prose and eight sentences, and strings.Fields makes it seventeen words,
// which failed audit rule T06 as a title block and made the book setter
// take the author line for the abstract. Han, hiragana and katakana are
// counted one character to the word, and everything else a run at a time.
//
// A character is not a word and this does not pretend otherwise. A passage
// counts longer in Chinese and longer again in Japanese than the same
// passage counts in English, and a floor written in English words is
// therefore a lower floor when the paragraph is Chinese. That is the right
// direction to be wrong in for a floor: a title block is a dozen characters
// in any language. It is the wrong direction for a ceiling, which is what
// Limit is for.
//
// Hangul is not here. Korean puts spaces between its words.
func Words(s string) int {
	n, run := 0, false
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana):
			if run {
				n, run = n+1, false
			}
			n++
		case unicode.IsSpace(r):
			if run {
				n, run = n+1, false
			}
		default:
			run = true
		}
	}
	if run {
		n++
	}
	return n
}

// Limit turns a ceiling written in English words into the same ceiling in
// another language, as Words counts it.
//
// A ceiling has to travel and a floor does not. The one ceiling in the
// corpus is audit rule S07, the most of a restricted paper that may be
// quoted, and the quotation a translation carries is the same quotation: a
// Japanese rendering of a 250 word abstract republishes exactly as much of
// the paper as the English does. Judging it against 250 of what Words
// counts would fail the corpus for translating something it was allowed to
// publish, which is a rule about the law getting the law backwards.
//
// The stretch is measured and not guessed. Over every pair of English and
// translated files in the corpus with more than thirty words in them, the
// translation counts this many times what the English counts:
//
//	vi  median 1.38, largest 1.58
//	zh  median 1.64, largest 1.85
//	ja  median 2.39, largest 2.81
//
// The numbers below are the next round figure above the largest of each,
// so an ordinary translation is nowhere near the ceiling and a file that
// has grown a section it was never given still hits it. Japanese stretches
// furthest because kana spell out what English spells with one word and
// Words counts every one of them.
//
// English is one because English is what the ceiling was written in.
func Limit(n int, l Lang) int {
	switch l {
	case VI:
		return n * 7 / 4
	case ZH:
		return n * 2
	case JA:
		return n * 3
	}
	return n
}
