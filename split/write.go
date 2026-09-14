package split

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/mathtex"
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
		old, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			b, err := f.Bytes()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			if err := write(path, b); err != nil {
				return nil, err
			}
			r.Created = append(r.Created, f.Name)
			continue
		case err != nil:
			return nil, err
		}
		f.Front.Tag = keepTag(f.Front.Tag, old)
		b, err := f.Bytes()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
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

// keepTag carries a section's own tag across a rewrite.
//
// A tag is append only and means one thing for ever, and the one thing this
// one means is the section this file is. The section is still the same
// section after a re-extraction: same paper, same file name, same place in
// the reading order. What changed is the words in it.
//
// Everything else anchored in the corpus carries its tag in an attribute
// block on its own line, so it travels with the body and needs nothing here.
// A section's tag has no line to sit on, which is why it is in the front
// matter, and the front matter is rebuilt from the manifest on every split.
// So without this the tag went out of every file the splitter touched, and
// papers split over the whole corpus quietly dropped 192 of them at once.
// Putting them back means running papers tags assign again, and links written
// against them are dangling in the meantime.
//
// A tag the caller already has wins, because that is papers tags assign
// handing out a new one or repairing a wrong one, and this is only here for
// the callers that know nothing about tags at all.
func keepTag(tag string, old []byte) string {
	if tag != "" {
		return tag
	}
	f, _, err := corpus.ParseFront(old)
	if err != nil {
		return tag
	}
	return f.Tag
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

// Prune deletes the files a split reported as stale.
//
// Write reports them and leaves them, which is right for the run that finds
// them: a section boundary that moved renames files, and a person should see
// that happen rather than read it out of a four hundred line diff. But
// leaving them for ever is not right either. Two files for one section is
// two files with the same section number in their front matter, which is an
// audit rule T04 failure, and it stays failing until somebody deletes one by
// hand. The BERT paper had three of those, from a split whose appendix
// headings kept their A.1 and A.2 prefixes and a later one whose did not.
//
// So this is the deliberate second step, and it deletes only what the report
// names, which is only the .md files in the paper's own directory that the
// split that produced the report did not write.
func Prune(dir string, stale []string) error {
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
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
	if corpus.Words(body) <= words {
		return strings.TrimRight(body, "\n")
	}
	var out []string
	left := words
	for _, p := range paragraphs(body) {
		n := corpus.Words(p)
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
	if out = closeMath(out); len(out) == 0 {
		// The block opens with one paragraph longer than the whole cap and
		// with no sentence end inside it. There is nothing to keep whole, so
		// the old word count is the only cut left.
		return strings.Join(strings.Fields(body)[:words], " ")
	}
	return strings.Join(out, "\n\n")
}

// closeMath drops paragraphs off the end of a cut abstract until nothing in
// it has an open math span.
//
// The cap falls where the words run out, and on the Cooley paper they ran
// out one line into a displayed equation. The published front matter ended
// on a $$ that nothing ever closed, and everything after an open dollar is
// mathematics, so the file read as a formula somebody had cut in half.
// Audit rules M01 and M03 both said so and both were right.
//
// Nothing is lost by stopping above it. A displayed equation is not part of
// an abstract, and a restricted paper publishes an abstract and no more.
func closeMath(out []string) []string {
	for len(out) > 0 {
		if _, unclosed := mathtex.Split(strings.Join(out, "\n\n")); unclosed == nil {
			return out
		}
		out = out[:len(out)-1]
	}
	return out
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
//
// What comes out is the front file whatever the split called it. Usually it
// already is one, because a paper prints its title above its first heading
// and the splitter opens a front section for the text above the first cut.
// But a scan whose first page begins with a running head the reader took for
// a heading has no text above the cut, so there is no front section and the
// first file is section 1. That is what happened to the AlphaGo paper: its
// one published file came out as 01_article.md with kind section, and audit
// rules S02 and T06 both refused it, S02 because a restricted paper gets
// 00_front.md and nothing else and T06 because the paper then has no title
// block at all. The heading is not lost, it is in the body underneath.
func Restrict(files []File, words int) []File {
	if len(files) == 0 {
		return nil
	}
	front := files[0]
	front.Front.Section = ""
	front.Front.SectionTitle = "Front Matter"
	front.Front.Kind = KindFront
	front.Name = Section{Kind: KindFront}.Filename()
	body := fromTitle(string(front.Body), front.Front.Title)
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

// fromTitle is the front block from this paper's title onwards.
//
// A journal that sets one article straight after another puts the tail of the
// previous one above this paper's title, and the reader transcribes the page
// it was given. The first page of the Dennard paper is the closing reference
// list of an article about CMOS on sapphire, numbered [3] to [14], and then
// the title of the Dennard paper in the last column. The cap spent all 250
// words on that bibliography, so the whole of what the corpus published
// about a paper it may not redistribute was somebody else's references.
//
// The match is on letters alone, lowercased, because the page and the
// manifest disagree about case, about the apostrophe in MOSFET's, and about
// the hyphen in Ion-Implanted.
//
// A title that is not found leaves the block exactly as it was. A page that
// does not print the title is a page this knows nothing about, and there are
// real ones: a cover sheet carries the title alone with no abstract under it,
// and a paper whose first page is a plate carries neither. Keeping too much
// is a rule the audit will complain about, and dropping the whole block would
// publish nothing at all and look like the paper was read and found empty.
func fromTitle(body, title string) string {
	want := onlyLetters(title)
	if want == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.Contains(onlyLetters(line), want) {
			return strings.Join(lines[i:], "\n")
		}
	}
	return body
}

// onlyLetters is the lowercase letters and digits of a string, everything
// else dropped, for comparing a title on a page against a title in a
// manifest.
func onlyLetters(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstParagraph is the first paragraph of these files that is long enough to
// be an abstract, and the file it came from.
func firstParagraph(files []File) (string, File) {
	for _, f := range files {
		for _, p := range strings.Split(string(f.Body), "\n") {
			p = strings.TrimSpace(p)
			if corpus.Words(p) >= AbstractParagraph {
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
		if n := corpus.Words(p); n > most {
			most = n
		}
	}
	return most
}
