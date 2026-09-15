package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

func runSeal(args []string) error {
	fs := flag.NewFlagSet("seal", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "seal these papers only, comma separated")
	field := fs.String("field", "", "only papers in this field: "+fieldNames())
	all := fs.Bool("all", false, "seal every English file that has been edited")
	dry := fs.Bool("dry-run", false, "say what would be sealed and write nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers seal [flags]

Records a hand edit of an English file: writes edited: true into the front
matter and puts content_sha256 back in step with the body.

Audit rule T03 hashes the body of every file and compares it with the hash
the front matter carries, so a file somebody has fixed by hand fails the
audit until the hash is written again. That is the rule working. A mangled
formula or an unfenced listing is worth fixing by hand, and the corpus
still has to be able to say which files are the pipeline's work and which
are a person's.

Only English files. A translation is not edited by hand, because its front
matter carries the hash of the English it answers and an edit either breaks
that hash or lies about what produced the text. Use papers translate
-force on the paper instead.

Resealing an English file leaves its translations pointing at an older
text, which is what the run reports at the end. That is not damage: it is
the corpus saying those translations are answers to a question that has
moved, and the next papers translate run rewrites them.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("name the papers with -id, or a field with -field, or pass -all")
	}

	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}

	sealed, stale := 0, map[string][]string{}
	for _, p := range todo {
		files, err := englishFiles(c, p.ID)
		if err != nil {
			return err
		}
		for _, path := range files {
			changed, err := seal(c, path, *dry)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			sealed++
			rel := relTo(c.Root, path)
			fmt.Printf("%s: sealed\n", rel)
			answers, err := translationsOf(c, p.ID, filepath.Base(path))
			if err != nil {
				return err
			}
			stale[rel] = answers
		}
	}

	if sealed == 0 {
		fmt.Println("nothing to seal: every file hashes to what its front matter says")
		return nil
	}
	n := 0
	for _, answers := range stale {
		n += len(answers)
	}
	if n > 0 {
		fmt.Printf("%d translations now answer an older English text and are owed a run of papers translate\n", n)
	}
	if *dry {
		fmt.Println("nothing was written")
	}
	return nil
}

// seal rewrites one file if its body no longer hashes to what its front
// matter says. It reports whether it had anything to do.
func seal(c *corpus.Corpus, path string, dry bool) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		return false, fmt.Errorf("%s: %w", relTo(c.Root, path), err)
	}
	sum := corpus.ContentSHA(body)
	if front.ContentSHA256 == sum {
		return false, nil
	}
	front.ContentSHA256 = sum
	front.Edited = true
	if dry {
		return true, nil
	}
	out, err := corpus.Render(front, body)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// englishFiles is the section files of one paper, in reading order.
func englishFiles(c *corpus.Corpus, id string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(c.Content(corpus.EN, id), "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// translationsOf is the translations of one English file that now answer an
// older version of it.
func translationsOf(c *corpus.Corpus, id, name string) ([]string, error) {
	var out []string
	for _, lang := range []corpus.Lang{corpus.VI, corpus.ZH, corpus.JA} {
		path := filepath.Join(c.Content(lang, id), name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		out = append(out, relTo(c.Root, path))
	}
	return out, nil
}

// relTo is a path as a finding prints it, relative to the corpus root and
// slash separated whatever the machine uses.
func relTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return strings.ReplaceAll(rel, string(filepath.Separator), "/")
}
