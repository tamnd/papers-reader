// Package audit is the contract the corpus is held to.
//
// Numbered rules in groups, each one a sentence a person can argue with, each
// saying what it found and where. A hard rule that fails fails the build.
//
// A rule reports three outcomes and they are different states: pass means it
// looked and found nothing, fail means it looked and found something, and
// "not run" means it had nothing to look at. Conflating the last two is how a
// corpus acquires a rule everybody believes is working.
package audit

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/refs"
)

// Group is the first letter of a rule id.
type Group string

const (
	GroupSources     Group = "S"
	GroupStructure   Group = "T"
	GroupMathematics Group = "M"
	GroupCode        Group = "C"
	GroupFigures     Group = "F"
	GroupReferences  Group = "R"
	GroupTags        Group = "G"
	GroupTranslation Group = "L"
	GroupPublication Group = "P"
)

var groupTitles = map[Group]string{
	GroupSources:     "Sources and licensing",
	GroupStructure:   "Structure",
	GroupMathematics: "Mathematics",
	GroupCode:        "Code",
	GroupFigures:     "Figures",
	GroupReferences:  "References",
	GroupTags:        "Tags",
	GroupTranslation: "Translation",
	GroupPublication: "Publication",
}

// Title is the group written out.
func (g Group) Title() string { return groupTitles[g] }

// Finding is one thing a rule found, in one place.
type Finding struct {
	Rule    string
	File    string
	Line    int
	Message string
}

func (f Finding) String() string {
	where := f.File
	if f.Line > 0 {
		where = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	if where == "" {
		return fmt.Sprintf("%s: %s", f.Rule, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.Rule, where, f.Message)
}

// Rule is one check.
type Rule struct {
	ID   string
	Hard bool
	// What is the rule as a sentence, and is what gets printed in the report
	// next to the findings. If it cannot be written as one sentence it is
	// more than one rule.
	What string
	// Check looks at the corpus. Returning ErrNotRun says the rule had
	// nothing to look at, which is not a pass.
	Check func(*Input) ([]Finding, error)
}

// Group is the group the rule belongs to, taken from its id.
func (r Rule) Group() Group { return Group(r.ID[:1]) }

// ErrNotRun is returned by a rule with nothing to look at.
var ErrNotRun = fmt.Errorf("not run")

// Input is everything the rules read, loaded once.
type Input struct {
	Corpus      *corpus.Corpus
	Papers      *corpus.Papers
	Sources     *corpus.Sources
	Collections *corpus.Collections
	// Tracked is every path git has in the index, relative to the corpus root
	// and slash separated. Rules S03 and F04 are about what git holds rather
	// than about what is on the disk, so they need this rather than a walk.
	Tracked []string
	// Refs is the parsed bibliography of every paper that has one, keyed by
	// paper id. A paper with no bibliography yet is missing from the map
	// rather than present and empty, so that group R can tell "nothing to
	// check" apart from "a bibliography with nothing in it".
	Refs map[string]*refs.Manifest
}

// Load reads everything the rules need out of a corpus.
func Load(c *corpus.Corpus) (*Input, error) {
	in := &Input{Corpus: c}
	var err error
	if in.Papers, err = c.LoadPapers(); err != nil {
		return nil, err
	}
	if in.Sources, err = c.LoadSources(); err != nil {
		return nil, err
	}
	if in.Collections, err = c.LoadCollections(); err != nil {
		return nil, err
	}
	if in.Tracked, err = trackedFiles(c.Root); err != nil {
		return nil, err
	}
	if in.Refs, err = loadRefs(c, in.Papers); err != nil {
		return nil, err
	}
	return in, nil
}

// loadRefs reads every bibliography the corpus has parsed so far. A paper
// without one is skipped and is not an error: the milestone that parses
// them is still working through the corpus and an audit that refused to run
// until it finished would be useless for the whole of it.
func loadRefs(c *corpus.Corpus, papers *corpus.Papers) (map[string]*refs.Manifest, error) {
	out := map[string]*refs.Manifest{}
	for _, p := range papers.Papers {
		m, err := refs.Load(c.Refs(p.ID))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[p.ID] = m
	}
	return out, nil
}

// Result is what one rule did.
type Result struct {
	Rule     Rule
	Findings []Finding
	NotRun   bool
	Err      error
}

// Failed reports whether the rule found something.
func (r Result) Failed() bool { return len(r.Findings) > 0 }

// Report is a whole run.
type Report struct {
	Results []Result
}

// Run runs the rules. With hardOnly set, the soft rules are skipped, which is
// what CI does.
func Run(in *Input, hardOnly bool) *Report {
	rep := &Report{}
	for _, rule := range Rules() {
		if hardOnly && !rule.Hard {
			continue
		}
		findings, err := rule.Check(in)
		res := Result{Rule: rule, Findings: findings}
		switch {
		case err == ErrNotRun:
			res.NotRun = true
		case err != nil:
			res.Err = err
		}
		sort.SliceStable(res.Findings, func(i, j int) bool {
			a, b := res.Findings[i], res.Findings[j]
			if a.File != b.File {
				return a.File < b.File
			}
			return a.Line < b.Line
		})
		rep.Results = append(rep.Results, res)
	}
	return rep
}

// Findings is every finding of the run, in rule order.
func (r *Report) Findings() []Finding {
	var out []Finding
	for _, res := range r.Results {
		out = append(out, res.Findings...)
	}
	return out
}

// HardFailures is how many hard rules found something. It is what decides the
// exit status, and with it whether a build fails.
func (r *Report) HardFailures() int {
	n := 0
	for _, res := range r.Results {
		if res.Rule.Hard && res.Failed() {
			n++
		}
	}
	return n
}

// Errors is every rule that could not run for a reason that is not "nothing
// to look at". A broken rule is not a passing rule.
func (r *Report) Errors() []error {
	var out []error
	for _, res := range r.Results {
		if res.Err != nil {
			out = append(out, fmt.Errorf("%s: %w", res.Rule.ID, res.Err))
		}
	}
	return out
}

// Summary is the one line a person reads first.
func (r *Report) Summary() string {
	var pass, fail, notRun, broken int
	for _, res := range r.Results {
		switch {
		case res.Err != nil:
			broken++
		case res.NotRun:
			notRun++
		case res.Failed():
			fail++
		default:
			pass++
		}
	}
	s := fmt.Sprintf("%d rules: %d passed, %d failed, %d not run", len(r.Results), pass, fail, notRun)
	if broken > 0 {
		s += fmt.Sprintf(", %d could not run", broken)
	}
	return s
}

// Markdown is reports/audit.md: every rule, what it says, and what it found.
func (r *Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# Audit\n\n")
	b.WriteString(r.Summary())
	b.WriteString("\n\n")

	current := Group("")
	for _, res := range r.Results {
		if g := res.Rule.Group(); g != current {
			current = g
			fmt.Fprintf(&b, "## %s %s\n\n", g, g.Title())
		}
		state := "pass"
		switch {
		case res.Err != nil:
			state = "could not run"
		case res.NotRun:
			state = "not run"
		case res.Failed():
			state = fmt.Sprintf("%d found", len(res.Findings))
		}
		hard := "soft"
		if res.Rule.Hard {
			hard = "hard"
		}
		fmt.Fprintf(&b, "**%s** (%s, %s) %s\n\n", res.Rule.ID, hard, state, res.Rule.What)
		if res.Err != nil {
			fmt.Fprintf(&b, "- the rule itself failed: %s\n\n", res.Err)
		}
		for _, f := range res.Findings {
			where := f.File
			if f.Line > 0 {
				where = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			if where != "" {
				fmt.Fprintf(&b, "- `%s` %s\n", where, f.Message)
			} else {
				fmt.Fprintf(&b, "- %s\n", f.Message)
			}
		}
		if res.Failed() {
			b.WriteString("\n")
		}
	}
	return b.String()
}
