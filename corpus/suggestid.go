package corpus

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SuggestID is the id a paper would get if nobody thought about it: the first
// author's surname, the year of publication, and the first word of the title
// that carries any meaning.
//
// It is a suggestion and it says so in the name, because the third part is a
// judgement a program cannot make. Half the hundred have a keyword that is
// nowhere in their title. He 2016 is called resnet and the title is "Deep
// Residual Learning for Image Recognition", Nakamoto 2008 is called bitcoin
// and the title is "A Peer-to-Peer Electronic Cash System", and both of those
// are the name the field actually uses. What this gets right is the two parts
// that are facts, and it gets the third part to something typeable so that a
// person editing it is editing rather than composing.
//
// An id is permanent, so the one place this is called from prints it and
// takes an override. See ID for why it can never be changed afterwards.
func SuggestID(authors []string, year int, title string) (string, error) {
	if len(authors) == 0 || strings.TrimSpace(authors[0]) == "" {
		return "", fmt.Errorf("an id starts with an author's surname and there are no authors")
	}
	if year < 1000 || year > 9999 {
		return "", fmt.Errorf("%d is not a year an id can carry", year)
	}
	surname := letters(Surname(authors[0]))
	if surname == "" {
		return "", fmt.Errorf("%q has no surname in it that an id could use", authors[0])
	}
	keyword := letters(Keyword(title))
	if keyword == "" {
		return "", fmt.Errorf("%q has no word in it that an id could use", title)
	}
	id := fmt.Sprintf("%s-%d-%s", surname, year, keyword)
	// A surname that is all digits, or a keyword that came out empty after
	// the accents were taken off, makes a string that is not an id. Better to
	// say so here than to have the manifest refuse it later with no idea
	// where it came from.
	if _, err := ParseID(id); err != nil {
		return "", err
	}
	return id, nil
}

// Surname is the family name in a name written the way a paper prints it,
// which is given names first.
//
// The suffixes come off. "Frederick P. Brooks Jr." is Brooks, and an id that
// read brooks-jr-1987 would be an id nobody types twice. A particle stays
// attached, so "George van den Driessche" is vandendriessche, because the
// alternative is deciding which particles are part of a surname in which
// language and the answer to that differs by person and not by language.
func Surname(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	for len(fields) > 1 && suffixes[strings.ToLower(strings.Trim(fields[len(fields)-1], ".,"))] {
		fields = fields[:len(fields)-1]
	}
	if len(fields) == 0 {
		return ""
	}
	// Walk back to the first particle, so that the whole of a compound
	// surname is taken and not only its last word.
	at := len(fields) - 1
	for at > 0 && particles[strings.ToLower(fields[at-1])] {
		at--
	}
	return strings.Join(fields[at:], " ")
}

// Keyword is the first word of a title that says anything, which is the first
// one that is not an article, a preposition or a conjunction.
//
// A hyphenated word is cut at the hyphen, because an id has hyphens of its
// own and "peer-to-peer" would make an id with five parts in it.
func Keyword(title string) string {
	for _, w := range strings.FieldsFunc(title, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == ':' || r == ','
	}) {
		w = strings.Trim(w, `.,;:?!"'()[]`)
		if w == "" || stopwords[strings.ToLower(w)] {
			continue
		}
		return w
	}
	return ""
}

// suffixes are the generational and honorific tails that are not a surname.
var suffixes = map[string]bool{
	"jr": true, "sr": true, "ii": true, "iii": true, "iv": true, "phd": true,
}

// particles are the words that belong to the surname that follows them.
var particles = map[string]bool{
	"van": true, "von": true, "de": true, "del": true, "della": true,
	"di": true, "da": true, "den": true, "der": true, "la": true,
	"le": true, "du": true, "dos": true, "ten": true, "ter": true,
}

// stopwords are the words a title can start with that say nothing about what
// the paper is. It is a short list on purpose: a word that is not on it and
// should be is one id somebody edits, and a word that is on it and should not
// be is a keyword silently thrown away.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "on": true, "of": true, "in": true,
	"for": true, "to": true, "and": true, "or": true, "with": true,
	"towards": true, "toward": true, "some": true, "new": true, "is": true,
	"at": true, "by": true, "from": true, "into": true, "over": true,
	// "no" is on the list even though it carries meaning, because it carries
	// it about the word after it and never on its own. Nobody would call the
	// Brooks paper brooks-1987-no.
	"no": true, "not": true,
}

// letters is s reduced to the lowercase ASCII letters and digits an id is
// allowed to contain.
//
// The accents come off rather than the letters carrying them, so that Bui
// Tuong Phong and Lukasz Kaiser get an id that can be typed on a keyboard
// that has no way to produce the letter. NFD splits a letter from its mark
// and the marks are then dropped; the handful of letters with no
// decomposition at all are spelled out in fold.
func letters(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteString(fold[r])
		}
	}
	return b.String()
}

// fold is the letters NFD leaves alone because the accent is part of the
// letter rather than a mark on it. A missing entry is the empty string, which
// is what a space and a full stop should come to as well.
var fold = map[rune]string{
	'ø': "o", 'đ': "d", 'ð': "d", 'ł': "l", 'ß': "ss",
	'æ': "ae", 'œ': "oe", 'þ': "th", 'ħ': "h", 'ı': "i",
}
