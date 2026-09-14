package emit

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/figures"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/refs"
	"github.com/tamnd/papers-reader/report"
)

// A Page is site/p/<id>/<lang>.json: one paper in one language, whole.
//
// One file per paper per language rather than one per section, because a
// reader opens a paper and reads it, and a request per section would be
// twenty requests to read one paper. A paper of this corpus runs to a few
// tens of kilobytes of JSON and a fraction of that on the wire.
type Page struct {
	Version int         `json:"version"`
	ID      string      `json:"id"`
	Lang    corpus.Lang `json:"lang"`
	// SourceLang is what a translation was made from, and is absent on the
	// English. Every translation in this corpus is made from the English and
	// the field says so anyway, because a corpus that one day translated the
	// Japanese from the Chinese would otherwise say nothing about it.
	SourceLang corpus.Lang `json:"source_lang,omitempty"`
	// Draft says this language is under the glossary coverage floor, which
	// is audit rule P04. The page is still built and still offered, with the
	// app saying what it is.
	Draft      bool       `json:"draft,omitempty"`
	Provenance Provenance `json:"provenance"`
	Front      Front      `json:"front"`
	Sections   []Section  `json:"sections"`
	Refs       []Ref      `json:"refs"`
	Notes      []Note     `json:"notes,omitempty"`
}

// Provenance is how this page came to exist, for the colophon at the foot of
// it.
//
// It is the whole paper's answer and the sections are what it is made of, so
// a field the sections disagree about is empty here rather than the first or
// the commonest of them. The two honesty flags go the other way: one section
// written by a small model makes the paper's answer true, because a reader
// deciding how far to trust a page wants to know that some of it is
// provisional, not that most of it is not.
type Provenance struct {
	Extraction       string `json:"extraction,omitempty"`
	ExtractionTool   string `json:"extraction_tool,omitempty"`
	TranslationModel string `json:"translation_model,omitempty"`
	// GlossaryVersion is the oldest glossary any section of this page was
	// translated against, because that is the one a reader would be caught
	// out by.
	GlossaryVersion int  `json:"glossary_version,omitempty"`
	SmallModel      bool `json:"small_model"`
	Gateway         bool `json:"gateway"`
}

// Front is the paper's own front matter, and its first page as printed.
//
// Blocks and not an abstract, which is what the specification for this file
// asked for, and the reason is the alignment key. Finding the abstract on a
// front page is a heuristic; the corpus has one and it works, but it runs
// per language and it can land on a different paragraph in the Vietnamese
// than it did in the English. Cutting the front page at a different place in
// each language is exactly the drift block indices exist to rule out, so the
// front page is carried whole and in order the way a section is, and the app
// decides how much of it to show.
type Front struct {
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	Venue   string   `json:"venue,omitempty"`
	Year    int      `json:"year,omitempty"`
	Blocks  []Block  `json:"blocks"`
}

// A Section is one content file of the paper.
type Section struct {
	Anchor string `json:"anchor"`
	Tag    string `json:"tag,omitempty"`
	Number string `json:"number,omitempty"`
	Title  string `json:"title"`
	// Level is how deep the section sits, counted off its number: 3 is level
	// two and 3.2 is level three. The paper's title is level one, which is
	// why a top level section is two.
	Level int `json:"level"`
	// Kind is section or appendix. The front matter is in Front above and
	// the references are in Refs below, so neither of those is here.
	Kind string `json:"kind"`
	// SmallModel and Gateway mark a section that is provisional because of
	// what wrote it, and the app prints that on the section rather than only
	// in the colophon. A reader who has just read a paragraph is owed the
	// warning where the paragraph is.
	SmallModel bool    `json:"small_model,omitempty"`
	Gateway    bool    `json:"gateway,omitempty"`
	Blocks     []Block `json:"blocks"`
}

// A Block is one paragraph, heading, list, formula, figure, listing or
// table.
//
// I is the block index and it is the alignment key. Every language of a
// section has the same blocks in the same order with the same indices, which
// is what audit rules L03, L04, L16 and L18 enforce, so laying the English
// beside the Vietnamese is a matter of putting block i against block i with
// no diffing and no guessing. The whole reason the audit is as strict as it
// is about structure is so that this one field can be trusted.
//
// Kind is p, heading, list, math, figure, code or table. The specification
// for this file named five of those. Headings and lists are the two it did
// not, and they are here rather than flattened into prose because a heading
// is what L03 compares across languages, and a list set as a paragraph would
// align against a real paragraph somewhere else.
type Block struct {
	Kind string `json:"kind"`
	I    int    `json:"i"`
	// HTML is the rendered block, for every kind that has one, sanitised
	// against the allowlist in this package. For a heading it is the heading
	// text and not an h element, because the level is a field of its own and
	// the app chooses the element.
	HTML string `json:"html,omitempty"`
	// Level is the depth of a heading, counted in hashes.
	Level int `json:"level,omitempty"`
	// TeX is the mathematics as the corpus holds it, carried beside the
	// rendered form so that a reader can copy the formula out and so that
	// the search can index it as text.
	TeX string `json:"tex,omitempty"`
	// Src is a figure, as a path relative to the root of the site. W and H
	// are its pixels, which the app needs before the image has loaded if the
	// page is not to jump under the reader.
	Src string `json:"src,omitempty"`
	W   int    `json:"w,omitempty"`
	H   int    `json:"h,omitempty"`
	// Number is what the paper printed: the equation's tag, the figure's
	// number. Never a number this toolchain invented, because every cross
	// reference in the prose is to the paper's own numbering.
	Number      string `json:"number,omitempty"`
	CaptionHTML string `json:"caption_html,omitempty"`
	// Syntax is the word on the opening fence of a listing. It is lang in
	// the JSON, which is the word a highlighter expects, and it is mostly
	// absent: what this corpus holds is pseudocode.
	Syntax string `json:"lang,omitempty"`
	Text   string `json:"text,omitempty"`
	Anchor string `json:"anchor,omitempty"`
	Tag    string `json:"tag,omitempty"`
}

// A Ref is one entry of the paper's bibliography.
//
// From the refs manifest and not from the references content file, because
// the manifest is the parsed form and it is what carries ResolvesTo. A
// bibliography is not translated, so this is the same list in every language
// of the page.
type Ref struct {
	Key string `json:"key"`
	Raw string `json:"raw"`
	// ResolvesTo is the identifier of the paper in this corpus the entry
	// names, where it names one, which is what turns a bibliography into
	// navigation.
	ResolvesTo string `json:"resolves_to,omitempty"`
}

// A Note is one footnote of the paper, gathered from wherever its definition
// fell.
//
// Gathered because the page break decides where a definition lands and it is
// regularly sections away from the marker that refers to it. The app puts
// them at the foot of the page or in a popup, and either way it wants them
// in one place.
type Note struct {
	Key  string `json:"key"`
	HTML string `json:"html"`
}

// A Fault is something a page refers to that is not there.
//
// It is not in the emitted JSON. A build is what the app reads and a fault
// is what the audit reads, and putting the second in the first would ship
// the toolchain's complaints to the reader. Rules P01, P02 and P03 are this
// list, sorted into three piles by Kind.
type Fault struct {
	// Page is the path of the page the fault is on, as PagePath writes it,
	// so a finding names a file somebody can open.
	Page string
	// Kind is math, figure, note, paper or citation.
	Kind string
	What string
}

// The kinds of fault, which are the three audit rules split by what went
// wrong: a formula that would not render or markup off the allowlist is
// P01, a link with nothing at the other end is P02, and a picture the build
// refers to and does not hold is P03.
const (
	FaultMath     = "math"
	FaultMarkup   = "markup"
	FaultFigure   = "figure"
	FaultNote     = "note"
	FaultPaper    = "paper"
	FaultCitation = "citation"
)

// BuildPages reads the corpus and builds a page for every paper in every
// language it has content in.
//
// One KaTeX engine for the whole build. Loading it costs a tenth of a second
// and a run over the corpus renders tens of thousands of spans, most of them
// the same span four times over in four languages, so the engine's cache
// does most of the work.
func BuildPages(c *corpus.Corpus) ([]*Page, []Fault, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, nil, err
	}
	k, err := katex.New()
	if err != nil {
		return nil, nil, err
	}
	g, err := glossary.Load(c.GlossaryManifest())
	if err != nil {
		return nil, nil, err
	}
	figs, err := loadFigures(c)
	if err != nil {
		return nil, nil, err
	}
	known := map[string]bool{}
	for _, p := range papers.Papers {
		known[p.ID] = true
	}

	var pages []*Page
	var faults []Fault
	for _, p := range papers.Papers {
		bib, err := refs.Load(c.Refs(p.ID))
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, err
		}
		for _, l := range corpus.Langs {
			found, err := report.Files(c, l, p.ID)
			if err != nil {
				return nil, nil, err
			}
			if len(found) == 0 {
				continue
			}
			page, bad, err := buildPage(pageInput{
				paper: p, lang: l, files: found, katex: k,
				figures: figs[p.ID], bib: bib, known: known,
				draft: l.Translated() && g.Under(l),
			})
			if err != nil {
				return nil, nil, err
			}
			pages = append(pages, page)
			faults = append(faults, bad...)
		}
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].ID != pages[j].ID {
			return pages[i].ID < pages[j].ID
		}
		return pages[i].Lang < pages[j].Lang
	})
	return pages, faults, nil
}

// pageInput is everything one page is built from, in one argument because
// there are eight of them and a call with eight positional arguments is a
// call nobody can read.
type pageInput struct {
	paper   corpus.Paper
	lang    corpus.Lang
	files   []string
	katex   *katex.Renderer
	figures map[string]figures.Figure
	bib     *refs.Manifest
	known   map[string]bool
	draft   bool
}

func buildPage(in pageInput) (*Page, []Fault, error) {
	pg := &Page{
		Version: Version, ID: in.paper.ID, Lang: in.lang, Draft: in.draft,
		Sections: []Section{}, Refs: []Ref{},
		Front: Front{
			Title: in.paper.Title, Authors: in.paper.Authors,
			Venue: in.paper.Venue, Year: in.paper.Year, Blocks: []Block{},
		},
	}
	if pg.Front.Authors == nil {
		pg.Front.Authors = []string{}
	}
	if in.lang.Translated() {
		pg.SourceLang = corpus.EN
	}

	r := &pager{paper: in.paper.ID, katex: in.katex, figures: in.figures,
		known: in.known, notes: map[string]string{}}
	if in.bib != nil {
		r.cites = map[string]bool{}
		r.byPaper = map[string]string{}
		for _, e := range in.bib.Entries {
			r.cites[e.Key] = true
			if e.ResolvesTo != "" {
				r.byPaper[e.ResolvesTo] = e.Key
			}
			pg.Refs = append(pg.Refs, Ref{Key: e.Key, Raw: e.Raw, ResolvesTo: e.ResolvesTo})
		}
	}

	// Two passes over the files. The footnote definitions are gathered first
	// because a marker in section one is regularly defined in section four,
	// and a renderer that had not read section four yet would call it an
	// orphan.
	type file struct {
		front corpus.Front
		body  string
	}
	var read []file
	for _, name := range in.files {
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, nil, err
		}
		front, body, err := corpus.ParseFront(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		read = append(read, file{front, markdown.Notes(string(body), r.notes)})
	}

	var prov provenance
	for _, f := range read {
		prov.add(f.front)
		switch f.front.Kind {
		case "front":
			pg.Front.Blocks = r.blocks(f.body)
			if f.front.Title != "" {
				pg.Front.Title = f.front.Title
			}
		case "references":
			// The bibliography is in Refs above, parsed and with its links
			// resolved. The content file is the same list unparsed, and
			// carrying both would be two lists for the app to disagree
			// about.
		default:
			pg.Sections = append(pg.Sections, Section{
				Anchor:     corpus.SectionAnchor(in.paper.ID, f.front.Section),
				Tag:        f.front.Tag,
				Number:     f.front.Section,
				Title:      f.front.SectionTitle,
				Level:      level(f.front.Section),
				Kind:       kindOf(f.front.Kind),
				SmallModel: f.front.SmallModel,
				Gateway:    f.front.Gateway,
				Blocks:     r.blocks(f.body),
			})
		}
	}
	pg.Provenance = prov.done()

	for _, key := range noteOrder(r.notes) {
		pg.Notes = append(pg.Notes, Note{Key: key, HTML: r.inline(r.notes[key])})
	}
	return pg, r.faults(PagePath(pg.ID, pg.Lang)), nil
}

// provenance gathers the front matter of every file of a page into the one
// answer the colophon prints.
type provenance struct {
	out Provenance
	// split says the sections disagreed about a field, which is recorded per
	// field so that one section extracted differently does not empty the
	// translation model as well.
	splitExtraction, splitTool, splitModel bool
}

// add folds one file's front matter into the answer.
//
// A file that records nothing is not a file that disagrees. The front matter
// file of a paper carries the paper's own bibliographic fields and often
// none of the toolchain's, and counting its silence as a different answer
// would empty the colophon of every paper in the corpus.
func (p *provenance) add(f corpus.Front) {
	agree := func(out *string, split *bool, v string) {
		switch {
		case v == "" || *split:
		case *out == "":
			*out = v
		case *out != v:
			*split = true
		}
	}
	agree(&p.out.Extraction, &p.splitExtraction, f.Extraction)
	agree(&p.out.ExtractionTool, &p.splitTool, f.ExtractionModel)
	agree(&p.out.TranslationModel, &p.splitModel, f.TranslationModel)
	if f.GlossaryVersion != 0 && (p.out.GlossaryVersion == 0 || f.GlossaryVersion < p.out.GlossaryVersion) {
		p.out.GlossaryVersion = f.GlossaryVersion
	}
	p.out.SmallModel = p.out.SmallModel || f.SmallModel
	p.out.Gateway = p.out.Gateway || f.Gateway
}

func (p *provenance) done() Provenance {
	if p.splitExtraction {
		p.out.Extraction = ""
	}
	if p.splitTool {
		p.out.ExtractionTool = ""
	}
	if p.splitModel {
		p.out.TranslationModel = ""
	}
	return p.out
}

// level is how deep a section sits, counted off the number the paper
// printed.
//
// The paper's title is level one, so a numbered top level section is two and
// each dot is one deeper. A section the paper did not number is level two,
// which is what an unnumbered Introduction or Acknowledgments is.
func level(number string) int {
	if number == "" {
		return 2
	}
	return 1 + len(strings.Split(number, "."))
}

// kindOf is the two section kinds a page carries. Anything else the splitter
// wrote is a section, because the app sets it as one and a kind the app did
// not know would be a kind it ignored.
func kindOf(s string) string {
	if s == "appendix" {
		return "appendix"
	}
	return "section"
}

// noteOrder is the footnote keys in the order a reader expects, which is
// numeric where they are numbers and alphabetical where they are not. Papers
// use both and a few use daggers.
func noteOrder(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, erra := strconv.Atoi(out[i])
		b, errb := strconv.Atoi(out[j])
		if erra == nil && errb == nil {
			return a < b
		}
		if erra == nil != (errb == nil) {
			return erra == nil
		}
		return out[i] < out[j]
	})
	return out
}

// loadFigures reads the figures manifest once and files every figure under
// the anchor the content refers to it by.
func loadFigures(c *corpus.Corpus) (map[string]map[string]figures.Figure, error) {
	out := map[string]map[string]figures.Figure{}
	m, err := figures.Load(c.FiguresManifest())
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, f := range m.Figures {
		if out[f.Paper] == nil {
			out[f.Paper] = map[string]figures.Figure{}
		}
		out[f.Paper][corpus.ItemAnchor(f.Paper, "fig", f.Number)] = f
	}
	return out, nil
}

// FigureSrc is where a figure sits in a build, relative to the root of the
// site. The same shape as in the corpus, so a reader who finds a picture on
// the site can find the same file in the repository.
func FigureSrc(paper, name string) string { return path.Join("figures", paper, name) }

// PagePath is where a page sits in a build.
func PagePath(id string, l corpus.Lang) string {
	return path.Join("p", id, string(l)+".json")
}
