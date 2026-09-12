package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
)

func runGlossary(args []string) error {
	if len(args) == 0 {
		glossaryUsage(os.Stderr)
		return fmt.Errorf("say what to do: extract or show")
	}
	switch args[0] {
	case "extract":
		return runGlossaryExtract(args[1:])
	case "show":
		return runGlossaryShow(args[1:])
	case "-h", "--help", "help":
		glossaryUsage(os.Stdout)
		return nil
	case "translate":
		return fmt.Errorf("glossary translate arrives with the translate command")
	}
	glossaryUsage(os.Stderr)
	return fmt.Errorf("there is no glossary %s", args[0])
}

func glossaryUsage(w *os.File) {
	fmt.Fprint(w, `usage: papers glossary <extract|show> [flags]

    extract    propose terms for the glossary from the English corpus
    show       print the glossary and what it covers
    translate  ask for the renderings of the approved terms (arrives with translate)

Run papers glossary extract -h for the flags.
`)
}

func runGlossaryExtract(args []string) error {
	fs := flag.NewFlagSet("glossary extract", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	least := fs.Int("papers", glossary.Defaults.Papers, "the least number of papers a term must appear in")
	top := fs.Int("top", glossary.Defaults.Top, "how many candidates to keep")
	words := fs.Int("words", glossary.Defaults.Words, "the longest phrase to propose, in words")
	field := fs.String("field", "", "read one field of the corpus only")
	write := fs.Bool("write", false, "write manifests/glossary-candidates.yaml as well as printing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers glossary extract [flags]

Proposes terms for the controlled vocabulary by counting the English
corpus, and writes manifests/glossary-candidates.yaml.

It counts papers and not occurrences. A phrase used forty times in one
paper is that paper's own notation and belongs in a note about that paper.
A phrase used three times each in nine papers is the vocabulary of the
field, and translating it two ways is what makes a corpus read as several
corpora.

What it counts is the prose. The mathematics, the listings, the citation
markers and the tag attributes come out first, because a corpus counted
with them in proposes a glossary of variable names.

The output is a proposal and not a glossary. Somebody reads it, keeps what
is really vocabulary, scopes what means different things in different
fields, and moves those into manifests/glossary.yaml.

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
	papers, err := c.LoadPapers()
	if err != nil {
		return err
	}
	have, err := glossary.Load(c.GlossaryManifest())
	if err != nil {
		return err
	}

	var texts []glossary.Text
	for _, p := range papers.Papers {
		if *field != "" && string(p.Field) != *field {
			continue
		}
		body, ok, err := english(c, p.ID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		texts = append(texts, glossary.Text{ID: p.ID, Field: p.Field, Body: body})
	}
	if len(texts) == 0 {
		return fmt.Errorf("no English content to count: extract some papers first")
	}

	cs := glossary.Extract(texts, have, glossary.Options{Papers: *least, Top: *top, Words: *words})
	fmt.Printf("%d papers read, %d candidates in %d or more of them\n",
		len(texts), len(cs.Candidates), *least)
	for i, cd := range cs.Candidates {
		if i >= 20 {
			fmt.Printf("... and %d more\n", len(cs.Candidates)-i)
			break
		}
		where := ""
		if len(cd.Fields) > 0 {
			where = "  " + join(cd.Fields)
		}
		fmt.Printf("%3d papers %5d uses  %s%s\n", cd.Papers, cd.Uses, cd.En, where)
	}
	if !*write {
		return nil
	}
	b, err := yaml.Marshal(cs)
	if err != nil {
		return err
	}
	out := filepath.Join(c.Manifests(), "glossary-candidates.yaml")
	head := "# Proposed terms for the controlled vocabulary, counted off the English corpus.\n" +
		"#\n" +
		"# This is a proposal and not a glossary. Keep what is really vocabulary, scope what\n" +
		"# means different things in different fields, and move those into glossary.yaml.\n" +
		"# Written by `papers glossary extract`, and safe to delete and rebuild.\n\n"
	if err := os.WriteFile(out, append([]byte(head), b...), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func runGlossaryShow(args []string) error {
	fs := flag.NewFlagSet("glossary show", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	missing := fs.Bool("missing", false, "list the terms with no rendering yet")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers glossary show [flags]

Prints the controlled vocabulary: its version, how many terms it holds, and
how many of them have a rendering in each language.

Coverage is what the translate command gates on. A body translated against
a third of a glossary is a body that will have to be translated again.

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
	g, err := glossary.Load(c.GlossaryManifest())
	if err != nil {
		return err
	}
	fmt.Printf("version %d, %d terms\n", g.Version, len(g.Terms))
	for _, l := range corpus.Langs {
		if l == corpus.EN {
			continue
		}
		have, total := g.Coverage(l)
		pct := 0
		if total > 0 {
			pct = have * 100 / total
		}
		fmt.Printf("  %s %d of %d (%d%%)\n", l, have, total, pct)
		if !*missing {
			continue
		}
		for _, t := range g.Missing(l) {
			fmt.Printf("      %s\n", t.En)
		}
	}
	return nil
}

// english is everything the corpus publishes of one paper in English, with
// the front matter of each section taken off, joined into one text.
func english(c *corpus.Corpus, id string) (string, bool, error) {
	dir := c.Content(corpus.EN, id)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var parts []string
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", false, err
		}
		front, body, err := corpus.ParseFront(b)
		if err != nil {
			return "", false, err
		}
		// The bibliography is not prose. It is a list of names, titles and
		// venues, and counting it proposed a glossary of "university",
		// "computer", "research" and "proceedings" before anything a
		// translator would ever look up: 36 papers and 64 uses of
		// "computer" against 19 papers and 248 of "model". Rule L14 says a
		// bibliography entry stands as printed, so none of it needs a
		// rendering either.
		//
		// The front file stays, affiliations and all. Most of this corpus is
		// three pages of a paper rather than all of it, and dropping the
		// front file took the count from 67 papers to 9.
		if front.Kind == "references" {
			continue
		}
		parts = append(parts, string(body))
	}
	if len(parts) == 0 {
		return "", false, nil
	}
	return strings.Join(parts, "\n\n"), true, nil
}

func join(fs []corpus.Field) string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = string(f)
	}
	return strings.Join(out, ", ")
}
