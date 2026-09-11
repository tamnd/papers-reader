// Package sources finds where a paper can legally be fetched from.
//
// The resolver tries, in order: an identifier pinned in the manifest, the
// arXiv API, Crossref, Unpaywall, OpenAlex, and then a handful of
// per-publisher rules for the sites with a stable URL shape and no usable
// API. It stops at the first candidate that yields a PDF URL with a licence.
//
// Everything except Verify talks to the network and arrives with milestone
// M1. Verify does not, and is here first because it is the part that keeps
// the corpus honest.
package sources

import (
	"regexp"
	"strings"
	"unicode"
)

// TitleFloor is how similar two titles must be before a candidate is
// accepted. It is deliberately high.
const TitleFloor = 0.92

// YearSlack is how far a candidate's year may be from the manifest's.
//
// One year, and it is not slack for its own sake: the manifest records the
// publication year and the resolver will usually find the preprint. ResNet is
// arXiv 2015 and CVPR 2016, P4 is arXiv 2013 and CCR 2014.
const YearSlack = 1

// Candidate is what a lookup came back with.
type Candidate struct {
	Title   string
	Authors []string
	Year    int
	DOI     string
	URL     string
	Licence string
	Source  string
}

// Want is what the manifest says the paper is.
type Want struct {
	ID      string
	Title   string
	Authors []string
	Year    int
}

// Verdict is why a candidate was accepted or refused. A refusal is recorded
// with its scores in reports/resolve.md, so the manual work left over is a
// short list rather than a search.
type Verdict struct {
	OK         bool
	TitleScore float64
	YearOff    int
	AuthorHit  bool
	Why        string
}

// Verify decides whether a candidate really is the paper that was asked for.
//
// It exists because title search is not trustworthy. Asked for the most cited
// paper in machine learning by title, one index answers with a 2025 preprint
// carrying a fabricated DOI. A resolver that took the first result would have
// fetched that, read it, translated it into three languages and published it
// under somebody else's name. So three predicates have to hold at once, and a
// candidate failing any of them leaves the paper unresolved, which is a fine
// state and a far better one than confidently wrong.
func Verify(want Want, got Candidate) Verdict {
	v := Verdict{
		TitleScore: TitleSimilarity(want.Title, got.Title),
		YearOff:    got.Year - want.Year,
		AuthorHit:  AuthorsOverlap(want.Authors, got.Authors),
	}
	switch {
	case v.TitleScore < TitleFloor:
		v.Why = "the titles are too different"
	case got.Year == 0:
		v.Why = "the candidate has no year"
	case abs(v.YearOff) > YearSlack:
		v.Why = "the years are more than a year apart"
	case !v.AuthorHit:
		v.Why = "no author of the candidate is an author of the paper"
	default:
		v.OK, v.Why = true, "title, year and authors all agree"
	}
	return v
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var punctuation = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// normalise casefolds a title and strips its punctuation, so that "Attention
// is all you need." and "Attention Is All You Need" are the same string.
func normalise(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = punctuation.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// TitleSimilarity is the Jaro-Winkler similarity of two titles, from 0 to 1,
// except that two titles which disagree about a negation score zero.
//
// Jaro-Winkler rather than an edit distance because it rewards a common
// prefix, and the way a title differs between two indexes is almost always a
// subtitle appended or a trailing phrase dropped rather than a change at the
// front.
//
// The negation check is there because that same prefix bonus is what makes
// Jaro-Winkler wrong in the one case worth being right about. "Attention Is
// All You Need" and "Attention Is Not All You Need" score 0.936, which clears
// the floor, and they are different papers arguing opposite things. Three
// letters in the middle of a title can invert it, and no character similarity
// measure will ever notice, so the words that do that are listed and counted.
//
// The cost of the check is a title that legitimately drops a subtitle
// containing one of these words, which leaves the paper unresolved. That is
// the cheap failure and it is the one to prefer.
func TitleSimilarity(a, b string) float64 {
	na, nb := normalise(a), normalise(b)
	if !polarityAgrees(na, nb) {
		return 0
	}
	return JaroWinkler(na, nb)
}

// polarity is the words that invert a title. It is short on purpose: every
// entry has to be a word whose presence changes what the paper claims,
// because a word added here that merely qualifies a claim costs a resolution.
var polarity = map[string]bool{
	"not": true, "no": true, "non": true, "never": true,
	"without": true, "neither": true, "nor": true, "cannot": true,
	"impossible": true, "impossibility": true,
}

// polarityAgrees reports whether two normalised titles use the negating words
// the same number of times.
func polarityAgrees(a, b string) bool {
	counts := map[string]int{}
	for _, w := range strings.Fields(a) {
		if polarity[w] {
			counts[w]++
		}
	}
	for _, w := range strings.Fields(b) {
		if polarity[w] {
			counts[w]--
		}
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// JaroWinkler is the standard similarity, with the usual 0.1 prefix scale
// over at most four leading characters.
func JaroWinkler(a, b string) float64 {
	j := Jaro(a, b)
	if j <= 0.7 {
		return j
	}
	ra, rb := []rune(a), []rune(b)
	prefix := 0
	for prefix < 4 && prefix < len(ra) && prefix < len(rb) && ra[prefix] == rb[prefix] {
		prefix++
	}
	return j + float64(prefix)*0.1*(1-j)
}

// Jaro is the Jaro similarity of two strings.
func Jaro(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}

	window := max(len(ra), len(rb))/2 - 1
	if window < 0 {
		window = 0
	}
	matchedA := make([]bool, len(ra))
	matchedB := make([]bool, len(rb))

	matches := 0
	for i, c := range ra {
		lo := max(0, i-window)
		hi := min(len(rb)-1, i+window)
		for j := lo; j <= hi; j++ {
			if matchedB[j] || rb[j] != c {
				continue
			}
			matchedA[i], matchedB[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	transpositions, k := 0, 0
	for i := range ra {
		if !matchedA[i] {
			continue
		}
		for !matchedB[k] {
			k++
		}
		if ra[i] != rb[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(len(ra)) + m/float64(len(rb)) + (m-float64(transpositions)/2)/m) / 3
}

// AuthorsOverlap reports whether any author of the candidate is an author of
// the paper, compared by surname.
//
// Surnames only, because the two sides disagree about initials, about the
// order of given names, and about diacritics, and none of those disagreements
// mean it is a different paper. An empty want matches nothing: a manifest
// entry with no authors has made no claim to check.
func AuthorsOverlap(want, got []string) bool {
	if len(want) == 0 || len(got) == 0 {
		return false
	}
	set := make(map[string]bool, len(want))
	for _, a := range want {
		if s := Surname(a); s != "" {
			set[s] = true
		}
	}
	for _, a := range got {
		if set[Surname(a)] {
			return true
		}
	}
	return false
}

// Surname is the family name of an author as written, folded to lowercase
// ASCII letters. "Ashish Vaswani" and "Vaswani, A." both give "vaswani".
func Surname(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if before, _, ok := strings.Cut(name, ","); ok {
		name = before
	} else if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[len(fields)-1]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(fold(name)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fold strips the diacritics this corpus actually meets in an author list.
// It is a table rather than a Unicode normalisation because the dependency is
// not worth it for eleven letters.
var folded = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ý", "y", "ÿ", "y", "ñ", "n", "ç", "c", "š", "s", "ž", "z", "ł", "l",
	"ć", "c", "č", "c", "đ", "d", "ğ", "g", "ı", "i", "ş", "s",
)

func fold(s string) string { return folded.Replace(strings.ToLower(s)) }
