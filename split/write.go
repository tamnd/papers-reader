package split

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// A File is one content file, ready to be written.
type File struct {
	Name  string
	Front corpus.Front
	Body  []byte
}

// Bytes is the file as it goes on disk.
func (f File) Bytes() ([]byte, error) { return corpus.Render(f.Front, f.Body) }

// Files turns a split into the content files of one paper.
//
// The base front matter is everything that is true of the whole paper: what it
// is, where the PDF came from, and which path read it. This fills in what is
// true of one section and nothing else.
func Files(base corpus.Front, r *Result) []File {
	out := make([]File, 0, len(r.Sections))
	for _, s := range r.Sections {
		f := base
		f.Section = s.Number
		f.SectionTitle = s.Title
		f.Kind = s.Kind
		f.PDFPages = pages(s.First, s.Last)
		body := []byte(s.Body)
		if len(body) > 0 && body[len(body)-1] != '\n' {
			body = append(body, '\n')
		}
		f.ContentSHA256 = corpus.ContentSHA(body)
		out = append(out, File{Name: s.Filename(), Front: f, Body: body})
	}
	return out
}

// pages renders the pdf_pages field: "4" for a section on one page, "3-6" for
// one that runs across four.
func pages(first, last int) string {
	switch {
	case first == 0:
		return ""
	case last <= first:
		return strconv.Itoa(first)
	}
	return strconv.Itoa(first) + "-" + strconv.Itoa(last)
}

// A Report is what a write did.
type Report struct {
	Created   []string
	Updated   []string
	Unchanged []string
	// Kept are the files that were left alone because somebody had edited
	// them. This is the point of the whole exercise: an extraction run that
	// silently overwrote a hand correction would make hand correcting the
	// corpus pointless, and the next run would do it again.
	Kept []string
	// Stale are files in the directory that this split did not produce. They
	// are reported and not deleted, because the usual reason for one is that
	// the section boundaries moved, and a person should see that happen
	// rather than find out from a diff of four hundred lines.
	Stale []string
}

// Write puts the files in dir, protecting whatever a person has edited.
//
// A file is protected by comparing the content_sha256 in its own front matter
// against the body next to it. They agree for a file that has not been touched
// since the toolchain wrote it, and they disagree the moment anybody edits the
// body, which is the whole signal and it needs no state kept anywhere else.
//
// force writes anyway. It exists because the alternative to having it is
// somebody deleting the directory, which loses the same work with none of the
// warning.
func Write(dir string, files []File, force bool) (*Report, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	r := &Report{}
	want := map[string]bool{}
	for _, f := range files {
		want[f.Name] = true
		path := filepath.Join(dir, f.Name)
		b, err := f.Bytes()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		old, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			if err := write(path, b); err != nil {
				return nil, err
			}
			r.Created = append(r.Created, f.Name)
			continue
		case err != nil:
			return nil, err
		}
		if bytes.Equal(old, b) {
			r.Unchanged = append(r.Unchanged, f.Name)
			continue
		}
		if edited(old) && !force {
			r.Kept = append(r.Kept, f.Name)
			continue
		}
		if err := write(path, b); err != nil {
			return nil, err
		}
		r.Updated = append(r.Updated, f.Name)
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, p := range names {
		if name := filepath.Base(p); !want[name] {
			r.Stale = append(r.Stale, name)
		}
	}
	sort.Strings(r.Stale)
	return r, nil
}

// edited says whether a file on disk has been changed since it was written.
// A file whose front matter does not parse, or which records no hash, is
// treated as edited: those are the two ways a file gets there other than from
// this code, and both of them are somebody's work.
func edited(b []byte) bool {
	f, body, err := corpus.ParseFront(b)
	if err != nil || f.ContentSHA256 == "" {
		return true
	}
	return corpus.ContentSHA(body) != f.ContentSHA256
}

// write replaces a file in one step, so that an interrupted run leaves the
// old file rather than half of the new one.
func write(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".split-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AbstractWords is the most of a restricted paper that may be published: the
// front matter and an abstract. It is not a licence anybody granted, it is
// the length at which quoting a paper to say what it is about stops being
// quoting it, and the number is deliberately on the short side of what the
// abstracts in the corpus actually run to.
const AbstractWords = 250

// Abstract cuts a front matter block down to what a restricted paper may
// publish.
//
// The cut is at a sentence end, because an abstract that stops mid clause
// reads as a file that got truncated and somebody will come looking for the
// bug. A block already short enough comes back exactly as it went in.
func Abstract(body string, words int) string {
	fields := strings.Fields(body)
	if len(fields) <= words {
		return strings.TrimRight(body, "\n")
	}
	cut := strings.Join(fields[:words], " ")
	if i := strings.LastIndexAny(cut, ".!?"); i > 0 {
		return cut[:i+1]
	}
	return cut
}
