package audit

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// A File is one committed Markdown file, read and parsed once.
//
// Two groups of rules read every file in the corpus and a third is coming, so
// reading them once here is the difference between one pass over the content
// and twenty-three. It also means the rules agree about what the body is: a
// rule that did its own splitting would eventually disagree with its
// neighbour about where the front matter ended, and the two findings would
// point at the same line and contradict each other.
type File struct {
	// Path is relative to the corpus root and is slash separated, because it
	// is what a finding prints and a finding is read on a web page.
	Path  string
	Paper string
	Lang  corpus.Lang
	// Name is the base name, 00_front.md and the like.
	Name string
	// Ordinal is the number the file name opens with, and -1 for a name that
	// does not open with one.
	Ordinal int
	Front   corpus.Front
	Body    string
	// Err is set when the file does not split into front matter and a body.
	// It is not returned from Load, because one unparseable file is rule T01's
	// finding rather than a reason to abandon the audit. Front and Body are
	// empty when it is set, and every other rule skips the file.
	Err error
	// Strict is what a decoder that refuses unknown fields made of the same
	// front matter, and is rule T02's finding. It is kept apart from Err
	// because a file with one field misspelled is still a file every other
	// rule should look at.
	Strict error
}

// Broken reports whether the file could not be read as a content file.
func (f *File) Broken() bool { return f.Err != nil }

// loadContent reads every content file the corpus has, in path order.
func loadContent(c *corpus.Corpus, papers *corpus.Papers) ([]*File, error) {
	var out []*File
	for _, p := range papers.Papers {
		for _, lang := range corpus.Langs {
			dir := c.Content(lang, p.ID)
			entries, err := os.ReadDir(dir)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				f, err := readFile(c.Root, lang, p.ID, e.Name())
				if err != nil {
					return nil, err
				}
				out = append(out, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func readFile(root string, lang corpus.Lang, id, name string) (*File, error) {
	rel := filepath.ToSlash(filepath.Join("content", string(lang), id, name))
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	f := &File{Path: rel, Paper: id, Lang: lang, Name: name, Ordinal: ordinalOf(name)}
	front, body, err := corpus.ParseFront(b)
	if err != nil {
		f.Err = err
		return f, nil
	}
	f.Front, f.Body = front, string(body)
	// The strict parse is separate from the one above, so that a file with an
	// unknown field still reads for every other rule. A misspelled field is
	// one rule's finding and not a reason to stop looking at the paper.
	f.Strict = corpus.StrictFront(b)
	return f, nil
}

// ordinalOf reads the number a content file name opens with. The names are
// written by papers split and nothing else writes them, so a name without one
// is a file somebody added by hand and rule T04 says so.
func ordinalOf(name string) int {
	cut := strings.IndexByte(name, '_')
	if cut <= 0 {
		return -1
	}
	n, err := strconv.Atoi(name[:cut])
	if err != nil {
		return -1
	}
	return n
}

// byPaper groups the files of one language, which is the unit most of group T
// works in: a section number is contiguous within one paper in one language
// and says nothing about the same paper in another.
func byPaper(files []*File) map[string][]*File {
	out := map[string][]*File{}
	for _, f := range files {
		key := f.Paper + "/" + string(f.Lang)
		out[key] = append(out[key], f)
	}
	for _, group := range out {
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })
	}
	return out
}

// keys is the group keys in a fixed order, so that a run over the same corpus
// reports the same findings in the same order twice.
func keys(m map[string][]*File) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
