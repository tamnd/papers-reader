package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/papers-reader/book"
	"github.com/tamnd/papers-reader/corpus"
)

func runBook(args []string) error {
	fs := flag.NewFlagSet("book", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	id := fs.String("id", "", "the paper to set")
	langs := fs.String("lang", "en", "languages to set, comma separated, or all")
	out := fs.String("out", "books", "where to put what is built")
	formats := fs.String("format", "pdf,epub", "what to build: tex, pdf, epub, or all")
	font := fs.String("font", "", "the main text face, by file name or by family name")
	cjk := fs.String("cjk-font", "", "the face the Chinese and the Japanese are set in")
	paper := fs.String("paper", "a4paper", "the page size, as geometry names it")
	size := fs.String("size", "11pt", "the body size")
	contents := fs.Bool("contents", true, "put a table of contents in a paper long enough to want one")
	keep := fs.Bool("keep", false, "keep the build directory even when the build works")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers book -id <paper> [-lang en,vi,zh,ja] [flags]

Sets one paper of the corpus as a document: the LaTeX, the PDF that tectonic
makes of it, and an EPUB with the mathematics rendered by the same KaTeX the
reading app uses.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		fs.Usage()
		return fmt.Errorf("say which paper with -id")
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	want, err := bookLangs(*langs)
	if err != nil {
		return err
	}
	tex, pdf, epub, err := wanted(*formats)
	if err != nil {
		return err
	}

	dir := *out
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(c.Root, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	ctx := context.Background()
	for _, l := range want {
		b, err := book.Load(c, *id, l)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("%s %s: nothing to set\n", *id, l)
				continue
			}
			return err
		}
		o := book.Options{Font: *font, CJKFont: *cjk, Paper: *paper, Size: *size, Contents: *contents}
		if o.CJKFont == "" {
			o.CJKFont = book.CJKFont(l)
		}
		if err := set(ctx, b, o, dir, *id, l, tex, pdf, epub, *keep); err != nil {
			return err
		}
	}
	return nil
}

// set builds one paper in one language, and says what it found on the way.
func set(ctx context.Context, b *book.Book, o book.Options, dir, id string, l corpus.Lang, tex, pdf, epub, keep bool) error {
	name := id + "." + string(l)
	fmt.Printf("%s: %d sections, %d references, %d figures, %d footnotes\n",
		name, len(b.Sections), len(b.Bibliography), len(b.Figures), len(b.Notes))

	if tex || pdf {
		source, r, err := book.Document(b, o)
		if err != nil {
			return err
		}
		say(name, r.Missing, "figures with no picture")
		say(name, r.Orphans, "footnote markers with no definition")
		say(name, r.Wide, "tables set small to fit the page")
		say(name, r.Unlinked, "corpus citations this bibliography does not list")
		if tex {
			path := filepath.Join(dir, name+".tex")
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				return err
			}
			fmt.Printf("%s: wrote %s\n", name, rel(path))
		}
		if pdf {
			if err := typeset(ctx, source, dir, name, keep); err != nil {
				return err
			}
		}
	}
	if epub {
		path := filepath.Join(dir, name+".epub")
		p, err := book.EPUB(b, path)
		if err != nil {
			return err
		}
		say(name, p.Refused, "formulas KaTeX would not read")
		say(name, p.Missing, "figures with no picture")
		say(name, p.Orphans, "footnote markers with no definition")
		say(name, p.Unlinked, "corpus citations this bibliography does not list")
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		fmt.Printf("%s: wrote %s, %d chapters, %d KB\n", name, rel(path), p.Chapters, info.Size()/1024)
	}
	return nil
}

// typeset runs tectonic in a directory beside the output and moves the PDF
// out of it.
//
// The directory is kept when the build fails, because the log in it is the
// only way to find out why, and thrown away when it works, because it is
// thirty megabytes of intermediates nobody reads.
func typeset(ctx context.Context, source, dir, name string, keep bool) error {
	work, err := os.MkdirTemp(dir, "."+name+".build-")
	if err != nil {
		return err
	}
	report, err := book.Build(ctx, work, source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: the build directory is %s\n", name, rel(work))
		return err
	}
	path := filepath.Join(dir, name+".pdf")
	if err := move(report.PDF, path); err != nil {
		return err
	}
	if keep {
		fmt.Printf("%s: the build directory is %s\n", name, rel(work))
	} else if err := os.RemoveAll(work); err != nil {
		return err
	}
	fmt.Printf("%s: wrote %s, %d pages, %d overfull and %d underfull boxes\n",
		name, rel(path), report.Pages, report.Overfull, report.Underfull)
	say(name, report.Errors, "errors the typesetter carried on past")
	say(name, report.Undefined, "references that pointed at nothing")
	say(name, report.Missing, "characters the fonts could not set")
	return nil
}

// move renames a file, and copies it when the rename crosses a device.
func move(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

// say prints a complaint, with the first few of whatever it is about.
//
// The first few and not all of them. A paper with forty unreadable formulas
// has one problem, not forty, and a terminal full of them hides the line
// above that says which paper it was.
func say(name string, list []string, what string) {
	if len(list) == 0 {
		return
	}
	const show = 5
	head := list
	more := ""
	if len(head) > show {
		head, more = head[:show], fmt.Sprintf(" and %d more", len(list)-show)
	}
	fmt.Fprintf(os.Stderr, "%s: %d %s: %s%s\n", name, len(list), what, strings.Join(head, "; "), more)
}

// bookLangs reads the -lang flag.
//
// It is not the languages helper beside it in glossary.go, which refuses
// English because nothing is translated into English. A book of the English
// is the commonest book there is.
func bookLangs(s string) ([]corpus.Lang, error) {
	if strings.TrimSpace(s) == "all" {
		return corpus.Langs, nil
	}
	var out []corpus.Lang
	for _, part := range commas(s) {
		l := corpus.Lang(part)
		if !l.Valid() {
			return nil, fmt.Errorf("%q is not a language of this corpus", part)
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("say which language with -lang")
	}
	return out, nil
}

// wanted reads the -format flag.
func wanted(s string) (tex, pdf, epub bool, err error) {
	if strings.TrimSpace(s) == "all" {
		return true, true, true, nil
	}
	for _, part := range commas(s) {
		switch part {
		case "tex", "latex":
			tex = true
		case "pdf":
			pdf = true
		case "epub":
			epub = true
		default:
			return false, false, false, fmt.Errorf("%q is not a format: tex, pdf or epub", part)
		}
	}
	if !tex && !pdf && !epub {
		return false, false, false, fmt.Errorf("say what to build with -format")
	}
	return tex, pdf, epub, nil
}

// rel shortens a path against the working directory, for a line somebody has
// to read. An absolute path under a home directory is noise, and a path that
// cannot be shortened is printed as it is.
func rel(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	short, err := filepath.Rel(wd, path)
	if err != nil || strings.HasPrefix(short, "../..") {
		return path
	}
	return short
}
