package roundtrip

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/llm"
)

// A Report is one run of the check, and is reports/roundtrip.md.
//
// The file is committed, so it holds no host names: the two models are
// named because a model is a published thing, and the routes that served
// them are not.
type Report struct {
	// Run is the run id, which is the timestamp the run started.
	Run string
	// Policy is what chose the sample, written into the report so that a
	// person reading a clean result can see how much of the corpus it is a
	// clean result for.
	Policy Policy
	Checks []Check
	// Failed is the pages that could not be checked, one line each. A page
	// the fleet could not answer for is not a page that passed, and leaving
	// it out of the report entirely is how a check quietly stops running.
	Failed []string
	Usage  llm.Usage
}

// Material is the checks that put a page back on the queue.
func (r *Report) Material() []Check {
	var out []Check
	for _, c := range r.Checks {
		if c.Verdict.Bad() {
			out = append(out, c)
		}
	}
	return out
}

// Count is how many checks came back with each verdict.
func (r *Report) Count() map[Verdict]int {
	out := map[Verdict]int{}
	for _, c := range r.Checks {
		out[c.Verdict]++
	}
	return out
}

// Summary is the one line a person reads first.
func (r *Report) Summary() string {
	n := r.Count()
	s := fmt.Sprintf("%d pages checked: %d the same, %d differ in wording, %d differ materially",
		len(r.Checks), n[Same], n[Wording], n[Material])
	if len(r.Failed) > 0 {
		s += fmt.Sprintf(", %d could not be checked", len(r.Failed))
	}
	return s
}

// Markdown is the report.
//
// The two Englishes go in it in full, side by side, for every page that
// differs. A verdict is a model's opinion and the whole point of writing it
// down is that somebody can disagree with it, which they cannot do from a
// label and a file name.
func (r *Report) Markdown() string {
	var b strings.Builder
	b.WriteString("# Back translation\n\n")
	b.WriteString(r.Summary())
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Run %s. Every abstract, every section of the first %d papers of the canon, and a %d%% sample of the rest.\n\n",
		r.Run, r.Policy.Top, r.Policy.Rate)
	fmt.Fprintf(&b, "%s tokens in, %s out.\n\n",
		thousands(r.Usage.InputTokens), thousands(r.Usage.OutputTokens))

	if len(r.Failed) > 0 {
		b.WriteString("## Not checked\n\n")
		for _, f := range r.Failed {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	for _, v := range Verdicts {
		checks := r.of(v)
		if len(checks) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", title(v))
		if v == Same {
			// Nothing to argue with, so the list is a list.
			for _, c := range checks {
				fmt.Fprintf(&b, "- `%s` %s\n", c.Path(), c.note())
			}
			b.WriteString("\n")
			continue
		}
		for _, c := range checks {
			fmt.Fprintf(&b, "### `%s`\n\n%s\n\n", c.Path(), c.note())
			for _, d := range c.Differences {
				fmt.Fprintf(&b, "- %s\n", d)
			}
			b.WriteString("\n")
			b.WriteString("The English as it stands:\n\n")
			quote(&b, c.English)
			b.WriteString("What came back:\n\n")
			quote(&b, c.Back)
		}
	}
	return b.String()
}

func (r *Report) of(v Verdict) []Check {
	var out []Check
	for _, c := range r.Checks {
		if c.Verdict == v {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

// note is the provenance line: who wrote the translation, who put it back
// and who judged it, and whether the judge was a second opinion at all.
func (c Check) note() string {
	s := fmt.Sprintf("%s, translated by %s, put back by %s, judged by %s",
		c.Why, name(c.Model), name(c.BackModel), name(c.JudgeModel))
	if c.SameModel {
		s += ". The fleet had nothing else free, so this is a model marking its own work"
	}
	return s + "."
}

func name(s string) string {
	if s == "" {
		return "an unrecorded model"
	}
	return s
}

func title(v Verdict) string {
	switch v {
	case Material:
		return "Differs materially"
	case Wording:
		return "Differs in wording"
	default:
		return "The same"
	}
}

// quote writes a passage as a block quote, which keeps the mathematics and
// the headings in it from being read as part of the report.
func quote(b *strings.Builder, text string) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString(">\n")
			continue
		}
		fmt.Fprintf(b, "> %s\n", line)
	}
	b.WriteString("\n")
}

// thousands groups a count so a reader can see the order of magnitude
// without counting digits.
func thousands(n int) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
