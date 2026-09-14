package split

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// A Scheme is how a paper numbers its sections. It is detected once per paper
// and then required, so that a line which happens to start with a digit in the
// middle of a paragraph is not read as the start of section 5.
type Scheme string

// The numbering schemes the hundred use. A paper that numbers nothing has
// SchemeNone and its headings are found by name and by typography.
const (
	SchemeNone   Scheme = ""
	SchemeArabic Scheme = "arabic" // 3 Model Architecture, 3.2 Multi-Head Attention
	SchemeRoman  Scheme = "roman"  // IV. RESULTS
	SchemeSign   Scheme = "sign"   // §4 The Consistency Condition
	SchemeLetter Scheme = "letter" // A Details of Common Crawl Filtering
)

// How says what found a heading, which is the confidence in it. A heading
// found by its typography alone is reported, because on a page of extracted
// text an emphasised line and a short paragraph look the same.
type How string

// The five detectors, in the order of how much they can be trusted.
const (
	Marked      How = "marked"      // the extractor wrote it as a heading
	Numbered    How = "numbered"    // it continues the paper's own numbering
	Lettered    How = "lettered"    // it continues the appendix lettering
	Named       How = "named"       // it is one of the forty section names
	Typographic How = "typographic" // it is set like a heading and nothing more
)

// A Heading is a paragraph the splitter read as a section heading.
type Heading struct {
	// Index is the paragraph it was found at.
	Index int
	// Number is the section number as the paper prints it, and is empty for an
	// unnumbered heading. It goes in the front matter as written, so a paper
	// that numbers its sections IV keeps IV.
	Number string
	// Level is 1 for a top level section and 2 for 3.2. Only level 1 cuts a
	// file; the rest are kept so the body can be rendered with its own
	// subheadings and so the audit can see the structure.
	Level int
	Title string
	Kind  string
	How   How
}

// maxHeading is the longest a heading is allowed to be, in runes. A heading
// is a name and a name is short. The longest in the hundred is Shannon's
// "Representation of the Encoding and Decoding Operations", at 54.
const maxHeading = 90

// minChain is how many headings a numbering scheme has to explain before it is
// believed. Two is a coincidence a paper with numbered equations produces on
// its own; three in sequence is a paper that numbers its sections.
const minChain = 3

// gap is the space between a section number and the name of the section.
//
// It is not always a space. A paper sets that gap with a quad, and a model
// transcribing the page writes the quad it sees: the BERT paper came back
// with an em space after every number, and "4\u2003Experiments" is a
// heading nothing here could read a number off. Section 4 of that paper
// disappeared into section 3, and so did its three subsections, and nothing
// said so because a section folded into the one above it is still in the
// document. All of the Unicode spaces are here rather than the two that
// turned up, because the next paper will use the third.
const gap = `[ \t\x{00A0}\x{1680}\x{2000}-\x{200A}\x{202F}\x{205F}\x{3000}]`

var (
	arabic = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?` + gap + `+(\S.*)$`)
	// A roman numeral without the full stop is the word I, the roman numeral
	// with it is a section, and the papers that use roman numerals all print
	// the stop.
	roman = regexp.MustCompile(`^([IVXL]+)\.` + gap + `+(\S.*)$`)
	sign  = regexp.MustCompile(`^§` + gap + `*(\d+(?:\.\d+)*)\.?` + gap + `*(\S.*)$`)
	// atx is a heading the extractor already found. The layout path writes
	// one for every block its model labelled a heading, and the native path
	// writes none at all, so a # at the start of a paragraph is a statement
	// and not a guess.
	atx = regexp.MustCompile(`^(#{1,6})` + gap + `+(\S.*)$`)
	// bold is a whole paragraph inside one pair of emphasis markers, which is
	// the other way a vision model writes a heading.
	bold = regexp.MustCompile(`^(\*\*|__|\*|_)([^*_].*[^*_])(\*\*|__|\*|_)$`)
	// A lettered appendix heading. One capital, the gap, and the name. The
	// stop is optional because a paper that prints "A. Proofs" and one that
	// prints "A Proofs" mean the same thing.
	letter = regexp.MustCompile(`^([A-Z])\.?` + gap + `+(\S.*)$`)
)

// boldParts takes the emphasis off a paragraph that is nothing but emphasis.
//
// A vision model asked for Markdown picks its own notation for a heading and
// does not always pick the same one twice. The ResNet paper came back with 2,
// 3, 3.1, 3.2, 3.3 and 4.2 in bold and 3.4, 4, 4.1 and 4.3 as plain lines, so
// the numbering had a hole at every bold heading, no scheme explained three
// sections in a row, and a twelve page paper split into one file of thirty
// eight thousand characters. Four papers of the six read that week did it.
//
// This is weaker evidence than a hash and the caller treats it that way. A
// hash is a statement by something that read the page, and markedHeading takes
// it on its own. Emphasis is also how a paper sets a theorem label, a figure
// caption's lead in and the first words of a run in paragraph, so what comes
// back from here is offered to the same numbering and name tests as a plain
// line and is a heading only if one of them agrees.
//
// The markers have to match and there must be none inside, so a paragraph with
// two emphasised phrases in it is left alone rather than read as one span from
// the first marker to the last.
func boldParts(text string) (rest string, ok bool) {
	if strings.ContainsRune(text, '\n') {
		return text, false
	}
	m := bold.FindStringSubmatch(text)
	if m == nil || m[1] != m[3] {
		return text, false
	}
	if strings.ContainsAny(m[2], "*_") {
		return text, false
	}
	return strings.TrimSpace(m[2]), true
}

// unmark takes off whichever heading notation the extractor used, and says
// whether the paragraph carried one at all.
func unmark(raw string) (text string, marked bool) {
	if _, rest, ok := atxParts(raw); ok {
		return rest, true
	}
	return boldParts(raw)
}

// atxParts takes the marker off a heading the extractor wrote.
//
// The paragraph has to be a single line, because a heading is one line and a
// paragraph of prose that happens to open with a hash is not. That is not a
// hypothetical: a paper about C writes a preprocessor directive in running
// text, and one about issue trackers writes "# 12" in a sentence.
func atxParts(text string) (level int, rest string, ok bool) {
	if strings.ContainsRune(text, '\n') {
		return 0, text, false
	}
	m := atx.FindStringSubmatch(text)
	if m == nil {
		return 0, text, false
	}
	return len(m[1]), strings.TrimSpace(m[2]), true
}

// parseNumber pulls the number and the title off a heading under one scheme.
// The second return is false when the paragraph is not of that shape.
func parseNumber(s Scheme, text string) (num string, parts []int, title string, ok bool) {
	var m []string
	switch s {
	case SchemeArabic:
		m = arabic.FindStringSubmatch(text)
	case SchemeRoman:
		m = roman.FindStringSubmatch(text)
	case SchemeSign:
		m = sign.FindStringSubmatch(text)
	case SchemeLetter:
		m = letter.FindStringSubmatch(text)
	}
	if m == nil {
		return "", nil, "", false
	}
	if s == SchemeLetter {
		return m[1], []int{int(m[1][0]-'A') + 1}, m[2], true
	}
	if s == SchemeRoman {
		n := romanValue(m[1])
		if n == 0 {
			return "", nil, "", false
		}
		return m[1], []int{n}, m[2], true
	}
	for _, f := range strings.Split(m[1], ".") {
		n, err := strconv.Atoi(f)
		if err != nil {
			return "", nil, "", false
		}
		parts = append(parts, n)
	}
	return m[1], parts, m[2], true
}

var romanDigits = map[rune]int{'I': 1, 'V': 5, 'X': 10, 'L': 50}

// romanValue reads a roman numeral, and returns zero for anything that is not
// one written the usual way. It is deliberately strict: it is asked about
// every line of every paper that starts with a capital and a full stop, and
// "I." at the head of a quoted sentence is not section one.
func romanValue(s string) int {
	total, prev := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := romanDigits[rune(s[i])]
		if !ok {
			return 0
		}
		if v < prev {
			total -= v
			continue
		}
		total += v
		prev = v
	}
	if total == 0 || total > 40 {
		return 0
	}
	return total
}

// candidate says whether a paragraph is short enough and plain enough to be a
// heading at all, before anything looks at what it says.
//
// A paragraph that ends in a full stop is a sentence. A paragraph that runs
// past a line is a paragraph. Neither is a heading, and both are what a
// numbered list inside the prose looks like.
func candidate(text string) bool {
	if text == "" || len([]rune(text)) > maxHeading {
		return false
	}
	switch text[len(text)-1] {
	case '.', ',', ';', ':':
		// A numbered heading may print its own trailing stop, and that is the
		// one exception: "3. Introduction." is rare but real. It is let
		// through and the stop comes off the title.
		return arabic.MatchString(text) || roman.MatchString(text)
	}
	return true
}

// DetectScheme works out how a paper numbers its sections, by trying each
// scheme over the whole document and keeping the one that explains the longest
// run of headings in sequence.
//
// In sequence is the whole of the idea. Any scheme matches a handful of lines
// in any paper; only the right one matches 1, 2, 3, 4 in the order they
// appear, with the subsections of section 3 between 3 and 4.
func DetectScheme(paragraphs []string) Scheme {
	best, length := SchemeNone, 0
	for _, s := range []Scheme{SchemeArabic, SchemeRoman, SchemeSign} {
		if n := len(chain(s, paragraphs)); n > length {
			best, length = s, n
		}
	}
	if length < minChain {
		return SchemeNone
	}
	return best
}

// chain is the run of headings one scheme explains, in document order, each
// one continuing the numbering of the one before it.
//
// A paper numbers its sections from one and this tries that first, because it
// is what almost every paper does and a chain that starts anywhere is a chain
// that can start in the wrong place. Only when one explains fewer than minChain
// headings does this go looking for a later start.
//
// Which happens when the extraction loses the first heading. Page 1 of the
// ResNet paper came back without "1. Introduction" in it, along with the
// stretch of the introduction under it, and the rest of the paper numbers 2,
// 3, 3.1, 3.2, 3.3, 3.4, 4, 4.1, 4.2, 4.3 in order. Insisting on the one made
// that no scheme at all, which is a whole paper filed as a single section over
// one missing line. Numbering from two is still numbering.
//
// Every start the document offers is tried and the longest run wins, rather
// than seeding from the first number seen. A cross-reference or a table row
// that happens to read as section 5 is one heading, and taking it as the start
// would refuse the four real sections in front of it.
func chain(s Scheme, paragraphs []string) []Heading {
	if out := chainFrom(s, paragraphs, 1); len(out) >= minChain {
		return out
	}
	var best []Heading
	for _, start := range starts(s, paragraphs) {
		if out := chainFrom(s, paragraphs, start); len(out) > len(best) {
			best = out
		}
	}
	return best
}

// starts is every top level number the document offers as a first section, in
// ascending order. One is not among them because the caller has tried it.
func starts(s Scheme, paragraphs []string) []int {
	seen := map[int]bool{}
	for _, raw := range paragraphs {
		text, _ := unmark(raw)
		if !candidate(text) {
			continue
		}
		if _, parts, _, ok := parseNumber(s, text); ok && len(parts) == 1 && parts[0] > 1 {
			seen[parts[0]] = true
		}
	}
	out := slices.Sorted(maps.Keys(seen))
	return out
}

// chainFrom is chain with the number the first top level section is expected
// to carry.
func chainFrom(s Scheme, paragraphs []string, start int) []Heading {
	var out []Heading
	// last is the last number accepted under each parent: the empty key for
	// top level sections, "3" for the subsections of section 3.
	last := map[string]int{"": start - 1}
	for i, raw := range paragraphs {
		// A heading the extractor marked keeps its printed number in its
		// title, so the marker comes off before the number is read and the
		// numbering of a paper reads the same down either path.
		text, marked := unmark(raw)
		if !candidate(text) {
			continue
		}
		num, parts, title, ok := parseNumber(s, text)
		if !ok {
			continue
		}
		parent := ""
		if len(parts) > 1 {
			parent = num[:strings.LastIndex(num, ".")]
			// A subsection of a section that has not been reached yet is a
			// cross-reference or a table row, not a heading.
			if last[""] != parts[0] {
				continue
			}
		}
		if parts[len(parts)-1] != last[parent]+1 {
			continue
		}
		last[parent] = parts[len(parts)-1]
		if len(parts) == 1 {
			// A new top level section ends every subsection count under the
			// old one.
			for k := range last {
				if k != "" {
					delete(last, k)
				}
			}
		}
		how := Numbered
		if marked {
			how = Marked
		}
		out = append(out, Heading{
			Index:  i,
			Number: num,
			Level:  len(parts),
			Title:  strings.TrimRight(title, "."),
			How:    how,
		})
	}
	return out
}

// Headings finds every section heading in a document.
//
// The three detectors run in the order of the confidence in them. A paper that
// numbers its sections gets the numbered headings plus the named ones it left
// unnumbered, which is almost always the abstract, the acknowledgements and
// the references. A paper that numbers nothing falls back to typography, and
// says so, because on extracted text an emphasised line and a short paragraph
// are the same thing.
func Headings(paragraphs []string) (Scheme, []Heading) {
	s := DetectScheme(paragraphs)
	top := topLevel(paragraphs)
	found := map[int]Heading{}
	body := chain(s, paragraphs)
	for _, h := range body {
		found[h.Index] = h
	}
	for _, h := range appendix(paragraphs, body) {
		if _, taken := found[h.Index]; !taken {
			found[h.Index] = h
		}
	}
	for i, raw := range paragraphs {
		if _, taken := found[i]; taken {
			continue
		}
		// A heading the extractor marked is a heading whether or not it
		// continues the numbering, because something that read the page
		// said so. An unnumbered heading in the middle of a numbered paper
		// is normally the acknowledgements, and the numbered pass above
		// skips it by design.
		if h, ok := markedHeading(s, top, i, raw); ok {
			found[i] = h
			continue
		}
		// Emphasis comes off here rather than in markedHeading, because it is
		// not on its own a reason to cut a file: what follows has to agree.
		text, _ := boldParts(raw)
		if !candidate(text) {
			continue
		}
		if title, ok := named(text); ok {
			found[i] = Heading{Index: i, Level: 1, Title: title, How: Named}
			continue
		}
		if s == SchemeNone && typographic(paragraphs, i) {
			found[i] = Heading{Index: i, Level: 1, Title: titleCase(text), How: Typographic}
		}
	}
	out := make([]Heading, 0, len(found))
	for i := range paragraphs {
		if h, ok := found[i]; ok {
			out = append(out, h)
		}
	}
	assignKinds(out)
	return s, out
}

// appendix is the run of lettered appendix headings a paper prints after its
// body: "A Details of Common Crawl Filtering", then B, then C.
//
// Lettering is not one of the schemes DetectScheme tries, and it must not
// become one. A line that starts with a capital and a space is most of the
// English language, and a paper that opens a section with "A simple model of
// the network" would number that section A. What makes it safe here is where
// it is allowed to look and how much it has to explain: only the paragraphs
// after the last heading of the body, and only a run of at least minChain
// letters in order, A then B then C. Nothing in the corpus produces that by
// accident, and the GPT-3 paper produces it eight times.
//
// It is worth the trouble because the paper it was written for is the largest
// in the corpus. Its appendices A to H are sixty four thousand characters of
// text, the letters are the only thing marking where each one starts, and
// with nothing to find them the whole of it was filed under the heading above
// it, which happened to be the acknowledgements.
//
// The headings come back as Lettered rather than Numbered so that assignKinds
// can call them appendices. They are appendices by construction: they are
// after the body and they are lettered.
func appendix(paragraphs []string, body []Heading) []Heading {
	from := 0
	if len(body) > 0 {
		from = body[len(body)-1].Index + 1
	}
	if from >= len(paragraphs) {
		return nil
	}
	out := chainFrom(SchemeLetter, paragraphs[from:], 1)
	if len(out) < minChain {
		return nil
	}
	for i := range out {
		out[i].Index += from
		out[i].How = Lettered
	}
	return out
}

// topLevel is the depth of hashes this document writes a top level section
// at, which is not always one.
//
// A vision model transcribing a page writes the title of the paper as a
// level one heading, because that is what it is on the page, and then every
// section of the paper a level under it. So "## Acknowledgments" is a
// section of the paper and not a subsection of anything, and reading its two
// hashes as level two puts the acknowledgements and the references inside
// the conclusion. The GAN paper lost both files that way.
//
// The depth is read off the document rather than assumed, because the other
// half of the corpus does write its sections at one hash. What gives it away
// is repetition: a paper has one title and eight sections, so the shallowest
// depth two headings share is the depth of a section, and a depth used once
// is a title. A document with no depth used twice has nothing to go on and
// gets its shallowest, which for a paper of one section is that section.
func topLevel(paragraphs []string) int {
	count := map[int]int{}
	for _, raw := range paragraphs {
		if level, _, ok := atxParts(raw); ok {
			count[level]++
		}
	}
	top, shallowest := 0, 0
	for level, n := range count {
		if shallowest == 0 || level < shallowest {
			shallowest = level
		}
		if n >= 2 && (top == 0 || level < top) {
			top = level
		}
	}
	if top == 0 {
		top = shallowest
	}
	if top == 0 {
		top = 1
	}
	return top
}

// markedHeading reads a heading the extractor wrote.
//
// The level is the one the extractor gave unless the paper's own numbering
// says otherwise, and then the numbering wins: a model that labelled 3.2 a
// top level heading is wrong about the structure of the paper in a way the
// number settles. The title is canonicalised the same way an unnumbered
// heading found by name is, so that Acknowledgements files under
// Acknowledgments however the paper spelled it and however it was found.
//
// An unnumbered heading is level one if the extractor wrote it no deeper
// than top, which is where this document puts a section. A heading deeper
// than that keeps its depth and stays inside the section it is in, which is
// what a "### Notation" under section 2 is.
func markedHeading(s Scheme, top, i int, raw string) (Heading, bool) {
	level, text, ok := atxParts(raw)
	if !ok {
		return Heading{}, false
	}
	h := Heading{Index: i, Level: level, Title: text, How: Marked}
	if num, parts, title, numbered := parseNumber(s, text); numbered {
		h.Number, h.Level, h.Title = num, len(parts), strings.TrimRight(title, ".")
		return h, true
	}
	if level <= top {
		h.Level = 1
	}
	if title, named := named(text); named {
		h.Title = title
		return h, true
	}
	h.Title = titleCase(text)
	return h, true
}

// named matches a paragraph against the section names English papers use. The
// list is about forty long and it is English only, because the hundred are.
func named(text string) (string, bool) {
	key := strings.ToLower(strings.Trim(text, " .:"))
	if title, ok := sectionNames[key]; ok {
		return title, true
	}
	// An appendix names itself and then says which one it is: "Appendix A",
	// "Appendix B: The Proof". The letter is part of the title because it is
	// what the paper refers to it by.
	if strings.HasPrefix(key, "appendix ") {
		return titleCase(strings.Trim(text, " .:")), true
	}
	return "", false
}

// sectionNames maps a heading as a paper might print it to the name this
// corpus files it under. The value is not always the key capitalised: a paper
// that writes ACKNOWLEDGMENT and one that writes Acknowledgements are filing
// the same section, and a reader moving between two papers should not have to
// notice which.
var sectionNames = func() map[string]string {
	groups := map[string][]string{
		"Abstract":            {"abstract"},
		"Introduction":        {"introduction", "motivation", "the problem"},
		"Background":          {"background", "preliminaries", "prior work", "previous work", "related work", "notation", "notation and definitions", "definitions", "terminology"},
		"Overview":            {"overview", "the model", "our approach", "approach"},
		"Method":              {"method", "methods", "methodology", "the algorithm", "algorithm", "algorithms"},
		"Design":              {"design", "system design", "architecture", "model architecture"},
		"Implementation":      {"implementation", "the implementation"},
		"Analysis":            {"analysis", "theory", "theoretical analysis", "correctness", "complexity"},
		"Experiments":         {"experiments", "experimental setup", "experimental results", "evaluation", "results", "results and discussion", "performance", "measurements"},
		"Discussion":          {"discussion", "limitations", "future work", "open problems", "further work"},
		"Conclusion":          {"conclusion", "conclusions", "concluding remarks", "conclusion and future work"},
		"Acknowledgments":     {"acknowledgment", "acknowledgments", "acknowledgement", "acknowledgements"},
		"References":          {"references", "bibliography", "literature cited", "references cited", "works cited"},
		"Appendix":            {"appendix", "appendices"},
		"Notes":               {"notes", "footnotes", "endnotes"},
		"Summary":             {"summary"},
		"Author Information":  {"about the author", "about the authors", "author information", "biographies"},
		"Availability":        {"availability", "artifact availability", "data availability", "code availability"},
		"Acronyms":            {"glossary", "acronyms", "abbreviations"},
		"Statement of Ethics": {"ethics statement", "broader impact", "impact statement"},
	}
	m := map[string]string{}
	for title, keys := range groups {
		for _, k := range keys {
			m[k] = title
		}
	}
	return m
}()

// typographic reads a heading off how the line is set: short, in capitals, no
// terminal punctuation, and followed by prose rather than by another short
// line. It is the last resort and it only runs on a paper that numbers
// nothing, because on a numbered paper every false positive it finds is a
// section boundary the numbering already said was not there.
func typographic(paragraphs []string, i int) bool {
	text := paragraphs[i]
	if len([]rune(text)) > 60 || !upper(text) {
		return false
	}
	if i+1 >= len(paragraphs) {
		return false
	}
	return len(paragraphs[i+1]) > 200
}

// upper says whether a line is set in capitals. A line with no letters in it
// at all is not, which keeps a row of numbers from being a heading.
func upper(s string) bool {
	letters := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		if unicode.IsLower(r) {
			return false
		}
		letters++
	}
	return letters >= 3
}

// titleCase turns a heading a paper set in capitals into the corpus's own
// capitalisation, because a section title is read in a table of contents next
// to forty others and SHOUTING in a list of forty is hard to read.
//
// Words of three letters or fewer inside the title stay lower case, which is
// the usual rule, and a word that is not all capitals is left exactly as it
// came, because that one was not shouting and its capitals are meant.
func titleCase(s string) string {
	if !upper(s) {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && len(w) <= 3 && small[w] {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

var small = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "but": true,
	"by": true, "for": true, "in": true, "is": true, "nor": true, "of": true,
	"on": true, "or": true, "the": true, "to": true, "up": true, "via": true,
}

// The kinds a content file can be, from 02-corpus.md §3.
const (
	KindFront      = "front"
	KindSection    = "section"
	KindReferences = "references"
	KindAppendix   = "appendix"
)

// assignKinds says what each heading starts. Everything after the first
// appendix heading is an appendix, because a paper that has appendix A has
// appendix B after it and neither is a section of the argument. References
// are the exception: a bibliography printed after the appendices is still the
// bibliography.
//
// A heading after the references is an appendix too, whatever it calls
// itself. Half the papers in the corpus that have one do not print the word:
// the MapReduce paper heads its last page "A Word Frequency", where the A is
// the appendix letter and not the article, and the page before it is the
// bibliography. Nothing else is printed after a bibliography. The letter
// becomes the heading's number, which is what the paper refers to it by, and
// it is only taken off a heading in this position: before the references
// "A Note on Two Problems" is a title that starts with an article, and after
// them it is not a title anybody prints.
func assignKinds(hs []Heading) {
	appendices, cited := false, false
	for i := range hs {
		title := strings.ToLower(hs[i].Title)
		switch {
		case sectionNames[title] == "References":
			hs[i].Kind = KindReferences
			cited = true
		case hs[i].How == Lettered || strings.HasPrefix(title, "appendix") || appendices || cited:
			hs[i].Kind = KindAppendix
			appendices = true
			if cited {
				lettered(&hs[i])
			}
		default:
			hs[i].Kind = KindSection
		}
	}
}

// lettered pulls the appendix letter off the front of a heading that has one
// and has no number of its own. "A Word Frequency" is appendix A, named Word
// Frequency.
func lettered(h *Heading) {
	if h.Number != "" {
		return
	}
	fields := strings.SplitN(strings.TrimSpace(h.Title), " ", 2)
	if len(fields) != 2 || len([]rune(fields[0])) != 1 {
		return
	}
	r := []rune(fields[0])[0]
	if r < 'A' || r > 'Z' {
		return
	}
	h.Number, h.Title = fields[0], strings.TrimSpace(fields[1])
}
