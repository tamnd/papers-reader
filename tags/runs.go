package tags

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// A Run is one assignment: the first and last tag it handed out, inclusive.
//
// tags/runs exists so the audit can tell a correct edit from a tag somebody
// pasted in the wrong place. Within one run the tags climb in reading order,
// because that is the order the assigner walked the corpus in. Across runs
// they do not: a section inserted into a paper next year gets a tag from the
// top of the register and sits between two much lower ones, and that is
// correct and permanent and exactly what append only means.
//
// Without the runs file the audit has to choose between a rule that demands
// tags climb everywhere, which forbids ever adding a section, and no rule at
// all, which lets a copied and pasted block go unnoticed. With it, rule G06
// can ask the useful question.
type Run struct {
	First, Last Tag
}

// Holds reports whether a tag was handed out by this run.
func (r Run) Holds(t Tag) bool { return t.Value() >= r.First.Value() && t.Value() <= r.Last.Value() }

// Together reports whether two tags came out of the same run, which is when
// their order is something the audit may insist on.
func Together(runs []Run, a, b Tag) bool {
	for _, r := range runs {
		if r.Holds(a) && r.Holds(b) {
			return true
		}
	}
	return false
}

// ReadRuns parses tags/runs, one "first,last" line at a time.
func ReadRuns(r io.Reader) ([]Run, error) {
	var out []Run
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		first, last, ok := strings.Cut(text, ",")
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not first,last", line, text)
		}
		f, err := ParseTag(first)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		l, err := ParseTag(last)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if l.Value() < f.Value() {
			return nil, fmt.Errorf("line %d: the run ends at %s and starts at %s", line, l, f)
		}
		out = append(out, Run{First: f, Last: l})
	}
	return out, sc.Err()
}

// LoadRuns reads the runs file. A file that does not exist yet is no runs,
// which is what a corpus before its first assignment has.
func LoadRuns(path string) ([]Run, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	runs, err := ReadRuns(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return runs, nil
}

// WriteRuns writes the runs out in the order they were made, which is the
// order they happened and the only order that means anything here.
func WriteRuns(w io.Writer, runs []Run) error {
	bw := bufio.NewWriter(w)
	for _, r := range runs {
		if _, err := fmt.Fprintf(bw, "%s,%s\n", r.First, r.Last); err != nil {
			return err
		}
	}
	return bw.Flush()
}
