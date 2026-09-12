package book

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// The typesetter is not a dependency of the toolchain: a machine that only
// reads the corpus never runs one, and CI has no reason to install a LaTeX
// distribution. So the build tests skip where tectonic is not on the path,
// and the log reading tests, which are where the mistakes actually are, run
// everywhere off a recorded log.
func needTectonic(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(Tectonic); err != nil {
		t.Skipf("%s is not installed here", Tectonic)
	}
	if testing.Short() {
		t.Skip("a real build fetches packages and takes several seconds")
	}
}

func TestAWholeBookSets(t *testing.T) {
	needTectonic(t)
	b := testBook(t)
	tex, r, err := Document(b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Missing) != 0 || len(r.Orphans) != 0 {
		t.Fatalf("the document is missing %v and orphaned %v", r.Missing, r.Orphans)
	}

	report, err := Build(context.Background(), t.TempDir(), tex)
	if err != nil {
		t.Fatal(err)
	}
	if report.Pages == 0 {
		t.Error("the typesetter wrote no pages")
	}
	if !report.Clean() {
		t.Errorf("the build is not clean: %v undefined, %v missing, %v errors",
			report.Undefined, report.Missing, report.Errors)
	}
}

// The log is the only thing the typesetter says about a document that set but
// set badly, and every one of these lines is one that turned up on the first
// real build.
func TestTheLogIsRead(t *testing.T) {
	const log = `Overfull \hbox (1112.5972pt too wide) in paragraph at lines 273--284
[]\TU/lmroman10-regular.otf(0)/m/n/10.95 a table
Underfull \vbox (badness 10000) has occurred while \output is active
LaTeX Warning: Reference ` + "`fermat-1637-margin-fig-9'" + ` on page 3 undefined on input line 40.
Missing character: There is no 的 in font lmroman10-regular.otf!
Missing character: There is no 的 in font lmroman10-regular.otf!
Output written on paper.xdv (13 pages, 200356 bytes).
`
	var b Report
	b.read(log)

	if b.Overfull != 1 || b.Underfull != 1 {
		t.Errorf("the log has %d overfull and %d underfull boxes", b.Overfull, b.Underfull)
	}
	if len(b.Undefined) != 1 || b.Undefined[0] != "fermat-1637-margin-fig-9" {
		t.Errorf("the undefined references are %v", b.Undefined)
	}
	// Once each. A page of Chinese in a Latin font is the same complaint
	// several thousand times and it is one problem.
	if len(b.Missing) != 1 || b.Missing[0] != "的" {
		t.Errorf("the missing characters are %v", b.Missing)
	}
	if b.Pages != 13 {
		t.Errorf("the log says %d pages", b.Pages)
	}
	if b.Clean() {
		t.Error("a build with an undefined reference and a missing glyph called itself clean")
	}
}

// An error used to stop the run and throw away the pages that had set. The
// build carries on now, and the error has to come back with the line of
// source that caused it or nobody can find it.
func TestAnErrorIsReportedWithItsLine(t *testing.T) {
	const log = `! You can't use ` + "`\\spacefactor'" + ` in math mode.
\@->\spacefactor
                 \@m {}
l.322 ...support with $\LaTeX
                             $ typesetting. We would al...
Output written on paper.xdv (13 pages, 200356 bytes).
`
	var b Report
	b.read(log)

	if len(b.Errors) != 1 {
		t.Fatalf("the log has %d errors and one error in it: %v", len(b.Errors), b.Errors)
	}
	if !strings.Contains(b.Errors[0], "spacefactor") || !strings.Contains(b.Errors[0], "line 322") {
		t.Errorf("the error is %q and it should name what went wrong and where", b.Errors[0])
	}
	if b.Clean() {
		t.Error("a build with an error called itself clean")
	}
}
