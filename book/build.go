package book

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Tectonic is the typesetter.
//
// tectonic rather than a TeX Live installation because it fetches the
// packages a document asks for and caches them, so a build on a machine that
// has never seen LaTeX produces the same PDF as a build on one that has. It
// is XeTeX underneath, which is what makes fontspec and xeCJK available, and
// those are what make a Vietnamese and a Japanese paper set at all.
const Tectonic = "tectonic"

// A Report is what came back from the typesetter: the file, and every
// complaint worth passing on.
//
// Overfull and Underfull are counted rather than listed. A run has dozens of
// them, they are almost all a millimetre of slack in a line of prose, and a
// list of them is noise that hides the one that matters. The count is what
// says whether a change to the document made the setting better or worse.
type Report struct {
	PDF       string
	Pages     int
	Overfull  int
	Underfull int
	// Undefined is every \ref and \hyperlink that pointed at nothing, by name.
	// These are listed and not counted, because each one is a hole in the
	// document somebody can go and fix.
	Undefined []string
	// Missing is every glyph the fonts could not set. A missing glyph is a
	// blank space on the page where a character should be, and it is the one
	// failure of a CJK build that looks like success.
	Missing []string
	// Errors is every error TeX carried on past, with the line of source that
	// caused it. See Keep for why the run carries on at all.
	Errors []string
	Log    string
}

// Runs is how many times tectonic is run.
//
// Two. The first pass writes the aux file that the table of contents and
// every \ref read, and the second pass sets them. tectonic reruns by itself
// when it can tell it needs to, and it cannot always tell with
// \addcontentsline, which is how every heading in this document gets into the
// contents.
const Runs = 2

// Keep is the tectonic switch that makes an error something to report rather
// than something to stop on.
//
// A book is two hundred paragraphs and one of them can be bad. The first real
// build of this stopped on a $\LaTeX$ in the acknowledgments of the eighth
// section and threw away the seven that had set, which tells somebody to fix
// one paragraph by hiding the other hundred and ninety nine. With this on the
// run finishes, the PDF exists, and every error is on Report.Errors for the
// caller to print. The document is still wrong and somebody still has to fix
// it, but they can see what they are fixing.
const Keep = "continue-on-errors"

// Build runs the typesetter over a LaTeX source and returns the PDF beside it.
//
// dir is where both the source and the PDF go, and it is the caller's to
// clean up or to keep. Keeping it is the only way to debug a document that
// will not set, so papers book keeps it whenever the build fails.
//
// A relative dir is made absolute before anything is done with it, because
// the run happens inside it. tectonic resolves --outdir against its own
// working directory, so "books/.paper.build-1234" as the directory and as
// the output directory means books/.paper.build-1234/books/.paper.build-1234,
// which does not exist, and the run fails with a message about an output
// directory that the caller can see is right there. That is what "papers
// book -corpus ." did and "papers book -corpus /absolute/path" did not.
func Build(ctx context.Context, dir, tex string) (*Report, error) {
	if _, err := exec.LookPath(Tectonic); err != nil {
		return nil, fmt.Errorf("%s is not installed, and it is what sets the PDF: brew install tectonic", Tectonic)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	src := filepath.Join(dir, "paper.tex")
	if err := os.WriteFile(src, []byte(tex), 0o644); err != nil {
		return nil, err
	}
	var log string
	for range Runs {
		// keep-intermediates so the aux file survives between the two runs,
		// and keep-logs so there is something to read when a run fails. Both
		// stay in dir, which is temporary unless the caller kept it.
		cmd := exec.CommandContext(ctx, Tectonic, "-Z", Keep,
			"--keep-intermediates", "--keep-logs", "--outdir", dir, "--chatter", "minimal", src)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		log = string(out)
		if err != nil {
			return &Report{Log: log}, fmt.Errorf("%s: %w\n%s", Tectonic, err, tail(log, 30))
		}
	}
	b := &Report{PDF: filepath.Join(dir, "paper.pdf"), Log: log}
	if text, err := os.ReadFile(filepath.Join(dir, "paper.log")); err == nil {
		b.read(string(text))
	} else {
		b.read(log)
	}
	if _, err := os.Stat(b.PDF); err != nil {
		return b, fmt.Errorf("%s said nothing and wrote no PDF", Tectonic)
	}
	return b, nil
}

var (
	overfull  = regexp.MustCompile(`(?m)^Overfull \\[hv]box`)
	underfull = regexp.MustCompile(`(?m)^Underfull \\[hv]box`)
	undefined = regexp.MustCompile(`Reference \x60([^']+)' on page`)
	// An error is a line starting with ! and the line of source that TeX was
	// reading when it hit it, a few lines further on and starting with l.NNN.
	texError  = regexp.MustCompile(`(?m)^! (.*)$`)
	texLine   = regexp.MustCompile(`(?m)^l\.([0-9]+) (.*)$`)
	missing   = regexp.MustCompile(`Missing character: There is no (.) in font ([^!]*)!`)
	pageCount = regexp.MustCompile(`Output written on \S+ \((\d+) pages?`)
)

func (b *Report) read(log string) {
	b.Overfull = len(overfull.FindAllString(log, -1))
	b.Underfull = len(underfull.FindAllString(log, -1))
	for _, m := range undefined.FindAllStringSubmatch(log, -1) {
		b.Undefined = append(b.Undefined, m[1])
	}
	seen := map[string]bool{}
	for _, m := range missing.FindAllStringSubmatch(log, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			b.Missing = append(b.Missing, m[1])
		}
	}
	if m := pageCount.FindStringSubmatch(log); m != nil {
		b.Pages, _ = strconv.Atoi(m[1])
	}
	b.errors(log)
}

// errors pairs each error with the source line under it.
//
// The pairing is by position: the l.NNN line that follows an error line is the
// source TeX was reading, and the next error starts the next pair. An error
// with no line under it, which is what a package warning raised to an error
// looks like, is reported on its own.
func (b *Report) errors(log string) {
	lines := texLine.FindAllStringSubmatchIndex(log, -1)
	for _, e := range texError.FindAllStringSubmatchIndex(log, -1) {
		text := strings.TrimSpace(log[e[2]:e[3]])
		for _, l := range lines {
			if l[0] > e[1] {
				if at := log[l[2]:l[3]]; strings.Count(log[e[1]:l[0]], "\n") < 8 {
					text += " at line " + at + ": " + strings.TrimSpace(log[l[4]:l[5]])
				}
				break
			}
		}
		b.Errors = append(b.Errors, oneLine(text))
	}
}

// Clean says whether the build is one nobody has to look at.
func (b *Report) Clean() bool {
	return len(b.Undefined) == 0 && len(b.Missing) == 0 && len(b.Errors) == 0
}

func tail(s string, lines int) string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(all) <= lines {
		return s
	}
	return strings.Join(all[len(all)-lines:], "\n")
}
