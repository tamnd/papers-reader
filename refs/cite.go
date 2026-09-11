package refs

import (
	"regexp"
	"strconv"
	"strings"
)

// citation is an in-text citation in a numbered paper: [31], [2, 3] or a
// span like [4-7]. Only numbers, because a bracket holding anything else is
// as likely to be a footnote marker, an editorial insertion or a piece of
// notation, and rewriting one of those would corrupt the sentence.
var citation = regexp.MustCompile(`\[(\d{1,3}(?:\s*[-–,]\s*\d{1,3})*)\]`)

// Rewrite turns the in-text citations of a body into links to the corpus.
//
// A citation whose entry resolved becomes [[id]], which is the same link
// form the rest of the corpus uses and which survives translation as a
// protected span. A citation whose entry did not resolve is left exactly as
// the paper printed it, and links to the entry in that paper's own
// reference list. Both forms are wanted: the second is the honest answer for
// the great majority of references, which are to papers this corpus will
// never hold.
//
// A group is rewritten only if at least one of its citations resolved,
// because rewriting [4-7] into [4], [5], [6], [7] with nothing gained is
// churn in a diff and a change to the author's text for no reason.
func Rewrite(body string, links map[string]string) string {
	if len(links) == 0 {
		return body
	}
	return outsideCode(body, func(prose string) string {
		return citation.ReplaceAllStringFunc(prose, func(m string) string {
			return rewriteGroup(m, links)
		})
	})
}

// Rewrite is the manifest's own citations.
func (m *Manifest) Rewrite(body string) string { return Rewrite(body, m.Links()) }

// Citations lists the in-text citation labels a body carries, in the order
// they appear and with the spans opened out. It is what audit rule R02
// checks against the bibliography, and it reads a body the same way Rewrite
// does, so a bracket the rewriter would not touch is not one the audit
// complains about either.
func Citations(body string) []string {
	var out []string
	outsideCode(body, func(prose string) string {
		for _, m := range citation.FindAllStringSubmatch(prose, -1) {
			out = append(out, expand(m[1])...)
		}
		return prose
	})
	return out
}

// linked is a link to another paper in the corpus, which is what a citation
// that resolved was rewritten into.
var linked = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// Linked lists the papers a body links to.
func Linked(body string) []string {
	var out []string
	for _, m := range linked.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func rewriteGroup(match string, links map[string]string) string {
	keys := expand(match[1 : len(match)-1])
	if len(keys) == 0 {
		return match
	}
	out := make([]string, 0, len(keys))
	hit := false
	for _, key := range keys {
		if id, ok := links[key]; ok {
			out = append(out, "[["+id+"]]")
			hit = true
			continue
		}
		out = append(out, "["+key+"]")
	}
	if !hit {
		return match
	}
	return strings.Join(out, ", ")
}

// expand reads the numbers out of a citation group, opening a span like 4-7
// into the entries it stands for. A span running the wrong way or longer
// than the bibliography plausibly is, is not a span.
func expand(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi, span := cutSpan(part)
		if !span {
			if part == "" {
				return nil
			}
			out = append(out, part)
			continue
		}
		if hi <= lo || hi-lo > 40 {
			return nil
		}
		for n := lo; n <= hi; n++ {
			out = append(out, strconv.Itoa(n))
		}
	}
	return out
}

func cutSpan(part string) (lo, hi int, ok bool) {
	i := strings.IndexAny(part, "-–")
	if i <= 0 {
		return 0, 0, false
	}
	left, right := strings.TrimSpace(part[:i]), strings.TrimSpace(strings.TrimLeft(part[i:], "-– "))
	lo, err := strconv.Atoi(left)
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.Atoi(right)
	if err != nil {
		return 0, 0, false
	}
	return lo, hi, true
}

// outsideCode applies a rewrite to the prose of a body and leaves the
// mathematics and the code listings alone.
//
// A bracket inside a listing is an array index and a bracket inside
// mathematics is a matrix or an interval, and turning either into a citation
// would be a silent corruption of the one thing in a paper that has to be
// exact.
func outsideCode(body string, f func(string) string) string {
	lines := strings.Split(body, "\n")
	fenced, display := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
			fenced = !fenced
			continue
		case fenced:
			continue
		case trimmed == "$$":
			display = !display
			continue
		case display:
			continue
		}
		lines[i] = outsideMath(line, f)
	}
	return strings.Join(lines, "\n")
}

// outsideMath applies a rewrite to one line, skipping the inline
// mathematics. A dollar sign with no closing partner is a dollar sign, so
// the rest of the line is prose.
func outsideMath(line string, f func(string) string) string {
	var b strings.Builder
	rest := line
	for {
		i := strings.Index(rest, "$")
		if i < 0 {
			b.WriteString(f(rest))
			return b.String()
		}
		j := strings.Index(rest[i+1:], "$")
		if j < 0 {
			b.WriteString(f(rest))
			return b.String()
		}
		j += i + 1
		b.WriteString(f(rest[:i]))
		b.WriteString(rest[i : j+1])
		rest = rest[j+1:]
	}
}
