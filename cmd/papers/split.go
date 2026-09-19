package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	prune := fs.Bool("prune", false, "delete the files an earlier split left behind, and their translations")
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

A file the last split wrote and this one did not is reported and left where
it is, because the usual reason for one is that a section boundary moved and
that is worth seeing rather than reading out of a diff. --prune is how you
delete them once you have looked. Two files for one section is two files
carrying the same section number, which fails audit rule T04 until one of
them goes. --prune takes the translations of those sections with them, for
the same reason: a Vietnamese file whose English is gone carries the old
section number too and no later run has any reason to touch it.

A correction is protected the moment it is made and it fails audit rule T03
until somebody says they meant it, which is the right way round: the audit
should be red while a correction is half done. --accept is how you say it.
It restamps the hash over the corrected body and writes edited: true in the
front matter, which is what the splitter then protects the file by, and the
rule goes green. It touches no file whose body still hashes to its own hash,
because putting a fence round a file nobody has edited would stop the
splitter keeping it up to date. It covers the translations as well as the
English, because a Japanese file gets corrected for the same reasons and
papers translate reads the same flag.

A restricted paper gets its front matter and an abstract of at most 250
words. Nothing else about it may be published, so nothing else is written.

None of this applies to a corpus whose manifests/policy.yaml says body.
That corpus publishes every paper in full, whatever its licence says, and
the decision to do so is recorded in that file rather than in this one.

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
		n, err := splitOne(c, p, rec, *force, *prune, *dry, *long)
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		}
		// After the split and not inside it, so that a paper with no pages
		// left to split still has its translations swept. That paper is the
		// one that needs it: nothing else in the toolchain will ever touch
		// its files again.
		if *prune && !*dry {
			left, err := orphans(c, p.ID)
			if err != nil {
				return err
			}
			for _, o := range left {
				n.notes = append(n.notes, fmt.Sprintf("%s is a translation of a section that has no English and has been deleted", o))
			}
		}
		if n.sections == 0 && len(n.notes) == 0 {
			continue
		}
		if n.sections > 0 {
			papers++
			written += n.written
			kept += n.kept
		}
		fmt.Printf("  %-34s %s\n", p.ID, n)
		for _, note := range n.notes {
			fmt.Printf("    %s\n", note)
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d papers split, %d files written, %d left as somebody edited them\n", papers, written, kept)
	return nil
}

// acceptEdits takes the hand corrections in a paper's content directories as
// the version of record. It writes nothing else: this is a separate pass from
// the split and not a mode of it, because a run that both re-split a paper
// and accepted the edits in it would be deciding which of the two won.
//
// Every language and not only the English. A translation gets corrected by
// hand for the same reasons the English does, and it is protected by the
// same hash and fails the same rule T03 until the correction is accepted,
// so leaving the translations out meant a person who fixed a Japanese file
// had no way to make the audit go green again. papers translate reads the
// same edited flag and skips a file that carries it, which is what keeps the
// correction from being asked for a second time and thrown away.
func acceptEdits(c *corpus.Corpus, todo []corpus.Paper, dry bool) error {
	var n int
	for _, p := range todo {
		for _, l := range corpus.Langs {
			dir := c.Content(l, p.ID)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			if dry {
				// Nothing to do here but say so. Accept reads and writes in
				// one pass and splitting it in two so that a dry run could
				// report without writing would be two ways to decide the
				// same thing.
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
			fmt.Printf("  %-34s %s, %d accepted\n", p.ID, l, len(names))
			for _, name := range names {
				fmt.Printf("    %s\n", name)
			}
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

func splitOne(c *corpus.Corpus, p corpus.Paper, rec *corpus.Source, force, prune, dry, long bool) (cut, error) {
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
	if !c.Publishes(rec.Access) {
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
		what := "is from an earlier split and this one did not produce it"
		if prune {
			what = "is from an earlier split and has been deleted"
		}
		n.notes = append(n.notes, fmt.Sprintf("%s %s", name, what))
	}
	if prune {
		dir := c.Content(corpus.EN, p.ID)
		short, err := partial(c, p.ID, rec)
		if err != nil {
			return n, err
		}
		gone := report.Stale
		if short != "" {
			// Part of the paper, so most of what is stale is the rest of it
			// and has to stay. The exception is a leftover that has taken a
			// section number this split just used, which is wrong whatever
			// else is missing. See Collided.
			gone = report.Collided()
			n.notes = append(n.notes, short)
			if len(gone) > 0 {
				n.notes = append(n.notes, fmt.Sprintf("%d of them share a section number with a file this split wrote and have been deleted anyway", len(gone)))
			}
		}
		if err := split.Prune(dir, gone); err != nil {
			return n, err
		}
	}
	return n, nil
}

// partial says why this paper must not be pruned, or the empty string if it
// may be.
//
// Pruning deletes what this run did not produce, which is the right answer
// when the run read the whole paper and the wrong one when it did not. Three
// papers in the corpus are extracted down to three pages each, left over from
// when the restricted papers were capped there, and a split of three pages of
// a thirty eight page paper produces one section. Pruning on that deleted
// nine committed sections of Gamma, eight of the Ethernet paper and the whole
// front matter of AlphaGo, in English and in Vietnamese, and every one of
// them was text an earlier and better extraction had produced. They were only
// still there because the work directory is not committed and the content is.
//
// The test is the same one audit rule S11 makes: the pages under work have to
// reach the last page the PDF has. A paper whose page count nobody recorded
// is pruned as before, because there is nothing to compare against and
// refusing on no evidence would leave every stale file in the corpus forever.
func partial(c *corpus.Corpus, id string, rec *corpus.Source) (string, error) {
	if rec == nil || rec.Pages <= 0 {
		return "", nil
	}
	numbers, err := (&extract.Store{Dir: c.Work(id, "pages")}).Pages()
	if err != nil {
		return "", err
	}
	if len(numbers) >= rec.Pages {
		return "", nil
	}
	return fmt.Sprintf("only %d of the %d pages are extracted, so nothing is pruned: a partial split would delete sections an earlier one wrote", len(numbers), rec.Pages), nil
}

// orphans deletes the translated files that have no English beside them, and
// names what it deleted.
//
// The English directory is pruned first and the three translated ones are the
// same problem seen a step later. A translator writes one file per English
// file, so a section that changed its title leaves a Vietnamese file behind
// exactly as it leaves an English one, and the Vietnamese copy is worse: it
// has the old section number in its front matter, rule T04 reads two files as
// section 17, the publish gate holds the paper, and no later run touches the
// file because there is no English to translate into it. Paxos sat in that
// state with two files numbered 02 in both languages and GPT-3 with two
// numbered 17 in Vietnamese.
//
// What it compares against is the English on disk and not the files this run
// produced, because a paper whose extracted pages have been cleaned up splits
// into nothing and still has translations to sweep. GPT-3 was that case: the
// splitter could not run on it at all and the stale Vietnamese file was
// beyond reach of the only thing that deletes one.
//
// Only alongside --prune, because this is the same decision about the same
// sections and splitting the two apart would give a corpus half cleaned.
func orphans(c *corpus.Corpus, id string) ([]string, error) {
	english, err := filepath.Glob(filepath.Join(c.Content(corpus.EN, id), "*.md"))
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, path := range english {
		want[filepath.Base(path)] = true
	}
	var out []string
	for _, l := range corpus.Langs {
		if !l.Translated() {
			continue
		}
		dir := c.Content(l, id)
		names, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			return nil, err
		}
		for _, path := range names {
			name := filepath.Base(path)
			if want[name] {
				continue
			}
			if err := os.Remove(path); err != nil {
				return nil, err
			}
			out = append(out, filepath.Join(string(l), name))
		}
	}
	sort.Strings(out)
	return out, nil
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
	// A heading a reader ran into the paragraph under it is separated here
	// rather than in the splitter, because the index has the same trouble
	// with it and had no answer for it. Fourteen papers ran References into
	// their first entry, and three of those are papers whose whole
	// bibliography the index then failed to find: it looks for the heading
	// and the heading was inside a paragraph. Rule R08 reported all three,
	// because what was in the manifest was an older extraction of the same
	// pages and no longer the text on the page.
	doc.Paragraphs = split.Unrun(doc.Paragraphs)
	// Both readers of a document go through here, so the reference list is
	// put back in order once rather than in each of them. The splitter cuts
	// the section file from this document and the index is built from the
	// same paragraphs, and moving the entries in one and not the other gives
	// an index that names text the page it points at does not have.
	refs.Reorder(doc)
	// After the join, because a listing can carry over a page break and the
	// tag belongs to the whole of it. Before the split, because the split is
	// what writes the file and a fence with no tag on it is rule C02.
	// Unmath after Label and not before, because the tag is what says
	// whether a dollar in the listing is a delimiter or a character the
	// page printed, and an untagged fence has not said yet.
	for i := range doc.Paragraphs {
		doc.Paragraphs[i].Text = code.Unmath(code.Label(doc.Paragraphs[i].Text))
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
