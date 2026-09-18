package split

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

// Folio matches a paragraph that is nothing but a page number, with or
// without the rules and dashes a paper sets around one.
//
// The extractor learns page furniture from the whole paper: a line in the
// same place on most pages is a running head and comes off there. A paper
// that prints its folio on six pages out of forty defeats that, because six
// is not most, and the number arrives here as a paragraph of its own. It is
// never anything else. A numbered list writes "1." with the stop, a display
// equation is inside its dollars, and body prose is not one number.
//
// Shared with the audit, which checks the same shape in rule T10. Two copies
// of it would mean the splitter writing a file the rule then refuses with
// nobody able to say which of the two was wrong.
var Folio = regexp.MustCompile(`(?i)^\s*[-–—|]*\s*(?:page\s+)?\d{1,4}\s*[-–—|]*\s*$`)

// unfolio takes the page numbers out of a paragraph, and returns the empty
// string for a paragraph that was nothing else.
//
// A folio arrives on its own most of the time and the whole paragraph goes.
// Paxos is the other case: the extractor read "The Part-Time Parliament ·"
// and "11" as one paragraph of two lines, because the running head and the
// number print on the same band of the page and nothing between them is wide
// enough to read as a break. Rule T10 refuses a page number on a line of its
// own wherever it sits, so the splitter has to read lines too.
//
// What is left of such a paragraph is the running head, which is furniture as
// much as the number is, so a short remainder goes with it. Short is ten
// words: a running head is a fragment of the title or the name of the
// journal, and "Communications of the ACM" is the longest of the four in the
// corpus at four. A longer paragraph keeps everything but the number,
// because a paragraph of real prose that swept a folio up has prose in it
// worth keeping.
func unfolio(text string) string {
	if table(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	cut := false
	for _, l := range lines {
		if Folio.MatchString(l) {
			cut = true
			continue
		}
		kept = append(kept, l)
	}
	if !cut {
		return text
	}
	rest := strings.TrimSpace(strings.Join(kept, "\n"))
	if len(strings.Fields(rest)) < 10 {
		return ""
	}
	return rest
}

// table says whether a paragraph is a Markdown table, which is the one place
// a line of one number is not a page number.
//
// A table has the delimiter row under its header and nothing else in a paper
// does: pipes, dashes, colons and space, with at least one of the first two.
// A row that holds a single number matches a folio exactly, and a table with
// a row taken out of it no longer says what the paper said.
func table(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		pipe, dash, only := false, false, true
		for _, r := range l {
			switch r {
			case '|':
				pipe = true
			case '-':
				dash = true
			case ':', ' ', '\t':
			default:
				only = false
			}
			if !only {
				break
			}
		}
		if only && pipe && dash {
			return true
		}
	}
	return false
}

// A Section is one top level section of a paper: the part of the document
// between one level one heading and the next, with its subheadings inside it.
//
// The unit is the top level section because that is what a reader navigates,
// what a translator is given in one ask, and what the audit compares between
// languages. Cutting finer would make a table of contents of forty entries
// for a paper with eight sections; cutting coarser would make the translation
// of one paper a single forty thousand character ask, which is the mistake
// Bourbaki made once and measured its way out of.
type Section struct {
	// Ordinal is what numbers the file. The front matter is zero, a section
	// the paper numbered keeps that number, and everything else counts up
	// from the section before it. So section 3 of the Transformer paper is
	// 03_model_architecture.md and its references, which the paper does not
	// number, come after section 7 as 08_references.md.
	Ordinal int
	Number  string
	Title   string
	Kind    string
	How     How
	// Body is the section as Markdown, its own subheadings included.
	Body string
	// First and Last are the PDF pages it came off, for pdf_pages in the front
	// matter and for a reader who wants to check it against the paper.
	First, Last int
}

// A Result is what a document split into.
type Result struct {
	Scheme   Scheme
	Sections []Section
	// Notes are what a person should look at: a heading found by typography
	// alone, a paper with no headings at all. They go in the run report rather
	// than stopping the split, because a paper read as one section is still a
	// paper in the corpus and a paper refused is not.
	Notes []string
}

// Split cuts a document into its top level sections.
//
// Everything before the first heading is the front matter of the paper: the
// title block, the authors, the abstract if it is not headed. It comes back as
// the first section, with the front kind, and the writer files it as
// 00_front.md.
func Split(d *assemble.Document) *Result { return Titled(d, "") }

// Titled is Split for a paper whose title is known, which lets it tell the
// title block from the first section.
//
// A vision model writes the title of the paper as a level one heading, which
// is what it is on the page and is not a section of the argument. Split on its
// own cannot tell: to it the title is a heading like any other, so it cuts
// there and 00_front.md keeps only whatever the page printed above the title.
// On the GAN paper that was the line arXiv prints down the side, so the front
// matter came out as one sentence with no authors and no abstract in it, and
// rule T06 said so.
//
// The title is compared and not guessed at. A heading that reads as the paper
// is the paper's title, and anything else is a section however it is set. Only
// the first heading is offered the comparison, because a paper that prints its
// title again is printing a running head.
func Titled(d *assemble.Document, title string) *Result {
	// Unrun first, because everything below counts paragraphs and a heading
	// that is still inside one is a heading nothing here can find.
	paragraphs := Unrun(d.Paragraphs)
	// After Unrun, because a bibliography that came back with its heading
	// stuck to the first entry is a bibliography that has its heading, and
	// before the headings are read, because that is the point of it.
	paragraphs, supplied := headReferences(paragraphs)
	texts := make([]string, len(paragraphs))
	for i, p := range paragraphs {
		texts[i] = p.Text
	}
	scheme, headings := Headings(texts)
	r := &Result{Scheme: scheme}

	// Cuts are the level one headings. A subheading stays inside its section
	// and is rendered there.
	var cuts []Heading
	sub := map[int]Heading{}
	for _, h := range headings {
		if h.Level == 1 {
			cuts = append(cuts, h)
			continue
		}
		sub[h.Index] = h
	}
	// bare is the headings that are headings on the page and are not
	// headings here. They keep their words and lose their marker: the title
	// is in the front matter as title, the paper's name is not a level of
	// anything, and a level one heading over 00_front.md puts the whole file
	// a level above the sections it introduces. Rule T05 read the pair the
	// ResNet paper published as a level three under a level one.
	bare := map[int]bool{}
	// The title goes before the abstract on the page, so it comes off the
	// front of the cuts before the abstract does.
	if len(cuts) > 0 && sameText(cuts[0].Title, title) {
		bare[cuts[0].Index] = true
		cuts = cuts[1:]
	}
	// The abstract is not a section. It is what 00_front.md is for, along
	// with the title and the authors, because the abstract is the one part of
	// a paper that is quoted on its own and a reader who wants it should not
	// have to know whether this paper happened to head it.
	if len(cuts) > 0 && cuts[0].Title == "Abstract" {
		bare[cuts[0].Index] = true
		cuts = cuts[1:]
	}

	front := len(cuts) == 0 || cuts[0].Index > 0
	starts := make([]int, 0, len(cuts)+1)
	if front {
		starts = append(starts, 0)
	}
	for _, h := range cuts {
		starts = append(starts, h.Index)
	}
	ordinal := 0
	for i, start := range starts {
		end := len(paragraphs)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		s := Section{Kind: KindFront, Title: "Front Matter"}
		if h, ok := headingAt(cuts, start); ok {
			s.Number, s.Title, s.Kind, s.How = h.Number, h.Title, h.Kind, h.How
			ordinal = next(ordinal, h.Number)
			start++
		}
		s.Ordinal = ordinal
		s.Body, s.First, s.Last = body(paragraphs, start, end, sub, bare)
		r.Sections = append(r.Sections, s)
	}
	if len(r.Sections) == 1 {
		r.Notes = append(r.Notes, "no section headings were found: the paper is one file")
	}
	if supplied {
		r.Notes = append(r.Notes, "the bibliography has no heading of its own and one was supplied where the entries begin")
	}
	for _, h := range cuts {
		if h.How == Typographic {
			r.Notes = append(r.Notes, fmt.Sprintf("the heading %q was found by how it is set and nothing else", h.Title))
		}
	}
	return r
}

// next is the number a section's file takes.
//
// A paper that numbers its own sections has already answered the question,
// and its section 3 is 03_model_architecture.md whether or not anything came
// before it. Everything else counts up: an unnumbered References after
// section 7 is 08, and a section numbered lower than the one before it (which
// happens when an appendix restarts at 1) counts up too, because two files
// cannot have the same name.
func next(last int, number string) int {
	if n := printed(number); n > last {
		return n
	}
	return last + 1
}

// printed reads a section number the way the paper wrote it, and returns zero
// for a section the paper did not number.
func printed(number string) int {
	if number == "" {
		return 0
	}
	top, _, _ := strings.Cut(number, ".")
	if n, err := strconv.Atoi(top); err == nil {
		return n
	}
	return romanValue(top)
}

func headingAt(cuts []Heading, index int) (Heading, bool) {
	for _, h := range cuts {
		if h.Index == index {
			return h, true
		}
	}
	return Heading{}, false
}

// body renders the paragraphs of one section, putting its subheadings back as
// Markdown headings. The pages are the first and last PDF page any of its
// paragraphs came off.
//
// The section's own heading is not in the body. It is in the front matter, as
// section and section_title, and a file that carried it in both would make
// the reading app choose which one to believe.
//
// bare is the paragraphs whose heading marker comes off and whose words stay,
// which is what happens to the two headings 00_front.md is made of.
func body(paragraphs []assemble.Paragraph, start, end int, sub map[int]Heading, bare map[int]bool) (text string, first, last int) {
	var b strings.Builder
	for i := start; i < end; i++ {
		p := paragraphs[i]
		_, isSub := sub[i]
		text := p.Text
		// A folio contributes nothing to the section and nothing to its page
		// range either: the page it sits on is in the range already, because
		// a page whose only content was its own number would not have been
		// cut into a section in the first place. A heading is left alone,
		// because a section numbered 11 is not page 11.
		if !isSub && !bare[i] {
			if text = unfolio(text); text == "" {
				continue
			}
		}
		if first == 0 || p.Page < first {
			first = p.Page
		}
		if p.Page+p.Pages-1 > last {
			last = p.Page + p.Pages - 1
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if bare[i] {
			text, _ = unmark(text)
		}
		if isSub {
			// Level three because the section's own heading is level two and
			// is in the front matter rather than in the file.
			//
			// Whichever notation the extractor wrote the heading in comes off
			// first. A model that writes its headings in Markdown hands this a
			// paragraph that already reads "## Abstract", and putting a level
			// in front of that wrote "### ## Abstract" into the file. The
			// reading app shows the second marker as text and rule T05 reads
			// the first one as a level one heading with a level three under
			// it, which is neither what the paper says nor what this meant.
			text, _ = unmark(text)
			b.WriteString("### ")
		}
		b.WriteString(text)
	}
	return b.String(), first, last
}

// Filename is what a section is filed as: the ordinal, then the title as a
// slug. Two digits because no paper in the hundred has a hundred sections and
// because a run of files that sorts as 1, 10, 2 is a run of files somebody has
// to sort by hand every time they list the directory.
func (s Section) Filename() string {
	slug := Slug(s.Title)
	if s.Kind == KindFront {
		slug = "front"
	}
	if slug == "" {
		slug = "section"
	}
	return fmt.Sprintf("%02d_%s.md", s.Ordinal, slug)
}

// Slug turns a section title into the part of a filename. Lower case ASCII,
// underscores, nothing else, and cut at a length that keeps the whole name
// readable in a directory listing.
//
// A title is not a unique key and this does not try to make it one. Two
// sections with the same title get different files because the ordinal is in
// the name, which is the other reason the ordinal is there.
func Slug(title string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if space && b.Len() > 0 {
				b.WriteByte('_')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	s := b.String()
	const maxSlug = 40
	if len(s) > maxSlug {
		s = s[:maxSlug]
		if i := strings.LastIndexByte(s, '_'); i > 0 {
			s = s[:i]
		}
	}
	return s
}

// sameText compares two titles as a reader would, ignoring the things an
// extraction changes and a person does not read as a difference.
//
// Case, because a model reads a title set in capitals and titleCase puts it
// back in a capitalisation of its own. Spacing, because a title set over two
// lines is joined with whatever the reader felt like. Punctuation, because the
// record has "MapReduce: Simplified Data Processing on Large Clusters" and the
// page prints the colon on the line break.
//
// An empty title matches nothing. A paper with no title on record is a paper
// this cannot help, and matching everything would eat its first section.
func sameText(a, b string) bool {
	if strings.TrimSpace(b) == "" {
		return false
	}
	return plain(a) == plain(b)
}

func plain(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}
