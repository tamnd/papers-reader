package split

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/assemble"
)

// entryLabel matches the label a bracketed bibliography prints in front of an
// entry, at the start of a paragraph and nowhere else.
//
// At the start only, because a bracketed number in the middle of a line is a
// citation and there are hundreds of those. Package refs has the same pattern
// written the other way round, matching anywhere and believed only when the
// count carries on, because it is reading a bibliography it has already been
// told is one. This is deciding whether there is a bibliography here at all,
// which is a question that wants a narrower question asked of it.
var entryLabel = regexp.MustCompile(`^\[(\d{1,3})\]\s`)

// entryYear is the year a reference carries. Every entry in every
// bibliography in the corpus has one, and it is what tells a reference from
// the other thing that begins with a bracketed number, which is a line of a
// numbered list that the extractor set in brackets.
var entryYear = regexp.MustCompile(`\b(1[6-9]\d{2}|20\d{2})\b`)

// entriesInARow is how many labelled entries counting up make a bibliography.
//
// Three, which is what package refs uses to decide the same thing for the
// same reason: two labels in a row is a coincidence and three counting up is
// a reference list.
const entriesInARow = 3

// headReferences supplies the heading a bibliography should have had.
//
// The Razborov paper is the reason. Its reference list runs over three pages
// and the first of those pages, the one with REFERENCES printed at the top of
// it, is one of the two pages the extraction never got back. So the document
// goes straight from the last sentence of the conclusion to entry [9], the
// splitter sees no heading there because there is none to see, and the whole
// bibliography is filed as the second half of the conclusion.
//
// What that costs is not cosmetic. The references are not in a references
// file, so rule L07 reads forty entries of English inside a Vietnamese
// section and reports every one of them as a paragraph the translator left
// alone, which is exactly what a reference is supposed to be. The Cyrillic in
// a Russian journal's name trips rule L13 for the same reason. None of it can
// be translated away, because all of it is correct.
//
// So the heading is supplied where the entries begin. It goes in as a
// paragraph rather than as a flag, so that everything downstream, the level,
// the kind, the ordinal, the filename and the page range, is worked out by
// the code that always works those out, and this function is the whole of the
// special case.
//
// It does nothing to a paper that heads its bibliography, which is ninety
// eight of the papers in the corpus, and nothing to a paper whose entries are
// not labelled, which cannot be told from prose with any confidence and is
// better left as it is than cut in the wrong place.
func headReferences(paragraphs []assemble.Paragraph) ([]assemble.Paragraph, bool) {
	if cited(paragraphs) {
		return paragraphs, false
	}
	texts := make([]string, len(paragraphs))
	for i, p := range paragraphs {
		texts[i] = p.Text
	}
	at := -1
	for i := range paragraphs {
		if entries(texts, i) >= entriesInARow {
			at = i
			break
		}
	}
	if at <= 0 {
		return paragraphs, false
	}
	out := make([]assemble.Paragraph, 0, len(paragraphs)+1)
	out = append(out, paragraphs[:at]...)
	// Unmarked, because a marked heading is read down a different path and
	// that path can see the rest of the paper. Written as "# References" this
	// put the only ATX heading into a document that had none, and the Razborov
	// paper came back a section short: the corollary the paper sets on a line
	// of its own stopped being a heading. The word on its own is found by
	// name, which is the same thing that finds an unheaded Acknowledgements,
	// and it leaves every other heading exactly where it was.
	//
	// The page is the page of the entry below it, because the heading is not a
	// thing that was on a page and the section it opens starts where that
	// entry starts.
	out = append(out, assemble.Paragraph{Text: "References", Page: paragraphs[at].Page, Pages: 1})
	return append(out, paragraphs[at:]...), true
}

// entries is how many labelled entries counting up start at a paragraph.
//
// Counting up, because a page of a paper has numbers all over it and the one
// thing a bibliography does that nothing else does is number its entries in
// order. Equal does not count: a list that prints [1] twice is not a list
// this should cut at.
func entries(texts []string, i int) int {
	n, last := 0, 0
	for ; i < len(texts); i++ {
		text := strings.TrimSpace(texts[i])
		m := entryLabel.FindStringSubmatch(text)
		if m == nil || !entryYear.MatchString(text) {
			break
		}
		label, err := strconv.Atoi(m[1])
		if err != nil || label <= last {
			break
		}
		last = label
		n++
	}
	return n
}

// cited says whether the document heads its bibliography somewhere, however it
// spells it and whether or not it numbers it.
//
// A false answer here means nothing is supplied, which is the status quo for
// the paper, so a spelling this misses costs nothing. A true answer where the
// heading is really a sentence about references would cost a paper its
// bibliography, so the line has to be short enough to be a heading and it is
// matched against the same name table the heading reader uses.
func cited(paragraphs []assemble.Paragraph) bool {
	for _, p := range paragraphs {
		if isReferences(p.Text) {
			return true
		}
	}
	return false
}

// isReferences says whether a paragraph is the heading over a bibliography.
func isReferences(text string) bool {
	text = strings.TrimSpace(text)
	if _, rest, ok := atxParts(text); ok {
		text = rest
	}
	if len([]rune(text)) > 60 {
		return false
	}
	title, ok := named(strings.TrimLeft(text, "0123456789. "))
	return ok && title == "References"
}

// oneBibliography takes out the References heading a bibliography printed
// again at the top of its next page.
//
// A reference list is the one part of a paper that runs for pages with
// nothing in it to say where it is, so a journal repeats the word at the
// head of each of them and a reader transcribing the page writes what it
// sees. The splitter then cuts at every one of them: the congestion
// avoidance paper came out with 11_references.md, 12_references.md and
// 13_references.md, holding entries 1 to 5, 6 to 22 and 23 to 25, and rule
// T07 reported all three. Nothing downstream can put them back together,
// and a reader following a citation to entry 23 has to guess which of the
// three files it is in.
//
// What makes a repeat a repeat is that there is nothing but the list
// between it and the heading above it. Every paragraph in between has to be
// an entry, and an entry is a paragraph that opens with the label the paper
// numbers it by. A paper that really does print two bibliographies has its
// appendix or its notes between them, and that ends the run.
func oneBibliography(paragraphs []assemble.Paragraph) ([]assemble.Paragraph, int) {
	out := make([]assemble.Paragraph, 0, len(paragraphs))
	inside, dropped := false, 0
	for _, p := range paragraphs {
		switch {
		case isReferences(p.Text):
			if inside {
				dropped++
				continue
			}
			inside = true
		case !label.MatchString(strings.TrimSpace(p.Text)):
			inside = false
		}
		out = append(out, p)
	}
	return out, dropped
}
