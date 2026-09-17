package tags

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// An Item is something in a body that can carry a permanent identifier.
//
// What counts is wider here than in Bourbaki, because a paper has fewer
// numbered statements and more of everything else: sections, figures, tables,
// numbered displays and listings are all things the prose refers to by number
// and all things a reader wants to link to. See 2141/02-corpus.md section 4.
//
// The scan is the one place that decides what is anchored. The assigner uses
// it to hand out tags and audit rule G05 uses it to ask whether everything it
// found carries one, so the command and the auditor cannot drift: a thing
// this function does not find is a thing neither of them asks about.
type Item struct {
	// Class is the word the attribute block carries after the anchor, one of
	// section, statement, equation, figure, table or code.
	Class string
	// Key is the anchor without the paper id: s3-2, thm-1, eq-4, fig-2. It is
	// derived from the number the paper printed and not from where the item
	// sits in the file, so it survives the paper being read again by a better
	// model, which is the whole point of a permanent identifier.
	Key string
	// Line is the index into the body's lines that the item was found on.
	Line int
	// Col is the byte offset within that line where an attribute block goes,
	// or -1 when the block belongs on a line of its own after it. An image
	// and a display equation take the second form because the block would
	// otherwise sit inside the syntax that carries them.
	Col int
	// Tag is the tag the item already carries, and is empty when it has none.
	Tag Tag
	// Anchor is the anchor it already carries, empty when it has none. A
	// mismatch between this and Key is an edit somebody made by hand, and the
	// assigner leaves it alone rather than renaming a published anchor.
	Anchor string
}

// Anchor is the anchor an item's key makes in a paper. The paper id is the
// prefix so that anchors sort by paper and so that a key is only ever unique
// within the paper that printed the number it came from.
func Anchor(paper, key string) string { return paper + "-" + key }

// Keys counts the keys one paper has asked for, so that a paper which printed
// the same number twice gets two anchors and not one.
//
// A key comes from what the paper printed, so two things a paper printed the
// same name for want the same key. The ResNet appendix has two sections headed
// MS COCO and two headed PASCAL VOC, one pair under Object Detection Baselines
// and one under Object Detection Improvements, and both pairs came out sharing
// an anchor and therefore a tag. Jacobson is the same shape a level up: the
// paper numbers three ways of losing equilibrium 1, 2 and 3, then numbers
// Slow-start 1 again, so two files claim section 1.
//
// The first of a name keeps the bare key, so nothing already published moves.
// The rest are numbered from two in reading order, which is as permanent as
// the key itself: both come from what the paper says and in the order it says
// it.
//
// Shared with the audit, which needs the same count in rule G04. The assigner
// had it and the rule did not, so the assigner wrote s1-2 on the second
// section numbered 1 and the rule read the file as s1 and refused the tag. Two
// papers were held out of the corpus on that and nothing could clear it.
//
// A Keys is per paper and per language, because an anchor is only unique
// within the paper it names and a translation has to start counting from the
// same place its English did.
type Keys map[string]int

// Unique is the key an item takes given what the paper has asked for already.
//
// It counts, so it has to be called for every item in reading order and not
// only for the ones the caller wants an answer about. A caller that skips the
// figures will number the sections wrong.
func (k Keys) Unique(key string) string {
	k[key]++
	if n := k[key]; n > 1 {
		return fmt.Sprintf("%s-%d", key, n)
	}
	return key
}

// SectionKey is the anchor key for the section a whole file is.
//
// papers split lifts a file's own heading into the front matter, so unlike
// every heading inside the body there is no line for an attribute block to
// sit on. The key is still the number the paper printed: file 03 of the
// Transformer paper is s3 whether its heading ended up in the body or in the
// front matter, and a link to s3 written against either one resolves.
//
// A file with no number and no kind the corpus names gets no key. An anchor
// made up from a file's position is not permanent, and two unnumbered
// sections of one paper would both want the same one.
func SectionKey(section, kind string) string {
	if section != "" {
		return "s" + strings.ToLower(strings.ReplaceAll(section, ".", "-"))
	}
	if kind == "front" || kind == "references" {
		return "s-" + kind
	}
	return ""
}

// Scan finds every anchorable item in a body, in reading order.
//
// It reads the Markdown line by line rather than parsing it, because the
// corpus is written by this toolchain and the four shapes it writes are
// known. A full parser would be more correct about Markdown nobody here
// writes and less clear about the four things that matter.
func Scan(body string) []Item {
	lines := strings.Split(body, "\n")
	var out []Item
	inFence, inDisplay := false, false
	display := 0
	// spoken is the caption lines an image above them has already claimed.
	spoken := map[int]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fenceLine.MatchString(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		switch {
		case inDisplay:
			if !strings.HasSuffix(trimmed, "$$") {
				continue
			}
			inDisplay = false
			if key := equationKey(lines[display : i+1]); key != "" {
				out = append(out, Item{Class: "equation", Key: key, Line: i, Col: -1})
			}
		case strings.HasPrefix(trimmed, "$$"):
			display = i
			// A display that opens and closes on the one line is finished
			// here. One that only opens is finished by the case above.
			if trimmed == "$$" || !strings.HasSuffix(trimmed, "$$") {
				inDisplay = true
				continue
			}
			if key := equationKey(lines[i : i+1]); key != "" {
				out = append(out, Item{Class: "equation", Key: key, Line: i, Col: -1})
			}
		case strings.HasPrefix(trimmed, "#"):
			if it, ok := heading(line); ok {
				it.Line = i
				out = append(out, it)
			}
		case imageLine.MatchString(trimmed):
			key, caption := figureKey(lines, i)
			if key == "" {
				continue
			}
			// The caption the number came from belongs to this image and is
			// not a second figure. Without this the corpus would anchor the
			// same diagram twice, once on the image and once on the words
			// underneath it.
			spoken[caption] = true
			out = append(out, Item{Class: "figure", Key: key, Line: i, Col: -1})
		case spoken[i]:
		default:
			if it, ok := labelled(line); ok {
				it.Line = i
				out = append(out, it)
			}
		}
	}
	return attach(out, lines)
}

// attach records the attribute block each item already carries. The blocks
// are matched by the line they sit on rather than by anchor, because an item
// whose anchor was edited by hand still has to be recognised as tagged.
func attach(items []Item, lines []string) []Item {
	for n, it := range items {
		at := it.Line
		if it.Col < 0 && at+1 < len(lines) {
			at++
		}
		for _, a := range ParseAttrs(lines[at]) {
			items[n].Tag = a.Tag
			items[n].Anchor = a.Anchor
			break
		}
	}
	return items
}

var (
	fenceLine = regexp.MustCompile("^(```|~~~)")
	imageLine = regexp.MustCompile(`^!\[[^\]]*\]\([^)]+\)\s*$`)
	// headingLine splits a heading into its hashes and its title. The title
	// keeps any attribute block, which is stripped before it is read.
	headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	// numberedTitle is a section title that opens with its number: "3.2
	// Attention". The trailing dot is optional because both styles are in
	// print.
	numberedTitle = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+(\S.*)$`)
	// captionLine is a caption paragraph, with or without the italics the
	// corpus wraps it in once the figure has been cropped out.
	captionLine = regexp.MustCompile(`^\*?\*?(Figure|Fig\.|Table|Algorithm|Listing)\s+(\d+(?:\.\d+)*)\*?\*?\s*[.:]`)
	// statementLine is a numbered statement, bold or plain. The number may be
	// sectioned, as Theorem 3.1, which is how a mathematics paper numbers.
	statementLine = regexp.MustCompile(`^\*?\*?(Theorem|Lemma|Proposition|Definition|Corollary|Claim|Conjecture|Remark|Example|Observation|Fact)\s+(\d+(?:\.\d+)*)\*?\*?\s*[.:]?`)
	// equationNumber is the number a paper prints beside a display, either as
	// the TeX the writer emits or as the bare parenthesis pdftotext leaves at
	// the end of the line.
	equationTag    = regexp.MustCompile(`\\tag\{(\d+(?:\.\d+)*)\}`)
	equationTrails = regexp.MustCompile(`\((\d+(?:\.\d+)*)\)\s*\$*\s*$`)
	// pathNumber is the digits in a cropped figure's file name, f02.png.
	pathNumber = regexp.MustCompile(`(\d+)`)
)

// classes maps the word a paper prints to the class the corpus files it
// under and the prefix its anchor uses.
var classes = map[string]struct{ class, prefix string }{
	"Figure":      {"figure", "fig"},
	"Fig.":        {"figure", "fig"},
	"Table":       {"table", "tab"},
	"Algorithm":   {"code", "alg"},
	"Listing":     {"code", "lst"},
	"Theorem":     {"statement", "thm"},
	"Lemma":       {"statement", "lem"},
	"Proposition": {"statement", "prop"},
	"Definition":  {"statement", "def"},
	"Corollary":   {"statement", "cor"},
	"Claim":       {"statement", "clm"},
	"Conjecture":  {"statement", "conj"},
	"Remark":      {"statement", "rem"},
	"Example":     {"statement", "ex"},
	"Observation": {"statement", "obs"},
	"Fact":        {"statement", "fact"},
}

// heading reads a section heading.
func heading(line string) (Item, bool) {
	m := headingLine.FindStringSubmatch(line)
	if m == nil {
		return Item{}, false
	}
	title := strings.TrimSpace(attrPattern.ReplaceAllString(m[2], ""))
	if title == "" {
		return Item{}, false
	}
	key := ""
	if n := numberedTitle.FindStringSubmatch(title); n != nil {
		key = "s" + strings.ReplaceAll(n[1], ".", "-")
	} else {
		key = "s-" + slug(title)
	}
	if key == "s-" {
		return Item{}, false
	}
	return Item{Class: "section", Key: key, Col: len(strings.TrimRight(line, " \t"))}, true
}

// labelled reads a caption or a numbered statement, which are the same shape:
// a word, a number, and then the prose.
func labelled(line string) (Item, bool) {
	m := captionLine.FindStringSubmatch(line)
	if m == nil {
		m = statementLine.FindStringSubmatch(line)
	}
	if m == nil {
		return Item{}, false
	}
	c, ok := classes[m[1]]
	if !ok {
		return Item{}, false
	}
	return Item{
		Class: c.class,
		Key:   c.prefix + "-" + strings.ReplaceAll(m[2], ".", "-"),
		Col:   len(strings.TrimRight(line, " \t")),
	}, true
}

// figureKey is the number of the figure an image line carries.
//
// The caption is the first place to look, because the number in it is the
// number the paper printed and is what the prose says when it says "see
// Figure 2". The file name is second, because papers figures names a crop
// after the figure it cropped. A figure with neither is not anchored: an
// anchor invented from a position in a file is not permanent, and an
// unnumbered figure is not something the prose can refer to anyway.
//
// It also returns the line the caption was on, so the caller can leave that
// line alone rather than anchoring the same diagram a second time.
func figureKey(lines []string, at int) (string, int) {
	for i := at + 1; i < len(lines) && i <= at+3; i++ {
		text := strings.TrimSpace(lines[i])
		if text == "" || strings.HasPrefix(text, "{#") {
			continue
		}
		if m := captionLine.FindStringSubmatch(text); m != nil {
			if c, ok := classes[m[1]]; ok {
				return c.prefix + "-" + strings.ReplaceAll(m[2], ".", "-"), i
			}
		}
		break
	}
	file := path.Base(strings.TrimSpace(imagePath(lines[at])))
	if m := pathNumber.FindStringSubmatch(file); m != nil {
		return "fig-" + strings.TrimLeft(m[1], "0"), -1
	}
	return "", -1
}

// imagePath is the target of a Markdown image.
func imagePath(line string) string {
	open := strings.Index(line, "](")
	if open < 0 {
		return ""
	}
	rest := line[open+2:]
	if end := strings.IndexByte(rest, ')'); end >= 0 {
		return rest[:end]
	}
	return ""
}

// equationKey is the number a paper printed beside a display.
//
// An unnumbered display gets no anchor. The corpus anchors an equation so
// that "as (3) shows" can be a link, and a display the paper did not number
// is one nothing refers to.
func equationKey(block []string) string {
	text := strings.Join(block, "\n")
	if m := equationTag.FindStringSubmatch(text); m != nil {
		return "eq-" + strings.ReplaceAll(m[1], ".", "-")
	}
	for i := len(block) - 1; i >= 0; i-- {
		line := strings.TrimSpace(block[i])
		if line == "" || line == "$$" {
			continue
		}
		if m := equationTrails.FindStringSubmatch(line); m != nil {
			return "eq-" + strings.ReplaceAll(m[1], ".", "-")
		}
		break
	}
	return ""
}

// slug is a title reduced to something that can sit in a URL fragment. It is
// only used for a heading the paper did not number, which in practice is
// Abstract, Acknowledgements and Appendix A.
func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// Apply writes an attribute block for each item into a body, and returns the
// body as it now stands.
//
// The items must be the ones Scan returned for this body, and the blocks must
// line up with them. It works from the last item to the first so that the
// line numbers Scan recorded stay true as lines are inserted above them.
func Apply(body string, items []Item, blocks []string) (string, error) {
	if len(items) != len(blocks) {
		return "", fmt.Errorf("%d items and %d blocks", len(items), len(blocks))
	}
	lines := strings.Split(body, "\n")
	for i := len(items) - 1; i >= 0; i-- {
		it, block := items[i], blocks[i]
		if block == "" {
			continue
		}
		if it.Line >= len(lines) {
			return "", fmt.Errorf("%s is on line %d of a body with %d lines", it.Key, it.Line+1, len(lines))
		}
		if it.Col < 0 {
			lines = append(lines[:it.Line+1], append([]string{block}, lines[it.Line+1:]...)...)
			continue
		}
		line := lines[it.Line]
		if it.Col > len(line) {
			return "", fmt.Errorf("%s is at column %d of a line %d long", it.Key, it.Col, len(line))
		}
		lines[it.Line] = line[:it.Col] + " " + block + line[it.Col:]
	}
	return strings.Join(lines, "\n"), nil
}
