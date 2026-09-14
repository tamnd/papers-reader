package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// RecordFile is what a run's record is called, inside work/<id>.
const RecordFile = "extraction.yaml"

// A Record is how one paper was read.
//
// It exists because the front matter of every content file names the
// extraction path, and the command that writes content files is papers
// split, which runs hours after the extraction and can see nothing but a
// directory of page files. Page files are deliberately plain text with no
// header on them, so there is nowhere in a page to write this, and guessing
// afterwards is not possible: the two paths are written to produce the same
// Markdown and a page that came out of mineru is meant to be
// indistinguishable from one that came out of pdftotext.
//
// So the run writes down what it did. A corpus whose front matter says
// "native" for a paper a model read would be lying about the one thing a
// reader most wants to know, which is whether anything guessed.
type Record struct {
	// Path is native, layout or vision.
	Path string `yaml:"path"`
	// Tool is the program that read the pages, and its version where the
	// program will say. For the native path this is pdftotext, which is not a
	// model and is recorded anyway: poppler's paragraph reconstruction has
	// changed between releases.
	Tool string `yaml:"tool,omitempty"`
	// Prompt is the sha256 of the instructions the pages were read with, and
	// empty for a path with no model in it.
	//
	// It goes into the front matter of every file the pages become, which is
	// the whole reason it is recorded. A prompt that is edited changes its
	// hash, and the pages produced under the old wording are then findable
	// rather than silently mixed in with pages produced under the new one.
	// That is also why it is the hash of the paper's own prompt, note and
	// all: two papers read with different notes were asked different
	// questions and saying otherwise would be a lie about both.
	Prompt string `yaml:"prompt_sha256,omitempty"`
	// Source is the sha256 of the PDF the pages were read from, which is the
	// same hash sources.yaml records for the file it fetched.
	//
	// It is here because a PDF gets replaced. Rule S09 found that the
	// resolver had fetched an eighteen page lecture deck about Royce's paper
	// rather than the paper, somebody corrected the record by hand, papers
	// fetch pulled the real eleven page scan, and the next extraction read
	// the deck anyway: the page images were still on disk from the first
	// fetch, the render cache keys on the page number and the resolution and
	// nothing else, and every page it needed was already there. The deck went
	// through the reader a second time and the corrected record made no
	// difference at all.
	//
	// So the run writes down which file it read, and a run that finds a
	// different hash throws away the pages and the images and starts again.
	// A record with no Source in it was written before this field existed and
	// is left alone, because there is nothing to compare and clearing on that
	// basis would re-read the whole corpus.
	Source string `yaml:"source_sha256,omitempty"`
	// First and Last are the pages of the PDF this covers.
	First int `yaml:"first_page"`
	Last  int `yaml:"last_page"`
	// When the run finished, so that a paper extracted before a fix to the
	// column splitter can be found and done again.
	When time.Time `yaml:"when"`
}

// ReadRecord loads the record from work/<id>.
//
// A directory with no record is not an error. Every paper extracted before
// this file existed is in that state, and so is one whose work directory was
// deleted to save space after the content was written. The caller gets nil
// and leaves the field out, which is better than a run that stops.
func ReadRecord(dir string) (*Record, error) {
	b, err := os.ReadFile(filepath.Join(dir, RecordFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r Record
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s does not parse: %w", RecordFile, err)
	}
	return &r, nil
}

// Write stores the record in work/<id>.
//
// A second run over more pages of the same paper widens the range rather
// than replacing it, because extraction is resumable and a run that did
// pages 9 to 12 did not undo pages 1 to 8. The path and the tool are the
// new run's: a paper re-read by a different tool is that tool's text now,
// whatever read it the first time.
//
// A run over a different PDF widens nothing. Pages 1 to 8 of the file that
// was there last week are pages of another document, and a range that
// covered both would say this record described pages it has never seen. A
// record from before Source existed does not say which file it read, and it
// widens as it always did rather than being treated as a different one.
//
// A run on a different path over only some of the pages is refused. "A paper
// re-read by a different tool is that tool's text now" is true of a whole
// paper and false of one page, and taking it for both is how the Bitcoin
// paper came to be labelled native after a model had read eight of its nine
// pages: page 5 was re-read with pdftotext, the record was replaced whole,
// and papers split stamped extraction: native on all fourteen content files.
// Nothing warned and the split succeeded. There is no honest record to write
// in that state, because a Record describes one paper read one way and the
// pages on disk are now some of each, so Write says so instead of picking
// one. It leaves the old record alone, which is stale about the pages the
// new run rewrote and is at least not a claim about the whole paper.
//
// Refusing is the small fix and it is the one that is here. The real fix is
// a range per path in the file, so that split can stamp each content file
// from the pages it actually came from, and it is worth doing when a paper
// needs it: one page no model could read and the rest read natively is a
// normal outcome rather than a mistake. Until then the way through is to
// re-extract the whole paper on the path you want.
func (r Record) Write(dir string) error {
	if old, err := ReadRecord(dir); err == nil && old != nil && !Replaced(old, r.Source) {
		switch {
		case old.Path == r.Path:
			if old.First > 0 && old.First < r.First {
				r.First = old.First
			}
			if old.Last > r.Last {
				r.Last = old.Last
			}
		case !r.covers(*old):
			return fmt.Errorf("%s had pages %d to %d read on the %s path and this run read pages %d to %d of it on the %s path, so the pages on disk are now some of each and no single record is true of them. the record was left as it was rather than made to say this paper is %s. re-extract the whole paper on one path, or delete %s and start again",
				dir, first(old.First), old.Last, old.Path, first(r.First), r.Last, r.Path, r.Path, filepath.Join(dir, RecordFile))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, RecordFile), b, 0o644)
}

// covers says this run read every page the old record covers, in which case
// there is nothing of the old path left in the paper and replacing the
// record outright is the whole truth about it.
func (r Record) covers(old Record) bool {
	return first(r.First) <= first(old.First) && r.Last >= old.Last
}

// first reads a missing first page as page one. A record written before the
// field was always filled in has a zero there, and zero as a page number
// would make every range look wider than it is.
func first(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

// Replaced says the PDF has changed since this paper was read.
//
// False for a record that does not name a file and false for a caller that
// does not know the hash, because a comparison needs two sides and guessing
// on one costs a whole corpus of re-reads. Both of those are the state the
// corpus was in the day this was added, so the field fills itself in on the
// next run of each paper and the check starts working from there.
func Replaced(old *Record, sha string) bool {
	return old != nil && old.Source != "" && sha != "" && old.Source != sha
}
