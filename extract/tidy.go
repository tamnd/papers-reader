package extract

import (
	"regexp"
	"strings"
)

// Tidy takes the wrapping off an answer.
//
// The prompt asks for the page and nothing else, and a reader that is having a
// good day sends the page and nothing else. Three habits survive the asking,
// and all three are packaging rather than content: a code fence around the
// whole answer, a line of introduction above it, and a line offering further
// help below it.
//
// This is repair and not judgement, and the line between them is the point.
// Acceptance rule A1 refuses a page that is a refusal, and it has to keep
// refusing those: an apology is not a page with an apology wrapped round it,
// it is a page that was never read. What is stripped here is the wrapping of a
// page that is present underneath it, and a page that is nothing but wrapping
// comes out empty and is refused by A1 on the next line, which is the right
// answer.
//
// The alternative is to refuse the page and ask again, and a page refused for
// a fence costs another minute and a half of a rationed reader to be told the
// same thing without it.
//
// Dollars and Unlink are the two habits that are not wrapping. They are here
// rather than beside the acceptance rules because both are things a reader
// does after it has read the page correctly, and a page refused for either
// would be asked again and come back the same way.
func Tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	s = dropPreamble(s)
	s = dropTrailer(s)
	s = unwrap(s)
	s = Dollars(s)
	s = Unlink(s)
	return strings.TrimSpace(s)
}

// preamble is a line of introduction: an opener, a word saying what is being
// introduced, and a colon, all on a line of its own.
//
// Three conditions and every one of them is carrying weight, which the first
// version of this found out. It asked for an opener and a colon, and "The
// following is proved in Section 3:" is a sentence a paper writes and that
// version deleted it. So the line has to name the thing as well, and a reader
// announcing a transcription says the word page or transcription or text where
// a mathematician announcing a proof does not.
//
// The colon and the line of its own do the rest. "Here is a result which is
// used repeatedly in what follows." is prose in the middle of a paragraph and
// never matches.
var preamble = regexp.MustCompile(`(?i)^(?:(?:sure|certainly|of course)[!,.]?\s*)?` +
	`(?:here (?:is|are)|here's|below is|the following is|this is)\s[^\n]{0,120}` +
	`\b(?:transcription|transcript|page|text|content|markdown)\b[^\n]{0,40}:$`)

// label is a preamble with the opener left off: the word on its own.
var label = regexp.MustCompile(`(?i)^(?:the )?(?:transcription|transcript|transcribed text)(?: of (?:the|this) page)?:$`)

func dropPreamble(s string) string {
	line, rest, found := strings.Cut(s, "\n")
	if !found {
		return s
	}
	line = strings.TrimSpace(line)
	if !preamble.MatchString(line) && !label.MatchString(line) {
		return s
	}
	return strings.TrimLeft(rest, "\n")
}

// trailers are the closing courtesies. They are matched on the whole of the
// last line rather than as a prefix, because a page of a paper about dialogue
// systems will one day contain one of these sentences inside a paragraph.
var trailers = []string{
	"let me know",
	"i hope this helps",
	"hope this helps",
	"if you need anything",
	"feel free to ask",
	"would you like me to",
}

func dropTrailer(s string) string {
	i := strings.LastIndex(s, "\n")
	if i < 0 {
		return s
	}
	last := strings.ToLower(strings.TrimSpace(s[i+1:]))
	for _, t := range trailers {
		if strings.HasPrefix(last, t) {
			return strings.TrimRight(s[:i], "\n")
		}
	}
	return s
}

// wrapper is an opening fence with nothing on it, or with the word markdown on
// it, which is a reader saying "the answer is Markdown" rather than "this page
// is a listing".
var wrapper = regexp.MustCompile("^(`{3,}|~{3,})[ \t]*(markdown|md)?$")

// unwrap takes off a code fence that has the whole answer inside it.
//
// Three conditions, and the third is what keeps a page whose own first block
// is a listing from being taken apart. A page that opens with a fence tagged
// with a language is not wrapped, because the wrapper never carries a language
// other than markdown. A page that opens and closes with bare fences and has
// an odd number of fences in it is a page with a fence problem of its own, and
// rule A7 should see it as it is rather than as this function left it.
func unwrap(s string) string {
	open, rest, found := strings.Cut(s, "\n")
	if !found {
		return s
	}
	m := wrapper.FindStringSubmatch(strings.TrimSpace(open))
	if m == nil {
		return s
	}
	body, last := "", rest
	if i := strings.LastIndex(rest, "\n"); i >= 0 {
		body, last = rest[:i], rest[i+1:]
	}
	// The closing fence is the same run of the same character. A close that
	// does not match the open is not the other end of this wrapper, and
	// guessing that it is would take a real fence off a page.
	if strings.TrimSpace(last) != m[1] {
		return s
	}
	if len(fence.FindAllString(s, -1))%2 == 1 {
		return s
	}
	return strings.Trim(body, "\n")
}
