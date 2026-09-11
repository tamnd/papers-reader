// Package mathtex is where the mathematics of a body starts and stops, and what
// to do about a character that was left outside its TeX.
//
// It is a leaf package on purpose. Two things need it and they sit at opposite
// ends of the pipeline: extract, which writes the pages, and audit, whose M
// rules read them back and say whether the formulae survived. If each had its
// own idea of where a math span begins they would disagree about the same file,
// and an audit that disagrees with the tool that produced the corpus is worse
// than no audit. So the splitter is written once, here, under both of them.
//
// It is carried over from bourbaki-solver, which used it over six volumes of
// the Eléments, and the evidence in the comments below is that corpus's. It is
// quoted rather than dropped because every narrowing in here was put there by
// a page that the wider rule got wrong, and a reader who is about to widen one
// deserves to know what it cost last time. None of it is a measurement of the
// papers corpus, and where a count is quoted the volume it was counted in is
// named.
package mathtex

import (
	"fmt"
	"regexp"
	"strings"
)

// A Span is one stretch of mathematics in a body, without its delimiters.
type Span struct {
	Text    string
	Display bool
	Line    int // the body line the opening delimiter sits on, counting from one

	// Start and End are where the text sits in the body, counted in runes
	// because Split walks it in runes. A rule needs only the text, but anything
	// that repairs the mathematics has to put it back where it came from, and
	// searching for the text again would find the wrong copy of a span that is
	// written twice on a line.
	Start, End int
}

// Split cuts a normalised body into its math spans.
//
// The rules are LaTeX's, which are not Markdown's: a backslash escapes the
// character after it, so \$ is a dollar sign and not a delimiter, two dollars
// open a display and one opens an inline span, and a display closes only on two.
// The Eléments had three \$ in them, all inside one mangled region, and
// getting them wrong was the difference between reporting that region and
// reporting the whole file as balanced.
//
// unclosed is the span left open at the end of the body, and nil when there is
// none. It carries the line the delimiter that opened it sits on, which is the
// line somebody has to go and look at; the end of the file is where the problem
// shows up and never where it is.
func Split(body string) (spans []Span, unclosed *Span) {
	const (
		none = iota
		inline
		display
	)
	state := none
	line := 1
	// braces is one entry per brace group open inside the span being read, true
	// for a group that is an argument of \text and its relatives. It is emptied
	// with the span, since a brace that opens outside the mathematics is prose.
	var braces []bool
	pendingText := false
	var open Span
	var start int
	rs := []rune(body)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n':
			line++
			continue
		case '\\':
			// The escape takes the next character with it, whatever it is, and
			// a newline still has to be counted. A control word is read whole,
			// because the one thing that has to be recognised here is the name
			// in front of a brace.
			if i+1 >= len(rs) {
				continue
			}
			if name, end := controlWord(rs, i+1); name != "" {
				pendingText, i = textArg[name], end-1
				continue
			}
			if rs[i+1] == '\n' {
				line++
			}
			i++
			pendingText = false
			continue
		case '{':
			if state != none {
				braces = append(braces, pendingText)
			}
			pendingText = false
			continue
		case '}':
			if state != none && len(braces) > 0 {
				braces = braces[:len(braces)-1]
			}
			pendingText = false
			continue
		case '$':
		case ' ', '\t':
			continue // \text {x} is still \text's argument
		default:
			pendingText = false
			continue
		}
		// A dollar inside the argument of a \text is the nested mathematics of
		// \text{$\Gamma$ correspondance} and not the end of the span. The French
		// index of notation of Théorie des ensembles wrote one entry that way,
		// and reading its dollars flat cut the entry into three spans with
		// \Gamma and \Gamma' left as prose between them; the build then wrapped
		// those in dollars of their own, as it is meant to for mathematics loose
		// in prose, and wrote \text{$$\Gamma$$, $$\Gamma$'$ correspondances},
		// which tectonic refused with "! Missing $ inserted." and no PDF for the
		// cover check to open.
		//
		// It is \text and not any open brace, deliberately. A dollar inside any
		// brace at all would mean that $\mathbf{Q$, which is a brace somebody
		// left open and which M04 exists to report, swallowed the rest of the
		// file into one unclosed span instead, and a rule that cannot see a
		// defect is worse than the defect.
		if state == inline && inText(braces) {
			continue
		}
		double := i+1 < len(rs) && rs[i+1] == '$'
		switch state {
		case none:
			open, start, braces = Span{Display: double, Line: line}, i+1, braces[:0]
			if double {
				state, start = display, i+2
				i++
			} else {
				state = inline
			}
			open.Start = start
		case inline:
			open.Text, open.End = string(rs[start:i]), i
			spans = append(spans, open)
			state, braces = none, braces[:0]
		case display:
			if !double {
				continue // a lone dollar inside a display is text
			}
			open.Text, open.End = string(rs[start:i]), i
			spans = append(spans, open)
			state, braces = none, braces[:0]
			i++
		}
		pendingText = false
	}
	if state != none {
		open.Text, open.End = string(rs[start:]), len(rs)
		return spans, &open
	}
	return spans, nil
}

// textArg are the commands whose argument is set as text and may therefore hold
// mathematics of its own. \text is amsmath's and is what the extraction
// writes; the rest are the ones a body could reasonably use for the same thing,
// and \mbox and \hbox are here because the papers of the 1970s and 1980s were
// set in plain TeX and a formula copied out of one has them.
var textArg = map[string]bool{
	"text": true, "textrm": true, "textit": true, "textbf": true, "textsf": true,
	"texttt": true, "textnormal": true, "textup": true, "textsc": true,
	"mbox": true, "hbox": true, "intertext": true,
}

// controlWord reads the name of the control word starting at i, which is the
// character after the backslash, and returns it with the index just past it. A
// control symbol, \$ or \\ or \{, is not a word and comes back empty, so the
// caller falls through to the escape rule that takes one character.
func controlWord(rs []rune, i int) (string, int) {
	j := i
	for j < len(rs) && (rs[j] >= 'a' && rs[j] <= 'z' || rs[j] >= 'A' && rs[j] <= 'Z') {
		j++
	}
	if j == i {
		return "", i
	}
	return string(rs[i:j]), j
}

// inText says whether any brace group still open is a \text argument. It is any
// and not the innermost, because \text{a $\{x : \text{...}\}$ b} nests and the
// dollars inside the inner one are still nested dollars.
func inText(braces []bool) bool {
	for _, t := range braces {
		if t {
			return true
		}
	}
	return false
}

// The repair is M03 read backwards. The rule says which characters are stranded
// out of their TeX and this puts them back, over the same split, so the two
// cannot disagree about where the mathematics is.
//
// A repair is not a rewrite. Every substitution here is one glyph for the TeX
// that prints that same glyph, so the corpus after it says exactly what it said
// before and compiles where it did not. Nothing here guesses at what the book
// meant, and the two characters where a guess would be needed are refused and
// handed back to whoever is running it. That is the difference between a repair
// and an invention, and it is why this is a table and not a model call.

// texOf is the glyph to TeX table, restricted to what M03 flags inside the
// mathematics: the Greek block and the increment sign.
//
// extract has its own and larger table and this is deliberately not that one.
// That table maps the glyphs the PDF fonts come out as and it runs while the
// run's font is still known. This one runs over Markdown with no font and no
// page behind it, so it holds only the characters somebody has actually seen
// stranded. A table of substitutions nobody has checked against a printed page
// is a table that will be wrong somewhere and nobody will know which entry.
var texOf = map[rune]string{
	'α': `\alpha`, 'β': `\beta`, 'γ': `\gamma`, 'δ': `\delta`, 'ε': `\varepsilon`,
	'ζ': `\zeta`, 'η': `\eta`, 'θ': `\theta`, 'ϑ': `\vartheta`, 'ι': `\iota`,
	'κ': `\kappa`, 'λ': `\lambda`, 'μ': `\mu`, 'ν': `\nu`, 'ξ': `\xi`,
	'π': `\pi`, 'ρ': `\rho`, 'σ': `\sigma`, 'ς': `\varsigma`, 'τ': `\tau`,
	'υ': `\upsilon`, 'φ': `\varphi`, 'ϕ': `\varphi`, 'χ': `\chi`, 'ψ': `\psi`,
	'ω': `\omega`,
	'Γ': `\Gamma`, 'Δ': `\Delta`, 'Θ': `\Theta`, 'Λ': `\Lambda`, 'Ξ': `\Xi`,
	'Π': `\Pi`, 'Σ': `\Sigma`, 'Υ': `\Upsilon`, 'Φ': `\Phi`, 'Ψ': `\Psi`,
	'Ω': `\Omega`,

	// U+2206, the increment sign, which is not U+0394 and is what a text layer
	// hands back for a capital delta often enough to be worth an entry. Six in
	// the Eléments, all naming the diagonal of a group.
	'∆': `\Delta`,

	// The compatibility characters, which are the same letters again at
	// different code points and are what a census turned up once the Greek
	// block was clear: in the Eléments, 303 micro signs, U+00B5 rather than
	// U+03BC, almost all of them the index µ of a family, and 42 ohm signs,
	// U+2126 rather than U+03A9, the Ω of an algebraically closed extension.
	// Neither is in the Greek block, so neither was caught by looking there,
	// and both print as the letter, so neither is visible by reading.
	// They are written as escapes and not as themselves, because µ and the
	// letter mu are the same shape in every font this source will be read in,
	// and a table where two entries look identical is a table somebody will
	// delete half of.
	'\u00b5': `\mu`,    // the micro sign, not the Greek mu above
	'\u2126': `\Omega`, // the ohm sign, not the Greek omega above

	// U+0131, the dotless i, which is what an accented i is set as. Three of
	// them in the Eléments, all \widetilde{ı}.
	'\u0131': `\imath`,
}

// ambiguous are the two capitals that are also operators. A capital sigma with
// a subscript under it is a sum and not the letter, a capital pi with one is a
// product, and neither difference survives being written as the letter.
//
// The Eléments had seven stranded sigmas. Six were the letter, a set named Σ,
// one was a sum over σ in Γ, and the only thing in the Markdown that told them
// apart was the subscript. So the shape decides: one of these followed by _ or
// ^ is refused and reported, and everything else is the letter. Pi is here for
// the same reason, against the paper that will strand one.
var ambiguous = map[rune]string{
	'Σ': `\sum`,
	'Π': `\prod`,
}

// A Refusal is a character the repair would not touch, and why.
type Refusal struct {
	File string
	Line int
	Rune rune
	Why  string
	Span string
}

func (r Refusal) String() string {
	at := fmt.Sprintf("%s:%d", r.File, r.Line)
	if r.File == "" {
		at = fmt.Sprintf("line %d", r.Line)
	}
	what := fmt.Sprintf("%q ", r.Rune)
	if r.Rune == 0 {
		what = "" // the refusal is about a whole span rather than one character
	}
	return fmt.Sprintf("%s  %s%s: %s", at, what, r.Why, oneLine(r.Span, 50))
}

// Repair puts the stranded characters of one body back into their TeX and says
// what it left alone.
//
// It returns the new body, how many characters it replaced, and the refusals. A
// body it has nothing to do with comes back unchanged, rune for rune, so a
// caller can compare and write only what moved.
func Repair(body string) (string, int, []Refusal) {
	spans, unclosed := Split(body)
	rs := []rune(body)
	var b strings.Builder
	var refused []Refusal
	n, at := 0, 0
	if unclosed != nil {
		// A span that never closes has no end, so from its opening dollar on
		// there is no telling where the mathematics stops and the prose starts,
		// and a repair that guessed would rewrite prose. Everything before it
		// is closed and is repaired as usual, which matters because a display
		// running over a page break leaves every page before it perfectly
		// sound, and M01 reports the open span separately anyway.
		refused = append(refused, Refusal{Line: unclosed.Line,
			Why:  "is in a math span that never closes, so the repair cannot see where it ends",
			Span: unclosed.Text})
	}
	for _, s := range spans {
		b.WriteString(string(rs[at:s.Start]))
		at = s.End
		fixed, count, ref := repairSpan(rs[s.Start:s.End])
		for i := range ref {
			ref[i].Line, ref[i].Span = s.Line, s.Text
		}
		b.WriteString(fixed)
		n += count
		refused = append(refused, ref...)
	}
	b.WriteString(string(rs[at:]))
	return b.String(), n, refused
}

// DropStray takes out a $$ that opens mathematics, closes nothing, and stands
// against the punctuation at the end of a sentence.
//
// The fault it repairs is one the text layer makes over and over. A numbered
// display set on its own line comes through as prose, with the pieces of it in
// inline spans and the display's own closing delimiter carried along to the end
// of the sentence:
//
//	(2) long$_A(M) =$ long(B) and long$_B(M) =$ long(A) $$.
//
// Nothing closes that $$, so everything after it on the page reads as
// mathematics. M01 reports the file, the M rules read prose as formulae, and
// Repair above will not touch a span whose end it cannot see.
//
// The three conditions are narrow on purpose, and each of them was put there by
// a page the wider rule got wrong. Of the fourteen pages in one Bourbaki
// chapter carrying an unclosed delimiter only eight were this fault; on the
// other six a display had lost its opening delimiter instead, so the first
// unclosed delimiter the splitter met was the closing one, and a repair that
// trusted the count alone deleted the end of a display that was perfectly good.
//
//   - It must be a display delimiter. A single stray $ is an inline formula
//     that was mangled far more often than it is a leftover, and taking it out
//     breaks a formula that reads correctly.
//   - The next character must be a full stop or a comma. A $$ alone on its line
//     is a display delimiter doing its job.
//   - The body must balance once it is gone, which is the check that the rest
//     of the page was sound.
//
// The caller has the last condition, and it is the page's flags: this is not
// run on a page the extractor has already said it could not read. A page that
// lost a tall delimiter or dropped a glyph is a matrix or a bracket that
// arrived in pieces, and on those the delimiters mean nothing.
func DropStray(body string) (string, bool) {
	_, un := Split(body)
	if un == nil || !un.Display {
		return body, false
	}
	rs := []rune(body)
	if un.Start >= len(rs) || (rs[un.Start] != '.' && rs[un.Start] != ',') {
		return body, false
	}
	at := un.Start - 2 // the $$ itself
	if at < 0 {
		return body, false
	}
	// The space in front of it goes too. It is there to hold the delimiter off
	// the words, and left behind it is a gap before a full stop.
	cut := at
	for cut > 0 && rs[cut-1] == ' ' {
		cut--
	}
	out := string(rs[:cut]) + string(rs[un.Start:])
	if _, un := Split(out); un != nil {
		return body, false
	}
	return out, true
}

// Unstraddle moves a closing bracket that belongs to the prose back out of the
// mathematics it was left inside.
//
// The fault is the text layer putting the delimiter in the wrong place around a
// function whose name is set upright. The name comes through as prose, its
// opening bracket comes through as prose, and its closing bracket is swept into
// the formula with the argument:
//
//	the sum is equal to Tr($u)$.
//
// It renders the same as Tr($u$) does, which is why nobody sees it by reading,
// and it is not the same text. The mathematics of that span is "u)", so a
// translator asked to copy the formulae hands back "u" with the bracket set as
// prose, which is right, and the audit refuses the section because a translation
// may not alter mathematics. That is a real refusal, and in bourbaki-solver it
// cost a seventeen minute run of one appendix with nothing written at the end
// of it.
//
// The rule is deliberately narrower than "the brackets inside a span balance".
// Measured over the Eléments, 979 of 18,664 spans did not balance their own
// brackets and most of them were innocent, because a book writes "(resp. $x$)"
// and labels list items "$\alpha$)". What makes this one a fault is the prose of
// the line holding a bracket open at the point the span starts, which is what
// straddles tests and what its comment sets out.
//
// Two conditions past the shape, and both are there to keep it from inventing:
//
//   - No more brackets come out than the prose has open. A span may close a
//     bracket opened earlier in the same line, and the count is taken over the
//     line outside the spans, so a bracket opened inside an earlier span is the
//     mathematics' own and buys the span nothing.
//   - Nothing but a delimiter moves. The body with every dollar sign taken out
//     has to be the body it started as, character for character, and a body that
//     fails that check is handed back untouched. It is a total proof and it is
//     cheap, and it is the only reason this can run unattended over a corpus.
//
// It returns the new body and how many spans it repaired.
// It repairs to a fixed point and not to the end of one walk. Taking a bracket
// out of one span changes what the prose of that line is holding open, and the
// next span along can be a straddle that was not one a moment ago:
// "(pr$_{J_\lambda})_{\lambda\in L}$ de $\prod$" gives one back on the first
// pass and the second is only visible after it. A round that repairs nothing
// ends it, and every round moves at least one bracket out of a span, so the
// count that bounds the loop is there against a bug and not against the corpus.
func Unstraddle(body string) (string, int) {
	total := 0
	for range 16 {
		out, n := unstraddleOnce(body)
		if n == 0 {
			break
		}
		body, total = out, total+n
	}
	return body, total
}

func unstraddleOnce(body string) (string, int) {
	spans, _ := Split(body)
	rs := []rune(body)
	var b strings.Builder
	n, at := 0, 0
	// What each line has already given back on this walk. The counts below are
	// read off rs, which is the body as it stood when the walk started, so a
	// bracket an earlier span on the line has just handed to the prose is still
	// sitting inside the mathematics as far as they can see. Without this the
	// line closes its own brackets twice over: "Card(W$\varpi_2), . . .$,
	// Card(W$.2\varpi_4)$ ... $d)$" gives two back and then reads as holding two
	// open still, and the list label "d)" at the end, which is not a straddle and
	// belongs to nobody's bracket, is taken for the third.
	given := map[int]int{}
	for _, s := range spans {
		from := lineStart(rs, s.Start)
		if !straddles(rs, spans, s, given[from]) {
			continue
		}
		text := []rune(s.Text)
		cut := looseCloser(text)
		run := 1
		for cut+run < len(text) && text[cut+run] == ')' {
			run++
		}
		// How many the line can afford to give back. Counting prose and
		// mathematics alike is the wider of the two most of the time, since an
		// opener left dangling in an earlier span is usually this same fault
		// mirrored, and it is not always: a closing bracket inside an earlier
		// span eats the depth the prose opened, and then the line reads as
		// holding nothing open while the prose plainly is. Neither count is
		// wrong, so the cap is whichever is larger and the trigger stays with
		// the prose.
		//
		// The loose count needs no credit taken off it. A bracket that moved out
		// of an earlier span on this line moved from one side of a delimiter to
		// the other and stayed in front of this span either way, so the loose
		// count had it before the repair and has it after.
		open := looseOpeners(rs[from:s.Start])
		if prose := proseOpeners(rs, spans, from, s.Start) - given[from]; prose > open {
			open = prose
		}
		if run > open {
			run = open
		}
		head := strings.TrimRight(string(text[:cut]), " \t")
		if run == 0 {
			continue
		}
		b.WriteString(string(rs[at : s.Start-1])) // up to but not into the delimiter
		if head == "" {
			// The bracket is the first thing in the span, as in
			// "(VIII, p. 5, Example 3$).*$", so nothing of the mathematics
			// stands in front of it. The opening delimiter moves to the far
			// side of the bracket rather than closing on an empty span.
			b.WriteString(string(text[:cut]))
		} else {
			b.WriteString("$")
			b.WriteString(head)
			b.WriteString("$")
			b.WriteString(string(text[:cut])[len(head):]) // the spaces trimmed off it
		}
		b.WriteString(strings.Repeat(")", run))
		b.WriteString(wrap(string(text[cut+run:])))
		at = s.End + 1 // the closing delimiter, which has been written already
		given[from] += run
		n++
	}
	if n == 0 {
		return body, 0
	}
	b.WriteString(string(rs[at:]))
	out := b.String()
	if strings.ReplaceAll(out, "$", "") != strings.ReplaceAll(body, "$", "") {
		return body, 0
	}
	return out, n
}

// Straddles are the spans a bracket from the prose closes inside, which is the
// fault Unstraddle repairs, read from the other side.
//
// The audit has it as M07 and the repair has it here, off the same test, so the
// rule cannot report a shape the repair does not know about and the repair
// cannot quietly leave one behind. Unstraddle repairs all but the spans where
// the line has fewer brackets open than the span closes, and those are the ones
// worth a person looking at, which is what M07 is for.
func Straddles(body string) []Span {
	spans, _ := Split(body)
	rs := []rune(body)
	var out []Span
	for _, s := range spans {
		// Nothing has been given back, because this reads a page as it stands
		// rather than repairing one as it goes.
		if straddles(rs, spans, s, 0) {
			out = append(out, s)
		}
	}
	return out
}

// straddles is the shape: a bracket the prose of the line opened and never
// closed, and a closing one inside the span that closes nothing of the span's
// own. A display is set on its own lines, so there is no prose against it and
// nothing to have been swept in.
//
// The opener does not have to stand against the delimiter. It did in the first
// version of this rule, which read Tr($u)$ and nothing else, and that left the
// commoner shape behind: the name takes an argument of its own and the bracket
// that comes through as prose is the outer one, as in Card(I$_L)$ and
// (resp. de $\widehat{G})$ and (cf. INT, VIII, §2, n$^o6)$. What the two have in
// common is not where the bracket sits, it is that the prose is holding a
// bracket open at the point the span starts.
//
// Only the prose counts. A bracket opened inside an earlier span on the line is
// the mathematics' own and it is not owed anything, so $f(x$ and $y)$ is left
// alone; the closing bracket there belongs where it is.
//
// The one shape past that which has to be refused is the half-open interval. A
// span reading [a, b) closes a bracket it did not open and looks exactly like a
// straddle, and moving that bracket out would break the notation, so a span
// still holding a square bracket open at the loose closer is not a straddle.
//
// given is what the line has already handed back on this walk, and it comes off
// the prose count. The walk reads the body as it stood when it started, so
// without it a line that has just been repaired twice still looks like a line
// holding two brackets open, and the next span along is convicted on them.
func straddles(rs []rune, spans []Span, s Span, given int) bool {
	if s.Display || s.Start < 2 {
		return false // the prose needs room for a bracket and the delimiter
	}
	text := []rune(s.Text)
	cut := looseCloser(text)
	if cut < 0 || squaresOpen(text[:cut]) > 0 {
		return false
	}
	return proseOpeners(rs, spans, lineStart(rs, s.Start), s.Start)-given > 0
}

// wrap puts what is left of a span back in delimiters, with the space at either
// end left outside where it reads as the space between two words.
func wrap(rest string) string {
	inner := strings.TrimSpace(rest)
	if inner == "" {
		return rest // nothing but space, and it is prose now
	}
	lead := rest[:len(rest)-len(strings.TrimLeft(rest, " \t\n\r"))]
	return lead + "$" + inner + "$" + rest[len(lead)+len(inner):]
}

// looseCloser is where the first bracket that closes nothing of its own sits.
func looseCloser(rs []rune) int {
	depth := 0
	for i, r := range rs {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// looseOpeners is how many brackets are still open at the end of a stretch of
// body, counting prose and mathematics alike. It caps how many a straddle gives
// back, and proseOpeners says what a straddle is in the first place.
func looseOpeners(rs []rune) int {
	depth := 0
	for _, r := range rs {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

// proseOpeners is how many brackets the prose of rs[from:to] still has open,
// counting only what falls outside the math spans.
//
// It is what decides whether a span is a straddle at all, and it is not what
// decides how many brackets come out of one. A bracket left open at the end of
// an earlier span is usually the same fault read from the other side, the text
// layer having swept the opener in rather than the closer, so it is fair to give
// back against it once the line is known to be holding a prose bracket open. It
// is not fair to start there: a line that reads $f(x$ and $y)$ is a function
// whose argument the text layer cut in half, the closing bracket belongs where
// it stands, and there is no prose bracket on that line to say otherwise.
func proseOpeners(rs []rune, spans []Span, from, to int) int {
	if from < 0 {
		from = 0
	}
	if to > len(rs) {
		to = len(rs)
	}
	depth := 0
	for i := from; i < to; i++ {
		if inSpan(spans, i) {
			continue
		}
		switch rs[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

// inSpan reports whether the rune at i is inside the mathematics of a span. The
// delimiters themselves are dollar signs and are neither bracket, so it does not
// matter which side of the line they are counted on.
func inSpan(spans []Span, i int) bool {
	for _, s := range spans {
		if s.Start > i {
			return false // Split hands them back in order
		}
		if i < s.End {
			return true
		}
	}
	return false
}

// squaresOpen is how many square brackets a stretch of a span still has open. It
// is what tells a half-open interval from a straddle.
func squaresOpen(rs []rune) int {
	depth := 0
	for _, r := range rs {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

// lineStart is the first rune of the line position i sits on.
func lineStart(rs []rune, i int) int {
	for i > 0 && rs[i-1] != '\n' {
		i--
	}
	return i
}

// repairSpan rewrites the inside of one math span.
func repairSpan(rs []rune) (string, int, []Refusal) {
	var b strings.Builder
	var refused []Refusal
	n := 0
	for i, r := range rs {
		if op, ok := ambiguous[r]; ok && i+1 < len(rs) && (rs[i+1] == '_' || rs[i+1] == '^') {
			refused = append(refused, Refusal{Rune: r,
				Why: fmt.Sprintf("carries a subscript, so it is %s rather than %s", op, texOf[r])})
			b.WriteRune(r)
			continue
		}
		tex, ok := texOf[r]
		if !ok {
			b.WriteRune(r)
			continue
		}
		b.WriteString(tex)
		n++
		// \Gamma written against a following letter names a command that does
		// not exist, so the two are separated the way extract separates them.
		if i+1 < len(rs) && isTeXLetter(rs[i+1]) {
			b.WriteByte(' ')
		}
	}
	return b.String(), n, refused
}

func isTeXLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// stackedRowRE is a matrix the layer flattened: a superscript holding one row
// and a subscript holding the other, on the same base, in either order.
//
// Two things have to hold and both are needed. The scripts are adjacent, which
// is what makes it a matrix rather than a script: the layer raised the top row
// and lowered the bottom row of the same thing, so they come back stuck
// together. And each holds two or more entries with a space between them,
// because the page puts a gap between two columns. Entries are letters and
// digits, which is what the corners of a matrix hold: A B over C D, 1 0 over
// 0 0, a b over c d.
//
// A lone script with a space in it is not this and must not be counted. M_{I J}
// was one, 21 times in one Bourbaki chapter, and it was a real defect of its
// own: the page printed the set difference M_{I−J} and the space was the room
// TeX left for a sign that is drawn rather than set, so it is in no font and no
// text layer. That is a different defect with a different repair, and reporting
// it here would have put 21 findings under a heading naming the wrong fault.
// The exclusion stays because it is what keeps a script with a space in it from
// being read as a row.
var stackedRowRE = regexp.MustCompile(
	`\^\{` + rowGroup + `\}_\{` + rowGroup + `\}|_\{` + rowGroup + `\}\^\{` + rowGroup + `\}`)

// rowGroup is one row: two or more entries with a space between them.
const rowGroup = `[A-Za-z0-9]+(?: +[A-Za-z0-9]+)+`

// StackedMatrices counts the matrices a body has flattened.
//
// It is exported because the same count is wanted from two sides: here, to flag
// the page while the paper is being read, and in the audit, to say what the
// committed corpus still carries. Two implementations of one shape would drift,
// and the one in the audit would be the one nobody checked against a PDF.
func StackedMatrices(body string) int {
	return len(stackedRowRE.FindAllString(body, -1))
}

// StackedRows is every flattened matrix row in a body, as it is written.
//
// StackedMatrices counts them; this names them, which is what an audit finding
// needs so that somebody can find the thing on the page without opening the
// file.
func StackedRows(body string) []string {
	return stackedRowRE.FindAllString(body, -1)
}

// displayRE is a display block, from its opening $$ to its closing $$, wherever
// the two stand.
var displayRE = regexp.MustCompile(`(?s)\$\$.*?\$\$`)

// BlankDisplays takes the display mathematics out of a body and leaves every
// line where it was.
//
// Three rules split a body into prose and displays and all three did it the same
// way, by walking the lines and toggling on a line that is nothing but $$. That
// reads the corpus correctly, because the extraction fences a display on its own
// lines, and it does not read a model's answer correctly, because a model
// reflows. One Bourbaki section came back with
//
//	Cụ thể, ta có $$
//	\left| \frac{1}{x} - \frac{1}{y} \right| = \frac{|x-y|}{xy}
//	$$; tồn tại một số nguyên m > 0 sao cho
//
// where the opening fence is welded to the prose before it and the closing one
// to the prose after. Neither line is nothing but $$, so the toggle never fired,
// the middle line has no dollar on it at all for Strip to catch, and \left and
// \right were read as the English words left and right. The glossary had a row
// for each, so the terminology rule asked for trái and phải to be written inside
// a formula, and the chunk was refused five times and the section died.
//
// The count of newlines is kept, because two of the three callers report a line
// number and a rule that moves the lines under the reader is worse than the
// rule it replaces.
//
// A one line display, $$ x = y $$, is not this function's business and never
// was: Strip reads $$ as one delimiter rather than two and takes it out along
// with the inline spans. It goes out here as well now, which changes nothing.
func BlankDisplays(body string) string {
	return displayRE.ReplaceAllStringFunc(body, func(d string) string {
		return strings.Repeat("\n", strings.Count(d, "\n"))
	})
}

// Strip takes the mathematics out of a line and leaves the prose.
//
// What comes back is not TeX and is not meant to be read as any: the spans
// become spaces, so words either side of a formula do not run together and the
// line can be searched for an English word without a symbol inside a formula
// answering for one. A backslash escape outside the mathematics keeps both of
// its characters, since that is prose with a literal dollar in it.
//
// It lives here because the dollar scanning is this package's and two copies of
// it would drift, which is exactly what happened: the audit had the only copy
// and the run needs the same one, so a term left in English is caught in the
// minute the chunk comes back rather than in a report nobody acts on.
func Strip(s string) string {
	var b strings.Builder
	in := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '\\' && i+1 < len(rs):
			if !in {
				b.WriteRune(rs[i])
				b.WriteRune(rs[i+1])
			}
			i++
		case rs[i] == '$':
			// A display opened and closed on one line is $$ at each end, and
			// counting each dollar of it flips the switch twice and leaves the
			// formula standing as prose. That is not a corner: most of the
			// displays of one Bourbaki chapter were written this way, and read
			// as prose they say the English words left, right and square,
			// which is where ten of twelve terminology findings against the
			// Vietnamese came from.
			if i+1 < len(rs) && rs[i+1] == '$' {
				i++
			}
			in = !in
			b.WriteRune(' ')
		case !in:
			b.WriteRune(rs[i])
		}
	}
	return b.String()
}
