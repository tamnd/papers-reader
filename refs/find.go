package refs

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

// Bibliography is the run of paragraphs holding the paper's reference list.
//
// It is found by its heading and not by position, because the reference list
// is very often not the last thing in the paper. Appendices come after it,
// and so do acknowledgements in about a third of what is on the list, and a
// parser that took everything after the heading would file an appendix full
// of proofs as forty malformed references.
//
// The heading is looked for from the end backwards. A paper that says "see
// the references" in its introduction has the word on page one, and the
// heading is the last one, not the first.
func Bibliography(d *assemble.Document) []assemble.Paragraph {
	if d == nil {
		return nil
	}
	start := -1
	for i := len(d.Paragraphs) - 1; i >= 0; i-- {
		if isBibliographyHeading(d.Paragraphs[i].Text) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	end := len(d.Paragraphs)
	for i := start; i < end; i++ {
		if endsBibliography(d.Paragraphs[i].Text) {
			end = i
			break
		}
	}
	if start >= end {
		return nil
	}
	return gathered(d.Paragraphs[:start], d.Paragraphs[start:end])
}

// Reorder moves the entries the column order scattered back into the
// reference section of the document itself, and says whether it moved any.
//
// Bibliography gathers the same entries for the index it builds, and that is
// not enough on its own. The section file the splitter writes is cut from
// this document, so with the document left as it came the index held seven
// references and the references section of the paper printed one, with the
// other six stranded at the foot of the appendix. Audit rule R08 is the one
// that notices, and it is right to: an index that names text the page it
// points at does not have is worse than no index.
//
// So the move is made once, here, before anything reads the document. The
// splitter and the index builder both go through it and both see the paper
// in the order the page meant.
func Reorder(d *assemble.Document) bool {
	if d == nil {
		return false
	}
	head, end := span(d)
	if head < 0 {
		return false
	}
	last := counted(d.Paragraphs[head+1 : end])
	if last == 0 {
		return false
	}
	at, from, to := runIn(d.Paragraphs[:head], last+1)
	if at < 0 {
		return false
	}
	lines := strings.Split(d.Paragraphs[at].Text, "\n")
	run := make([]assemble.Paragraph, 0, to-from)
	for _, line := range lines[from:to] {
		run = append(run, assemble.Paragraph{Text: line, Page: d.Paragraphs[at].Page, Pages: 1})
	}
	kept := strings.TrimSpace(strings.Join(append(append([]string{}, lines[:from]...), lines[to:]...), "\n"))

	out := make([]assemble.Paragraph, 0, len(d.Paragraphs)+len(run))
	for i, p := range d.Paragraphs {
		if i == at {
			if kept == "" {
				continue
			}
			p.Text = kept
		}
		out = append(out, p)
		if i == end-1 {
			out = append(out, run...)
		}
	}
	d.Paragraphs = out
	return true
}

// span is the index of the bibliography heading and the index one past the
// last paragraph of the section, or -1 if the paper has no heading.
func span(d *assemble.Document) (int, int) {
	head := -1
	for i := len(d.Paragraphs) - 1; i >= 0; i-- {
		if isBibliographyHeading(d.Paragraphs[i].Text) {
			head = i
			break
		}
	}
	if head < 0 {
		return -1, 0
	}
	end := len(d.Paragraphs)
	for i := head + 1; i < end; i++ {
		if endsBibliography(d.Paragraphs[i].Text) {
			end = i
			break
		}
	}
	if head+1 >= end {
		return -1, 0
	}
	return head, end
}

// counted is the last entry number of a section whose entries count up from
// one with no gaps, and zero for a section that does anything else. A
// bibliography that is already out of order is left alone: there is no
// telling where a run belongs in it.
func counted(section []assemble.Paragraph) int {
	last := 0
	for _, p := range section {
		n, ok := labelled(p.Text)
		if !ok {
			continue
		}
		if n != last+1 {
			return 0
		}
		last = n
	}
	return last
}

// runIn is the paragraph holding a run of at least two entries numbered from
// first upwards, and the half open range of its lines the run covers.
func runIn(ps []assemble.Paragraph, first int) (int, int, int) {
	for i, p := range ps {
		lines := strings.Split(p.Text, "\n")
		for from := range lines {
			to := from
			for to < len(lines) {
				n, ok := labelled(lines[to])
				if !ok || n != first+to-from {
					break
				}
				to++
			}
			if to-from >= 2 {
				return i, from, to
			}
		}
	}
	return -1, 0, 0
}

// label is a bracketed entry number at the head of a paragraph, which is the
// one bibliography style whose entries can be put back in order by reading
// them. The other three are not gathered, because a surname or a bare "12."
// is not evidence enough to move a paragraph on.
var label = regexp.MustCompile(`^\[([0-9]{1,3})\]\s`)

// gathered puts back the entries that the reading order scattered.
//
// The last page of a two column paper is where this goes wrong. McCabe's
// page 13 sets the end of the appendix and the REFERENCES heading and entry
// [1] down the left column, and entries [2] to [7] carry on at the top of
// the right one. The reader took the right column first, so six of the seven
// references arrived before the heading and were filed as the last six lines
// of the appendix: the bibliography was one entry long, three citations in
// the body pointed at nothing, and rules R02 and P02 reported nine findings
// between them for what is one misread page.
//
// A run is only moved if its numbers carry straight on from the last one the
// section already has, counting up by one, with no gaps and nothing else
// mixed in. That is a strong test and it is meant to be: a numbered list in
// the body of a paper is common, and one that happens to continue the
// bibliography's numbering from exactly where it stopped is not.
//
// The search is over lines and not paragraphs, because six entries with no
// blank line between them are one paragraph. Each line of the run becomes a
// paragraph of its own on the way out, which is what the rest of the parse
// expects and what the page meant.
func gathered(before, section []assemble.Paragraph) []assemble.Paragraph {
	last := 0
	for _, p := range section {
		n, ok := labelled(p.Text)
		if !ok {
			continue
		}
		if n != last+1 {
			return section
		}
		last = n
	}
	if last == 0 {
		return section
	}
	lines := loose(before)
	for i := range lines {
		run := runFrom(lines[i:], last+1)
		if len(run) < 2 {
			continue
		}
		out := make([]assemble.Paragraph, 0, len(section)+len(run))
		out = append(out, section...)
		return append(out, run...)
	}
	return section
}

// loose is the paragraphs one line at a time, so that a run of entries set
// with no blank line between them can be seen for what it is. The page a
// paragraph began on is carried onto every line of it, which is as close as
// the assembler can say.
func loose(ps []assemble.Paragraph) []assemble.Paragraph {
	var out []assemble.Paragraph
	for _, p := range ps {
		for _, line := range strings.Split(p.Text, "\n") {
			out = append(out, assemble.Paragraph{Text: line, Page: p.Page, Pages: 1})
		}
	}
	return out
}

// runFrom is the run of entries at the head of ps numbered from first
// upwards, and is empty if the first line is not entry first.
func runFrom(ps []assemble.Paragraph, first int) []assemble.Paragraph {
	n := 0
	for n < len(ps) {
		got, ok := labelled(ps[n].Text)
		if !ok || got != first+n {
			break
		}
		n++
	}
	return ps[:n]
}

// labelled reads the entry number off a line.
func labelled(text string) (int, bool) {
	m := label.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// bibliographyNames is every heading the reference list is printed under,
// casefolded and with the section number already taken off.
var bibliographyNames = map[string]bool{
	"references":           true,
	"reference":            true,
	"references cited":     true,
	"bibliography":         true,
	"literature cited":     true,
	"works cited":          true,
	"notes and refs":       true,
	"notes and references": true,
	"references and notes": true,
}

// afterNames is every heading that can follow the reference list. A heading
// that is not on this list does not end the bibliography, because the risk
// runs the other way: a reference beginning with a short institutional name
// would otherwise cut the list in half.
var afterNames = map[string]bool{
	"appendix":                true,
	"appendices":              true,
	"acknowledgement":         true,
	"acknowledgements":        true,
	"acknowledgment":          true,
	"acknowledgments":         true,
	"author contributions":    true,
	"supplementary material":  true,
	"supplementary materials": true,
	"supporting information":  true,
	"about the authors":       true,
	"biographies":             true,
}

func isBibliographyHeading(text string) bool {
	key, ok := headingKey(text)
	return ok && bibliographyNames[key]
}

func endsBibliography(text string) bool {
	if isBiography(text) {
		return true
	}
	key, ok := headingKey(text)
	if !ok {
		return false
	}
	return afterNames[key] || strings.HasPrefix(key, "appendix")
}

// biography is the opening of an author biography as the journals of the
// nineteen seventies printed one: the author's name, sometimes with an IEEE
// membership grade after it, then "was born".
var biography = regexp.MustCompile(`^` + nameWord + `(?:\s+` + nameWord + `){0,6}\s+w(?:as|ere) born\b`)

// nameWord is one word of a person's name as a biography prints it: a
// capitalised word, an initial, the "and" between two authors, or the
// membership grade the IEEE sets in parentheses. Nothing else is allowed
// through, which is what keeps a reference out: a title is quoted and an
// author list is punctuated with commas, and neither is a name word.
const nameWord = `(?:[A-Z][\pL.'-]*|and|\([^()\n]*\))`

// isBiography says a paragraph is where the author biographies start.
//
// A biography is set straight after the reference list with no heading over
// it, so nothing on afterNames catches it and the parse runs on through the
// lot. All four papers in the corpus that print one had their biography read
// as references: McCabe's bibliography came out as one entry and three
// paragraphs about where he went to school, and Cerf, Dennard and Brin are
// the same. The paragraphs are still in the section file, where they belong,
// because the page printed them there. They are just not references.
//
// The name is capped at sixty characters before "was born" so that a
// reference whose title happens to contain the phrase is not mistaken for
// one. Two authors sharing a paragraph is why "were born" is here too.
func isBiography(text string) bool {
	return biography.MatchString(strings.TrimSpace(text))
}

// headingKey reduces a paragraph to the words of its heading, or says it is
// not short enough to be one.
func headingKey(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	// The length cap is a guess at what a heading looks like, and it only has
	// to be guessed at for a paragraph with no marker on it. A line that
	// starts with an ATX marker is a heading whatever its length, and some of
	// them are long: BERT heads its appendix with the whole title of the
	// paper in quotation marks, a hundred and two characters of it, and under
	// the cap that heading did not end the bibliography and the parse ran on
	// through the appendix.
	if !strings.HasPrefix(text, "#") && len([]rune(text)) > 60 {
		return "", false
	}
	text = strings.TrimLeft(text, "# \t")
	text = strings.TrimLeft(text, "§ \t")
	// Drop a leading section number in any of the schemes package split
	// knows: "6", "6.", "A.", "VII.".
	if i := strings.IndexFunc(text, unicode.IsSpace); i > 0 && i <= 6 && isNumbering(text[:i]) {
		text = text[i:]
	}
	text = strings.Trim(text, " \t.:*#")
	if text == "" {
		return "", false
	}
	return strings.ToLower(strings.Join(strings.Fields(text), " ")), true
}

// isNumbering says whether a word is a section number rather than the first
// word of the heading.
func isNumbering(s string) bool {
	s = strings.TrimRight(s, ".")
	if s == "" {
		return false
	}
	digits, romans := true, true
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			digits = false
		}
		if !strings.ContainsRune("IVXL", r) {
			romans = false
		}
	}
	return digits || romans
}
