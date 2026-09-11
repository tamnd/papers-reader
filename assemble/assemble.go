// Package assemble joins the page files of one paper into a document.
//
// A page is where the paper ran out of room. Nothing a reader wants to know
// is expressed by it: a sentence runs across a page break, a word is broken
// across one, and a paragraph that starts at the top of page six started on
// page five. So the pages are joined and the joins are healed, and what
// comes out is a run of paragraphs with the page each one started on kept
// alongside, because the reading app says "page 6 of the PDF" and a person
// checking the extraction against the paper needs to find the page.
package assemble

import (
	"strings"
	"unicode"
)

// A Page is one extracted page as it sits in work/<id>/pages/NNNN.txt:
// paragraphs one to a line with blank lines between them.
type Page struct {
	Number int
	Text   string
}

// A Paragraph is one paragraph of the assembled paper.
type Paragraph struct {
	Text string
	// Page is the page it began on, which is the page a reader is sent to.
	Page int
	// Pages is how many pages it ran across, and is one for almost all of
	// them. A paragraph that ran across three pages is worth a look.
	Pages int
}

// A Document is one paper, joined.
type Document struct {
	Paragraphs  []Paragraph
	First, Last int
}

// Join assembles the pages in the order they are given.
//
// The joining rule is the one Bourbaki uses and it is deliberately narrow: a
// page that ends without terminal punctuation and whose next page starts
// lower case is a continuation. Anything else starts a new paragraph.
//
// Narrow because the two mistakes are not equal. Two paragraphs wrongly run
// together read as one confused paragraph and the split point is gone; a
// paragraph wrongly broken in two reads as two paragraphs and a person can
// see where it happened. Over a hundred papers the second is the one to
// prefer.
func Join(pages []Page) *Document {
	d := &Document{}
	for _, p := range pages {
		if d.First == 0 || p.Number < d.First {
			d.First = p.Number
		}
		if p.Number > d.Last {
			d.Last = p.Number
		}
		for i, text := range paragraphs(p.Text) {
			n := len(d.Paragraphs)
			// Only the first paragraph of a page can continue the page
			// before it. Inside a page the break was decided by the
			// geometry, which knows more than the punctuation does.
			if i == 0 && n > 0 && continues(d.Paragraphs[n-1].Text, text) {
				d.Paragraphs[n-1].Text = joinText(d.Paragraphs[n-1].Text, text)
				d.Paragraphs[n-1].Pages++
				continue
			}
			d.Paragraphs = append(d.Paragraphs, Paragraph{Text: text, Page: p.Number, Pages: 1})
		}
	}
	return d
}

// Text is the document as one string, a paragraph to a line.
func (d *Document) Text() string {
	if len(d.Paragraphs) == 0 {
		return ""
	}
	parts := make([]string, len(d.Paragraphs))
	for i, p := range d.Paragraphs {
		parts[i] = p.Text
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func paragraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// continues says whether the second paragraph is the rest of the first.
func continues(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	r := []rune(right)[0]
	if !unicode.IsLower(r) && !strings.ContainsRune(",;", r) {
		return false
	}
	return !closed(left)
}

// joinText puts two halves of a paragraph back together, healing the word
// that was broken across the page break.
func joinText(left, right string) string {
	l := []rune(left)
	if strings.ContainsRune(hyphens, l[len(l)-1]) {
		word, _, _ := strings.Cut(right, " ")
		if !strings.ContainsAny(word, hyphens) {
			return strings.TrimRight(left, hyphens) + right
		}
		return left + right
	}
	return left + " " + right
}

// hyphens is every character a line break hyphen is written with. It is the
// same set package extract heals inside a page, and the two have to agree:
// a word broken across a page break is the same word broken at a line end,
// and a reader who saw one healed and the other left would be right to
// wonder which spelling the corpus uses.
const hyphens = "-\u2010\u2011\u00ad"

// closed says whether a paragraph ends where a sentence ends.
//
// A full stop after a single capital is an initial and not the end of
// anything, which is a real case at a page break: a bibliography entry
// broken after "C." is one entry and not two.
func closed(s string) bool {
	s = strings.TrimRight(s, `"'”’)]}`)
	if s == "" {
		return false
	}
	r := []rune(s)
	last := r[len(r)-1]
	if last == '.' && len(r) >= 2 && unicode.IsUpper(r[len(r)-2]) &&
		(len(r) == 2 || r[len(r)-3] == ' ') {
		return false
	}
	switch last {
	case '.', '!', '?', ':', ';':
		return true
	}
	return false
}
