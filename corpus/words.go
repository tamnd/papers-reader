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
// A character is not a word and this does not pretend otherwise. Chinese
// runs at something like one and a half to two characters per English word,
// so a floor of forty words is nearer twenty-five English words' worth when
// the paragraph is Chinese. That is the right direction to be wrong in for
// everything that counts here: a title block is a dozen characters in any
// language, and a quotation from a restricted paper is refused sooner
// rather than later.
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
