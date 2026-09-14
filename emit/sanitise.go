package emit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// allowed is every element a page's HTML may hold.
//
// It is short on purpose. The corpus is text, the app supplies the
// typography, and every element on this list is here because a paper needs
// it to mean what it says: emphasis, a code face, a superscript for a
// footnote, a table, a list, a link, and a span for a formula. There is no
// div, no class the app did not ask for, no style attribute and no image:
// a figure is a block with a source and a size rather than markup, so the
// app can lay it out and the allowlist never has to reason about a URL.
//
// Nothing on this list can execute. That matters more here than it would in
// most places, because most of the text on this site was written by a model
// reading a photograph of a page, and a model that returned a script tag
// instead of a sentence must produce a broken paragraph and not a broken
// site.
var allowed = map[string]bool{
	"p": true, "em": true, "strong": true, "code": true, "a": true,
	"sup": true, "sub": true, "ul": true, "ol": true, "li": true,
	"table": true, "thead": true, "tbody": true, "tr": true, "th": true,
	"td": true, "span": true,
}

// Allowed is the allowlist, in the order it is written above, for anything
// that wants to say what it is.
func Allowed() []string {
	out := make([]string, 0, len(allowed))
	for t := range allowed {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

var (
	closeTag = regexp.MustCompile(`</\s*([A-Za-z][-A-Za-z0-9]*)\s*>`)
	anyTag   = regexp.MustCompile(`</?\s*([A-Za-z][-A-Za-z0-9]*)([^>]*)>`)
	// katexRoot is the span KaTeX wraps its output in, which is katex for an
	// inline span and katex-display for a displayed one. Everything inside
	// one is KaTeX's own markup and is not checked against the allowlist.
	// That is the one exception this file makes, which is why it is pinned
	// to the two class names KaTeX opens with rather than to any class with
	// the word katex somewhere in it.
	katexRoot = regexp.MustCompile(`^<span class="katex(-display)?(| [^"]*)"`)
	// handler is an attribute that would run something. KaTeX emits none and
	// this file writes none, so a match is either a bug here or markup that
	// got through the escaping, and both are worth failing over.
	handler = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	// scheme is a link target this site will follow. Relative, or an anchor
	// on the page, or plain http. Anything else, javascript: above all, is
	// refused.
	scheme = regexp.MustCompile(`(?i)href\s*=\s*"(#|/|https?://)`)
)

// Check reads one run of a page's HTML and says what is wrong with it.
//
// It is a check and not a filter. The emitter builds this markup itself, out
// of escaped text and its own tags, so in a correct build there is nothing
// to strip and a filter would be a quiet place for a fault to be swallowed.
// What a check does instead is fail the audit and name the page. Rule P01
// runs it over every block of every page.
//
// Inside a KaTeX span nothing is checked. KaTeX emits MathML as well as
// HTML, which is what a screen reader reads the formula from, and listing
// thirty MathML elements here would be this file claiming to know what a
// future version of KaTeX emits. What makes that safe is on the other side:
// KaTeX is given TeX with throwOnError set and no macros and no trust, so
// it cannot be talked into emitting markup, and the TeX it is given was
// already checked by audit rule M04.
func Check(s string) []string {
	var out []string
	say := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		for _, seen := range out {
			if seen == msg {
				return
			}
		}
		out = append(out, msg)
	}
	depth := 0 // how deep inside a KaTeX span we are, zero being outside
	for _, loc := range anyTag.FindAllStringIndex(s, -1) {
		tag := s[loc[0]:loc[1]]
		name := strings.ToLower(anyTag.FindStringSubmatch(tag)[1])
		if depth > 0 {
			switch {
			case closeTag.MatchString(tag):
				depth--
			case !strings.HasSuffix(tag, "/>"):
				depth++
			}
			continue
		}
		if katexRoot.MatchString(tag) {
			depth = 1
			continue
		}
		if !allowed[name] {
			say("<%s> is not on the allowlist", name)
		}
		if closeTag.MatchString(tag) {
			// A closing tag carries no attributes, so the two checks below
			// would read every </a> as a link to nowhere.
			continue
		}
		if handler.MatchString(tag) {
			say("<%s> carries an event handler", name)
		}
		if name == "a" && !scheme.MatchString(tag) {
			say("<a> links somewhere this site will not follow")
		}
	}
	if depth > 0 {
		say("a KaTeX span is not closed")
	}
	return out
}

// CheckPage reads every run of HTML one page carries.
func CheckPage(p *Page) []Fault {
	var out []Fault
	at := PagePath(p.ID, p.Lang)
	add := func(where, s string) {
		for _, why := range Check(s) {
			out = append(out, Fault{Page: at, Kind: FaultMarkup, What: where + ": " + why})
		}
	}
	for _, b := range p.Front.Blocks {
		add(fmt.Sprintf("front block %d", b.I), b.HTML)
		add(fmt.Sprintf("front block %d caption", b.I), b.CaptionHTML)
	}
	for _, s := range p.Sections {
		for _, b := range s.Blocks {
			add(fmt.Sprintf("%s block %d", s.Anchor, b.I), b.HTML)
			add(fmt.Sprintf("%s block %d caption", s.Anchor, b.I), b.CaptionHTML)
		}
	}
	for _, n := range p.Notes {
		add("note "+n.Key, n.HTML)
	}
	return out
}
