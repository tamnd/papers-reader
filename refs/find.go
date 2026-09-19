package refs

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
	"github.com/tamnd/papers-reader/split"
)

// Bibliography is the run of paragraphs holding the paper's reference list.
//
// It is found by its heading and not by position, because the reference list
// is very often not the last thing in the paper. Appendices come after it,
// and so do acknowledgements in about a third of what is on the list, and a
// parser that took everything after the heading would file an appendix full
// of proofs as forty malformed references.
//
// Which heading, when a paper has more than one, is the one with the
// longest list of numbered entries under it.
//
// A paper that says "see the references" in its introduction has the word on
// page one, and a journal prints it again at the top of every page of a long
// list. Jacobson's runs over three pages of the SIGCOMM proceedings and the
// document has References above entry 1, then REFERENCES above entry 6 and
// REFERENCES again above entry 23, both of them running heads that the
// furniture pass keeps because a heading is not furniture. Taking the last
// gave a bibliography of four entries starting at 23, which no citation in
// the paper could reach, and it is the whole of R02 on that paper: 49
// citations pointing at an index of four. Counting through the repeats puts
// all 25 against the first of the three, which wins.
//
// A PDF can also carry a second list that is nothing to do with the paper.
// The Borg PDF closes with a one page errata sheet with two references of
// its own, and taking the last heading there gave an index whose entries 1
// and 2 were the errata's and whose entries 3 to 84 were Borg's own 3 to 84,
// with Borg's first two lost and every citation in the paper off by two.
// Eighty four beats two, so Borg's own heading wins.
//
// Counting stops rather than running on; see listed. Looser is how this goes
// wrong, because a contents page lists References along with the other
// headings and a walk that went past anything unnumbered would hand that
// heading the whole of the paper.
func Bibliography(d *assemble.Document) []assemble.Paragraph {
	if d == nil {
		return nil
	}
	head, end := span(d)
	if head < 0 {
		return nil
	}
	return gathered(d.Paragraphs[:head+1], unheaded(d.Paragraphs[head+1:end]))
}

// unheaded is the section with the running heads taken out of it.
//
// They have to go, because everything downstream reads the section as
// entries. The parse cuts at the labels, so a REFERENCES between entry 5 and
// entry 6 is filed as the last two words of entry 5, and the splitter writes
// the section from these same paragraphs and would print it in the middle of
// the reference list.
func unheaded(ps []assemble.Paragraph) []assemble.Paragraph {
	out := make([]assemble.Paragraph, 0, len(ps))
	for _, p := range ps {
		if isBibliographyHeading(p.Text) {
			continue
		}
		out = append(out, p)
	}
	return out
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
// last paragraph of the section, or -1 if the paper has no heading. The
// heading it finds is the first of the run of them a list printed over
// several pages has, so both callers see the whole section.
func span(d *assemble.Document) (int, int) {
	// The heading with the longest list under it. See Bibliography for the
	// running heads this walks through and the second bibliography it walks
	// past, and listed for how far each walk goes. Ties go to the last
	// heading, which is where a bibliography belongs and is the answer for
	// every paper whose entries are not numbered in brackets.
	head, most, tail := -1, -1, -1
	for i := range d.Paragraphs {
		if !isBibliographyHeading(d.Paragraphs[i].Text) {
			continue
		}
		if n, t := listed(d.Paragraphs, i+1); n >= most {
			head, most, tail = i, n, t
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
	if next := resumes(d.Paragraphs, head); next < end {
		end = next
	}
	// Another list after this one means everything between the two belongs
	// to the second of them, so this one ends at its own last entry. The
	// errata sheet at the back of the Borg PDF prints a title, a date and
	// three short sections of its own before its two references, and without
	// this the last of Borg's eighty four entries came back with all of that
	// on the end of it. Nothing is cut where there is no second list, because
	// what trails the last entry there is the rest of the last entry: four
	// papers in the corpus set one that runs into a second paragraph, and
	// three more scatter their last few entries down a column.
	if tail > head && tail < end && repeated(d.Paragraphs[tail:]) {
		end = tail
	}
	if head+1 >= end {
		return -1, 0
	}
	return head, end
}

// resumes is where the paper takes up again after a bibliography heading,
// and is the length of the document for a list nothing follows.
//
// The splitter is asked rather than the name list above, because the
// splitter is what writes the section file and rule R08 is what checks the
// index and the file agree. A name list can only end the section at a
// heading somebody thought of, and what follows a bibliography is not
// always one of those. The Karp PDF is the Springer reprint, and page 2
// carries the two books the editors of the collection suggest reading under
// a heading that says References. Under it the reprinted article starts,
// and nothing on the name list stops the parse there: the index came back
// with 21 entries of which 19 were Karp's own numbered list of combinatorial
// problems, read as references because the paper numbers them, and R08
// reported all 19 of them.
//
// It also takes the appendix off the end of the last entry, which is worth
// as much. An appendix heading does end the section, but only once the
// entry it is printed under has already swallowed it: MapReduce's last
// reference came back with the whole of appendix A and its forty line C++
// listing on the end of it, ResNet's with the first paragraph of the
// detection baselines, and Saltzer's with the opening of the footnotes.
// Four papers in the corpus change and the other ninety seven do not.
//
// A bibliography heading is walked past, because a list long enough to run
// over a page has the word printed again at the head of the next one and
// the splitter reads that repeat as a heading like any other.
func resumes(ps []assemble.Paragraph, head int) int {
	texts := make([]string, len(ps))
	for i := range ps {
		texts[i] = ps[i].Text
	}
	_, hs := split.Headings(texts)
	for _, h := range hs {
		if h.Index > head && !isBibliographyHeading(texts[h.Index]) {
			return h.Index
		}
	}
	return len(ps)
}

// repeated says whether a bibliography heading stands in a run of
// paragraphs, which after the end of a list means a second list is coming.
//
// The run is the rest of the document and not the rest of the section,
// because what ends the section can stand between the two lists: the errata
// sheet heads its own acknowledgements before its own references, and that
// heading ends any bibliography wherever it is.
func repeated(ps []assemble.Paragraph) bool {
	for _, p := range ps {
		if isBibliographyHeading(p.Text) {
			return true
		}
	}
	return false
}

// listed is how many entries counting up follow a bibliography heading.
//
// It walks through a bibliography heading, because a list long enough to run
// over a page has the word printed again at the head of the next one and the
// count carries straight on through it. Once the count has started it walks
// through an unlabelled paragraph too, which is what the second half of an
// entry the reader broke in two looks like.
//
// It stops at a heading that only ever comes after a bibliography, at a
// label that does not carry the count on, and at the first unlabelled
// paragraph if the count has not started yet. That last one is what keeps a
// contents page out of this: it lists References along with every other
// heading, and without the stop the walk would run from the contents page
// through the whole of the paper and come back with the real bibliography's
// count against the wrong heading.
//
// The second return is the index one past the last entry, and it is only
// given when the walk stopped on a label that began the numbering again. A
// second numbered list says that whatever stands between the two lists
// belongs to the second of them: the errata sheet at the back of the Borg
// PDF prints a title, a date and three short sections of its own before its
// two references, and without this the last of Borg's own eighty four
// entries came back with all of that on the end of it. Otherwise the second
// return is -1, because a list that simply runs out has nothing to say about
// where it ends that the heading after it does not say better.
func listed(ps []assemble.Paragraph, at int) (int, int) {
	n, last, tail := 0, 0, at
	for i := at; i < len(ps); i++ {
		text := ps[i].Text
		if isBibliographyHeading(text) {
			continue
		}
		if endsBibliography(text) {
			break
		}
		k, ok := labelled(text)
		if !ok {
			if n == 0 {
				break
			}
			continue
		}
		if n > 0 && k != last+1 {
			break
		}
		n, last, tail = n+1, k, i+1
	}
	return n, tail
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
