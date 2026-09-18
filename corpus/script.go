package corpus

import "unicode"

// Which writing systems a corpus language is written in.
//
// This is a fact about the four languages and it lives here with the rest of
// them, because two different parts of the toolchain ask it and they have to
// agree. The translator asks before it writes a chunk, so that an answer in
// the wrong alphabet is asked for again while there is still something to ask.
// The audit asks about the files on disk, so that one which got written
// anyway does not get published. Two copies of this table would be two
// answers to one question, and the one on disk would be the one that mattered.

// scripts is the writing systems each language uses.
//
// The four languages disagree about nearly every entry. Han characters in a
// Vietnamese page are wrong and in a Japanese one are right. Kana in a
// Chinese page are wrong. Latin letters are right everywhere, because every
// one of these languages writes a model's name, an author's name and an
// acronym in Latin.
var scripts = map[Lang][]string{
	EN: {"Latin"},
	VI: {"Latin"},
	ZH: {"Latin", "Han"},
	JA: {"Latin", "Han", "Hiragana", "Katakana"},
}

// alphabets are the writing systems that can be named, keyed by the name to
// call them by.
//
// A rune in none of them is not a word in another alphabet. Mathematical
// symbols, arrows, box drawing and punctuation all land there, and none of
// them says anything about what language a passage is in.
var alphabets = map[string]*unicode.RangeTable{
	"Latin":      unicode.Latin,
	"Han":        unicode.Han,
	"Hiragana":   unicode.Hiragana,
	"Katakana":   unicode.Katakana,
	"Hangul":     unicode.Hangul,
	"Cyrillic":   unicode.Cyrillic,
	"Arabic":     unicode.Arabic,
	"Hebrew":     unicode.Hebrew,
	"Thai":       unicode.Thai,
	"Devanagari": unicode.Devanagari,
}

// Scripts is the writing systems l is written in, and nil for a code that is
// not a corpus language, which is how a caller tells that there is nothing to
// check rather than that everything fails.
func (l Lang) Scripts() []string { return scripts[l] }

// Writes reports whether l is written in the named script.
func (l Lang) Writes(name string) bool {
	for _, s := range scripts[l] {
		if s == name {
			return true
		}
	}
	return false
}

// Script is the name of the writing system r belongs to, and the empty
// string for a rune that belongs to none of the named ones.
func Script(r rune) string {
	for name, table := range alphabets {
		if unicode.Is(table, r) {
			return name
		}
	}
	return ""
}

// Foreign is the first rune of s that l is not written in, with the name of
// its writing system.
//
// Both are zero for a passage that is all in l, and for a language that is
// not a corpus language, since there is nothing to compare it against.
//
// What is passed in should be the prose, with the mathematics and the
// listings already taken out. A Greek letter in a formula is a formula.
func Foreign(s string, l Lang) (rune, string) {
	if scripts[l] == nil {
		return 0, ""
	}
	for _, r := range s {
		name := Script(r)
		if name == "" || l.Writes(name) {
			continue
		}
		return r, name
	}
	return 0, ""
}
