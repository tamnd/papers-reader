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

// The eleven acceptance rules. A page is checked before it is written, and a
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
	A9 = "A9 the page skips a stretch of the file's own text layer"
	// A10 is the one that catches a model writing the web instead of the
	// page. See markup.
	A10 = "A10 the page came back with markup the corpus has no spelling for"
	// A11 is the one that catches mathematics flattened into the prose.
	A11 = "A11 the page is doing mathematics and has not one math span"
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

// A Checker applies the ten rules to the pages of one paper.
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
	// Layer is the page's own text layer, for A9, and nil turns A9 off.
	//
	// It is a function and not a map because reading a text layer is a
	// process per page and most pages never need one: the rule only runs on a
	// page a model read, on a paper whose layer is worth comparing against.
	// The second return says there is no layer for that page, which is not
	// the same as an empty one.
	//
	// Whether a layer is worth comparing against is the caller's to decide
	// and it is not a detail. A scan's OCR layer is itself a reading, and a
	// worse one, so where the two disagree the layer is as likely to be the
	// wrong one: over the Codd paper the words it says are missing are
	// "ficlds", "fczc" and "segnren". That scan is a good scan and it stays
	// well clear of MinCoverage, but nothing says the next one will, and a
	// rule that refuses a page for not reproducing a typo would be worse than
	// no rule. Born digital only.
	Layer func(page int) (string, bool)
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

	// ln and lmean are the same running mean over how much prose the file's
	// own text layer holds on each of those pages, which is what lets A5 ask
	// whether a short answer is a short page or a short reading. Counted
	// separately from n because a paper can have a layer on some pages and
	// not on others, and an average over the pages that have one is the only
	// average worth comparing a page that has one against.
	ln    int
	lmean float64
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
		c.observe(page, text)
	}
	return faults
}

// Faults is Check without counting the page toward the length statistics.
//
// It is for a caller that has already shown the checker every page and is
// now going back over them to report. Counting a page twice would make rule
// A5's sample half of it the same pages over again, which narrows the
// variance it is measured against and is a quiet way of making the rule
// stricter than it was written to be.
func (c *Checker) Faults(page int, text string) []Fault { return c.faults(page, text) }

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

func (c *Checker) observe(page int, text string) {
	n := float64(len(text))
	c.n++
	d := n - c.mean
	c.mean += d / float64(c.n)
	c.m2 += d * (n - c.mean)
	if l, ok := c.layer(page); ok {
		c.ln++
		c.lmean += (l - c.lmean) / float64(c.ln)
	}
}

// tooShort is what A5 says about a page, which is two different sentences.
//
// When the expectation came from the page's own text layer, saying the paper
// averages 4426 characters would be telling a person the wrong thing to go
// and check: the complaint is not that the page is unlike the others, it is
// that the reading is unlike the page. Naming the file's own count is what
// sends them to the right place.
func (c *Checker) tooShort(text string, want float64, fromLayer bool) string {
	if fromLayer {
		return fmt.Sprintf("the page is %d characters where the file's own text layer on it is worth about %.0f", len(text), want)
	}
	return fmt.Sprintf("the page is %d characters where this paper's pages average %.0f", len(text), c.mean)
}

// layer is how much prose the file's own text layer holds on a page, and
// whether it holds any. A paper read by a model off a scan has no layer and
// this says so on every page of it.
func (c *Checker) layer(page int) (float64, bool) {
	if c.Layer == nil {
		return 0, false
	}
	text, ok := c.Layer(page)
	if !ok {
		return 0, false
	}
	return float64(len(text)), true
}

// expected is how long a reading of this page ought to be, in characters,
// given that it came back got characters long.
//
// The paper's average, scaled by how much prose the file says is on this
// page against how much it says is on an average page of it. A page with a
// full page figure on it holds a third of the words of an ordinary page and
// its reading is a third as long, and A5 refusing it for that was the whole
// of issue #47: three pages of the Transformer paper, nine asks of a
// rationed reader, and the answer was right the first time.
//
// The scaling is what keeps the rule's teeth rather than blunting them. A
// reading that stopped half way down a full page of prose is short while the
// layer says the page is full, so the expectation stays high and the gap is
// as wide as it ever was. It is only a page the file itself calls short that
// is now allowed to come back short.
//
// Prose is a floor under what is on a page and not an estimate of it: it
// drops the labels inside a figure and the cells of a table by design, and a
// reader transcribes a table. So the file can say a page is at least this
// full and it cannot say a page is at most this full, and a floor can raise
// what is expected of a reading and must never lower it. That is the whole
// of the rule here: the expectation is the paper's average scaled by the
// layer, or the paper's average, whichever is the larger.
//
// Both halves of that were paid for. Page 11 of the ResNet paper is 6062
// characters of correct reading where the layer on it is worth 1848, because
// most of the page is a table and a table is not prose, and scaling the
// expectation down to match the layer would refuse it. Page 63 of the GPT-3
// paper is the other way about: 11556 characters of correct reading against
// a paper that averages 3188, because the page is one appendix table of
// every score in the paper, and the layer on it is worth 8744 and says so.
// Measuring that against the plain average refused nine correct readings in
// a row and left the page written out as a fence with most of its rows
// missing.
//
// It needs a sample of pages that have a layer before it means anything, the
// same as the mean it scales, and on a paper read off a scan there is no
// layer at all and this is the plain average.
func (c *Checker) expected(page, got int) (float64, bool) {
	if c.ln < MinLengthSample || c.lmean <= 0 {
		return c.mean, false
	}
	l, ok := c.layer(page)
	if !ok {
		return c.mean, false
	}
	want := c.mean * l / c.lmean
	if float64(got) >= c.mean && want <= c.mean {
		return c.mean, false
	}
	return want, true
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
		want, fromLayer := c.expected(page, len(text))
		if d := math.Abs(float64(len(text)) - want); d > Sigma*c.stddev() && d > Slack*want {
			add(A5, c.tooShort(text, want, fromLayer), 0)
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

	if c.Layer != nil {
		if layer, ok := c.Layer(page); ok {
			if share, missing := Coverage(layer, text); share < MinCoverage {
				add(A9, fmt.Sprintf("a run of %d words in the page's own text layer comes back only %.0f%% accounted for, missing %s",
					Window, share*100, quoteFew(missing)), 0)
			}
		}
	}

	// A11 is the page whose mathematics was dissolved rather than mangled.
	//
	// Rules A2, A3 and A4 all read the math spans and ask whether they are
	// right, so a page with no spans at all passes every one of them and
	// goes into the corpus with its formulas written out as words. The
	// Paxos paper is six pages of set theory read that way: a hundred and
	// fifty six characters that are nothing but mathematics, not one dollar
	// sign, and every math rule in the audit reporting that it had nothing
	// to look at.
	//
	// pdftotext cannot write a delimiter, so on the native path this is a
	// page that has to be read by something that can, and refusing it here
	// is what sends it to the vision path on the next pass. The threshold
	// is mathtex.Enough, because one of these characters is a glyph that
	// wandered into a sentence and three is a page doing mathematics.
	//
	// The listings are taken out first, the same as the math rules above
	// and for the same reason. An unclosed span counts as a span, because
	// that page is already A2's and two findings about one fault is one
	// finding too many.
	if n := mathtex.Signs(prose); n >= mathtex.Enough && len(spans) == 0 && unclosed == nil {
		add(A11, fmt.Sprintf("the page carries %d characters that are nothing but mathematics and not one math span, so its formulas were flattened into the prose", n), mathtex.FirstSign(prose))
	}

	if tag, n := markup(text); n > 0 {
		add(A10, fmt.Sprintf("%q is HTML, and there are %d tags on this page the tidier could not convert", tag, n), 0)
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

// quoteFew is a handful of the missing words, for a queue entry somebody has
// to read. Six of them out of a window of twenty is enough to recognise a
// sentence by, and on the page this rule was written for they read "should
// noted where depends several those", which is the dropped paragraph.
func quoteFew(words []string) string {
	const few = 6
	if len(words) > few {
		words = words[:few]
	}
	return strings.Join(words, ", ")
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
