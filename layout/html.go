package layout

import (
	"html"
	"regexp"
	"strings"
)

// Marker and Docling both hand back fragments of HTML where MinerU hands
// back plain text and a span type. These are the readers for that: enough
// HTML to get the text, the mathematics and the picture out of a fragment
// that is at most a few tags deep, and no more.

// mathHolders is every tag that either tool has been seen to put an equation
// in: <math> for MathML, and a span, a p or a div carrying a class that says
// so.
var mathHolders = []string{"math", "span", "p", "div"}

var (
	// imgTag is a picture, whose src is the file written next to the JSON.
	imgTag = regexp.MustCompile(`(?is)<img\b[^>]*\bsrc\s*=\s*["']([^"']+)["']`)
	// headingTag is <h1> to <h6>, which is how marker says how deep a
	// section heading is.
	headingTag = regexp.MustCompile(`(?is)<h([1-6])\b`)
	// blockEnd is a tag that ends a line rather than joining words. Without
	// these two paragraphs run into one word at the join.
	blockEnd = regexp.MustCompile(`(?is)</(p|div|li|tr|h[1-6])>|<br\s*/?>`)
)

// htmlText is a fragment as prose, with the mathematics kept as $..$.
//
// Walked rather than matched with one expression, because finding the end of
// a <span> means finding its own closing tag and Go's regexp has no back
// reference to match one with. The walk is the honest way to do it and it is
// the same scanner the table reader uses.
func htmlText(fragment string) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	var out strings.Builder
	// Prose has its tags taken out as it is written, and mathematics goes in
	// untouched. Stripping tags from the whole string at the end would be
	// shorter and wrong: "$a < b$" holds a less than sign, and a tag stripper
	// reads that as the start of a tag and eats the rest of the sentence.
	prose := func(s string) {
		out.WriteString(stripTags(blockEnd.ReplaceAllString(s, " ")))
	}
	lower := strings.ToLower(fragment)
	i := 0
	for {
		at, tag := nextTag(lower, mathHolders, i)
		if at < 0 {
			prose(fragment[i:])
			break
		}
		gt := strings.IndexByte(fragment[at:], '>')
		if gt < 0 {
			prose(fragment[i:])
			break
		}
		body := at + gt + 1
		end := indexTag(lower, "</"+tag, body)
		if end < 0 || !isMath(fragment[at:body]) {
			// Not mathematics, or a tag that never closes. Write out the
			// opening tag and carry on inside it: what it holds may still be
			// an equation, and a <p> wrapping a <span class="math"> is the
			// commonest shape marker writes.
			prose(fragment[i:body])
			i = body
			continue
		}
		prose(fragment[i:at])
		// No space is added either side. The original spacing is already in
		// the prose around it, and adding one turns "$x$-axis" into two words.
		if tex := unwrapMath(strings.TrimSpace(stripTags(fragment[body:end]))); tex != "" {
			out.WriteString(wrapMath(tex))
		}
		i = end
	}
	return collapse(out.String())
}

// htmlMath is a fragment that is one display equation, as bare TeX.
func htmlMath(fragment string) string {
	tex := strings.TrimSpace(stripTags(fragment))
	return unwrapMath(tex)
}

// htmlImage is the src of the first picture in a fragment.
func htmlImage(fragment string) string {
	m := imgTag.FindStringSubmatch(fragment)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// htmlLevel is how deep a heading is, defaulting to one. A tool that
// reported no level at all is reporting a section, and a section is level
// one; guessing deeper would bury it inside whatever came before.
func htmlLevel(fragment string) int {
	m := headingTag.FindStringSubmatch(fragment)
	if m == nil {
		return 1
	}
	return int(m[1][0] - '0')
}

// codeText is a listing with its whitespace intact.
//
// Everything else in this file collapses whitespace, because a PDF breaks
// its lines where the typesetter chose and those breaks mean nothing. A
// listing is the exception: it is the one place in the corpus where
// whitespace is content, so the tags come out and nothing else does.
func codeText(fragment string) string {
	s := blockEnd.ReplaceAllString(fragment, "\n")
	s = stripTags(s)
	s = html.UnescapeString(s)
	return strings.Trim(s, "\n")
}

// isMath says whether a tag that could hold mathematics does. <math> always
// does; a span or a div does when it is classed as one.
func isMath(tag string) bool {
	lower := strings.ToLower(tag)
	if strings.HasPrefix(lower, "<math") {
		return true
	}
	i := strings.IndexByte(lower, '>')
	if i < 0 {
		return false
	}
	open := lower[:i]
	return strings.Contains(open, "math") ||
		strings.Contains(open, "equation") ||
		strings.Contains(open, "formula")
}

// unwrapMath takes the delimiters off TeX that arrived with them, so that
// every equation in the corpus is wrapped exactly once. A double wrap is an
// empty math span followed by prose that a renderer then reads as TeX.
func unwrapMath(tex string) string {
	tex = strings.TrimSpace(tex)
	for {
		before := tex
		switch {
		case strings.HasPrefix(tex, "$$") && strings.HasSuffix(tex, "$$") && len(tex) > 4:
			tex = tex[2 : len(tex)-2]
		case strings.HasPrefix(tex, "$") && strings.HasSuffix(tex, "$") && len(tex) > 2:
			tex = tex[1 : len(tex)-1]
		case strings.HasPrefix(tex, `\[`) && strings.HasSuffix(tex, `\]`):
			tex = tex[2 : len(tex)-2]
		case strings.HasPrefix(tex, `\(`) && strings.HasSuffix(tex, `\)`):
			tex = tex[2 : len(tex)-2]
		}
		tex = strings.TrimSpace(tex)
		if tex == before {
			return tex
		}
	}
}

// stripTags takes the markup out of a fragment and leaves the text.
//
// A bare < is left where it is. "a < b" is a comparison and not the start of
// a tag, papers are full of them, and a stripper that took every < as a tag
// would swallow everything up to the next > and lose the rest of the
// sentence with it. So a < only opens a tag when a letter, a slash, a bang
// or a question mark follows it, which is what a tag can start with.
func stripTags(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '<' || !tagStart(s, i+1) {
			b.WriteByte(s[i])
			i++
			continue
		}
		gt := strings.IndexByte(s[i:], '>')
		if gt < 0 {
			// A tag that never closes. What is left is text as far as anyone
			// can tell, and dropping it would drop the end of the page.
			b.WriteString(s[i:])
			break
		}
		i += gt + 1
	}
	return html.UnescapeString(b.String())
}

// tagStart reports whether what follows a < can begin a tag.
func tagStart(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	switch c := s[i]; {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	case c == '/', c == '!', c == '?':
		return true
	}
	return false
}
