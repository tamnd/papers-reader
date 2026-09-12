// Package roundtrip is the only check that reads what a translation means.
//
// Every other check on a translated page is a comparison of form. The audit's
// group L counts the formulas, the citations, the listings and the tags, and
// looks for prose that is still in English. All of it passes a page that is
// well formed, glossary-obeying, fluent and wrong: a dropped negation, a
// bound that turned into its converse, a hedge that hardened into a claim.
// Those are the mistranslations that survive to print, because nothing about
// them looks like a mistake.
//
// The check is Bourbaki's roundtrip. Put the translation back into English
// with a model that has not seen the original, then put the two Englishes in
// front of a judge and ask whether they claim the same things. Two asks per
// page, which is why it runs on a sample.
//
// The back-translation is done somewhere other than where the translation was
// written, and the judge somewhere other than that again where the fleet
// allows it. A model checking its own work agrees with itself. That is not a
// suspicion about models, it is what the failures look like: the model that
// dropped the negation reads its own output as though the negation were
// there, because as far as it is concerned it is.
package roundtrip

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/translate"
)

// A Verdict is what the judge said.
type Verdict string

const (
	// Same means the two Englishes claim the same things.
	Same Verdict = "same"
	// Wording means something is phrased differently or slightly blurred and
	// no claim of the paper has changed. It is recorded and it is not acted
	// on: a literal back-translation of a good translation reads badly, and
	// a check that treated bad reading as a fault would report every page.
	Wording Verdict = "differs-in-wording"
	// Material means a reader of the translation would come away believing
	// something the paper does not say, or would miss something it does.
	// The page goes back on the queue.
	Material Verdict = "differs-materially"
)

// Verdicts is the three of them, worst first, for a report that groups by
// verdict and for the parser.
var Verdicts = []Verdict{Material, Wording, Same}

// Bad reports whether the verdict puts the page back on the queue.
func (v Verdict) Bad() bool { return v == Material }

// A Sample is one file the policy chose, and why it chose it.
//
// Why is carried through to the report because the sample is the part of
// this check somebody will want to argue with, and an argument about a
// sampling policy needs to see which rule pulled in which page.
type Sample struct {
	Paper string
	Lang  corpus.Lang
	Name  string
	Why   string
	// Model is what wrote the translation, out of the front matter. It is
	// the model the two asks are steered away from.
	Model string
}

// Path is where the translated file sits, relative to the corpus root.
func (s Sample) Path() string {
	return fmt.Sprintf("content/%s/%s/%s", s.Lang, s.Paper, s.Name)
}

// A Page is one translated file the policy may choose, with everything Pick
// needs to decide.
type Page struct {
	Paper string
	Lang  corpus.Lang
	Name  string
	// Kind is the front matter kind. A front file holds the abstract and is
	// always checked.
	Kind string
	// Number is the paper's position in the seed hundred, and zero for a
	// paper added afterwards.
	Number int
	// Model is the translation_model out of the front matter.
	Model string
}

// Policy is how much of the corpus the check reads.
//
// The defaults are the spec's: every abstract, every section of the ten
// papers at the head of the canon, and a twentieth of everything else. Two
// asks per page and a corpus of a hundred papers at ten sections each is
// four thousand asks to check all of it, which is not a check that runs on
// a Tuesday afternoon.
type Policy struct {
	// Top is how many papers, counting from the head of the seed hundred,
	// are checked section by section rather than sampled.
	Top int
	// Rate is the percentage of the remaining sections that are checked.
	Rate int
	// Seed makes the choice reproducible, and is the hash of the translation
	// prompt. Two runs of the same build pick the same pages, so a fix can
	// be checked against the sample that found the fault. A change to the
	// prompt picks a different sample, which is what the spec means by the
	// sample being refreshed when the prompt changes.
	Seed string
}

// Default is the policy from the spec.
func Default(seed string) Policy { return Policy{Top: 10, Rate: 5, Seed: seed} }

// Pick chooses the pages to check.
//
// Deterministic, and deliberately not math/rand. The draw is a hash of the
// seed and the page's path, which means the sample does not move when a
// paper is added to the corpus or when a page is translated in a different
// order. A sample that reshuffles itself every run cannot be used to tell
// whether a prompt change helped.
func Pick(pages []Page, p Policy) []Sample {
	var out []Sample
	for _, page := range pages {
		why := ""
		switch {
		case page.Kind == "front":
			why = "every abstract is checked"
		case page.Kind == "references":
			// Copied rather than asked for, which is rule L14. There is
			// nothing here a back-translation could disagree with.
			continue
		case page.Number > 0 && page.Number <= p.Top:
			why = fmt.Sprintf("paper %d of the canon, checked whole", page.Number)
		case draw(p.Seed, path(page)) < p.Rate:
			why = fmt.Sprintf("the %d%% sample", p.Rate)
		default:
			continue
		}
		out = append(out, Sample{
			Paper: page.Paper, Lang: page.Lang, Name: page.Name, Why: why, Model: page.Model,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

// draw turns a seed and a path into a number from 0 to 99.
func draw(seed, path string) int {
	// FNV-1a, written out rather than imported, because what this needs is a
	// number that does not move between builds and hash/fnv is a Hash32 with
	// a Write that returns an error nobody can get.
	const offset, prime = 2166136261, 16777619
	h := uint32(offset)
	for _, b := range []byte(seed + "\x00" + path) {
		h ^= uint32(b)
		h *= prime
	}
	return int(h % 100)
}

func path(p Page) string {
	return fmt.Sprintf("content/%s/%s/%s", p.Lang, p.Paper, p.Name)
}

// A Check is one page put back into English and judged.
type Check struct {
	Sample
	// English is the original, and Back is what came back. Both go in the
	// report, side by side, because a verdict nobody can check is an opinion.
	English string
	Back    string
	Verdict Verdict
	// Differences is the judge's list, one line each. Empty on a verdict of
	// same, and that is the judge's doing rather than this package's.
	Differences []string
	// BackModel and JudgeModel are who did each half, so a report can say
	// whether the second opinion was a second opinion.
	BackModel  string
	JudgeModel string
	// SameModel is true when the fleet had nothing else to offer and the
	// judge is the model that wrote the translation. The check still runs,
	// because a self-check catches a dropped paragraph even when it will not
	// catch a dropped negation, and the report says so on the line.
	SameModel bool
	Usage     llm.Usage
}

// A Checker runs the two asks.
//
// Ask is the caller's, for the same reason it is the caller's in translate:
// this package knows what question to put and the caller is what knows about
// routes. avoid is the model that wrote the translation, and the caller is
// expected to send the question somewhere else if it can.
type Checker struct {
	Ask  func(ctx context.Context, target, avoid string, req llm.Request) (translate.Reply, error)
	Logf func(string, ...any)
}

// Run puts one page back into English and judges it.
func (c *Checker) Run(ctx context.Context, s Sample, p translate.Paper, english, body string) (Check, error) {
	out := Check{Sample: s, English: strings.TrimSpace(english)}
	if strings.TrimSpace(body) == "" {
		return out, fmt.Errorf("%s: there is nothing in the file to check", s.Path())
	}

	back, err := prompt.Get(prompt.RoundtripBack)
	if err != nil {
		return out, err
	}
	text, err := back.Render(map[string]string{
		"SOURCE": p.ID,
		"FIELD":  string(p.Field),
		"BODY":   body,
	})
	if err != nil {
		return out, err
	}
	c.logf("%s: putting it back into English", s.Path())
	first, err := c.Ask(ctx, s.Path()+" back", s.Model, llm.Request{
		Instructions: text,
		Input:        "Write the passage between the equals signs in English, literally, and nothing else.",
	})
	if err != nil {
		return out, fmt.Errorf("%s: %w", s.Path(), err)
	}
	out.Back = strings.TrimSpace(first.Text)
	out.BackModel = first.Model
	out.Usage = add(out.Usage, first.Usage)
	if out.Back == "" {
		return out, fmt.Errorf("%s: the back-translation came back empty", s.Path())
	}

	judge, err := prompt.Get(prompt.RoundtripJudge)
	if err != nil {
		return out, err
	}
	text, err = judge.Render(map[string]string{
		"LANGUAGE": s.Lang.Name(),
		"ENGLISH":  out.English,
		"BACK":     out.Back,
	})
	if err != nil {
		return out, err
	}
	c.logf("%s: judging it against the English", s.Path())
	second, err := c.Ask(ctx, s.Path()+" judge", first.Model, llm.Request{
		Instructions: text,
		Input:        "Give the verdict line and the differences, and nothing else.",
	})
	if err != nil {
		return out, fmt.Errorf("%s: %w", s.Path(), err)
	}
	out.JudgeModel = second.Model
	out.Usage = add(out.Usage, second.Usage)
	out.SameModel = second.Model == s.Model || first.Model == s.Model

	out.Verdict, out.Differences, err = Parse(second.Text)
	if err != nil {
		return out, fmt.Errorf("%s: %w", s.Path(), err)
	}
	c.logf("%s: %s", s.Path(), out.Verdict)
	return out, nil
}

func (c *Checker) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// Parse reads the judge's answer.
//
// The verdict is looked for anywhere in the answer rather than only on the
// first line, because a judge that opens with a sentence of preamble has
// still judged and throwing the answer away costs two asks. What is not
// tolerated is an answer with no verdict in it at all, or with two different
// verdicts: the first is a judge that did something else, and the second is
// a judge arguing with itself, and neither is a result.
func Parse(answer string) (Verdict, []string, error) {
	var found Verdict
	var differences []string
	for _, line := range strings.Split(answer, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := cut(line, "verdict:"); ok {
			v := Verdict(strings.ToLower(strings.Trim(rest, " `*.")))
			if !known(v) {
				return "", nil, fmt.Errorf("the judge answered with a verdict of %q, which is not one of the three", rest)
			}
			if found != "" && found != v {
				return "", nil, fmt.Errorf("the judge answered with two verdicts, %s and %s", found, v)
			}
			found = v
			continue
		}
		if d := strings.TrimSpace(strings.TrimPrefix(line, "-")); d != line && d != "" {
			differences = append(differences, d)
		}
	}
	if found == "" {
		return "", nil, fmt.Errorf("the judge answered without a verdict line: %s", shorten(answer))
	}
	// A verdict of same with a list of differences under it is the judge
	// contradicting itself, and the differences are what it actually found.
	// Believing the list rather than the label is the conservative reading
	// and it is the one that puts a page in front of a person.
	if found == Same && len(differences) > 0 {
		found = Wording
	}
	if found == Same {
		differences = nil
	}
	return found, differences, nil
}

// cut finds a prefix on a line that may be quoted, bulleted or bolded, which
// is how a judge writes the verdict when it decides the answer is prose.
func cut(line, prefix string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimLeft(line, "-*` >#"))
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	return trimmed[len(prefix):], true
}

func known(v Verdict) bool {
	for _, k := range Verdicts {
		if k == v {
			return true
		}
	}
	return false
}

func shorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

func add(a, b llm.Usage) llm.Usage {
	a.InputTokens += b.InputTokens
	a.CachedInputTokens += b.CachedInputTokens
	a.OutputTokens += b.OutputTokens
	a.ReasoningTokens += b.ReasoningTokens
	a.TotalTokens += b.TotalTokens
	return a
}
