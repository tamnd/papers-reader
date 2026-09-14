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
// Dollars, Money, Untable and Unlink are the habits that are not wrapping.
// They are here rather than beside the acceptance rules because all of them
// are things a reader does after it has read the page correctly, and a page
// refused for any of them would be asked again and come back the same way.
// The price on page 1 of the Unix paper was asked for three times, at three
// resolutions, and read correctly all three times.
//
// Delink runs after Unlink so that an image link is still an image link
// when Unlink looks at it. Unlink matches a whole line and Delink would
// leave it half a line, with the caption in the prose and the picture gone.
//
// Money runs after Dollars because it counts delimiters, and a page in the
// other dialect has not got any until Dollars has run.
//
// Dollars runs before Untable so that a cell already written in TeX's own
// delimiters is in this corpus's delimiters by the time the table is read.
// Untable puts dollars round a cell that is bare TeX, and a cell it had
// already put dollars round is a cell it must leave alone, so the two have to
// happen in this order and not the other one.
func Tidy(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	s = dropPreamble(s)
	s = dropTrailer(s)
	s = unwrap(s)
	s = Dollars(s)
	s = Money(s)
	s = Untable(s)
	s = Unlink(s)
	s = Delink(s)
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

// dropPreamble takes the introduction and the progress line off the head of
// the page, and keeps going until the first line is neither. A relay that
// reports how long it worked and then announces the transcription has put
// two lines above the page, and taking one of them off leaves the other.
func dropPreamble(s string) string {
	for {
		line, rest, found := strings.Cut(s, "\n")
		if !found {
			return s
		}
		line = strings.TrimSpace(line)
		if !preamble.MatchString(line) && !label.MatchString(line) && !status.MatchString(line) {
			return s
		}
		s = strings.TrimLeft(rest, "\n")
	}
}

// trailers are the closing courtesies. They are matched on the whole of the
// last line rather than as a prefix, because a page of a paper about dialogue
// systems will one day contain one of these sentences inside a paragraph.
//
// "would you like" and not "would you like me to", which is what this said
// until the last two pages of the MapReduce paper came back ending "Would
// you like a concise summary of the paper's main contributions?" and "Would
// you like a concise explanation of how this MapReduce word-count example
// works?". Neither has a "me to" in it and both went into the corpus, and
// they took the printed page number with them: the folio was the line above
// the offer, so it was no longer the last line of the page and the furniture
// pass left it alone. One courtesy sentence cost two pages their page
// numbers as well as their last paragraph.
var trailers = []string{
	"let me know",
	"i hope this helps",
	"hope this helps",
	"if you need anything",
	"feel free to ask",
	"would you like",
	"do you want me to",
	"shall i",
}

// chrome is not a courtesy. It is the relay's own user interface, which the
// tool that read the page sometimes picks up along with the answer and hands
// back as the last line of the transcription. Page 10 of the MapReduce paper
// came back ending " Give feedback", and it cost the page its running head
// as well: the furniture pass takes one folio and one head off an edge, the
// chrome took the head's turn, and the conference line published inside the
// experience section.
//
// Matched on the whole line and not as a prefix, and the list holds only
// what has actually been seen, because these are two and three word phrases
// and a paper will one day end a page on one of them. allChrome is what
// makes "whole line" hold when the relay hands back two of these run
// together with nothing between them.
//
// The two questions are the same thing as the button and they cost the same
// thing. Page 2 of the MapReduce paper came back ending "Is this
// conversation helpful so far?" and page 3 ending "Do you like this
// personality?", and both pages published with their printed page number on
// them, 138 and 139, because the folio was the line above the question and
// so was no longer at the edge for the furniture pass to see.
var chrome = []string{
	"give feedback",
	"is this conversation helpful so far?",
	"do you like this personality?",
}

// status is the relay's progress line, which it sometimes puts at the top of
// the answer where the page's first line should be. Page 3 of the MapReduce
// paper opened on "Worked for 9s" and page 8 on "Worked for 17s".
//
// A whole line, a small vocabulary of verbs and a duration, because the risk
// here is the other way round from the trailers: this is the first line of
// the page, and the first line of a page is the running head, which is what
// the furniture pass counts. A page whose head has been eaten is a page that
// publishes its head, and a paper that opens a page on the words "Worked for
// 9s" does not exist.
var status = regexp.MustCompile(`(?i)^(?:worked|thought|thinking|reasoned|searched|read)` +
	`(?: for)? [0-9]+(?:\.[0-9]+)? ?(?:ms|s|m|sec|secs|second|seconds|min|mins|minute|minutes)$`)

// allChrome says whether a line is nothing but chrome: one of the phrases,
// or several of them one after another.
//
// The relay puts its buttons side by side, and the tool that reads the page
// picks them up in the order they are drawn and with nothing between them.
// The re-read of page 13 of the MapReduce paper came back ending "Give
// feedbackDo you like this personality?", which is two phrases and no line
// break, so neither of them was the whole of the last line and the line
// stayed. Optional whitespace between them, because whether there is any
// depends on how the buttons were drawn.
//
// The whole of the line has to be consumed. A sentence of a paper that ends
// on one of these phrases has words in front of it that nothing here
// matches, so it keeps them and stays.
func allChrome(line string) bool {
	if line == "" {
		return false
	}
	for _, c := range chrome {
		rest, ok := strings.CutPrefix(line, c)
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if rest == "" || allChrome(rest) {
			return true
		}
	}
	return false
}

// dropTrailer takes the courtesies and the chrome off the foot of the page,
// and keeps going until the last line is neither. A reader that signs off
// twice, with "I hope this helps." under "Would you like a summary?", is a
// reader whose page ends two lines above where it looks like it ends.
func dropTrailer(s string) string {
	for {
		i := strings.LastIndex(s, "\n")
		if i < 0 {
			return s
		}
		last := strings.ToLower(strings.TrimSpace(s[i+1:]))
		found := false
		for _, t := range trailers {
			if strings.HasPrefix(last, t) {
				found = true
				break
			}
		}
		if allChrome(last) {
			found = true
		}
		if !found {
			return s
		}
		s = strings.TrimRight(s[:i], "\n")
	}
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
