package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/papers-reader/assemble"
	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/refs"
	"github.com/tamnd/papers-reader/split"
)

func runAssemble(args []string) error {
	fs := flag.NewFlagSet("assemble", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "assemble these papers only, comma separated")
	field := fs.String("field", "", "assemble one field only")
	all := fs.Bool("all", false, "assemble every paper that has extracted pages")
	long := fs.Bool("v", false, "print the first line of every paragraph that ran across a page")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers assemble [flags]

Joins the extracted page files of a paper into one document, healing the
joins, and writes it to work/<id>/document.md for a person to read.

A page is where the paper ran out of room and nothing a reader wants to know
is expressed by it. A sentence runs across a page break, a word is broken
across one, and a paragraph that starts at the top of page six started on
page five. The rule for putting them back together is narrow on purpose: a
page that ends without terminal punctuation and whose next page starts lower
case is a continuation, and anything else starts a new paragraph. Two
paragraphs wrongly run together read as one confused paragraph and the split
point is gone; a paragraph wrongly broken in two reads as two paragraphs and
a person can see where it happened.

Nothing here is committed. papers split is what writes the corpus, and it
joins the pages again itself, so this command is for looking at.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to assemble: --id, --field or --all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}

	var done int
	for _, p := range todo {
		d, err := document(c, p.ID)
		switch {
		case err != nil:
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		case d == nil:
			continue
		}
		path := c.Work(p.ID, "document.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(d.Text()), 0o644); err != nil {
			return err
		}
		done++
		across := 0
		for _, par := range d.Paragraphs {
			if par.Pages > 1 {
				across++
				if *long {
					fmt.Printf("    page %d: %s\n", par.Page, first(par.Text, 72))
				}
			}
		}
		fmt.Printf("  %-34s %d paragraphs from pages %d to %d, %d joined across a page\n",
			p.ID, len(d.Paragraphs), d.First, d.Last, across)
	}
	fmt.Printf("%d papers assembled\n", done)
	return nil
}

func runSplit(args []string) error {
	fs := flag.NewFlagSet("split", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "split these papers only, comma separated")
	field := fs.String("field", "", "split one field only")
	all := fs.Bool("all", false, "split every paper that has extracted pages")
	force := fs.Bool("force", false, "overwrite files somebody has edited by hand")
	accept := fs.Bool("accept", false, "restamp files somebody has edited by hand and mark them edited")
	dry := fs.Bool("dry-run", false, "print what would be written and write nothing")
	long := fs.Bool("v", false, "print every section")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers split [flags]

Cuts a paper into one file per top level section under content/en/<id>/,
writes the front matter, and assigns no tags: tags are a separate pass.

The section boundaries come from the paper's own headings. A numbered
heading is the most reliable and the numbering scheme is worked out once per
paper and then required, so a line that happens to start with a digit is not
read as the start of section 5. A heading with no number is matched against
the section names English papers use. A heading found by nothing but how it
is set is reported, and is only looked for at all in a paper that numbers
nothing.

A file whose content_sha256 does not match the body next to it has been
edited by somebody and is left alone. That is how a hand correction survives
the next extraction run, and --force is how you throw it away on purpose.

A correction is protected the moment it is made and it fails audit rule T03
until somebody says they meant it, which is the right way round: the audit
should be red while a correction is half done. --accept is how you say it.
It restamps the hash over the corrected body and writes edited: true in the
front matter, which is what the splitter then protects the file by, and the
rule goes green. It touches no file whose body still hashes to its own hash,
because putting a fence round a file nobody has edited would stop the
splitter keeping it up to date.

A file from an earlier split that this one did not produce is reported and
not deleted, because the usual reason for one is that a section boundary
moved and that is worth seeing.

A restricted paper gets its front matter and an abstract of at most 250
words. Nothing else about it may be published, so nothing else is written.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to split: --id, --field or --all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	recorded, err := c.LoadSources()
	if err != nil {
		return err
	}

	if *accept {
		return acceptEdits(c, todo, *dry)
	}

	var papers, written, kept int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		n, err := splitOne(c, p, rec, *force, *dry, *long)
		switch {
		case err != nil:
			fmt.Printf("  %-34s %v\n", p.ID, err)
		case n.sections == 0:
			continue
		default:
			papers++
			written += n.written
			kept += n.kept
			fmt.Printf("  %-34s %s\n", p.ID, n)
			for _, note := range n.notes {
				fmt.Printf("    %s\n", note)
			}
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d papers split, %d files written, %d left as somebody edited them\n", papers, written, kept)
	return nil
}

// acceptEdits takes the hand corrections in a paper's content directory as
// the version of record. It writes nothing else: this is a separate pass from
// the split and not a mode of it, because a run that both re-split a paper
// and accepted the edits in it would be deciding which of the two won.
func acceptEdits(c *corpus.Corpus, todo []corpus.Paper, dry bool) error {
	var n int
	for _, p := range todo {
		dir := c.Content(corpus.EN, p.ID)
		if dry {
			// Nothing to do here but say so. Accept reads and writes in one
			// pass and splitting it in two so that a dry run could report
			// without writing would be two ways to decide the same thing.
			fmt.Printf("  %-34s would accept the hand edits under %s\n", p.ID, dir)
			continue
		}
		names, err := split.Accept(dir)
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		}
		if len(names) == 0 {
			continue
		}
		n += len(names)
		fmt.Printf("  %-34s %d accepted\n", p.ID, len(names))
		for _, name := range names {
			fmt.Printf("    %s\n", name)
		}
	}
	if dry {
		fmt.Println("dry run, nothing written")
		return nil
	}
	fmt.Printf("%d hand edits accepted\n", n)
	return nil
}

// cut is what one paper's split came to.
type cut struct {
	sections, written, kept int
	notes                   []string
}

func (n cut) String() string {
	s := fmt.Sprintf("%d sections, %d written", n.sections, n.written)
	if n.kept > 0 {
		s += fmt.Sprintf(", %d kept as edited", n.kept)
	}
	return s
}

func splitOne(c *corpus.Corpus, p corpus.Paper, rec *corpus.Source, force, dry, long bool) (cut, error) {
	var n cut
	if rec == nil || rec.Access == corpus.AccessUnknown || rec.Access == "" {
		return n, fmt.Errorf("nothing is known about what may be published from it, so nothing is written")
	}
	d, err := document(c, p.ID)
	if err != nil || d == nil {
		return n, err
	}
	r := split.Titled(d, p.Title)
	n.notes = r.Notes
	n.notes = append(n.notes, cite(c, p.ID, r)...)

	front := corpus.Front{
		Paper:     p.ID,
		Title:     p.Title,
		Authors:   p.Authors,
		Year:      p.Year,
		Venue:     p.Venue,
		Field:     p.Field,
		Lang:      corpus.EN,
		Source:    sourceRef(p, rec),
		PDFSHA256: rec.SHA256,
	}
	// What read the pages is what the extraction run wrote down. A paper
	// extracted before the run kept a record, or one whose work directory
	// has been cleaned out, leaves these fields empty rather than claiming
	// a path nobody can check.
	record, err := extract.ReadRecord(c.Work(p.ID))
	if err != nil {
		return n, err
	}
	if record != nil {
		front.Extraction = record.Path
		front.ExtractionModel = record.Tool
		front.PromptSHA256 = record.Prompt
	} else {
		n.notes = append(n.notes, "no extraction record, so the front matter cannot say what read the pages")
	}
	files := split.Files(front, r)
	if !rec.Access.Body() {
		// Restricted. The front matter and a short abstract is the whole of
		// what may ever be published, so the rest of the paper is not written
		// at all rather than written and then guarded by an audit rule.
		files = split.Restrict(files, split.AbstractWords)
		n.notes = append(n.notes, "restricted: the front matter and an abstract, and nothing else")
	}
	n.sections = len(files)
	if long {
		for _, f := range files {
			fmt.Printf("    %-36s %s\n", f.Name, f.Front.SectionTitle)
		}
	}
	if dry {
		n.written = len(files)
		return n, nil
	}
	report, err := split.Write(c.Content(corpus.EN, p.ID), files, force)
	if err != nil {
		return n, err
	}
	n.written = len(report.Created) + len(report.Updated)
	n.kept = len(report.Kept)
	for _, name := range report.Kept {
		n.notes = append(n.notes, fmt.Sprintf("%s was edited by hand and is left alone", name))
	}
	for _, name := range report.Stale {
		n.notes = append(n.notes, fmt.Sprintf("%s is from an earlier split and this one did not produce it", name))
	}
	return n, nil
}

// cite rewrites the in-text citations of every section into links, using
// the bibliography papers refs build already parsed.
//
// It runs here rather than in package split because split is the only thing
// that writes a content file, and a rewrite done after the file was written
// would have to defeat the hand-edit protection to do it. The reference
// section itself is left alone: the labels in it are the entries, not
// citations of them.
func cite(c *corpus.Corpus, id string, r *split.Result) []string {
	m := loadRefs(c, id)
	if m == nil {
		return nil
	}
	links := m.Links()
	if len(links) == 0 {
		return nil
	}
	for i := range r.Sections {
		if r.Sections[i].Kind == split.KindReferences {
			continue
		}
		r.Sections[i].Body = refs.Rewrite(r.Sections[i].Body, links)
	}
	return []string{fmt.Sprintf("%d references link into the corpus", len(links))}
}

// document joins one paper's extracted pages. A paper with no pages yet is
// not an error: the usual way to run these commands is over the whole corpus
// while extraction is still working through it.
func document(c *corpus.Corpus, id string) (*assemble.Document, error) {
	store := extract.Store{Dir: c.Work(id, "pages")}
	numbers, err := store.Pages()
	if err != nil {
		return nil, err
	}
	if len(numbers) == 0 {
		return nil, nil
	}
	text := make(map[int]string, len(numbers))
	for _, page := range numbers {
		s, err := store.Read(page)
		if err != nil {
			return nil, err
		}
		text[page] = s
	}
	undress, err := modelRead(c, id)
	if err != nil {
		return nil, err
	}
	if undress {
		text = extract.Undress(text)
	}
	pages := make([]assemble.Page, 0, len(numbers))
	for _, page := range numbers {
		// Unalign runs whatever read the paper. A line the page set to its
		// two margins collapses in Markdown the same way however it was
		// read, and the repair is in the assembly rather than in the reading
		// so that fixing it does not mean reading a hundred papers again.
		pages = append(pages, assemble.Page{
			Number: page,
			Text:   extract.Unalign(text[page]),
			// The same answer that decides whether the furniture is still
			// on the page decides whether the blank lines in it are a
			// guess, because both follow from having read the page without
			// the coordinates of its lines.
			Model: undress,
		})
	}
	doc := assemble.Join(pages)
	// After the join, because a listing can carry over a page break and the
	// tag belongs to the whole of it. Before the split, because the split is
	// what writes the file and a fence with no tag on it is rule C02.
	for i := range doc.Paragraphs {
		doc.Paragraphs[i].Text = code.Label(doc.Paragraphs[i].Text)
	}
	return doc, nil
}

// modelRead is whether a paper's pages came out of a model rather than out
// of the PDF's own text layer.
//
// What it decides is whether the running heads and the folios are still on
// the pages. The native path takes them off while it reads, because it has
// the coordinates of every line and can see which of them are in the
// margin. A model has no such thing to hand, so the prompt asks it to
// transcribe them where they are and says a later program takes them out,
// and extract.Undress is that program.
//
// A paper with no record at all is read as a model's work. That was the
// state of every work directory written before records existed, there are
// none of those left in the corpus, and of the two ways to be wrong here,
// leaving a running head in the published text is the one a reader sees.
func modelRead(c *corpus.Corpus, id string) (bool, error) {
	record, err := extract.ReadRecord(c.Work(id))
	if err != nil {
		return false, err
	}
	return record == nil || record.Path != string(classify.PathNative), nil
}

// sourceRef is the source field of the front matter: where this text came
// from, in the shortest form that identifies it.
func sourceRef(p corpus.Paper, rec *corpus.Source) string {
	switch {
	case p.ArXiv != "":
		return "arxiv:" + p.ArXiv
	case p.DOI != "":
		return "doi:" + p.DOI
	case rec != nil && rec.Landing != "":
		return rec.Landing
	case rec != nil:
		return rec.URL
	}
	return ""
}

// chooseFrom opens the corpus and picks the papers a run is about.
func chooseFrom(root, ids, field string) (*corpus.Corpus, []corpus.Paper, error) {
	c, err := openCorpus(root)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return nil, nil, err
	}
	todo, err := choosePapers(manifest, ids, field)
	if err != nil {
		return nil, nil, err
	}
	return c, todo, nil
}

// first is the opening of a paragraph, for a line of output that has to fit
// on a terminal.
func first(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "..."
}
