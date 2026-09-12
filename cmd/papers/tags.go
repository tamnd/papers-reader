package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/tags"
)

func runTags(args []string) error {
	if len(args) == 0 {
		tagsUsage(os.Stderr)
		return fmt.Errorf("say what to do: assign or list")
	}
	switch args[0] {
	case "assign":
		return runTagsAssign(args[1:])
	case "list":
		return runTagsList(args[1:])
	case "-h", "--help", "help":
		tagsUsage(os.Stdout)
		return nil
	}
	tagsUsage(os.Stderr)
	return fmt.Errorf("there is no tags %s", args[0])
}

func tagsUsage(w *os.File) {
	fmt.Fprint(w, `usage: papers tags <assign|list> [flags]

    assign     hand out permanent identifiers to the items that have none
    list       print the register

Run papers tags assign -h for the flags.
`)
}

func runTagsAssign(args []string) error {
	fs := flag.NewFlagSet("tags assign", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "assign these papers only, comma separated")
	field := fs.String("field", "", "assign one field only")
	all := fs.Bool("all", false, "assign every paper that has content")
	dry := fs.Bool("dry-run", false, "print what would be written and write nothing")
	long := fs.Bool("v", false, "print every tag handed out")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers tags assign [flags]

Hands out a permanent identifier to every section, numbered statement,
numbered display, figure, table and listing that does not have one, and
writes it into the content file as an attribute block.

A tag is four hex characters. Tags are append only, never reused and never
edited, because a tag is what lets the four translations point at the same
paragraph and what a reference to "Theorem 2 of [7]" resolves through. An
item that already carries a tag is left exactly as it is, even when the
anchor beside it is not the one this program would choose, because renaming
a published anchor breaks every link to it.

The anchor comes from the number the paper printed and not from where the
item sits in the file, so it survives the paper being read again by a
better model. Section 3.2 is s3-2 whatever line it lands on.

Each run that hands out tags appends a line to tags/runs recording the range
it handed out. Tags climb in reading order within a run and do not climb
across runs, and audit rule G06 needs to know where one run stopped to tell a
section inserted next year from a block somebody copied and pasted. A run
that only puts known tags back into files the splitter rewrote adds no line,
because it did not hand anything out.

Nothing is written unless every paper in the run scanned cleanly, so a
failure part way through does not leave the register describing a corpus
that was only half written.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to assign: --id, --field or --all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	reg, err := tags.Load(filepath.Join(c.Tags(), "tags"))
	if err != nil {
		return err
	}
	runs, err := tags.LoadRuns(filepath.Join(c.Tags(), "runs"))
	if err != nil {
		return err
	}

	a := &assignment{corpus: c, register: reg, verbose: *long}
	for _, p := range todo {
		if err := a.paper(p.ID); err != nil {
			return fmt.Errorf("%s: %w", p.ID, err)
		}
	}
	fmt.Printf("%d files, %d items already tagged, %d tags handed out, %d put back, %d files rewritten\n",
		a.files, a.had, len(a.handed), a.wrote-len(a.handed), len(a.writes))
	if len(a.writes) == 0 {
		return nil
	}
	if *dry {
		fmt.Println("dry run, nothing written")
		return nil
	}
	for _, w := range a.writes {
		if err := os.WriteFile(w.path, w.body, 0o644); err != nil {
			return err
		}
	}
	// A run that only put tags back has nothing to add to the register and
	// nothing to add to the runs file. The register is unchanged and the range
	// of tags belongs to whichever run first handed them out.
	if len(a.handed) == 0 {
		return nil
	}
	if err := writeFile(filepath.Join(c.Tags(), "tags"), reg.Write); err != nil {
		return err
	}
	runs = append(runs, tags.Run{First: a.handed[0], Last: a.handed[len(a.handed)-1]})
	return writeFile(filepath.Join(c.Tags(), "runs"), func(w io.Writer) error {
		return tags.WriteRuns(w, runs)
	})
}

// a write is one content file, rewritten in memory. The whole run is held
// until every paper has scanned, because a register that names anchors in a
// file that was never written is a register nobody can trust.
type write struct {
	path string
	body []byte
}

type assignment struct {
	corpus   *corpus.Corpus
	register *tags.Register
	verbose  bool

	files int
	had   int
	// wrote is every tag written into a file, new ones and ones the register
	// already knew. It is more than len(handed) after a re-split, where the
	// files lost their attribute blocks and the register did not lose
	// anything, and the difference between the two is what tells a caller
	// that a run which handed out nothing still had work to do.
	wrote  int
	handed []tags.Tag
	writes []write
	// used counts the keys one paper has asked for, and is emptied between
	// papers because an anchor is only unique within the paper it names.
	used map[string]int
}

// paper walks one paper's English content in file order, which is reading
// order, and that is what makes the tags climb.
func (a *assignment) paper(id string) error {
	a.used = map[string]int{}
	dir := a.corpus.Content(corpus.EN, id)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := a.file(id, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (a *assignment) file(id, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	front, body, err := corpus.ParseFront(raw)
	if err != nil {
		return err
	}
	a.files++
	handed := 0
	// The file's own section comes first, because it is the heading every
	// heading inside the body sits under and reading order starts with it.
	if key := tags.SectionKey(front.Section, front.Kind); key != "" {
		t, err := a.tag(id, a.unique(key), "section")
		if err != nil {
			return err
		}
		if front.Tag == "" {
			front.Tag = string(t)
			handed++
		} else {
			a.had++
		}
	}
	items := tags.Scan(string(body))
	// Every key is counted, including the ones that already carry a tag,
	// because the count is what tells the second heading of a name from the
	// first and a run that skipped one would give the next one its number.
	for i := range items {
		items[i].Key = a.unique(items[i].Key)
	}
	blocks := make([]string, len(items))
	for i, it := range items {
		if it.Tag != "" {
			a.had++
			continue
		}
		t, err := a.tag(id, it.Key, it.Class)
		if err != nil {
			return err
		}
		blocks[i] = tags.Format(tags.Attr{Anchor: tags.Anchor(id, it.Key), Classes: []string{it.Class}, Tag: t})
		handed++
	}
	if handed == 0 {
		return nil
	}
	a.wrote += handed
	next, err := tags.Apply(string(body), items, blocks)
	if err != nil {
		return err
	}
	front.ContentSHA256 = corpus.ContentSHA([]byte(next))
	out, err := corpus.Render(front, []byte(next))
	if err != nil {
		return err
	}
	a.writes = append(a.writes, write{path: path, body: out})
	return nil
}

// unique is the key a paper gets for something it has asked for by that name
// before.
//
// A key comes from what the paper printed, so two things a paper printed the
// same name for want the same key. The ResNet appendix has two sections
// headed MS COCO and two headed PASCAL VOC, one pair under Object Detection
// Baselines and one under Object Detection Improvements, and both pairs came
// out sharing an anchor and therefore a tag. That is four ways wrong: the
// HTML has a repeated id, a link to one of them lands on the other, the
// register says one tag names two things, and rule G06 reads the second
// occurrence as a tag that went backwards.
//
// The first of a name keeps the bare key, so nothing already published moves.
// The rest are numbered from two in reading order, which is as permanent as
// the key itself: both come from what the paper says and in the order it says
// it.
func (a *assignment) unique(key string) string {
	a.used[key]++
	if n := a.used[key]; n > 1 {
		return fmt.Sprintf("%s-%d", key, n)
	}
	return key
}

// tag is the tag for one anchor, handing out a new one if the register does
// not have it already.
//
// An anchor the register already knows gets the tag it already has. That is
// what a re-split looks like: the file was rewritten and lost its attribute
// blocks, and the thing the anchor names has not changed, so handing out a
// second tag for it would retire a tag that is still in use.
func (a *assignment) tag(paper, key, class string) (tags.Tag, error) {
	anchor := tags.Anchor(paper, key)
	if t, ok := a.register.Tag(anchor); ok {
		return t, nil
	}
	t, err := a.register.Next()
	if err != nil {
		return "", err
	}
	if err := a.register.Add(t, anchor); err != nil {
		return "", err
	}
	a.handed = append(a.handed, t)
	if a.verbose {
		fmt.Printf("  %s  %-10s %s\n", t, class, anchor)
	}
	return t, nil
}

func runTagsList(args []string) error {
	fs := flag.NewFlagSet("tags list", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers tags list [flags]

Prints the register, lowest tag first, and then the runs.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	reg, err := tags.Load(filepath.Join(c.Tags(), "tags"))
	if err != nil {
		return err
	}
	if err := reg.Write(os.Stdout); err != nil {
		return err
	}
	runs, err := tags.LoadRuns(filepath.Join(c.Tags(), "runs"))
	if err != nil {
		return err
	}
	fmt.Printf("\n%d tags in %d runs\n", reg.Len(), len(runs))
	return nil
}

// writeFile creates the file's directory and hands the file to a writer. The
// tags directory does not exist in a corpus that has never been assigned.
func writeFile(path string, fn func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
