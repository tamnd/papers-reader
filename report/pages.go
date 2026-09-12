package report

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// A Read is one extraction path and what came through it: how many pages of
// how many papers, and what read them.
//
// It is counted off the committed English content rather than off the
// ledger, because the native path never asks a model and so never writes a
// ledger line. A report of what the corpus cost that counted only the asks
// would say nothing at all about the pages that cost nothing, and those are
// most of them and the ones whose text was never guessed.
type Read struct {
	Path   string
	Papers int
	Pages  int
	// Models is what did the reading, as the front matter records it: a
	// pdftotext version for the native path and a model name for the others.
	Models []string
}

// Paths is the three extraction paths in the order the corpus trusts them,
// which is also the order they are worth reading a table in.
var Paths = []string{"native", "layout", "ocr"}

// ReadPages counts the corpus by the path that read it.
//
// A page counts once per paper however many sections were cut from it, so a
// section boundary in the middle of a page does not turn one page into two.
func ReadPages(c *corpus.Corpus) ([]Read, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	pages := map[string]map[string]bool{}
	seen := map[string]map[string]bool{}
	models := map[string]map[string]bool{}
	for _, id := range papers.IDs() {
		dir := c.Content(corpus.EN, id)
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
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			front, _, err := corpus.ParseFront(b)
			if err != nil {
				return nil, err
			}
			path := front.Extraction
			if path == "" {
				continue
			}
			if pages[path] == nil {
				pages[path] = map[string]bool{}
				seen[path] = map[string]bool{}
				models[path] = map[string]bool{}
			}
			seen[path][id] = true
			if front.ExtractionModel != "" {
				models[path][front.ExtractionModel] = true
			}
			first, last := span(front.PDFPages)
			for p := first; p <= last && first > 0; p++ {
				pages[path][id+" "+strconv.Itoa(p)] = true
			}
		}
	}

	out := make([]Read, 0, len(pages))
	for path, set := range pages {
		r := Read{Path: path, Papers: len(seen[path]), Pages: len(set)}
		for m := range models[path] {
			r.Models = append(r.Models, m)
		}
		sort.Strings(r.Models)
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := rank(out[i].Path), rank(out[j].Path)
		if a != b {
			return a < b
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// rank orders a path, with anything unrecognised after the three that are.
func rank(path string) int {
	for i, p := range Paths {
		if p == path {
			return i
		}
	}
	return len(Paths)
}

// span reads a pdf_pages field, which is one number or two with a hyphen
// between them. Anything else is a field this cannot count, and it counts
// nothing rather than guessing.
func span(s string) (first, last int) {
	from, to, ok := strings.Cut(strings.TrimSpace(s), "-")
	a, err := strconv.Atoi(strings.TrimSpace(from))
	if err != nil || a <= 0 {
		return 0, 0
	}
	if !ok {
		return a, a
	}
	b, err := strconv.Atoi(strings.TrimSpace(to))
	if err != nil || b < a {
		return a, a
	}
	return a, b
}
