package extract

import (
	"regexp"
	"strings"
)

// address is a bare web address, ending before the punctuation a sentence
// put after it. A paper writes "available at http://example.org/x." and the
// full stop is the sentence's.
var address = regexp.MustCompile(`(?:https?://|www\.)[^\s<>()\[\]"]*[^\s<>()\[\]".,;:!?]`)

// Nearest is how far a transcribed address may be from one in the file's own
// text layer and still be taken for the same address.
//
// Four characters. The one this was written for is three: page 13 of the
// MapReduce paper prints
// "http://research.microsoft.com/barc/SortBenchmark/" and the reading came
// back with "bench" where the page has "barc", which is barc to bench, two
// substitutions and an insertion. Five would start to reach across the
// numbered addresses a paper lists one after another, of which the corpus
// has several runs, and four is the most that never does.
const Nearest = 4

// Readdress puts back the addresses a reader retyped, using the file's own
// text layer as the authority.
//
// A web address is the one thing on a page that a reader cannot improve and
// can only get wrong. It is not a word, so nothing about the sentence
// constrains it, and a model that has seen a million addresses will supply
// the one it expects rather than the one printed. That is what happened to
// reference 10 of the MapReduce paper, where "barc" came back "bench" and
// broke rule R08, because the bibliography no longer matched the text the
// paper printed.
//
// Re-reading the page does not fix it. The mistake is not a misread glyph,
// it is a plausible address, and a second reading produces the same
// plausible address. But the right characters are sitting in the file: the
// text layer of a scholarly PDF carries the address exactly even where its
// layout is worthless, which is why this is worth doing on the vision path,
// where the layer was rejected for everything else.
//
// Only a near miss is corrected. An address on the page with nothing like it
// in the layer is left alone, because that is a layer that does not have the
// address rather than a reading that got it wrong, and rewriting one address
// into an unrelated one is a worse failure than the one being repaired.
func Readdress(text, layer string) string {
	found := address.FindAllString(layer, -1)
	if len(found) == 0 {
		return text
	}
	have := map[string]bool{}
	for _, a := range found {
		have[a] = true
	}
	lines := strings.Split(text, "\n")
	code := codeLines(lines)
	for i, line := range lines {
		if code[i] {
			continue
		}
		lines[i] = address.ReplaceAllStringFunc(line, func(got string) string {
			if have[got] {
				return got
			}
			if want, ok := nearest(got, found); ok {
				return want
			}
			return got
		})
	}
	return strings.Join(lines, "\n")
}

// nearest is the layer address closest to the one transcribed, and whether
// it is close enough to be the same address. A tie is no answer: two layer
// addresses the same distance away means this cannot tell which was meant.
func nearest(got string, layer []string) (string, bool) {
	best, at, ties := Nearest+1, "", 0
	for _, want := range layer {
		d := distance(got, want)
		switch {
		case d < best:
			best, at, ties = d, want, 1
		case d == best:
			ties++
		}
	}
	return at, best <= Nearest && ties == 1
}

// distance is Levenshtein, over runes, with one row of the matrix kept.
func distance(a, b string) int {
	x, y := []rune(a), []rune(b)
	row := make([]int, len(y)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(x); i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= len(y); j++ {
			was := row[j]
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			row[j] = min(row[j]+1, min(row[j-1]+1, prev+cost))
			prev = was
		}
	}
	return row[len(y)]
}
