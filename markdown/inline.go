package markdown

import "regexp"

// The patterns of the inline markup the corpus writes.
//
// They are exported and the renderers are not built on top of a shared
// renderer, because each of the three writes different markup for the same
// span and they share only the question of where the span is. A pattern here
// is read by all three, so a citation that one of them stopped recognising
// would be a citation none of them recognised, which is the kind of fault
// somebody notices.
var (
	// Code is a span in backticks, any number of them.
	Code = regexp.MustCompile("`+[^`]+`+")

	// Note is a footnote marker.
	Note = regexp.MustCompile(`\[\^([^\]\s]+)\]`)

	// LooseNote is a space in front of a footnote marker. The corpus has
	// them, off pages where the marker was set raised and the reader put a
	// space where the baseline dropped. A footnote hangs on the word before
	// it with no space, in every one of the four languages, so the space
	// goes.
	LooseNote = regexp.MustCompile(`[ \t]+(\[\^[^\]\s]+\])`)

	// PaperCite is a citation of another paper of this corpus, written as
	// the identifier in double brackets. It is the one piece of navigation
	// the corpus adds that the paper did not have.
	PaperCite = regexp.MustCompile(`\[\[([a-z][a-z0-9]*-[0-9]{4}-[a-z0-9]+)\]\]`)

	// NumCite is a numeric citation, single or a list or a range.
	//
	// Three digits at most, which is the cap the refs package already puts
	// on the citations it rewrites. A bibliography label is a small number
	// and the longest reference list in the corpus is under two hundred
	// entries, so a four digit number in brackets is the year of an author
	// and year citation and not a label. The Paxos paper writes "the
	// explanation of the algorithm for computer scientists by Lampson
	// [1996]", its bibliography has no entry 1996 and never will, and
	// without the cap that page carried six links to nothing and rule P02
	// held the paper out of the corpus for all six.
	NumCite = regexp.MustCompile(`\[([0-9]{1,3}(?:\s*[,\x{2013}-]\s*[0-9]{1,3})*)\]`)

	// Digits picks the numbers out of one of those, so that [3, 7] links
	// twice and the comma between stays punctuation.
	Digits = regexp.MustCompile(`[0-9]+`)

	Strong = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	Emph   = regexp.MustCompile(`(^|[^*])\*([^*]+)\*`)
)
