package audit

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/katex"
	"github.com/tamnd/papers-reader/mathtex"
)

// mathematicsRules is group M, and it is carried over from
// bourbaki-solver/quality/math.go with the reasoning intact, because the
// faults are the same faults. A PDF's text layer holds a formula as a run of
// glyphs with no structure, and everything that puts the structure back can
// put it back wrong in a small number of ways that this group enumerates.
//
// Every rule works off mathtex.Split and none of them decides for itself
// where the mathematics is. Two opinions about that would disagree about the
// same file, and the rule that said the file was fine would be the one nobody
// had read.
func mathematicsRules() []Rule {
	return []Rule{
		{
			ID: "M01", Hard: true,
			What:  "every math span is closed.",
			Check: ruleM01,
		},
		{
			ID: "M02", Hard: true,
			What:  "the number sets are written with \\mathbb, consistently.",
			Check: ruleM02,
		},
		{
			ID: "M03", Hard: true,
			What:  "no character is stranded out of its TeX.",
			Check: ruleM03,
		},
		{
			ID: "M04", Hard: true,
			What:  "every math span parses under KaTeX.",
			Check: ruleM04,
		},
		{
			ID: "M05", Hard: true,
			What:  "no illegible marker is left in the corpus.",
			Check: ruleM05,
		},
		{
			ID:    "M06",
			What:  "displays per page are within 3 sigma of the paper's mean.",
			Check: ruleM06,
		},
		{
			ID: "M07", Hard: true,
			What:  "no bracket from the prose closes inside the mathematics.",
			Check: ruleM07,
		},
		{
			ID: "M08", Hard: true,
			What:  "no matrix is left flattened into a pair of scripts.",
			Check: ruleM08,
		},
		{
			ID:    "M09",
			What:  "no base carries two superscripts or two subscripts.",
			Check: ruleM09,
		},
		{
			ID: "M10", Hard: true,
			What:  "no relation sign has lost the stroke that negates it.",
			Check: ruleM10,
		},
		{
			ID: "M11", Hard: true,
			What:  "the mathematics is written between dollars, never \\( or \\[.",
			Check: ruleM11,
		},
		{
			ID:    "M12",
			What:  "an inline formula is written tight against its dollars.",
			Check: ruleM12,
		},
		{
			ID: "M13", Hard: true,
			What:  "no $ inside a fenced code block opened a span.",
			Check: ruleM13,
		},
		{
			ID: "M14", Hard: true,
			What:  "a paper with mathematics in its prose has mathematics in its markup.",
			Check: ruleM14,
		},
	}
}

// eachSpan runs a check over every math span of every file that parsed. The
// group stands down on a corpus with no mathematics in it at all, which is
// the state of a corpus before extraction has run and not a pass.
func eachSpan(in *Input, rule string, check func(mathtex.Span) string) ([]Finding, error) {
	if !anyMath(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		spans, _ := mathtex.Split(f.Body)
		for _, s := range spans {
			if msg := check(s); msg != "" {
				out = append(out, Finding{Rule: rule, File: f.Path, Line: s.Line, Message: msg})
			}
		}
	}
	return out, nil
}

// eachMathFile is eachSpan for the rules that read the body rather than the
// spans: the ones about a delimiter that never opened a span, or a marker
// that is nowhere near the mathematics.
func eachMathFile(in *Input, rule string, check func(*File) []Finding) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		out = append(out, check(f)...)
	}
	return out, nil
}

// anyMath says whether the corpus has any mathematics in it. A paper with no
// formulae in it is normal and a corpus with none is a corpus that has not
// been extracted yet, and the two have to be told apart: the rules would all
// pass on the second and the pass would mean nothing.
func anyMath(in *Input) bool {
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		if spans, unclosed := mathtex.Split(f.Body); len(spans) > 0 || unclosed != nil {
			return true
		}
	}
	return false
}

// ruleM01 is the cheapest rule in the audit and it has found the most. An
// unclosed dollar swallows everything after it: the rest of the paragraph
// renders as italic mathematics, and in a translation the model reads it as
// mathematics and leaves it untranslated.
func ruleM01(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M01", func(f *File) []Finding {
		_, unclosed := mathtex.Split(f.Body)
		if unclosed == nil {
			return nil
		}
		d := "$"
		if unclosed.Display {
			d = "$$"
		}
		return []Finding{{
			Rule: "M01", File: f.Path, Line: unclosed.Line,
			Message: fmt.Sprintf("a %s was opened here and never closed", d),
		}}
	})
}

// numberSet matches a number set set in any font but the one the corpus uses,
// and the bare Unicode letter as well. The five letters are the five sets and
// nothing else: \mathbf{A} is a vector in half the corpus and a matrix in the
// other half, and neither is a number set.
var numberSet = regexp.MustCompile(`\\(mathbf|mathrm|bf|boldsymbol)\{([NZQRC])\}|[ℕℤℚℝℂ]`)

// ruleM02 asks for one spelling, not for the right one.
//
// Papers are inconsistent with each other and inconsistent inside themselves,
// so fidelity to the source is not a property anybody can check. The corpus
// picks \mathbb, normalises on the way in, and this rule checks that the
// normalisation ran. A reader moving between four papers should not have to
// work out that one paper's bold R is another paper's blackboard R.
func ruleM02(in *Input) ([]Finding, error) {
	return eachSpan(in, "M02", func(s mathtex.Span) string {
		m := numberSet.FindString(s.Text)
		if m == "" {
			return ""
		}
		return fmt.Sprintf("the number set is written %s and the corpus writes \\mathbb: %s", m, mathtex.Strip(s.Text))
	})
}

// ruleM03 is the audit half of mathtex.Repair. The repair runs during
// extraction and writes each stranded character as the TeX it stands for;
// this asks whether the committed corpus still has any, which covers the
// spans the repair refused and the text somebody has edited since.
//
// A stranded character is a glyph inside a math span that the text layer
// handed over as a letter rather than as a command: an α where the paper set
// \alpha, a ∈ where it set \in. It sets nearly right, which is what makes it
// worth a rule. It takes its shape from whatever font the browser falls back
// to rather than from the one KaTeX sets the rest of the formula in, it does
// not match a search for the command, and a translator hands it through into
// three languages unchanged.
func ruleM03(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M03", func(f *File) []Finding {
		fixed, n, refused := mathtex.Repair(f.Body)
		var out []Finding
		if n > 0 && fixed != f.Body {
			out = append(out, Finding{
				Rule: "M03", File: f.Path,
				Message: fmt.Sprintf("%s stranded out of its TeX, and papers extract would write it out", plural(n, "character")),
			})
		}
		for _, r := range refused {
			out = append(out, Finding{
				Rule: "M03", File: f.Path, Line: r.Line,
				Message: fmt.Sprintf("%s, and the repair will not guess: %s", r.Why, oneLine(r.Span)),
			})
		}
		return out
	})
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

// ruleM04 is the only rule in the group that asks the thing that will
// actually set the formula. The others encode a shape somebody has seen go
// wrong; this one asks KaTeX, which is what the reading app runs, so a span
// that passes here renders in the browser and a span that fails here is a
// hole on the page.
//
// It builds its own renderer rather than taking one from Input, because
// loading KaTeX costs a tenth of a second and every other rule in the audit
// should not pay it.
func ruleM04(in *Input) ([]Finding, error) {
	if !anyMath(in) {
		return nil, ErrNotRun
	}
	r, err := katex.New()
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		spans, _ := mathtex.Split(f.Body)
		for _, s := range spans {
			if err := r.Valid(s.Text, s.Display); err != nil {
				out = append(out, Finding{
					Rule: "M04", File: f.Path, Line: s.Line,
					Message: fmt.Sprintf("KaTeX will not read %s: %v", oneLine(mathtex.Strip(s.Text)), err),
				})
			}
		}
	}
	return out, nil
}

// ruleM05 is not really about mathematics, and it is here because the marker
// it looks for is written by the same reader that writes the formulae and for
// the same reason: the page was hard to read.
//
// A marker is allowed during extraction on a page that errata.yaml records as
// damaged. It is allowed nowhere in what gets published. The corpus either
// has the text or says in its front matter that the paper could not be read,
// and a published page with [?] in the middle of it is neither.
func ruleM05(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M05", func(f *File) []Finding {
		var out []Finding
		for n, line := range strings.Split(f.Body, "\n") {
			for _, m := range extract.Illegible.FindAllString(line, -1) {
				out = append(out, Finding{
					Rule: "M05", File: f.Path, Line: n + 1,
					Message: fmt.Sprintf("%s is an illegible marker, and the published corpus carries none", m),
				})
			}
		}
		return out
	})
}

// minDisplaySample is how many files of a paper have to record which pages
// they came from before the count of one of them means anything. A paper with
// two mathematical sections tells you nothing about the third.
const minDisplaySample = 5

// ruleM06 is the one rule in the group that is about a quantity rather than a
// shape, and it is soft because the thing it measures is allowed to vary.
//
// What it is for: a section that lost its display mathematics. The formulae
// come out of the text layer as prose, the extraction writes the prose, and
// nothing anywhere is malformed. Every other rule in this group passes. The
// only sign is that one section of a paper full of equations has none.
//
// Per page rather than per file, because the files are sections and a section
// is as long as the author made it. The page count comes out of the front
// matter, which is where the splitter recorded which pages of the PDF the
// file was cut from.
//
// The mean and the deviation a section is compared against are taken over
// the other sections and not over all of them, which is the difference
// between a rule that fires and a rule that cannot. A point that is in the
// sample pulls the mean toward itself and widens the deviation, and with n
// sections the furthest any one of them can be from a mean it is part of is
// (n-1)/sqrt(n) deviations: under twelve sections that is less than three, so
// a rule phrased against the whole sample would be arithmetically incapable
// of reporting anything on a paper of ordinary length. Leaving the section
// out is also the question the rule is actually asking, which is whether this
// section looks like the rest of the paper.
func ruleM06(in *Input) ([]Finding, error) {
	if !anyMath(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	groups := byPaper(in.Content)
	sampled := 0
	for _, key := range keys(groups) {
		rates, files := displayRates(groups[key])
		if len(rates) < minDisplaySample {
			continue
		}
		// A paper with no displays anywhere is a paper this rule has no
		// opinion about. Every section matches every other and the one
		// section that does have a display would be the finding, which is
		// the wrong way round.
		overall, _ := meanSD(rates)
		if overall == 0 {
			continue
		}
		sampled++
		for i, rate := range rates {
			mean, sd := meanSD(without(rates, i))
			// Below the rest of the paper and not away from it. A section
			// with more displays than its neighbours is the section the
			// mathematics is in, and every paper has one: nakamoto keeps all
			// of its algebra in the calculations section and vaswani keeps
			// all of its in the model architecture, so a two sided test
			// reports the best read section of every paper in the corpus and
			// says nothing about the one this rule was written for. A section
			// that is short of displays is the interesting direction and the
			// only one.
			d := mean - rate
			// Both tests have to fail, as in acceptance rule A5. Three
			// deviations alone refuses a section that is one display short of
			// a paper whose sections are otherwise identical, and a share of
			// the paper's own rate alone says nothing about a paper that
			// varies honestly.
			if d <= extract.Sigma*sd || d <= extract.Slack*overall {
				continue
			}
			msg := fmt.Sprintf("%.1f displays a page where the rest of this paper runs %.1f", rate, mean)
			if sd > 0 {
				msg += fmt.Sprintf(", which is %.1f sigma out", d/sd)
			} else {
				msg += ", and the rest of it does not vary at all"
			}
			out = append(out, Finding{Rule: "M06", File: files[i].Path, Message: msg})
		}
	}
	if sampled == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// without is v with one element left out. It allocates rather than swapping
// in place, because the caller is iterating over v.
func without(v []float64, i int) []float64 {
	out := make([]float64, 0, len(v)-1)
	out = append(out, v[:i]...)
	return append(out, v[i+1:]...)
}

// displayRates is the displays per page of each file of one paper that says
// how many pages it came from. A file with no page range recorded is left out
// rather than counted as one page, because a wrong denominator here produces
// a finding about a file that is perfectly fine.
func displayRates(files []*File) ([]float64, []*File) {
	var rates []float64
	var kept []*File
	for _, f := range files {
		if f.Broken() {
			continue
		}
		pages := pageSpan(f.Front.PDFPages)
		if pages <= 0 {
			continue
		}
		n := 0
		spans, _ := mathtex.Split(f.Body)
		for _, s := range spans {
			if s.Display {
				n++
			}
		}
		rates = append(rates, float64(n)/float64(pages))
		kept = append(kept, f)
	}
	return rates, kept
}

// pageSpan reads how many pages a file came from out of the pdf_pages field,
// which is written either as one number or as a range.
func pageSpan(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	from, to, ok := strings.Cut(s, "-")
	a, err := strconv.Atoi(strings.TrimSpace(from))
	if err != nil {
		return 0
	}
	if !ok {
		return 1
	}
	b, err := strconv.Atoi(strings.TrimSpace(to))
	if err != nil || b < a {
		return 0
	}
	return b - a + 1
}

func meanSD(v []float64) (mean, sd float64) {
	for _, x := range v {
		mean += x
	}
	mean /= float64(len(v))
	for _, x := range v {
		sd += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(sd / float64(len(v)-1))
}

// ruleM07 is the fault that makes a sentence unreadable without making the
// mathematics wrong. The prose opens a bracket, the formula inside it closes
// it, and the delimiter has landed on the wrong side of the dollar: the
// formula sets with a stray bracket hanging off it and the sentence never
// closes the one it opened.
func ruleM07(in *Input) ([]Finding, error) {
	if !anyMath(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		fenced := codeRanges(f.Body)
		for _, s := range mathtex.Straddles(f.Body) {
			// A span that opens inside a listing is M13's finding and not
			// this one. The dollar in McCabe's `FORMAT(DOMOLKI STRUCTURE
			// FILE NAME? $)` is a FORTRAN format descriptor, the span it
			// opened ran to the next dollar six lines down, and the bracket
			// it was reported for closing is part of the FORTRAN.
			if inRanges(fenced, s.Line) {
				continue
			}
			out = append(out, Finding{
				Rule: "M07", File: f.Path, Line: s.Line,
				Message: fmt.Sprintf("the mathematics closes a bracket the prose opened: %s", oneLine(mathtex.Strip(s.Text))),
			})
		}
	}
	return out, nil
}

// ruleM08 catches a matrix that the text layer handed over as a pair of
// scripts. A two by two matrix drawn in the PDF comes back as ^{a b}_{c d},
// which renders as a superscript over a subscript: the rows are there, the
// brackets are not, and it looks enough like mathematics that a reader
// skimming will not stop.
func ruleM08(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M08", func(f *File) []Finding {
		var out []Finding
		for _, row := range mathtex.StackedRows(f.Body) {
			out = append(out, Finding{
				Rule: "M08", File: f.Path,
				Message: fmt.Sprintf("%s is a matrix flattened into a pair of scripts", oneLine(row)),
			})
		}
		return out
	})
}

// ruleM09 is soft because TeX allows what it reports and a person sometimes
// means it. A base with two superscripts does not set, so KaTeX catches the
// ones that are outright broken and M04 reports those; what is left here is
// the chain that does set and sets as something other than what the paper
// printed.
func ruleM09(in *Input) ([]Finding, error) {
	return eachSpan(in, "M09", func(s mathtex.Span) string {
		doubles := mathtex.DoubleScripts(s.Text)
		if len(doubles) == 0 {
			return ""
		}
		return fmt.Sprintf("%s carries two marks of the same kind on one base", oneLine(doubles[0]))
	})
}

// ruleM10 is the smallest difference in the group and the largest in meaning.
// A relation with the stroke lost says the opposite of what the paper said:
// "a is not congruent to b" becomes "a is congruent to b", and nothing
// anywhere looks wrong.
//
// The stroke comes off in the text layer because it is drawn over the sign
// rather than set as part of it, so the glyph run holds the sign and a
// solidus beside it.
func ruleM10(in *Input) ([]Finding, error) {
	if !anyMath(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		spans, signs := mathtex.Strokes(f.Body)
		for i, s := range spans {
			out = append(out, Finding{
				Rule: "M10", File: f.Path, Line: s.Line,
				Message: fmt.Sprintf("%s has lost the stroke that negates it, so the formula says the opposite: %s",
					signs[i], oneLine(mathtex.Strip(s.Text))),
			})
		}
	}
	return out, nil
}

// texDelim matches the LaTeX delimiters the corpus does not use. The escape
// is doubled in the pattern because these are backslashes in the file.
var texDelim = regexp.MustCompile(`\\\(|\\\)|\\\[|\\\]`)

// ruleM11 is about having one way to write a thing. \( and $ mean the same in
// LaTeX and they do not mean the same to anything downstream: mathtex.Split
// reads dollars, the reading app's renderer is configured for dollars, and a
// span written with \( is a span every rule in this group walks straight
// past. A rule that can be evaded by writing the same mathematics another way
// is not a rule.
func ruleM11(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M11", func(f *File) []Finding {
		var out []Finding
		for n, line := range prose(f.Body) {
			// Inside the mathematics \[ is a spacing command and \) closes a
			// group somebody opened, so only the prose is read.
			if m := texDelim.FindString(blankMath(line)); m != "" {
				out = append(out, Finding{
					Rule: "M11", File: f.Path, Line: n + 1,
					Message: fmt.Sprintf("%s opens or closes mathematics, and the corpus writes dollars", m),
				})
			}
		}
		return out
	})
}

// blankMath is one line with its math spans replaced by spaces, so a rule
// about the prose does not read the mathematics.
func blankMath(line string) string {
	spans, _ := mathtex.Split(line)
	if len(spans) == 0 {
		return line
	}
	rs := []rune(line)
	out := append([]rune(nil), rs...)
	for _, s := range spans {
		for i := s.Start; i < s.End && i < len(out); i++ {
			out[i] = ' '
		}
	}
	return string(out)
}

// ruleM12 is the smallest rule in the audit and the one a reader notices
// most. KaTeX sets `$ x $` with the space inside the formula, so the symbol
// sits a space away from the word before it and the line looks loose. It is
// soft because it is a matter of setting rather than of meaning.
func ruleM12(in *Input) ([]Finding, error) {
	return eachSpan(in, "M12", func(s mathtex.Span) string {
		if s.Display {
			// A display is set on its own lines and the extraction puts a
			// newline after the delimiter deliberately.
			return ""
		}
		switch {
		case strings.HasPrefix(s.Text, " "), strings.HasPrefix(s.Text, "\t"):
			return fmt.Sprintf("the formula opens with a space inside the dollars: %s", oneLine(s.Text))
		case strings.HasSuffix(s.Text, " "), strings.HasSuffix(s.Text, "\t"):
			return fmt.Sprintf("the formula closes with a space inside the dollars: %s", oneLine(s.Text))
		}
		return ""
	})
}

// ruleM13 is the one interaction between mathematics and code that has to be
// got right. A shell listing full of $PATH and $HOME opens a math span on
// every other line, and by the end of the file every rule in this group is
// reading a listing as mathematics. The Eléments had no code in them, so this
// rule is new and the corpus it is for is this one.
//
// A listing in a language that writes a dollar of its own is not asked
// about, and code.Sigil is the same answer the repair uses. McCabe's FORTRAN
// was three findings nobody could act on: `FORMAT(DOMOLKI STRUCTURE FILE
// NAME? $)` is a format descriptor and `CALL READB(...,$990,$990)` is a pair
// of alternate return labels, and both are what the page printed. A shell
// listing is the case the rule was written for and it is the same case, so
// the rule can no longer see the thing it was written to find. That is the
// right trade: the dollars in a shell listing are not a defect either, they
// are the program, and what the rule is really for is the page where a
// reader marked up a listing as mathematics.
func ruleM13(in *Input) ([]Finding, error) {
	return eachMathFile(in, "M13", func(f *File) []Finding {
		var fenced [][2]int
		blocks, _ := code.Blocks(f.Body)
		for _, b := range blocks {
			if code.Sigil(b.Lang) {
				continue
			}
			end := b.End
			if b.End == 0 {
				end = strings.Count(f.Body, "\n") + 1
			}
			fenced = append(fenced, [2]int{b.Line, end})
		}
		if len(fenced) == 0 {
			return nil
		}
		var out []Finding
		prose := commentClause(f.Body)
		spans, unclosed := mathtex.Split(f.Body)
		if unclosed != nil {
			spans = append(spans, *unclosed)
		}
		for _, s := range spans {
			if !inRanges(fenced, s.Line) || prose[s.Line] {
				continue
			}
			out = append(out, Finding{
				Rule: "M13", File: f.Path, Line: s.Line,
				Message: fmt.Sprintf("a $ inside a fenced code block opened a math span: %s", oneLine(mathtex.Strip(s.Text))),
			})
		}
		return out
	})
}

// commentClause marks the lines of a body that fall inside an ALGOL comment
// clause, counting lines from one.
//
// A comment clause is the one place inside a listing where mathematics set
// as mathematics is right. ALGOL 60 writes documentation as `comment` and
// then English up to the next semicolon, and it is English: Pfaltz explains
// his line integral procedure by giving the Riemann-Stieltjes sum it
// approximates, and Floyd's LOGC says which principal value it computes.
// The reader wrote both as math spans and both are inside the fence, and
// they were the last three M13 findings in the corpus.
//
// Flattening them is not an option. A sum written as the word sum is a lie
// about the page, which is the whole argument code.Unmath rests on, and it
// cuts the other way here: the page really did print a formula. So the rule
// looks away instead.
//
// The keyword has to be at the head of a line, which is where ALGOL sets it
// and where both of these are. A clause opened in the middle of a line is
// left for the rule to report, because that shape has not turned up and a
// rule that guessed at it would be guessing.
func commentClause(body string) map[int]bool {
	out := map[int]bool{}
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if !commentKeyword.MatchString(lines[i]) {
			continue
		}
		rest := lines[i][strings.Index(strings.ToLower(lines[i]), "comment")+len("comment"):]
		for {
			out[i+1] = true
			if strings.Contains(rest, ";") || i+1 >= len(lines) {
				break
			}
			i++
			rest = lines[i]
		}
	}
	return out
}

// commentKeyword is the ALGOL comment keyword at the head of a line. Upper
// case is allowed because the older papers set their listings that way.
var commentKeyword = regexp.MustCompile(`^\s*(?i:comment)\b`)

// codeRanges is the line ranges of the fenced code blocks in a body,
// inclusive of the fences, counting lines from one.
func codeRanges(body string) [][2]int {
	var out [][2]int
	open := 0
	for i, line := range strings.Split(body, "\n") {
		if !codeFence.MatchString(line) {
			continue
		}
		if open == 0 {
			open = i + 1
			continue
		}
		out = append(out, [2]int{open, i + 1})
		open = 0
	}
	if open != 0 {
		// A fence that never closed runs to the end of the file, which is how
		// the file reads on the page. Rule A7 refuses the page during
		// extraction, so this is here for a file somebody has edited.
		out = append(out, [2]int{open, strings.Count(body, "\n") + 1})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func inRanges(ranges [][2]int, line int) bool {
	for _, r := range ranges {
		if line >= r[0] && line <= r[1] {
			return true
		}
	}
	return false
}

// notation is the characters that appear in mathematics and do not appear in
// English prose.
//
// The list is short on purpose. A middle dot separates authors, a times sign
// gives the size of a kernel, a plus-or-minus reports an error bar and an
// arrow points at something in a figure, so none of those is here. Greek
// letters are not here either: a paper can name one in a sentence about
// notation without writing any mathematics at all.
//
// What is left is glyphs a sentence has no use for. A body carrying several
// of them is a body with mathematics in it, whatever its markup says.
// notation and enough live in mathtex, because the extraction path asks the
// same question of one page that this rule asks of a whole paper. Two lists
// of what counts as mathematics would be two lists that drifted.
const notation, enough = mathtex.Notation, mathtex.Enough

// ruleM14 catches the mathematics that was flattened rather than mangled.
//
// Every other rule in this group reads the math spans and asks whether they
// are right. That leaves the worst outcome unexamined, because a paper whose
// formulas were dissolved into the prose has no spans to read and every one
// of those rules reports that it had nothing to look at. Fifty-five green
// ticks over a corpus with no mathematics in it is the failure this rule
// exists to stop.
//
// It is per paper and per language rather than per file, because a paper
// keeps its mathematics in the sections that need it and a report that
// pointed at the introduction would be pointing at the wrong page.
func ruleM14(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	type reading struct {
		spans int
		signs int
		path  string
		line  int
	}
	seen := map[string]*reading{}
	var order []string
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		key := f.Paper + "\x00" + string(f.Lang)
		r := seen[key]
		if r == nil {
			r = &reading{}
			seen[key] = r
			order = append(order, key)
		}
		// Fences are blanked first. A code block full of arithmetic is code
		// and is not this paper's mathematics going missing.
		prose := mathtex.BlankFences(f.Body)
		spans, unclosed := mathtex.Split(prose)
		r.spans += len(spans)
		if unclosed != nil {
			r.spans++
		}
		for n, line := range strings.Split(prose, "\n") {
			for _, c := range line {
				if !strings.ContainsRune(notation, c) {
					continue
				}
				r.signs++
				if r.path == "" {
					r.path, r.line = f.Path, n+1
				}
			}
		}
	}
	var out []Finding
	for _, key := range order {
		r := seen[key]
		if r.spans > 0 || r.signs < enough {
			continue
		}
		paper, lang, _ := strings.Cut(key, "\x00")
		out = append(out, Finding{
			Rule: "M14", File: r.path, Line: r.line,
			Message: fmt.Sprintf("%s in %s carries %s and not one math span, so its formulas were flattened into the prose and the paper needs reading again",
				paper, lang, plural(r.signs, "mathematical character")),
		})
	}
	return out, nil
}
