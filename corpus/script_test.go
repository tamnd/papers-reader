package corpus

import "testing"

func TestEachLanguageIsWrittenInItsOwnScripts(t *testing.T) {
	for _, c := range []struct {
		l      Lang
		writes string
		not    string
	}{
		{EN, "Latin", "Han"},
		{VI, "Latin", "Cyrillic"},
		{ZH, "Han", "Hiragana"},
		{JA, "Katakana", "Cyrillic"},
	} {
		if !c.l.Writes(c.writes) {
			t.Errorf("%s is not written in %s", c.l, c.writes)
		}
		if c.l.Writes(c.not) {
			t.Errorf("%s is written in %s", c.l, c.not)
		}
	}
	// Han in a Vietnamese page is wrong and in a Japanese one is right,
	// which is the pair the table exists for.
	if VI.Writes("Han") || !JA.Writes("Han") {
		t.Error("the table does not tell Vietnamese and Japanese apart over Han")
	}
	if Lang("de").Scripts() != nil {
		t.Error("a language that is not a corpus language has scripts")
	}
}

func TestScriptNamesTheAlphabetAndNothingElse(t *testing.T) {
	for r, want := range map[rune]string{
		'a': "Latin",
		'л': "Cyrillic",
		'見': "Han",
		'の': "Hiragana",
		'ア': "Katakana",
		// None of these is a word in another alphabet, and a rule that
		// called them one would report every formula in the corpus.
		'α': "",
		'∑': "",
		'→': "",
		'7': "",
		'.': "",
	} {
		if got := Script(r); got != want {
			t.Errorf("%q is %q, want %q", string(r), got, want)
		}
	}
}

func TestForeignFindsTheFirstWordInAnotherAlphabet(t *testing.T) {
	// The shape that went into the corpus: the Russian for "either" in
	// the middle of a Vietnamese sentence.
	r, script := Foreign("Điều này đạt được либо bằng cách này", VI)
	if r != 'л' || script != "Cyrillic" {
		t.Errorf("Foreign gave %q and %q, want the Cyrillic л", string(r), script)
	}
	for _, c := range []struct {
		s string
		l Lang
	}{
		{"Điều này đạt được bằng cách này và cách kia", VI},
		{"这是一段中文，其中有 BLEU 这个英文缩写", ZH},
		{"これはカタカナと漢字とひらがなの文です", JA},
		{"The formula is $\\alpha \\sum x_i \\to 0$ and nothing else", EN},
	} {
		if r, script := Foreign(c.s, c.l); r != 0 {
			t.Errorf("%s: %q was called %s", c.l, string(r), script)
		}
	}
	if r, _ := Foreign("либо", Lang("de")); r != 0 {
		t.Error("a language that is not a corpus language has a foreign alphabet")
	}
}
