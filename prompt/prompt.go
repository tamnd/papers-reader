// Package prompt holds the prompts the toolchain sends to a model.
//
// They are files rather than string literals so that a prompt can be read and
// argued with as prose by somebody who is not reading Go, and they are
// embedded rather than loaded from beside the binary so that a build carries
// the exact text it was built with. Every prompt has a hash, and that hash
// goes in the front matter of every file the prompt produced: when the prompt
// changes, the pages it produced are detectably stale rather than silently
// mixed in with pages produced by a different one.
//
// The mechanism is llm/prompt and only the prompts are ours. This package is
// the corpus's list of them plus the one rule that is about papers rather than
// about prompts, which is the per-paper note.
package prompt

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"

	llmprompt "github.com/tamnd/llm/prompt"
)

//go:embed *.md
var files embed.FS

var set = llmprompt.New(files)

// A Prompt is one prompt and the hash of its text.
type Prompt = llmprompt.Prompt

// Get reads a prompt by name, with or without its extension.
func Get(name string) (Prompt, error) { return set.Get(name) }

// MustGet is Get for a prompt the program cannot run without. It panics,
// because a missing embedded prompt is a build mistake and not a runtime
// condition.
func MustGet(name string) Prompt { return set.MustGet(name) }

// Names lists every prompt, for papers prompts and for an error message.
func Names() []string { return set.Names() }

// All reads every prompt, for the command that prints the hashes.
func All() ([]Prompt, error) { return set.All() }

// SHA256 hashes a prompt, in full hex. This is what goes in a file's
// prompt_sha256.
func SHA256(text string) string { return llmprompt.SHA256(text) }

// OCR is the prompt for reading a picture of a page of a paper.
const OCR = "ocr_paper"

//go:embed notes
var notes embed.FS

// noteDir is where the per-paper notes sit, one file per paper id.
const noteDir = "notes"

// Note is what one paper adds to the shared reading prompt, and false for the
// papers that add nothing, which is nearly all of them.
//
// The mechanism is Bourbaki's bookNote and the reasoning carries over exactly.
// A prompt's hash goes in the front matter of every page it produced, so a
// sentence about one paper's notation added to the shared prompt puts every
// page of every other paper back in the queue for a rule that is about none of
// them. A paper with nothing to add gets the shared prompt byte for byte.
//
// The measurement behind it is Bourbaki's two printings of Theory of Sets: the
// reader that was told the script T is never \mathcal wrote \mathcal zero
// times, and the one that was not wrote it 738 times. A note is worth more
// than its length suggests, and it is worth it only to the paper it is about.
//
// A note is written after somebody has looked at the pages and not before. The
// spec expects turing-1936-computable, mccarthy-1960-lisp, hoare-1962-quicksort,
// sutherland-1963-sketchpad and engelbart-1968-augmenting to need one, and none
// of them has been read yet, so none of them has one. A note guessed from a
// paper's reputation is a paragraph of instructions about a page nobody has
// seen, and the reader follows it just as carefully as it follows the rest.
func Note(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	b, err := fs.ReadFile(notes, path.Join(noteDir, id+".md"))
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return "", false
	}
	return text + "\n", true
}

// Noted lists the papers that have a note, for papers prompts and for the
// report.
func Noted() []string {
	entries, err := notes.ReadDir(noteDir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".md"))
	}
	slices.Sort(out)
	return out
}

// Page is the prompt one paper's pages are read with: the shared reading
// prompt, and this paper's note under it where it has one.
//
// The note goes last because it is the exception and the shared rules are the
// general case, and a reader meets the exception after the rule it bends. The
// returned Prompt keeps the shared prompt's name, so that a report groups
// every page of every paper under ocr_paper, and carries the hash of the text
// as the model will actually see it, so that two papers read with different
// notes are not recorded as having been asked the same question.
func Page(id string) (Prompt, error) {
	p, err := Get(OCR)
	if err != nil {
		return Prompt{}, err
	}
	note, ok := Note(id)
	if !ok {
		return p, nil
	}
	p.Text = strings.TrimSpace(p.Text) + "\n\n" + note
	p.SHA = SHA256(p.Text)
	return p, nil
}
