package extract

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/code"
	"github.com/tamnd/papers-reader/grid"
)

// Untable rewrites the HTML tables a reader wrote as the pipe tables this
// corpus is written in.
//
// The prompt asks for a pipe table where the cells form a grid and a `text`
// fence where they do not. A model trained on web pages answers with
// `<table>` anyway, the same way olmOCR answers in TeX's own math delimiters
// however the question is worded, and for the same reason: it is writing what
// its training data looks like. Three sections of the Transformer paper came
// back as raw HTML.
//
// Left alone it is worse than it looks. The reading app renders Markdown and
// not HTML, so the table comes out as a column of angle brackets. The
// translator has no idea which parts of it are text. And `<td>1.0 \cdot
// 10^{20}</td>` is mathematics with no dollars round it, so mathtex never sees
// it, the M rules never check it, and rule M14 reads the section as one whose
// formulas were flattened.
//
// Like Dollars this is a translation between two spellings and not a repair.
// Nothing here decides that something is a table: the `<table>` says so. What
// it will not do is guess. A table it cannot read as a rectangle of cells is
// left exactly as it came, where rule T11 reports it and a person looks at it,
// because a table quietly turned into the wrong table is worse than a table
// that is obviously still HTML.
func Untable(s string) string {
	lines := strings.Split(s, "\n")
	// A listing about HTML is full of `<table>` and none of it is a table.
	// Inside counts from one, so the line at index i is at fenced[i+1].
	fenced := code.Inside(s)
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if fenced[i+1] || !opensTable(lines[i]) {
			out = append(out, lines[i])
			continue
		}
		end := closesTable(lines, i, fenced)
		if end < 0 {
			out = append(out, lines[i])
			continue
		}
		pipe, ok := pipeTable(strings.Join(lines[i:end+1], "\n"))
		if !ok {
			out = append(out, lines[i:end+1]...)
			i = end
			continue
		}
		out = append(out, strings.Split(pipe, "\n")...)
		i = end
	}
	return strings.Join(out, "\n")
}

var (
	tableOpen  = regexp.MustCompile(`(?i)^\s*<table\b`)
	tableClose = regexp.MustCompile(`(?i)</table\s*>`)
	rowTag     = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr\s*>`)
	cellTag    = regexp.MustCompile(`(?is)<(th|td)\b([^>]*)>(.*?)</(?:th|td)\s*>`)
	spanAttr   = regexp.MustCompile(`(?i)\b(rowspan|colspan)\s*=\s*"?(\d+)"?`)
	// leftover is any tag at all. A cell that still has one after the
	// conversions below is a cell this function did not understand, and one
	// cell it did not understand is enough to leave the whole table alone.
	leftover = regexp.MustCompile(`<[^>]*>`)
)

func opensTable(line string) bool { return tableOpen.MatchString(line) }

// closesTable is the line the table ends on, and -1 for a table that never
// ends. A table that runs to the end of the page is a page that went wrong
// somewhere else and is not this function's to tidy away.
func closesTable(lines []string, from int, fenced []bool) int {
	for i := from; i < len(lines); i++ {
		if i > from && fenced[i+1] {
			return -1
		}
		if tableClose.MatchString(lines[i]) {
			return i
		}
	}
	return -1
}

// pipeTable reads the HTML and writes the Markdown, and says no rather than
// guessing.
func pipeTable(html string) (string, bool) {
	rows := rowTag.FindAllStringSubmatch(html, -1)
	if len(rows) == 0 {
		// No rows is markup this function misread.
		return "", false
	}
	// One row is allowed and comes out as a header with nothing under it,
	// which is a table GitHub Flavored Markdown can write and every renderer
	// draws. It used to be refused on the grounds that a heading with nothing
	// under it is not a table, and that was wrong: the Spanner paper draws
	// the interleaving of its example schema as a strip of seven boxes in a
	// row, which is one row of cells and reads perfectly well as one. The
	// cost of refusing it was that the strip stayed as raw HTML in the
	// corpus, which is worse by every measure.
	// grid is the table being filled in, and held is the cells still coming
	// down from a rowspan above. A cell that spans four rows is written once
	// and the three rows under it get an empty cell in that column, which is
	// what every Markdown renderer does with a span anyway because GitHub
	// Flavored Markdown has no way to write one. The alternative is to drop
	// the row the span came from, and then the table says something the paper
	// does not.
	var out [][]string
	held := map[int]int{}
	for _, r := range rows {
		cells := cellTag.FindAllStringSubmatch(r[1], -1)
		if len(cells) == 0 {
			return "", false
		}
		var row []string
		put := func(text string) {
			for held[len(row)] > 0 {
				held[len(row)]--
				row = append(row, "")
			}
			row = append(row, text)
		}
		for _, c := range cells {
			text, ok := cellText(c[3])
			if !ok {
				return "", false
			}
			down, across := spans(c[2])
			if down > 1 {
				// The column the cell lands in is not known until the held
				// cells before it have been laid down, so the hold is
				// recorded after the put.
				put(text)
				held[len(row)-1] += down - 1
				for i := 1; i < across; i++ {
					put("")
				}
				continue
			}
			put(text)
			for i := 1; i < across; i++ {
				put("")
			}
		}
		for held[len(row)] > 0 {
			held[len(row)]--
			row = append(row, "")
		}
		out = append(out, row)
	}
	// Every row the same width, and no span left hanging over the end of the
	// table. This is the rectangle test and it is the whole of what keeps a
	// wrong table out of the corpus.
	//
	// It is not a formality. A reader that writes spans writes them from
	// looking at a picture, and the Transformer paper's third table came back
	// with `rowspan="4"` over a group of five rows and `colspan="7"` used to
	// mean seven empty columns rather than one cell seven wide. Expanded as
	// written, the numbers land under the wrong headings, and a table whose
	// numbers are under the wrong headings is worse than no table at all
	// because it reads like a table. The same paper's second table uses both
	// kinds of span correctly and converts.
	//
	// grid.Pipe pads a short row, which is right for a cell the paper left
	// empty and wrong here: a ragged row out of a span arithmetic that does
	// not add up is a row this function read wrong, not a row the paper left
	// short.
	for _, row := range out {
		if len(row) != len(out[0]) {
			return "", false
		}
	}
	for _, n := range held {
		if n > 0 {
			return "", false
		}
	}
	return grid.Pipe(out), true
}

// Fence writes the tables Untable would not touch as the text fences the
// prompt asks for when the cells do not form a grid.
//
// This is the last resort and it runs only when the resolution ladder is out
// of rungs. Untable refuses a table it cannot read as a rectangle, acceptance
// rule A10 sees the markup that is left and the page is asked again higher
// up, and most of the time that is the end of it. A second sample from the
// model is a different sample: the Hoare paper's fourth page came back with
// 108 tags on it at 300 dpi and came back clean at 400.
//
// Some tables never come back right. The Transformer paper's third table
// groups five rows under one label and spells a row of blanks as a single
// cell seven columns wide, and it came back the same way at 300, 400 and 600,
// because the picture is the same picture and the model reads it the same way
// every time. A fourth ask costs another page of quota and returns the same
// answer.
//
// So the table is written the other way the prompt allows, which is not a
// compromise invented here but the instruction the reader was given and did
// not follow. Nothing is lost: every cell is there, in reading order, one row
// to a line. What is given up is the claim that the third cell of one row is
// in the same column as the third cell of the next, and that claim is the one
// this table cannot support. A pipe table is better than a fence for the
// renderer, the translator and the M rules alike, which is why a page that
// might still convert on the next attempt gets the next attempt, and why
// none of this is in Tidy.
func Fence(s string) string {
	lines := strings.Split(s, "\n")
	fenced := code.Inside(s)
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if fenced[i+1] || !opensTable(lines[i]) {
			out = append(out, lines[i])
			continue
		}
		end := closesTable(lines, i, fenced)
		if end < 0 {
			out = append(out, lines[i])
			continue
		}
		block, ok := fenceTable(strings.Join(lines[i:end+1], "\n"))
		if !ok {
			out = append(out, lines[i:end+1]...)
			i = end
			continue
		}
		out = append(out, block...)
		i = end
	}
	return strings.Join(out, "\n")
}

// fenceTable is the rows of an HTML table as the lines of a text fence, two
// spaces between the cells of a row.
//
// The spans are dropped rather than expanded. Expanding them is what Untable
// already tried and what the rectangle test already refused, and a fence full
// of padding it invented would be the same wrong table with a different set
// of delimiters round it.
func fenceTable(html string) ([]string, bool) {
	rows := rowTag.FindAllStringSubmatch(html, -1)
	if len(rows) == 0 {
		return nil, false
	}
	out := []string{"```text"}
	for _, r := range rows {
		cells := cellTag.FindAllStringSubmatch(r[1], -1)
		if len(cells) == 0 {
			return nil, false
		}
		row := make([]string, 0, len(cells))
		for _, c := range cells {
			row = append(row, plain(c[3]))
		}
		out = append(out, strings.TrimRight(strings.Join(row, "  "), " "))
	}
	return append(out, "```"), true
}

// tidyScript is a subscript or a superscript as Unscript has already written
// it, which is how most of them arrive here: Unscript runs in Tidy and Fence
// runs at the top of the ladder, so by the time a cell is read the `<sub>`
// the reader wrote has been mathematics for some time.
//
// Only this exact shape, a token and a mark and one group of braces with no
// mathematics inside it. A cell holding a real formula is left with its
// dollars on, because `1.0 \cdot 10^{20}` with the dollars taken off is TeX
// source pretending to be text, and inside a fence nobody is going to render
// it either way.
var tidyScript = regexp.MustCompile(`\$([\p{L}\p{N}.)\]]+)([_^])\{([^{}$\\]*)\}\$`)

// plain is one cell as the words in it and nothing else.
//
// A subscript comes out the way a person types one at a terminal rather than
// as mathematics, because dollars inside a fence are four characters the
// renderer prints as themselves, and because mathtex reads a fence as a
// listing and would never see them anyway. Everything else in angle brackets
// goes: a fence carries no markup, and bold in a table of numbers was
// presentation to begin with.
func plain(s string) string {
	s = linebreak.ReplaceAllString(s, " ")
	s = tidyScript.ReplaceAllString(s, "$1$2$3")
	s = script.ReplaceAllStringFunc(s, func(m string) string {
		p := script.FindStringSubmatch(m)
		mark := "_"
		if strings.EqualFold(p[2], "sup") {
			mark = "^"
		}
		return p[1] + mark + strings.TrimSpace(p[3])
	})
	s = bareScript.ReplaceAllStringFunc(s, func(m string) string {
		p := bareScript.FindStringSubmatch(m)
		mark := "_"
		if strings.EqualFold(p[1], "sup") {
			mark = "^"
		}
		return mark + strings.TrimSpace(p[2])
	})
	s = leftover.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// spans reads rowspan and colspan off a cell's attributes. Anything else on
// them is ignored: an align or a style is presentation and this corpus does
// not carry presentation.
func spans(attrs string) (down, across int) {
	down, across = 1, 1
	for _, m := range spanAttr.FindAllStringSubmatch(attrs, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil || n < 1 || n > 64 {
			continue
		}
		if strings.EqualFold(m[1], "rowspan") {
			down = n
		} else {
			across = n
		}
	}
	return down, across
}

var (
	bold      = regexp.MustCompile(`(?is)<(b|strong)\b[^>]*>(.*?)</(?:b|strong)\s*>`)
	italic    = regexp.MustCompile(`(?is)<(i|em)\b[^>]*>(.*?)</(?:i|em)\s*>`)
	linebreak = regexp.MustCompile(`(?i)<br\s*/?>`)
	// script is a subscript or a superscript together with the token it
	// belongs to. A subscript is mathematics wherever it appears in these
	// tables and it is written as mathematics, because the alternative is to
	// leave it as a tag that the renderer will not render and the M rules
	// cannot see.
	script = regexp.MustCompile(`(?is)([\p{L}\p{N}.)\]]+)<(sub|sup)\b[^>]*>(.*?)</(?:sub|sup)\s*>`)
	// texRun is a run of TeX with no dollars round it, which is how a reader
	// writes a number in scientific notation inside a cell: `1.0 \cdot
	// 10^{20}`. The whole reason for converting these tables is that TeX the
	// splitter cannot see is mathematics the audit cannot check, and a cell
	// left like this would be exactly that with the angle brackets taken off.
	//
	// It has to be a control sequence with something on both sides of it,
	// because a lone backslash is an escape and a lone `\cdot` in a column of
	// prose is a typo rather than a formula.
	texRun = regexp.MustCompile(`[0-9A-Za-z.]+(?:\s*\\[a-zA-Z]+\s*[0-9A-Za-z.^_{}]+)+`)
)

// cellText turns the inside of one cell into Markdown, and says no if
// anything is left that it did not understand.
func cellText(s string) (string, bool) {
	s = linebreak.ReplaceAllString(s, " ")
	s = bold.ReplaceAllString(s, "**$2**")
	s = italic.ReplaceAllString(s, "*$2*")
	s = script.ReplaceAllStringFunc(s, func(m string) string {
		p := script.FindStringSubmatch(m)
		mark := "_"
		if strings.EqualFold(p[2], "sup") {
			mark = "^"
		}
		return "$" + p[1] + mark + "{" + strings.TrimSpace(p[3]) + "}$"
	})
	// Only a cell with no dollars anywhere in it. A cell that has some is a
	// cell where the reader already marked the mathematics, and guessing at
	// the rest of it would put dollars inside dollars.
	if !strings.Contains(s, "$") {
		s = texRun.ReplaceAllString(s, "$$$0$$")
	}
	// A pipe inside a cell would end the cell, and grid.Pipe escapes it on
	// the way out. Doing it here as well would escape the backslash.
	s = strings.Join(strings.Fields(s), " ")
	if leftover.MatchString(s) {
		return "", false
	}
	return s, true
}

// HTMLTags is the markup a reader actually writes. It is a closed list rather
// than "anything in angle brackets" because angle brackets are also an email
// address in an author block, a comparison in a sentence the extractor failed
// to wrap in dollars, and a placeholder in a grammar, and a rule that reported
// all three would be turned off within a week. The Transformer paper prints
// `<pad>` and `<EOS>` and means the tokens.
//
// Everything on the list has either a Markdown spelling or no business in the
// corpus at all. Nothing here is a judgement call about presentation: the
// corpus is Markdown, and a file with markup in it is a file one of the three
// extraction paths did not finish converting.
//
// Acceptance rule A10 and audit rule T11 read the same list, because the two
// are the same question asked at two moments. A10 asks it of a page before
// the page is written and T11 asks it of the corpus afterwards, and two
// opinions about what counts as markup would mean the reader writing pages
// the audit then refuses with nobody able to say which of them was wrong.
var HTMLTags = map[string]bool{
	"a": true, "b": true, "big": true, "blockquote": true, "body": true,
	"br": true, "caption": true, "center": true, "code": true, "col": true,
	"colgroup": true, "dd": true, "div": true, "dl": true, "dt": true,
	"em": true, "figcaption": true, "figure": true, "font": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"hr": true, "i": true, "img": true, "li": true, "ol": true, "p": true,
	"pre": true, "small": true, "span": true, "strong": true, "sub": true,
	"sup": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "u": true, "ul": true,
}

var (
	// anyTag is an opening, closing or self closing tag. The name has to be
	// letters and digits, which is what keeps `<satoshin@gmx.com>` and
	// `<n, k>` out of it before the list above is even consulted.
	//
	// What comes after the name matters just as much. A tag closes on the
	// name, or has a space and then its attributes, and nothing else: there
	// is no such thing as an HTML tag whose name is followed by a comma.
	// Ending the name on a word boundary instead was reading Karp's ordered
	// pairs as markup. He writes an edge as `<u,v>`, u is the underline tag,
	// and page 18 of the reducibility paper was refused as HTML three times
	// over and then lost. The same shape is everywhere in this corpus, `<i,
	// j>` for an index pair, `<s,t>` for the ends of a path, `<p,q>` for a
	// pair of predicates, and every one of those letters is a tag name.
	anyTag = regexp.MustCompile(`<\s*/?\s*([a-zA-Z][a-zA-Z0-9]*)\s*(?:/?>|\s[^>]*>)`)
	// backticks is an inline code span. A paper about the web writes
	// `<table>` in prose and means the word, not the markup.
	backticks = regexp.MustCompile("`[^`]*`")
)

// markup is the first HTML tag left on a page and how many there are.
//
// This runs after the tidier, which is the whole point of where it sits. The
// prompt says in as many words that there is no HTML in the answer, not a
// table, not a br, not a sub or a sup. A model trained on web pages writes
// one anyway, and Untable and Unscript turn most of what it writes into the
// spelling this corpus uses. What is left is what they would have had to
// guess at, and the two of them refuse to guess for a reason: the TPU paper's
// first table came back with a header spanning four columns over five, and
// expanded as written the numbers land under the wrong headings, which is
// worse than no table at all because it reads like a table.
//
// So the page goes back and is asked again. That is cheaper than it sounds
// and better than the alternatives, which are a corpus with unreadable tables
// in it or a person editing HTML by hand. A second ask at a higher resolution
// is a different sample from the model and usually comes back as a fence.
//
// Fenced blocks and inline code are skipped, because a listing about HTML is
// full of tags and every one of them is content.
func markup(s string) (string, int) {
	lines := strings.Split(s, "\n")
	fenced := code.Inside(s)
	first, count := "", 0
	for i, line := range lines {
		if fenced[i+1] {
			continue
		}
		for _, m := range anyTag.FindAllStringSubmatch(backticks.ReplaceAllString(line, " "), -1) {
			if !HTMLTags[strings.ToLower(m[1])] {
				continue
			}
			count++
			if first == "" {
				first = strings.TrimSpace(m[0])
			}
		}
	}
	return first, count
}

// Unscript rewrites the subscripts and superscripts a reader wrote in HTML
// as the mathematics they are.
//
// Untable does this inside a table cell and has since the Transformer paper.
// It happens in running prose too, and there it is if anything more common,
// because a paper that names its quantities in the text names them the way it
// sets them: the GPT-3 paper writes "n<sub>params</sub> is the total number of
// trainable parameters, n<sub>layers</sub> is the total number of layers", and
// that is one paragraph with fourteen tags in it.
//
// A subscript on a name is mathematics and is written as mathematics. The
// prompt says so and says what to write, `$x_i$` and `$2^n$`, and this is the
// same conversion applied to a reader that answered in the other spelling.
// Both the name and the script go inside the dollars, because `$n$_params_ is
// not what the page prints and `n$_{params}$` is not mathematics anybody
// writes.
//
// A script with nothing in front of it is a table note rather than a script
// on a symbol, and it comes out as a superscript over nothing, `$^{a}$`.
// That is what the page prints and KaTeX reads it, and it is the same
// spelling the cells of the table above already use: the GPT-3 translation
// table has `45.6<sup>a</sup>` in a cell and `<sup>a</sup>[Tur20]` in the
// caption under it, the first meaning the score and the second saying whose
// score it is, and the two have to come out as the same mark or the caption
// stops explaining the table.
//
// The two it will not touch are the two where the guess would be wrong. A
// script inside a fence is a listing and its angle brackets are content. A
// script inside a math span is already inside dollars and nesting them would
// close the span early.
func Unscript(s string) string {
	lines := strings.Split(s, "\n")
	fenced := code.Inside(s)
	for i, line := range lines {
		if fenced[i+1] || !hasScript(line) {
			continue
		}
		lines[i] = joinScripts(line)
	}
	return strings.Join(lines, "\n")
}

var (
	hasScriptTag = regexp.MustCompile(`(?i)<(sub|sup)\b`)
	// bareScript is a script with no token in front of it, which is a table
	// note. script above will not match one, because it needs something to
	// attach to.
	bareScript = regexp.MustCompile(`(?is)<(sub|sup)\b[^>]*>(.*?)</(?:sub|sup)\s*>`)
)

func hasScript(line string) bool { return hasScriptTag.MatchString(line) }

// joinScripts converts every script on one line that is outside the line's
// mathematics.
//
// The offsets come from marksIn, which is the same delimiter scan Money uses,
// so a line where the reader mixed its own dollars with its HTML keeps them
// apart. An odd delimiter on the line means the line's mathematics is not
// paired and nothing here can tell inside from outside, so the line is left
// as it is and the page is refused.
func joinScripts(line string) string {
	at := marksIn(line)
	if len(at)%2 == 1 {
		return line
	}
	var b strings.Builder
	from := 0
	for i := 0; i+1 < len(at); i += 2 {
		b.WriteString(scripts(line[from:at[i].at]))
		b.WriteString(line[at[i].at : at[i+1].at+1])
		from = at[i+1].at + 1
	}
	b.WriteString(scripts(line[from:]))
	return b.String()
}

// scripts converts the scripts in one stretch of a line that is outside the
// line's mathematics. The attached ones first, so that a bare one is what is
// left over rather than the tail of one that had a token.
func scripts(s string) string {
	s = script.ReplaceAllStringFunc(s, scriptTeX)
	return bareScript.ReplaceAllStringFunc(s, bareTeX)
}

// bareTeX is a table note, as a superscript over nothing.
func bareTeX(m string) string {
	p := bareScript.FindStringSubmatch(m)
	mark := "_"
	if strings.EqualFold(p[1], "sup") {
		mark = "^"
	}
	return "$" + mark + "{" + strings.TrimSpace(p[2]) + "}$"
}

// scriptTeX is one matched token and script, as mathematics.
func scriptTeX(m string) string {
	p := script.FindStringSubmatch(m)
	mark := "_"
	if strings.EqualFold(p[2], "sup") {
		mark = "^"
	}
	return "$" + p[1] + mark + "{" + strings.TrimSpace(p[3]) + "}$"
}
