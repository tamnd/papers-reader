package extract

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/mathtex"
)

// The eight acceptance rules. A page is checked before it is written, and a
// page that fails goes back on the queue rather than into the corpus.
//
// They are numbered because the numbers end up in the queue, in the reports
// and in what a person says to another person about a paper that will not
// come out right. A rule that is only a sentence in a log line gets described
// three different ways by three different people.
const (
	// A1 is the one that catches a model answering the question instead of
	// doing the work.
	A1 = "A1 the page came back empty or as a refusal"
	A2 = "A2 a math delimiter was left open"
	A3 = "A3 a bracket is stranded outside its mathematics"
	A4 = "A4 a math span does not parse"
	A5 = "A5 the page is far shorter or longer than this paper's pages"
	A6 = "A6 the printed page number is not the one expected here"
	A7 = "A7 a code fence was left open"
	A8 = "A8 the page is marked illegible and is not recorded as damaged"
)

// A Fault is one rule one page broke.
type Fault struct {
	Rule string
	// Detail says what on the page broke it, and is written to be read on its
	// own in a queue entry.
	Detail string
	// Line is the body line to go and look at, counting from one, and zero
	// when the fault is about the page as a whole.
	Line int
}

func (f Fault) Error() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s (line %d): %s", f.Rule, f.Line, f.Detail)
	}
	return fmt.Sprintf("%s: %s", f.Rule, f.Detail)
}

// A Checker applies the eight rules to the pages of one paper.
//
// One per paper and not one per page, because rule A5 is about the paper: a
// page is too short relative to the other pages of the same paper, and there
// is no number of characters that means "too short" for both a 1967
// proceedings set in three columns and a modern preprint with a figure on
// every page. So a Checker accumulates as it goes and the rule only starts
// to bite once it has seen enough pages to have an opinion.
//
// The zero Checker works and applies every rule that needs no help. Give it
// a Renderer for A4 and a Map for A6.
type Checker struct {
	// Math is what A4 asks. Nil turns A4 off, which is what a caller with no
	// working KaTeX build gets, and it says so in the report rather than
	// passing pages nobody checked.
	Math *katex.Renderer
	// Map turns a page of the PDF into the number printed on it, for A6.
	Map Map
	// Damaged is the pages of this paper that are genuinely unreadable, from
	// errata.yaml. A page in here is allowed exactly one illegible marker.
	Damaged map[int]bool
	// Model says these pages were read by a model, which is what turns on
	// rule A5.
	//
	// A5 asks whether a page came back much shorter than this paper's pages
	// usually are, and the answer only means something when a page can come
	// back short: a model stops early, loses its place or answers about the
	// top half of the page. pdftotext cannot truncate a page. Run against
	// native extraction the rule refuses the three pages of attention
	// visualisations at the end of the Transformer paper, which are short
	// because they are pictures, and refusing a page that is correct is
	// worse than not asking.
	Model bool

	// n, mean and m2 are Welford's running variance over the lengths of the
	// pages that have been accepted so far. Running rather than a second pass
	// because extraction is resumable: a paper's pages come back over hours
	// and in no particular order, and a rule that needed all of them first
	// would be a rule that never ran.
	n        int
	mean, m2 float64
}

// MinLengthSample is how many pages have to have been seen before A5 says
// anything. Three pages of a paper tell you nothing about the fourth.
const MinLengthSample = 5

// Sigma is how far from this paper's usual page length a page may be. Three
// is wide on purpose: a section end, a page with a full page figure on it and
// the first page of a bibliography are all genuinely short, and the rule is
// here for the page that came back at a tenth of the usual length because it
// was truncated.
const Sigma = 3

// Slack is how far from the mean a page has to be before the rule applies at
// all, as a share of the mean. Both tests have to fail before a page is
// refused, and each one covers the other's blind spot.
//
// Three sigma alone is wrong in both directions. A paper whose pages are all
// within a few characters of one another has a standard deviation near zero,
// and three times near zero refuses a page that is a paragraph short. And a
// paper with one full page figure in it has a standard deviation wide enough
// that a page which came back as one line is inside it.
const Slack = 0.25

// Check applies every rule to one page and returns what it broke. An accepted
// page returns nothing and is counted toward the length statistics; a page
// that broke a rule is not counted, because a truncated page that widens the
// variance makes the next truncated page acceptable.
func (c *Checker) Check(page int, text string) []Fault {
	faults := c.faults(page, text)
	if len(faults) == 0 {
		c.observe(text)
	}
	return faults
}

// Accept is Check for a caller that wants one error or nil.
func (c *Checker) Accept(page int, text string) error {
	faults := c.Check(page, text)
	if len(faults) == 0 {
		return nil
	}
	msgs := make([]string, len(faults))
	for i, f := range faults {
		msgs[i] = f.Error()
	}
	return fmt.Errorf("page %d was refused: %s", page, strings.Join(msgs, "; "))
}

// Pages is how many pages have been accepted, which is what tells a report
// that A5 had enough to work with.
func (c *Checker) Pages() int { return c.n }

func (c *Checker) observe(text string) {
	n := float64(len(text))
	c.n++
	d := n - c.mean
	c.mean += d / float64(c.n)
	c.m2 += d * (n - c.mean)
}

func (c *Checker) faults(page int, text string) []Fault {
	var out []Fault
	add := func(rule, detail string, line int) {
		out = append(out, Fault{Rule: rule, Detail: detail, Line: line})
	}

	if strings.TrimSpace(text) == "" {
		add(A1, "there is nothing on the page", 0)
		return out
	}
	if s, at := refusal(text); s != "" {
		add(A1, fmt.Sprintf("the page reads like an answer to the question rather than the page: %q", s), at)
		return out
	}

	// The math rules read the page with the listings taken out of it. A
	// fenced block is not prose and not mathematics: a dollar in it is a shell
	// prompt or a price, a backslash is a path, and a brace is code. A table
	// this path could not carry as a pipe table is written in a fence too, and
	// the page it is on should not be refused for what the table prints.
	prose := mathtex.BlankFences(text)
	spans, unclosed := mathtex.Split(prose)
	if unclosed != nil {
		d := "$"
		if unclosed.Display {
			d = "$$"
		}
		add(A2, fmt.Sprintf("a %s was opened and never closed", d), unclosed.Line)
	}
	for _, s := range mathtex.Straddles(prose) {
		add(A3, fmt.Sprintf("the mathematics closes a bracket the prose opened: %s", mathtex.Strip(s.Text)), s.Line)
	}
	if c.Math != nil {
		for _, s := range spans {
			if err := c.Math.Valid(s.Text, s.Display); err != nil {
				add(A4, fmt.Sprintf("KaTeX will not read %q: %v", mathtex.Strip(s.Text), err), s.Line)
			}
		}
	}

	// A5 is the only rule that can be wrong about a page that is perfectly
	// fine, so it is worded as a doubt and the report says so.
	if c.Model && c.n >= MinLengthSample {
		if d := math.Abs(float64(len(text)) - c.mean); d > Sigma*c.stddev() && d > Slack*c.mean {
			add(A5, fmt.Sprintf("the page is %d characters where this paper's pages average %.0f", len(text), c.mean), 0)
		}
	}

	if printed, ok := c.Map.Printed(page); ok {
		if got, found := folioOf(text); found && got != printed {
			add(A6, fmt.Sprintf("the page prints %d and page %d of the file should print %d", got, page, printed), 0)
		}
	}

	if n := len(fence.FindAllString(text, -1)); n%2 == 1 {
		add(A7, "a code fence was opened and never closed", 0)
	}
	// This one reads the page as it is rather than with the fences blanked,
	// because it is the fences it is about. A fence inside a formula is a page
	// that came back with its blocks tangled, and blanking would take the
	// evidence away before the rule saw it.
	raw, _ := mathtex.Split(text)
	for _, s := range raw {
		if strings.Contains(s.Text, "```") {
			add(A7, "a code fence was opened inside a math span", s.Line)
		}
	}

	if markers := Illegible.FindAllString(text, -1); len(markers) > 0 {
		switch {
		case !c.Damaged[page]:
			add(A8, fmt.Sprintf("the page is marked %s and is not in errata.yaml", markers[0]), 0)
		case len(markers) > 1:
			add(A8, fmt.Sprintf("a damaged page is allowed one illegible marker and this one has %d", len(markers)), 0)
		}
	}
	return out
}

func (c *Checker) stddev() float64 {
	if c.n < 2 {
		return 0
	}
	return math.Sqrt(c.m2 / float64(c.n-1))
}

// fence is a line that opens or closes a code block. Matched at the start of
// a line rather than counted as a substring, because a page whose very first
// block is a listing has its opening fence at the start of the text with no
// newline in front of it, and a count of "\n```" sees the closing fence and
// not the opening one. That page reads as a fence that was never closed, and
// the layout path writes it on any paper whose page starts with an
// algorithm.
var fence = regexp.MustCompile("(?m)^[ \t]*(```|~~~)")

// Illegible is the markers the prompt asks a reader to leave where a page is
// unreadable. Anything else it invents is caught by A1 or by a person.
//
// Exported because audit rule M05 reads the committed corpus for the same
// markers, and a second list of them would be a list that drifted from this
// one. The marker is allowed here, on a page recorded as damaged, and it is
// allowed nowhere in what gets published.
var Illegible = regexp.MustCompile(`\[\?\]|⟨illegible⟩|\[illegible\]`)

// refusals is what a model says when it will not do the work, and what a
// gateway says when it is the one answering. Matched case insensitively
// against the first part of the page, because a page whose body quotes one of
// these sentences in the middle of a paragraph about model behaviour is a
// page this corpus will eventually contain.
var refusals = []string{
	"i'm sorry",
	"i am sorry",
	"i cannot",
	"i can't",
	"i'm unable",
	"i am unable",
	"as an ai",
	"i apologize",
	"i apologise",
	"unable to process",
	"unable to assist",
	"cannot assist with",
	"no text is visible",
	"the image does not contain",
	"rate limit",
	"error code",
	"bad gateway",
	"service unavailable",
	"<!doctype html",
	"<html",
}

// refusalHead is how much of a page is looked at. A refusal is the whole
// answer or the start of it, and a page of a paper that happens to quote one
// of these sentences quotes it somewhere in the middle.
const refusalHead = 400

func refusal(text string) (string, int) {
	head := strings.ToLower(strings.TrimSpace(text))
	if len(head) > refusalHead {
		head = head[:refusalHead]
	}
	for _, r := range refusals {
		if i := strings.Index(head, r); i >= 0 {
			return r, 1 + strings.Count(head[:i], "\n")
		}
	}
	return "", 0
}

// folioOf is the page number the page itself prints, from a line that is a
// number and nothing else. It is what A6 compares against the page map, and
// it finds nothing on most pages, which is not a failure: a page that prints
// no number cannot print the wrong one.
func folioOf(text string) (int, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || !folio.MatchString(l) {
			continue
		}
		n, err := strconv.Atoi(strings.Trim(l, "-[]() "))
		if err == nil {
			return n, true
		}
	}
	return 0, false
}
