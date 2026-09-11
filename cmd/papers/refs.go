package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/refs"
)

func runRefs(args []string) error {
	if len(args) == 0 {
		refsUsage(os.Stderr)
		return fmt.Errorf("say what to do: build or resolve")
	}
	switch args[0] {
	case "build":
		return runRefsBuild(args[1:])
	case "resolve":
		return runRefsResolve(args[1:])
	case "-h", "--help", "help":
		refsUsage(os.Stdout)
		return nil
	}
	refsUsage(os.Stderr)
	return fmt.Errorf("there is no refs %s", args[0])
}

func refsUsage(w *os.File) {
	fmt.Fprint(w, `usage: papers refs <build|resolve> [flags]

    build      parse the bibliography of a paper and link what it can
    resolve    link the bibliographies already parsed, without re-parsing

Run papers refs build -h for the flags.
`)
}

func runRefsBuild(args []string) error {
	fs := flag.NewFlagSet("refs build", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "build these papers only, comma separated")
	field := fs.String("field", "", "build one field only")
	all := fs.Bool("all", false, "build every paper that has extracted pages")
	dry := fs.Bool("dry-run", false, "print what would be written and write nothing")
	long := fs.Bool("v", false, "print every entry that resolved")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers refs build [flags]

Parses the bibliography of a paper into manifests/refs/<id>.yaml and links
the entries that name papers the corpus already has.

The reference list is found by its heading and the label style is worked out
once per paper and then required, so a continuation line that happens to
start with a year is not read as the start of entry 1996. Each entry keeps
its raw text, always, and the structured fields are a parse that is only
used for resolution. The reference section on the page renders raw, so a
reference the parser read badly still reads correctly.

Linking runs against papers.yaml and never against a service, so it is
offline and deterministic and re-runs on every build. A paper added today
retroactively resolves references in papers extracted last month.

A restricted paper gets no manifest. Its bibliography is part of the paper
and the corpus may publish its front matter and an abstract, nothing else.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to build: --id, --field or --all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return err
	}
	recorded, err := c.LoadSources()
	if err != nil {
		return err
	}

	var papers, entries, linked int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		if rec == nil || !rec.Access.Body() {
			continue
		}
		d, err := document(c, p.ID)
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			continue
		}
		if d == nil {
			continue
		}
		section := refs.Bibliography(d)
		if len(section) == 0 {
			fmt.Printf("  %-34s no reference section found\n", p.ID)
			continue
		}
		r := refs.Parse(section)
		if len(r.Entries) == 0 {
			fmt.Printf("  %-34s the reference section parsed into no entries\n", p.ID)
			continue
		}
		n := refs.Resolve(r.Entries, manifest.Papers, p.ID)
		m := refs.NewManifest(p.ID, r)
		papers++
		entries += len(r.Entries)
		linked += n
		fmt.Printf("  %-34s %s, %d entries, %d into the corpus\n", p.ID, r.Style, len(r.Entries), n)
		for _, note := range r.Notes {
			fmt.Printf("    %s\n", note)
		}
		if *long {
			printLinks(m)
		}
		if *dry {
			continue
		}
		if err := m.Save(c.Refs(p.ID)); err != nil {
			return err
		}
	}
	if *dry {
		fmt.Println("dry run, nothing written")
	}
	fmt.Printf("%d bibliographies, %d entries, %d resolved into the corpus\n", papers, entries, linked)
	return nil
}

func runRefsResolve(args []string) error {
	fs := flag.NewFlagSet("refs resolve", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "resolve these papers only, comma separated")
	field := fs.String("field", "", "resolve one field only")
	all := fs.Bool("all", false, "resolve every bibliography already parsed")
	long := fs.Bool("v", false, "print every entry that resolved")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers refs resolve [flags]

Re-links the bibliographies that have already been parsed, and writes the
manifests back.

This is what to run after adding a paper. Resolution is offline and cheap,
the PDFs do not have to be on the disk for it, and a paper added today
resolves references in papers extracted months ago.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to resolve: --id, --field or --all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return err
	}

	var papers, linked, changed int
	for _, p := range todo {
		m, err := refs.Load(c.Refs(p.ID))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		before := m.Resolved()
		n := refs.Resolve(m.Entries, manifest.Papers, p.ID)
		papers++
		linked += n
		if n != before {
			changed++
			fmt.Printf("  %-34s %d into the corpus, was %d\n", p.ID, n, before)
		}
		if *long {
			printLinks(m)
		}
		if err := m.Save(c.Refs(p.ID)); err != nil {
			return err
		}
	}
	fmt.Printf("%d bibliographies, %d resolved into the corpus, %d changed\n", papers, linked, changed)
	return nil
}

func printLinks(m *refs.Manifest) {
	for _, e := range m.Entries {
		if e.ResolvesTo != "" {
			fmt.Printf("    [%s] %s\n", e.Key, e.ResolvesTo)
		}
	}
}

// loadRefs reads one paper's bibliography if it has been parsed. A paper
// with no manifest is not an error: refs build and split are separate
// passes on purpose, and a corpus half way through the first one should
// still split.
func loadRefs(c *corpus.Corpus, id string) *refs.Manifest {
	m, err := refs.Load(c.Refs(id))
	if err != nil {
		return nil
	}
	return m
}
