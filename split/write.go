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
// Once the correction has been through Accept the file says edited: true in
// its front matter and the hash agrees again, and this honours that instead.
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

// edited says whether a file on disk holds somebody's work.
//
// Two ways to be. An accepted correction says so in its front matter, which
// is the tidy case and the one that passes audit rule T03. An unaccepted one
// is a body that no longer hashes to the hash recorded beside it, which is
// the signal a person leaves without meaning to, and it has to keep working:
// somebody who edits a file and runs the splitter should not lose the work by
// not having read the manual first.
//
// A file whose front matter does not parse, or which records no hash, is
// treated as edited too. Those are the two ways a file gets there other than
// from this code, and both of them are somebody's work.
func edited(b []byte) bool {
	f, body, err := corpus.ParseFront(b)
	if err != nil || f.ContentSHA256 == "" {
		return true
	}
	return f.Edited || corpus.ContentSHA(body) != f.ContentSHA256
}

// Accept restamps the files in dir that somebody has corrected by hand, so
// that the correction is the version of record rather than a hash that does
// not match.
//
// It is the other half of the protection in Write. An edit is protected the
// moment it is made, and it stays a hard T03 failure until it is accepted,
// which is the right way round: the audit should be red while a correction is
// half done and green when a person has said they meant it.
//
// Only files that are already changed. A file whose body still hashes to its
// own hash is a file nobody has touched, and marking one of those as edited
// would put a fence round a file the splitter should keep updating.
func Accept(dir string) ([]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, path := range names {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, body, err := corpus.ParseFront(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		sum := corpus.ContentSHA(body)
		if f.ContentSHA256 == sum {
			continue
		}
		f.Edited = true
		f.ContentSHA256 = sum
		next, err := corpus.Render(f, body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		if err := write(path, next); err != nil {
			return nil, err
		}
		out = append(out, filepath.Base(path))
	}
	sort.Strings(out)
	return out, nil
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
//
// It counts a paragraph at a time and not a word at a time, so that the
// paragraphs that fit come out as paragraphs. Joining the first 250 words of
// the Paxos front matter into one string gave a published file in which the
// title, the author, the journal citation and the abstract ran together into
// a single line of prose, and every one of those is a separate thing a reader
// is looking for.
func Abstract(body string, words int) string {
	if len(strings.Fields(body)) <= words {
		return strings.TrimRight(body, "\n")
	}
	var out []string
	left := words
	for _, p := range paragraphs(body) {
		n := len(strings.Fields(p))
		if n <= left {
			out = append(out, p)
			left -= n
			continue
		}
		if cut := sentences(p, left); cut != "" {
			out = append(out, cut)
		}
		break
	}
	if len(out) == 0 {
		// The block opens with one paragraph longer than the whole cap and
		// with no sentence end inside it. There is nothing to keep whole, so
		// the old word count is the only cut left.
		return strings.Join(strings.Fields(body)[:words], " ")
	}
	return strings.Join(out, "\n\n")
}

// sentences is the opening of a paragraph, at most words words long and
// stopping where a sentence stops. It is empty when nothing that fits ends a
// sentence, because half a clause is not worth publishing when there are
// whole paragraphs above it already.
func sentences(p string, words int) string {
	fields := strings.Fields(p)
	if words <= 0 || len(fields) <= words {
		return ""
	}
	cut := strings.Join(fields[:words], " ")
	if i := strings.LastIndexAny(cut, ".!?"); i > 0 {
		return cut[:i+1]
	}
	return ""
}

// paragraphs is the body's paragraphs, blanks dropped. A body written by this
// toolchain is one paragraph to a line, which is what extract.Page.Text
// writes and what the assembler preserves, so the lines are the structure.
func paragraphs(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// AbstractParagraph is how long a paragraph has to run before it is taken for
// an abstract rather than for a title, a byline, an affiliation or a
// copyright line. Forty words is longer than any of those and shorter than
// the shortest abstract in the corpus.
//
// Audit rule T06 reads the same number from here, because two opinions about
// what an abstract looks like would mean the splitter writing a front file
// the audit then refuses, over and over, with nobody able to say which of the
// two was wrong.
const AbstractParagraph = 40

// Restrict cuts a split down to the one file a restricted paper may publish:
// the front matter and an abstract of at most words words.
//
// The abstract is not always in the front block. A paper can open with a
// cover sheet, and three of the first eight papers in this corpus do: the
// Paxos paper's first page is Lamport's own note about where the article
// appeared, and the abstract is on the page after it. Taking the front block
// and stopping produced a published file that was a title, an author and a
// citation, which is a catalogue entry rather than a paper, and it is the
// whole of what the corpus would ever say about a paper it may not
// redistribute.
//
// So the front block is kept whatever it holds, because the title and the
// authors are in it, and if it carries no paragraph long enough to be an
// abstract then the first one that is, from the sections after it, is
// appended. The cap is applied last and to the result, so a paper whose front
// block already runs long publishes no more than one whose abstract had to be
// fetched from the next page.
func Restrict(files []File, words int) []File {
	if len(files) == 0 {
		return nil
	}
	front := files[0]
	body := string(front.Body)
	if longestParagraph(body) < AbstractParagraph {
		if p, from := firstParagraph(files[1:]); p != "" {
			body = strings.TrimRight(body, "\n") + "\n\n" + p + "\n"
			// The page range is a claim about where the published text came
			// from, so it has to cover the page the abstract was taken off.
			front.Front.PDFPages = spanPages(front.Front.PDFPages, from.Front.PDFPages)
		}
	}
	front.Body = []byte(Abstract(body, words) + "\n")
	front.Front.ContentSHA256 = corpus.ContentSHA(front.Body)
	return []File{front}
}

// firstParagraph is the first paragraph of these files that is long enough to
// be an abstract, and the file it came from.
func firstParagraph(files []File) (string, File) {
	for _, f := range files {
		for _, p := range strings.Split(string(f.Body), "\n") {
			p = strings.TrimSpace(p)
			if len(strings.Fields(p)) >= AbstractParagraph {
				return p, f
			}
		}
	}
	return "", File{}
}

// spanPages is the pdf_pages field that covers both of two others. A field
// nobody recorded is not a page zero, so an empty one leaves the other as it
// stands.
func spanPages(a, b string) string {
	af, al := pageRange(a)
	bf, bl := pageRange(b)
	if af == 0 {
		return b
	}
	if bf == 0 {
		return a
	}
	return pages(min(af, bf), max(al, bl))
}

// pageRange reads a pdf_pages field back, as one number or as a range. It
// gives back zeroes for anything it does not recognise, which is the same
// answer as a field nobody wrote.
func pageRange(s string) (first, last int) {
	from, to, ok := strings.Cut(strings.TrimSpace(s), "-")
	a, err := strconv.Atoi(strings.TrimSpace(from))
	if err != nil {
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

// longestParagraph is the word count of the longest paragraph of a body.
func longestParagraph(body string) int {
	most := 0
	for _, p := range paragraphs(body) {
		if n := len(strings.Fields(p)); n > most {
			most = n
		}
	}
	return most
}
