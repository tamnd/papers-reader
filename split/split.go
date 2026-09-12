package split

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

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
func Split(d *assemble.Document) *Result {
	// Unrun first, because everything below counts paragraphs and a heading
	// that is still inside one is a heading nothing here can find.
	paragraphs := Unrun(d.Paragraphs)
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
	// The abstract is not a section. It is what 00_front.md is for, along
	// with the title and the authors, because the abstract is the one part of
	// a paper that is quoted on its own and a reader who wants it should not
	// have to know whether this paper happened to head it.
	if len(cuts) > 0 && cuts[0].Title == "Abstract" {
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
		s.Body, s.First, s.Last = body(paragraphs, start, end, sub)
		r.Sections = append(r.Sections, s)
	}
	if len(r.Sections) == 1 {
		r.Notes = append(r.Notes, "no section headings were found: the paper is one file")
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
func body(paragraphs []assemble.Paragraph, start, end int, sub map[int]Heading) (text string, first, last int) {
	var b strings.Builder
	for i := start; i < end; i++ {
		p := paragraphs[i]
		if first == 0 || p.Page < first {
			first = p.Page
		}
		if p.Page+p.Pages-1 > last {
			last = p.Page + p.Pages - 1
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		// Level three because the section's own heading is level two and is
		// in the front matter rather than in the file.
		if _, ok := sub[i]; ok {
			b.WriteString("### ")
		}
		b.WriteString(p.Text)
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
