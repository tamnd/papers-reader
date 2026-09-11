package refs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

// Style is how a bibliography labels its entries. It is detected once per
// paper and then required, for the same reason package split detects the
// numbering scheme once: a rule that is re-decided at every entry will be
// talked into the wrong answer by one odd line.
type Style string

// The four label styles. Every bibliography in the corpus is one of them.
const (
	// StyleBracket is "[1] C. E. Shannon, ...", the usual computer science
	// style, and the only one whose in-text citations can be rewritten into
	// links without guessing.
	StyleBracket Style = "bracket"
	// StyleNumber is "1. C. E. Shannon, ...".
	StyleNumber Style = "number"
	// StyleAuthorYear is "Shannon, C. E. (1948). ...", which is what the
	// older journals and most of the statistics on the list use.
	StyleAuthorYear Style = "author-year"
	// StyleHanging has no label at all and marks each entry with an indent.
	// The indent does not survive extraction, so a hanging bibliography is
	// read one entry per paragraph and that is the best that can be done.
	StyleHanging Style = "hanging"
)

// An Entry is one reference, as printed and as parsed.
//
// Raw is the reference as it appears in the paper and is the only field that
// is always right. Everything else is a parse, is used for resolution, and
// is allowed to be empty.
type Entry struct {
	Key     string   `yaml:"key"`
	Raw     string   `yaml:"raw"`
	Authors []string `yaml:"authors,omitempty"`
	Title   string   `yaml:"title,omitempty"`
	Venue   string   `yaml:"venue,omitempty"`
	Year    int      `yaml:"year,omitempty"`
	DOI     string   `yaml:"doi,omitempty"`
	ArXiv   string   `yaml:"arxiv,omitempty"`
	URL     string   `yaml:"url,omitempty"`
	Pages   string   `yaml:"pages,omitempty"`
	// ResolvesTo is the id of the paper in the corpus this reference names,
	// or the empty string for the great majority of references, which are to
	// papers the corpus does not have. It is written out even when empty so
	// that a reader of the file can see the question was asked.
	ResolvesTo string `yaml:"resolves_to"`
}

// Result is one parsed bibliography.
type Result struct {
	Style   Style
	Entries []Entry
	// Notes are what the parse could not do. Nothing here stops it.
	Notes []string
}

// Parse reads a bibliography.
//
// The paragraphs it is given are not the entries. A reference list is set
// with a hanging indent, so one entry is several paragraphs to the
// extractor, and a single column list comes back the other way round with
// three entries run into one paragraph. So the whole section is put back
// into one stream and cut at the labels, which is the only thing in a
// bibliography that reliably marks where an entry begins.
func Parse(paragraphs []assemble.Paragraph) *Result {
	texts := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		if t := strings.TrimSpace(p.Text); t != "" {
			texts = append(texts, t)
		}
	}
	if len(texts) == 0 {
		return &Result{Style: StyleHanging}
	}
	text := stream(texts)

	best := &Result{Style: StyleHanging}
	for _, s := range []Style{StyleBracket, StyleNumber, StyleAuthorYear} {
		r := read(s, text)
		if len(r.Entries) > len(best.Entries) {
			best = r
		}
	}
	// Three is the floor because two labels in a row is a coincidence and
	// three counting up is a bibliography. Below it the paper is read as
	// having no labels at all, which gets the text onto the page even if it
	// gets the entry boundaries wrong.
	if len(best.Entries) < 3 {
		best = hanging(texts)
	}
	for i := range best.Entries {
		fields(best.Style, &best.Entries[i])
	}
	return best
}

// stream puts the section back into one text, with the paragraph breaks the
// extractor made kept as newlines.
//
// The newlines are kept because one style needs them: an author-year entry
// begins with a surname, which is not a rare thing to find in the middle of
// a reference, so those labels are only believed at the start of a line.
// The numbered styles do not need them and are read straight through.
func stream(texts []string) string {
	var parts []string
	for _, t := range texts {
		if n := len(parts); n > 0 && assemble.Hyphenated(parts[n-1]) {
			parts[n-1] = assemble.JoinText(parts[n-1], t)
			continue
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "\n")
}

var (
	// A bracket key is usually a number and is sometimes the initials and
	// year the older ACM papers use, as in [Sha48].
	bracketLabel = regexp.MustCompile(`(?:^|[ \n])\[([\p{L}\d+.\-]{1,16})\][ \n]\s*`)
	numberLabel  = regexp.MustCompile(`(?:^|[ \n])(\d{1,3})[.)][ \n]\s*`)
	yearLabel    = regexp.MustCompile(`(?m)^(\p{Lu}[^()\n]{0,200}?)\(((?:1[6-9]|20)\d{2}[a-z]?)\)[.,]?\s*`)
)

func pattern(s Style) *regexp.Regexp {
	switch s {
	case StyleBracket:
		return bracketLabel
	case StyleNumber:
		return numberLabel
	case StyleAuthorYear:
		return yearLabel
	}
	return nil
}

// A mark is one place a label was found: where the entry starts, where the
// text after the label starts, and what the entry is keyed by.
type mark struct {
	at, after int
	key       string
	number    int
}

// marks finds every place in the stream that could be the start of an entry
// in one style.
func marks(s Style, text string) []mark {
	re := pattern(s)
	if re == nil {
		return nil
	}
	var out []mark
	for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
		m := mark{at: loc[0], after: loc[1]}
		if m.at > 0 {
			// The separator the pattern matched belongs to the entry before.
			m.at++
		}
		switch s {
		case StyleAuthorYear:
			authors, year := text[loc[2]:loc[3]], text[loc[4]:loc[5]]
			if !surnameFirst(authors) {
				continue
			}
			m.key = authorYearKey(authors, year)
			// The authors are part of the entry, not a label in front of it.
			m.after = m.at
		default:
			m.key = text[loc[2]:loc[3]]
			m.number = printed(m.key)
		}
		out = append(out, m)
	}
	return out
}

// read cuts the stream into entries in one style.
//
// A label is believed when it carries on the count, which is what makes the
// whole thing work on a page that has numbers all over it. "In Proc. 1980
// Symposium, pages 122-133" has three of them and none of them continues
// the sequence.
func read(s Style, text string) *Result {
	r := &Result{Style: s}
	var (
		found  = marks(s, text)
		taken  []mark
		last   int
		gaps   []string
		spaced bool
	)
	for i, m := range found {
		switch {
		case m.number == 0:
			// A key that is not a number, so there is no count to carry on.
			// Only the bracket and author-year styles produce these and both
			// are specific enough to stand on their own.
			if s == StyleNumber {
				continue
			}
		case m.number <= last:
			continue
		case last == 0 && m.number > 2:
			// A bibliography starts at one, or at two if the extractor lost
			// the first entry into a running head.
			continue
		case m.number > last+1:
			// A gap is either a lost entry or a number that is not a label.
			// The next label decides: a real gap is followed by the entry
			// after it, and a stray number is followed by the entry the
			// count was already expecting.
			if i+1 >= len(found) || found[i+1].number != m.number+1 {
				continue
			}
			gaps = append(gaps, missing(last, m.number))
			spaced = true
		}
		if m.number != 0 {
			last = m.number
		}
		taken = append(taken, m)
	}
	for i, m := range taken {
		end := len(text)
		if i+1 < len(taken) {
			end = taken[i+1].at
		}
		raw := flatten(text[m.after:end])
		if raw == "" {
			continue
		}
		r.Entries = append(r.Entries, Entry{Key: m.key, Raw: raw})
	}
	if spaced {
		r.Notes = append(r.Notes, "the bibliography skips "+strings.Join(gaps, ", "))
	}
	return r
}

// hanging reads a bibliography with no labels, one entry per paragraph.
func hanging(texts []string) *Result {
	r := &Result{Style: StyleHanging}
	for i, text := range texts {
		r.Entries = append(r.Entries, Entry{Key: strconv.Itoa(i + 1), Raw: flatten(text)})
	}
	if len(r.Entries) > 0 {
		r.Notes = append(r.Notes, "the bibliography has no labels that count up, so each paragraph is read as one entry")
	}
	return r
}

// surnameFirst says whether a string looks like an author list written
// surname first, which is what tells an author-year entry apart from a
// sentence that happens to have a year in brackets in it.
func surnameFirst(s string) bool {
	s = strings.TrimSpace(s)
	i := strings.Index(s, ",")
	if i <= 0 {
		return false
	}
	for _, r := range s[:i] {
		if !unicode.IsLetter(r) && r != '\'' && r != '’' && r != '-' && r != ' ' {
			return false
		}
	}
	return true
}

// authorYearKey is what an author-year paper cites the entry by: the first
// surname and the year, as in "Shannon 1948".
func authorYearKey(authors, year string) string {
	name := strings.TrimSpace(authors)
	if i := strings.IndexAny(name, ",&"); i > 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(strings.Trim(name, ".,"))
	if name == "" {
		return year
	}
	return name + " " + year
}

// printed is the number a numeric key stands for, or zero for a key that is
// not a number.
func printed(key string) int {
	n, err := strconv.Atoi(key)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// missing names a gap in the numbering, for a note.
func missing(last, next int) string {
	if next == last+2 {
		return strconv.Itoa(last + 1)
	}
	return fmt.Sprintf("%d to %d", last+1, next-1)
}

// Keys is every key in order.
func (r *Result) Keys() []string {
	out := make([]string, len(r.Entries))
	for i, e := range r.Entries {
		out[i] = e.Key
	}
	return out
}
