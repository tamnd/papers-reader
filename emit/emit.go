package emit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// A Site is everything a build of the corpus produces, held in memory.
//
// Built whole and then written, rather than written as it is built. The
// audit validates a build without writing anything, and a build that failed
// halfway would otherwise leave a site directory that is half of one corpus
// and half of the last, which is the kind of thing that is only noticed in a
// browser a week later.
type Site struct {
	Index *Index
	Graph *Graph
	Pages []*Page
	// Search is one index per language the corpus has pages in.
	Search []*Search
	// Figures is every picture a page refers to, as a path relative to the
	// root of both the corpus and the site, which are deliberately the same
	// path. They are copied rather than rendered, so they are not in Files
	// below: a build writes JSON and copies pictures.
	Figures []string
	// Faults is everything the pages refer to that is not there. It is not
	// written anywhere. Rules P01, P02 and P03 read it, and the command
	// prints a count of it, and that is all it is for.
	Faults []Fault

	// root is the corpus the figures are copied from.
	root string
}

// Build reads the corpus and builds the site.
func Build(c *corpus.Corpus) (*Site, error) {
	ix, err := BuildIndex(c)
	if err != nil {
		return nil, err
	}
	g, err := BuildGraph(c)
	if err != nil {
		return nil, err
	}
	pages, faults, err := BuildPages(c)
	if err != nil {
		return nil, err
	}
	s := &Site{Index: ix, Graph: g, Pages: pages, Faults: faults, root: c.Root}
	for _, p := range pages {
		s.Faults = append(s.Faults, CheckPage(p)...)
	}
	s.Figures = usedFigures(pages)
	s.Faults = append(s.Faults, absentFigures(c.Root, pages)...)
	s.Search = BuildSearch(pages)
	return s, nil
}

// absentFigures is every picture a page refers to that is not on the disk.
//
// The manifest saying a figure exists and the file being there are two
// different claims and the audit wants both. A page that refers to a picture
// papers figures never rendered would render as a broken image, which is the
// one thing a build should not be able to ship, and rule P03 is this list.
func absentFigures(root string, pages []*Page) []Fault {
	var out []Fault
	there := map[string]bool{}
	for _, p := range pages {
		at := PagePath(p.ID, p.Lang)
		for _, b := range append(blocksOf(p), p.Front.Blocks...) {
			if b.Src == "" {
				continue
			}
			ok, seen := there[b.Src]
			if !seen {
				_, err := os.Stat(filepath.Join(root, filepath.FromSlash(b.Src)))
				ok = err == nil
				there[b.Src] = ok
			}
			if !ok {
				out = append(out, Fault{Page: at, Kind: FaultFigure, What: b.Src + " is not in the corpus"})
			}
		}
	}
	return out
}

// usedFigures is every picture the pages refer to, once each and in path
// order. A picture two languages of one paper both show is one file to copy.
func usedFigures(pages []*Page) []string {
	seen := map[string]bool{}
	for _, p := range pages {
		for _, b := range append(blocksOf(p), p.Front.Blocks...) {
			if b.Src != "" {
				seen[b.Src] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for src := range seen {
		out = append(out, src)
	}
	sort.Strings(out)
	return out
}

// blocksOf is every block of every section of a page, which several things
// want and none of them want to write twice.
func blocksOf(p *Page) []Block {
	var out []Block
	for _, s := range p.Sections {
		out = append(out, s.Blocks...)
	}
	return out
}

// Files is the site as paths and bodies, in no particular order.
//
// The JSON is indented with two spaces and ends in a newline. It is served
// gzipped, where the indentation costs almost nothing, and the difference it
// makes is that a diff of two builds is readable and a person can open the
// file and find out what the app is being told.
func (s *Site) Files() (map[string][]byte, error) {
	out := map[string][]byte{}
	write := func(name string, v any) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		out[name] = append(b, '\n')
		return nil
	}
	if err := write("index.json", s.Index); err != nil {
		return nil, err
	}
	if err := write("graph.json", s.Graph); err != nil {
		return nil, err
	}
	for _, p := range s.Pages {
		if err := write(PagePath(p.ID, p.Lang), p); err != nil {
			return nil, err
		}
	}
	// The search index is the one file here that nobody reads, so it is
	// written compactly rather than indented. It is also the largest, and
	// two spaces in front of every posting is a megabyte of nothing.
	for _, ix := range s.Search {
		b, err := json.Marshal(ix)
		if err != nil {
			return nil, err
		}
		out[SearchPath(ix.Lang)] = append(b, '\n')
	}
	return out, nil
}

// Write puts the site in a directory, making it if it is not there.
func (s *Site) Write(dir string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	for _, name := range SortedNames(files) {
		if err := put(filepath.Join(dir, filepath.FromSlash(name)), files[name]); err != nil {
			return err
		}
	}
	for _, src := range s.Figures {
		from := filepath.Join(s.root, filepath.FromSlash(src))
		if err := copyFile(from, filepath.Join(dir, filepath.FromSlash(src))); err != nil {
			return err
		}
	}
	return nil
}

// SortedNames is the paths of a build in order, which is the order anything
// walking a build should walk it in: a run that printed its files in map
// order would print them differently every time.
func SortedNames(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func put(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

// copyFile copies a picture into the build.
//
// Copied and not linked. A build is handed to a deploy that tars it or
// uploads it from somewhere else on the machine, and a tree of hard links
// into the corpus is a tree that means something different depending on how
// it is read.
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Summary is the one line a run prints.
func (s *Site) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "version %d, %d papers, %d fields, %d reading lists, %d edges, %d pages, %d figures",
		s.Index.Version, len(s.Index.Papers), len(s.Index.Fields),
		len(s.Index.Collections), len(s.Graph.Edges), len(s.Pages), len(s.Figures))
	terms := 0
	for _, ix := range s.Search {
		terms += len(ix.Terms) + len(ix.Symbols)
	}
	fmt.Fprintf(&b, ", %d search terms", terms)
	switch n := len(s.Faults); {
	case n == 1:
		b.WriteString(", 1 fault")
	case n > 1:
		fmt.Fprintf(&b, ", %d faults", n)
	}
	return b.String()
}
