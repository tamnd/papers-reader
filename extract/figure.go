package extract

import (
	"regexp"
	"strings"
	"unicode"
)

// link is a Markdown image on a line of its own.
var link = regexp.MustCompile(`^!\[[^\]]*\]\([^)]*\)$`)

// Unlink takes out the image links a reader invented.
//
// Nothing in this toolchain asks a reader for one. The prompt says to write
// the caption of a figure and nothing else, because the words inside a diagram
// read as word soup in the middle of a paragraph, and the pictures themselves
// are cropped later by papers figures under names the corpus chooses. A reader
// that writes `![a chain of transactions](../images/transactions.png)` has
// invented three things at once: a description of a picture, a directory and a
// file name, and none of them is on the page.
//
// Left alone it fails the audit rather than passing it, which is the one
// consolation. Rule F01 follows every image reference in the published content
// and reports the ones that point at nothing, so four invented links on one
// paper were four hard failures. They are a hard failure for a good reason: a
// broken image is the first thing a reader of the corpus sees. But the way to
// clear them is not to crop a picture to fit a name a model made up.
//
// What replaces the link is what the prompt asked for in the first place. A
// figure the page captions keeps its caption and loses the link; a figure the
// page leaves uncaptioned becomes `Figure.` on a line of its own, which is the
// corpus's way of writing down that a picture sits here and says nothing.
//
// The two are told apart by the white space around the link, which sounds
// thin and is what the pages actually do. A reader writing a caption puts it
// on the line under the image with no blank line between, because that is what
// a caption is. A reader that found no caption to write leaves the image in a
// paragraph of its own with blank lines on both sides. So a link pressed up
// against a line of text has its caption already and is dropped, and a link
// standing alone in white space is the one that becomes `Figure.`.
//
// An image that shares its line with something else is a third case and it
// takes the picture out and keeps the line. Page 6 of McCabe's paper prints
// three little flow graphs in a row with a name and a number beside each
// one, and the reader wrote them as `G1: ![Graph G1](../images/graph_G1.png)
// $v = 6$`. The name and the number are on the page, the file is not, and
// there is no caption here to drop the line in favour of.
//
// An image with prose on both sides of it stays where it is, because that
// one is a word of the sentence rather than a picture beside it. "The symbol
// ![dagger](dagger.png) marks the second author" is a sentence with a hole
// in it once the image goes, and a hole in a sentence is worse than a broken
// image: F01 reports the broken image and nothing reports the hole.
func Unlink(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if code[i] {
			out = append(out, line)
			continue
		}
		if !link.MatchString(strings.TrimSpace(line)) {
			out = append(out, unpicture(line))
			continue
		}
		if touching(lines, i) {
			continue
		}
		out = append(out, "Figure.")
	}
	return strings.Join(out, "\n")
}

var (
	// inlineImage is a Markdown image anywhere on a line.
	inlineImage = regexp.MustCompile(`!\[[^\]\n]*\]\([^)\n]*\)`)
	// runOfSpaces is what taking the image out leaves behind.
	runOfSpaces = regexp.MustCompile(`  +`)
	// mathSpan is a formula between dollars, which is not prose however many
	// letters it has in it.
	mathSpan = regexp.MustCompile(`\$[^$\n]*\$`)
)

// unpicture takes the images out of a line that has other text on it, and
// leaves the ones that stand between two pieces of prose.
//
// The words in the alt text go with the image. They are the reader's
// description of a picture and not anything the page prints, which is the
// same reason the link goes, and Delink's rule about keeping the text of a
// link does not carry over: the text of an ordinary link is what the page
// printed and the alt text of an invented image is not.
func unpicture(line string) string {
	if !strings.Contains(line, "](") {
		return line
	}
	var out strings.Builder
	at := 0
	for _, s := range inlineImage.FindAllStringIndex(line, -1) {
		if hasProse(line[:s[0]]) && hasProse(line[s[1]:]) {
			continue
		}
		out.WriteString(line[at:s[0]])
		at = s[1]
	}
	if at == 0 {
		return line
	}
	out.WriteString(line[at:])
	return strings.TrimRight(runOfSpaces.ReplaceAllString(out.String(), " "), " \t")
}

// hasProse reports whether a piece of a line carries a word, counting only
// what is outside the mathematics. `$v = 6$` is a formula and not a word,
// and a label like `G1:` is a word.
func hasProse(s string) bool {
	return strings.IndexFunc(mathSpan.ReplaceAllString(s, ""), unicode.IsLetter) >= 0
}

// touching reports whether the line above or the line below could be the
// caption of the line at i.
//
// The edges of the page count as blank: a page that opens or closes with a
// figure has nothing above or below it to be its caption. So does another
// image link, because two pictures in a row are two pictures and neither of
// them is the caption of the other.
func touching(lines []string, i int) bool {
	return captions(lines, i-1) || captions(lines, i+1)
}

func captions(lines []string, i int) bool {
	if i < 0 || i >= len(lines) {
		return false
	}
	trimmed := strings.TrimSpace(lines[i])
	return trimmed != "" && !link.MatchString(trimmed)
}
