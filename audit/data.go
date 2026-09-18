package audit

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/translate"
)

// A paper's own data is not a paper's prose, and the translation rules were
// written as though everything between two blank lines was prose.
//
// The rules that compare a translation with its English report a passage that
// came back unchanged. That is the right thing to report about a paragraph and
// the wrong thing to report about the material a paper prints as evidence. The
// GPT-3 paper prints the prompt it fed the model and the completion that came
// back, in English, because the English is the experiment. The LISP paper
// prints the value of car and cdr on an S-expression, in a notation that has
// no words in it at all. Every one of those came back unchanged because moving
// it would destroy it, and the corpus was refused over all of them.
//
// So the four shapes below are read as data and the rules skip them. Each one
// is a shape a translator is right to leave and a reader would be worse off
// for having moved. Nothing here guesses at the meaning of a passage: a table
// is a table, a blank is a blank, a quotation has quotation marks around it,
// and a block with no sentence in it has no sentence in it.
//
// The cost is that a paragraph of real English that happens to take one of
// those shapes stops being reported. That is the right way round for a hard
// rule, which stops the corpus from publishing at all: a rule that refuses the
// paper's own evidence refuses every release until somebody deletes the
// evidence, and no amount of retranslation moves it.
func data(block string) bool {
	return markdown.IsTable(block) || blanked(block) || quotation(block) || !sentential(block)
}

// blank is the run of underscores a paper prints where the reader, or the
// model, is meant to fill something in.
//
// Three, because two underscores are how markdown sets bold and one is how it
// sets a subscript in the text this corpus extracts.
var blank = regexp.MustCompile(`_{3,}`)

// blanked says the block holds a fill-in blank, which makes it a question and
// not a paragraph.
//
// "George bought some baseball equipment, a ball, a glove, and a ______." is
// the GPT-3 paper showing what it asked, and the blank is the whole point of
// the sentence. Translating it leaves a Vietnamese sentence that asks for an
// English word.
func blanked(block string) bool { return blank.MatchString(block) }

// quotation says the block is one quotation and nothing else.
//
// A block that opens and closes with a quotation mark is words quoted as
// words. The GPT-3 paper prints a paragraph the model wrote about Buddhism
// that way, to show what the model wrote, and what the model wrote is English.
//
// Both kinds of mark, because the extraction keeps whichever the paper set.
func quotation(block string) bool {
	block = strings.TrimSpace(block)
	rs := []rune(block)
	if len(rs) < 2 {
		return false
	}
	return openQuote(rs[0]) && closeQuote(rs[len(rs)-1])
}

func openQuote(r rune) bool  { return r == '"' || r == '“' || r == '«' }
func closeQuote(r rune) bool { return r == '"' || r == '”' || r == '»' }

// sentential says the block holds at least one finished sentence long enough
// to be prose.
//
// Long enough is prosePerParagraph, which is the length the rules already use
// to tell a paragraph from a caption, and finished means it ends in a stop.
// Between them they are the difference between a paragraph and a listing.
//
// The listings are real and there are a lot of them. The GPT-3 few-shot
// figure is four lines of "sea otter => loutre de mer". The LISP paper sets
// "car [(X · A)] = X car [((X · A) · Y )] = (X · A)" as a paragraph, which is
// mathematics the extraction never marked as mathematics, and it counts as
// eight words because car and X and A each have a letter in them. The running
// head "SALTZER ET AL. End-To-End Arguments in System Design" ends in a stop
// after three words and then runs out. None of the three is a sentence and all
// three were reported as a paragraph a translator had skipped.
func sentential(block string) bool {
	text := cased(block)
	at := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(text, -1) {
		if proseWords(text[at:loc[1]]) >= prosePerParagraph {
			return true
		}
		at = loc[1]
	}
	return false
}

// cased is the prose of a passage with the protected spans and the runs of
// whitespace taken out, in the case the paper set it in.
//
// plainProse is this lowercased, and lowercased is what the rules compare,
// because a translation that changes only the case of a word has not been
// translated. The case is kept here because two of the tests below are about
// the case and nothing else.
func cased(s string) string {
	return strings.Join(strings.Fields(translate.Prose(s)), " ")
}

// spared is the sentences of an English block that a translation is right to
// have left, keyed the way the rules compare sentences.
//
// This is the sentence-sized half of data above. A paragraph can be prose all
// the way through and still hold one run of words that has to stand: the
// Ethernet paper's front page translates "The present addresses of the
// authors are" and then prints two postal addresses, and the end-to-end paper
// translates its first footnote and then prints the notice the IEEE requires
// it to print. Both are correct and both were reported.
func spared(block string) map[string]bool {
	out := map[string]bool{}
	keep := func(text string) {
		for s := range sentences(text) {
			out[strings.ToLower(s)] = true
		}
	}
	for s := range sentences(cased(block)) {
		if namesOnly(s) || rights(s) {
			out[strings.ToLower(s)] = true
		}
	}
	for _, q := range quoted.FindAllString(block, -1) {
		keep(cased(q))
	}
	return out
}

// quoted is a run of words a paper has quoted, the marks included.
//
// Not greedy, so two quotations in a paragraph are two runs rather than one
// run with the paragraph's own words in the middle of it.
var quoted = regexp.MustCompile(`"[^"]+"|\x{201c}[^\x{201d}]+\x{201d}`)

// namesOnly says the sentence has no word in it that starts in lower case, which
// makes it a name, an address or a title rather than a sentence.
//
// The threshold is zero rather than a fraction, because a fraction would be a
// number picked to fit the two addresses that led to this and a sentence of
// English has articles and prepositions in it wherever you cut it. "R. M.
// Metcalfe, Transaction Technology, Inc., 10880 Wilshire Boulevard, Los
// Angeles, CA 94304" has none.
func namesOnly(s string) bool {
	words := 0
	for _, f := range strings.Fields(s) {
		r := first(f)
		if r == 0 {
			continue
		}
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			words++
		}
	}
	return words > 0
}

// first is the first letter or digit of a word, past whatever punctuation the
// typesetting put in front of it.
func first(f string) rune {
	for _, r := range f {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
	}
	return 0
}

// notice is the wording a rights statement uses.
var notice = regexp.MustCompile(`(?i)\bcopyright\b|©|\ball rights reserved\b|\breprinted with permission\b`)

// rights says the sentence is a rights notice, which is a statement of law and
// is reproduced as it was printed.
//
// The end-to-end paper carries "Copyright 1981 by The Institute of Electrical
// and Electronics Engineers, Inc." in its first footnote, under a sentence the
// translation did move. A translated copyright notice is a notice that no
// longer says what the rightsholder wrote.
func rights(s string) bool { return notice.MatchString(s) }
