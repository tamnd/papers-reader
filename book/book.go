package book

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/refs"
)

// A Section is one content file, ready to be set.
//
// Body is the Markdown with the footnote definitions taken out of it, because
// a definition belongs where it is used and the corpus splitter puts it where
// the page break fell. Everything else is as committed.
type Section struct {
	Front corpus.Front
	// Name is the file name, so that an error about a section says which file
	// to open.
	Name   string
	Number string
	Title  string
	Anchor string
	Kind   string
	Body   string
}

// Numbered says whether the section carries a number of its own, which
// decides whether it is set as \section or as \section*.
func (s Section) Numbered() bool { return s.Number != "" }

// A Book is one paper in one language, everything needed to set it and
// nothing that came from outside the corpus.
type Book struct {
	ID    string
	Lang  corpus.Lang
	Title string
	// TitleAs is the title in the language of the book, and empty for an
	// English book or for a translation that left the title alone. The title
	// page prints it above the English one, because a reader of the Japanese
	// edition wants the Japanese title and a reader looking the paper up
	// wants the name it is catalogued under.
	TitleAs string
	Authors []string
	Year    int
	Venue   string
	Field   corpus.Field
	// Source is what the front matter says the pages were read from, an arXiv
	// identifier or a DOI. It goes on the title page because a translation
	// that does not say what it is a translation of is not much use.
	Source string
	// Masthead is the affiliation and the arXiv stamp off the first page, kept
	// because they are the only part of the front page the title page does not
	// already carry.
	Masthead []string
	Abstract string
	Sections []Section
	// Bibliography is the entries of the references file, one per entry, still
	// as Markdown. It is separate from Sections because it is set as a list
	// rather than as prose and because it is the one section that is copied
	// rather than translated.
	Bibliography []string
	// Notes is the footnote definitions gathered from every file, by label.
	// They are collected book wide because a marker and its definition are
	// regularly in different files: the marker is on the author line of the
	// front page and the definition is wherever the page break put it.
	Notes map[string]string
	// Figures is every figure of this paper, by anchor, so that a caption
	// paragraph can find the picture that belongs above it.
	Figures map[string]figures.Figure
	// Dir is where the figure files are, absolute, because tectonic is run in
	// a temporary directory and \includegraphics needs to find them from
	// there.
	Dir string
	// Refs is the parsed bibliography, for turning [12] into a link in the
	// EPUB. Nil when the paper has no refs manifest, which is most of them.
	Refs *refs.Manifest
}

// Empty says the paper published no body, which is what a restricted paper
// looks like from here. The book is still worth making, it is just a title
// page and an abstract.
func (b *Book) Empty() bool { return len(b.Sections) == 0 && len(b.Bibliography) == 0 }

// Cite is the number the paper printed for another paper of the corpus.
//
// The prose carries corpus identifiers, written [[lecun-1998-lenet]], because
// that is a link the reading app can follow and a number is not. A book has
// no such link and a reader who sees a slug in the middle of a sentence has
// been shown the plumbing. So the identifier is turned back into the number
// the page it came from printed, which is in the refs manifest already: every
// entry there carries the key it was labelled with and the paper it resolves
// to. A paper that is in the corpus and not in this bibliography comes back
// false, and the caller decides what to print.
func (b *Book) Cite(id string) (string, bool) {
	if b.Refs == nil {
		return "", false
	}
	for _, e := range b.Refs.Entries {
		if e.ResolvesTo == id && e.Key != "" {
			return e.Key, true
		}
	}
	return "", false
}

// Load reads one paper in one language out of a corpus.
//
// Everything it needs is committed: the content files, the figures manifest,
// the figure files themselves and the refs manifest where there is one. A
// missing refs manifest is not an error, because refs only runs on papers
// whose bibliography parsed, and a missing figure file is not an error either,
// because the manifest is the record and a working tree can be shallow.
func Load(c *corpus.Corpus, id string, l corpus.Lang) (*Book, error) {
	dir := c.Content(l, id)
	names, err := contents(dir)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s has no %s content at %s", id, l, dir)
	}

	b := &Book{ID: id, Lang: l, Notes: map[string]string{}, Figures: map[string]figures.Figure{}}
	b.Dir, err = filepath.Abs(c.Figures(id))
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		front, body, err := corpus.ParseFront(raw)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", id, name, err)
		}
		text := notes(string(body), b.Notes)
		switch front.Kind {
		case "front":
			b.title(front)
			var printed string
			printed, b.Masthead, b.Abstract = masthead(text)
			b.TitleAs = translated(printed, b.Title, l)
		case "references":
			b.Bibliography = entries(text)
		default:
			b.Sections = append(b.Sections, Section{
				Front:  front,
				Name:   name,
				Number: front.Section,
				Title:  front.SectionTitle,
				Anchor: corpus.SectionAnchor(id, front.Section),
				Kind:   front.Kind,
				Body:   text,
			})
		}
	}
	if b.Title == "" {
		return nil, fmt.Errorf("%s has %s content and no front matter file, so there is no title to set", id, l)
	}

	if m, err := figures.Load(c.FiguresManifest()); err == nil {
		for _, f := range m.Of(id) {
			b.Figures[corpus.ItemAnchor(id, "fig", f.Number)] = f
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if m, err := refs.Load(c.Refs(id)); err == nil {
		b.Refs = m
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return b, nil
}

// title takes the bibliographic facts off whichever file carries them. They
// are on every file, and the front matter file is the one to believe because
// it is the one a person would have corrected.
func (b *Book) title(f corpus.Front) {
	b.Title, b.Authors, b.Year = f.Title, f.Authors, f.Year
	b.Venue, b.Field, b.Source = f.Venue, f.Field, f.Source
}

// contents lists the Markdown of a content directory in file name order,
// which the splitter made reading order by numbering the files.
func contents(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

var notePattern = regexp.MustCompile(`^\[\^([^\]]+)\]:\s*(.*)$`)

// notes lifts the footnote definitions out of a body and into a book wide
// table, and returns the body without them.
//
// They have to come out. LaTeX has no free standing footnote definition, so a
// definition left in the prose would set as a paragraph reading "[^1]: Jean
// Pouget-Abadie is visiting Universite de Montreal", in the middle of section
// one, three pages after the marker that refers to it. The marker is on the
// author line of the front page and the definition is in the first section
// because that is where the page break fell, and no rearranging of the corpus
// would fix that: the corpus is right and it is the setting that has to
// gather them.
//
// A continuation line, indented under a definition, is joined to it. Nothing
// in this corpus has one yet and the format allows it.
func notes(body string, into map[string]string) string {
	var out []string
	label := ""
	for _, line := range strings.Split(body, "\n") {
		if m := notePattern.FindStringSubmatch(line); m != nil {
			label = m[1]
			into[label] = m[2]
			continue
		}
		if label != "" {
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				into[label] = strings.TrimSpace(into[label] + " " + strings.TrimSpace(line))
				continue
			}
			if strings.TrimSpace(line) == "" {
				// A blank line after a definition ends it, and is dropped along
				// with it so the body does not grow a gap where it stood.
				label = ""
				continue
			}
			label = ""
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// masthead splits the front page into the lines worth keeping and the
// abstract.
//
// The abstract is the longest paragraph. That is not a guess about this paper,
// it is the shape of a first page: the title is a line, the authors are a
// line, the affiliation is three short lines, the arXiv stamp is a line, the
// word Abstract is a word, and then there is a paragraph of two hundred
// words. It holds in the three translations as well, where every one of those
// parts is translated and the shape is not.
//
// Longest is corpus.Words and not strings.Fields, because the Japanese
// abstract has no spaces in it and the author line has seven. Measured with
// spaces, the byline is the longest paragraph of the Japanese front page, so
// the setting printed the eight authors twice and set the abstract as part of
// the masthead.
//
// What comes back as the masthead is the paragraphs before it that the title
// page does not already print: not the title, not the author line, and not the
// bare word Abstract, which is a heading in the source and becomes one in the
// setting.
// The first paragraph comes back on its own, because on a translated front
// page it is the title as the translator wrote it and the title page wants it.
func masthead(body string) (string, []string, string) {
	paras := split(body)
	at := -1
	for i, p := range paras {
		if at < 0 || corpus.Words(p) > corpus.Words(paras[at]) {
			at = i
		}
	}
	if at < 0 {
		return "", nil, ""
	}
	var title string
	var keep []string
	for i, p := range paras[:at] {
		if i == 0 {
			// The title, which the title page sets.
			title = p
			continue
		}
		if strings.HasPrefix(p, "**") && strings.HasSuffix(strings.TrimSpace(p), "**") {
			// The author line, which the title page sets.
			continue
		}
		if corpus.Words(p) < 3 {
			// The word Abstract, in whichever language.
			continue
		}
		keep = append(keep, p)
	}
	return title, keep, strings.Join(paras[at:], "\n\n")
}

// translated is the paper's title in the language of the book, and empty
// where there is nothing to print beside the English one.
//
// It comes off the front page rather than out of papers.yaml. The spec asks
// for a title_<lang> field there, and that field is for the reading app,
// which lists papers it has not loaded and so cannot read a front page. The
// setter has the front page open, and the first paragraph of a translated
// 00_front.md is the title as the translator wrote it, which is the same
// string a person would have to copy into papers.yaml by hand.
//
// English gets nothing, since the title page already prints it. Neither does
// a translation that left the title alone, which is the right answer for a
// paper named after a person or an algorithm and which would otherwise set
// the same line twice.
func translated(printed, english string, l corpus.Lang) string {
	printed = strings.TrimSpace(printed)
	if l == corpus.EN || printed == strings.TrimSpace(english) {
		return ""
	}
	return printed
}

// entries cuts a bibliography into its entries.
//
// One paragraph is one entry. That is how refs.Parse reads it and how the
// extractor writes it, and an entry that wrapped onto a second paragraph would
// already be two entries everywhere else in the toolchain.
func entries(body string) []string { return split(body) }

// split cuts a body on blank lines. It is the same cut the chunker and the
// verifier make, so a block here is a block there.
func split(body string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if p = strings.Trim(p, "\n"); strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
