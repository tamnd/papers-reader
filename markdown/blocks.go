package markdown

import (
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/mathtex"
	"github.com/tamnd/papers-reader/tags"
)

// Blocks cuts a body into blocks on blank lines, keeping a fenced block whole.
//
// A listing has blank lines in it and they are part of the listing. Cutting
// on every blank line would set the halves of a shell transcript as three
// paragraphs of prose with the escaping applied, which is how a document
// grows a stray backslash in the middle of a command line.
func Blocks(body string) []string {
	var out []string
	var cur []string
	fence := ""
	flush := func() {
		if len(cur) > 0 {
			if s := strings.Trim(strings.Join(cur, "\n"), "\n"); strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		switch {
		case fence != "":
			cur = append(cur, line)
			if strings.HasPrefix(strings.TrimSpace(line), fence) {
				fence = ""
				flush()
			}
		case strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~"):
			flush()
			fence = line[:3]
			cur = append(cur, line)
		case strings.TrimSpace(line) == "":
			flush()
		default:
			cur = append(cur, line)
		}
	}
	flush()
	return out
}

var attrTail = regexp.MustCompile(`\s*\{#[A-Za-z0-9][-A-Za-z0-9_.]*(?:\s+\.[a-z]+)*\s+tag=[0-9A-Fa-f]{4}\}$`)

// TakeAttr pulls the attribute block off the end of a block.
//
// It goes one of two ways and papers tags writes both. A displayed equation
// carries it on the line under the closing dollars, because a line of
// mathematics ends where it ends. A caption or a heading carries it at the
// end of its own last line, because the caption is one paragraph and an
// attribute block on a line of its own after it would be a second one.
//
// Either way it is not part of the text. Left in, the first form sets as a
// paragraph of braces in the middle of the paper and the second sets as a
// sentence of braces on the end of a caption, which is what the first build
// of this did.
func TakeAttr(b string) (string, tags.Attr, bool) {
	text := strings.TrimRight(b, "\n")
	loc := attrTail.FindStringIndex(text)
	if loc == nil {
		return b, tags.Attr{}, false
	}
	all := tags.ParseAttrs(text[loc[0]:])
	if len(all) != 1 {
		return b, tags.Attr{}, false
	}
	rest := strings.TrimRight(text[:loc[0]], " \t\n")
	if strings.TrimSpace(rest) == "" {
		// The whole block was the attribute block and nothing else, which is
		// not a thing the corpus writes. Leave it alone rather than returning
		// an anchor on an empty paragraph.
		return b, tags.Attr{}, false
	}
	return rest, all[0], true
}

// Has says a set of attribute classes holds one.
func Has(classes []string, want string) bool {
	for _, c := range classes {
		if c == want {
			return true
		}
	}
	return false
}

var headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

// Heading reads a block that opens with a heading and gives its depth and its
// text. Depth is the number of hashes, so a line of four is depth four.
func Heading(text string) (depth int, title string, ok bool) {
	m := headingLine.FindStringSubmatch(strings.SplitN(text, "\n", 2)[0])
	if m == nil {
		return 0, "", false
	}
	return len(m[1]), m[2], true
}

// IsHeading says the block opens with a heading line.
func IsHeading(text string) bool { return headingLine.MatchString(text) }

// IsFenced says the block is a fenced listing.
func IsFenced(text string) bool {
	return strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~")
}

// Fence is the word on the opening fence, which is the language of the
// listing where whoever wrote it said so and the empty string where they did
// not. The corpus mostly does not: what it holds is pseudocode.
func Fence(text string) string {
	line := strings.SplitN(text, "\n", 2)[0]
	return strings.TrimSpace(strings.TrimLeft(line, "`~"))
}

// Fenced strips the two fence lines off a code block.
func Fenced(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		lines = lines[1:]
	}
	if n := len(lines); n > 0 {
		if t := strings.TrimSpace(lines[n-1]); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			lines = lines[:n-1]
		}
	}
	return strings.Join(lines, "\n")
}

// IsDisplay says the block is one displayed equation and nothing else.
func IsDisplay(text string) bool {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "$$") || !strings.HasSuffix(t, "$$") || len(t) < 5 {
		return false
	}
	spans, unclosed := mathtex.Split(t)
	return unclosed == nil && len(spans) == 1 && spans[0].Display
}

// TeX is the mathematics of a displayed equation, without its dollars.
func TeX(text string) string {
	t := strings.TrimSpace(text)
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(t, "$$"), "$$"))
}

var tagged = regexp.MustCompile(`\\tag\{([^}]*)\}`)

// Tagged says a displayed equation carries a number the paper printed.
func Tagged(tex string) bool { return tagged.MatchString(tex) }

// Tag is the number a displayed equation carries in a \tag, which is the
// number the paper printed, and the empty string where the paper printed
// none. A number this toolchain invented would be worse than no number,
// because every cross reference in the prose is to the paper's numbering.
func Tag(tex string) string {
	m := tagged.FindStringSubmatch(tex)
	if m == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(m[1]), "()")
}

var (
	// Bullet and Ordinal open a list item. They are exported because a
	// renderer strips the marker off each line before setting the item.
	Bullet  = regexp.MustCompile(`^\s*[-*+]\s+`)
	Ordinal = regexp.MustCompile(`^\s*[0-9]+[.)]\s+`)
)

// IsList says every line of the block opens an item. A block where only some
// lines do is prose that happens to start with a dash, and setting it as a
// list would break the sentence across items.
func IsList(text string) bool {
	lines := strings.Split(text, "\n")
	kind := 0
	for _, l := range lines {
		switch {
		case Bullet.MatchString(l):
			if kind == 2 {
				return false
			}
			kind = 1
		case Ordinal.MatchString(l):
			if kind == 1 {
				return false
			}
			kind = 2
		default:
			return false
		}
	}
	return kind != 0
}

// Ordered says a list is numbered rather than bulleted.
func Ordered(text string) bool {
	return Ordinal.MatchString(strings.Split(text, "\n")[0])
}

// Items is the lines of a list with the markers taken off.
func Items(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		item := Bullet.ReplaceAllString(l, "")
		out = append(out, Ordinal.ReplaceAllString(item, ""))
	}
	return out
}

var ruleRow = regexp.MustCompile(`^\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?$`)

// IsTable says the block is a pipe table: at least two rows, and the second
// one is the rule.
func IsTable(text string) bool {
	lines := strings.Split(text, "\n")
	return len(lines) >= 2 && strings.Contains(lines[0], "|") && ruleRow.MatchString(strings.TrimSpace(lines[1]))
}

// Cells cuts one row of a pipe table into its cells.
func Cells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	out := strings.Split(line, "|")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

// Rows cuts a pipe table into a header row and the rest, the rule dropped.
func Rows(text string) (head []string, body [][]string) {
	for i, l := range strings.Split(text, "\n") {
		switch {
		case i == 0:
			head = Cells(l)
		case i == 1:
			continue
		default:
			body = append(body, Cells(l))
		}
	}
	return head, body
}

// CaptionPrefix is the word and the number a caption opens with, as in
// "Figure 2:" or "Hinh 1.". The word is whatever the paper printed, which in
// a translation is the translated word, so it is read off the text rather
// than looked up in a table of four languages.
//
// The bold runs either side of the delimiter because the corpus writes both
// and it depends on what the paper printed: "**Figure 2:** a network" bolds
// up to the colon and "**Hinh 2.** mot mang" bolds the full stop as well.
var CaptionPrefix = regexp.MustCompile(`^\*{0,2}([^\s*]+)\s*([0-9]+(?:\.[0-9]+)*)\s*(?:[:.]\s*\*{0,2}|\*{0,2}\s*[:.])\s*`)

// CaptionNumber is the number a caption opens with, and the caption with that
// opening cut off. A caption whose number is not the one the figure is filed
// under is left whole, because the two disagreeing means the prefix was not a
// caption prefix at all.
func CaptionNumber(text, want string) (number, rest string) {
	m := CaptionPrefix.FindStringSubmatch(text)
	if m == nil || (want != "" && m[2] != want) {
		return "", text
	}
	return m[2], text[len(m[0]):]
}
