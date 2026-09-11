package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A Store is the page files of one paper, under work/<id>/pages.
//
// One file per page, named for the page of the PDF and zero padded so that
// ls and a glob both put them in order. This is what makes extraction
// resumable, and resumable is not a nicety here: a hundred papers at a
// hundred and fifty seconds a page is days of wall clock, spread over
// machines that get rebooted, and a run that started again from page one
// every time would never finish.
//
// The page file holds the text of one page and nothing else. No front
// matter, no page number heading, no marker: the assembler needs to join
// these across page breaks and every line it has to skip is a line it can
// get wrong.
type Store struct {
	Dir string
}

// Path is the file one page is written to.
func (s Store) Path(page int) string {
	return filepath.Join(s.Dir, fmt.Sprintf("%04d.txt", page))
}

// Has reports whether a page has been extracted. An empty file counts as
// extracted: a blank verso is a real answer and asking for it again on every
// run would mean asking for it forever.
func (s Store) Has(page int) bool {
	_, err := os.Stat(s.Path(page))
	return err == nil
}

// Read is the text of one page.
func (s Store) Read(page int) (string, error) {
	b, err := os.ReadFile(s.Path(page))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Write stores one page.
//
// Written to a temporary file and renamed, because the alternative is a
// half written page file left behind by a machine that was rebooted mid run,
// and the next run would see that the page is there and never look at it
// again. Rename within a directory is atomic on every filesystem this runs
// on.
func (s Store) Write(page int, text string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	tmp, err := os.CreateTemp(s.Dir, ".page-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path(page))
}

// Pages is every page that has been extracted, in order.
func (s Store) Pages() ([]int, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".txt") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".txt"))
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

// Missing is the pages of a range that are not extracted yet, which is the
// work a run has left to do.
func (s Store) Missing(first, last int) []int {
	var out []int
	for p := first; p <= last; p++ {
		if !s.Has(p) {
			out = append(out, p)
		}
	}
	return out
}
