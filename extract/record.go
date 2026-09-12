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
func (r Record) Write(dir string) error {
	if old, err := ReadRecord(dir); err == nil && old != nil && old.Path == r.Path {
		if old.First > 0 && old.First < r.First {
			r.First = old.First
		}
		if old.Last > r.Last {
			r.Last = old.Last
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
