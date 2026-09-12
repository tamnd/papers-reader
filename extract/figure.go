package extract

import (
	"regexp"
	"strings"
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
func Unlink(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if code[i] || !link.MatchString(strings.TrimSpace(line)) {
			out = append(out, line)
			continue
		}
		if touching(lines, i) {
			continue
		}
		out = append(out, "Figure.")
	}
	return strings.Join(out, "\n")
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
