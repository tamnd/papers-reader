package audit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/split"
	"github.com/tamnd/papers-reader/translate"
)

// The L group is about the three translations, and every rule in it is a
// comparison against the English rather than a judgement of the Vietnamese,
// the Chinese or the Japanese.
//
// That is the whole shape of the group and it is deliberate. Nothing here
// can tell you that a sentence reads badly. What it can tell you is that a
// formula changed, that a citation was renumbered, that a paragraph came
// back in English, that a glossary term was rendered two ways in one corpus,
// or that what is on disk is an apology from a provider rather than a
// translation. Those are the failures that a reader cannot see and that
// nothing else downstream will catch, and they are the ones that make a
// corpus untrustworthy rather than merely awkward.
//
// The translator refuses an answer whose spans moved, so L01, L16 and L18
// ought never to fire on a file it wrote. They are here because a file can
// also arrive by hand, by a merge, or from a build of the toolchain that had
// a bug in the check, and a rule that only ever passes is still the rule
// that tells you the day it stops passing.

// A pair is a translated file and the English it was made from.
type pair struct {
	en *File
	tr *File
}

// pairs matches every translated file with its English.
//
// By translated_from where the file records it, and by name where it does
// not, because a file with no such field is rule L05's finding and not a
// reason for every other rule in the group to skip it.
//
// An English file with no translation yet is not a pair and is not a
// finding. The corpus is translated paper by paper over weeks, and a group
// that failed for every language of every paper not yet reached would be a
// group nobody reads.
func pairs(in *Input) []pair {
	english := map[string]*File{}
	for _, f := range in.Content {
		if f.Lang == corpus.EN && !f.Broken() {
			english[f.Paper+"/"+f.Name] = f
		}
	}
	var out []pair
	for _, f := range in.Content {
		if f.Lang == corpus.EN || f.Broken() {
			continue
		}
		en := english[f.Paper+"/"+f.Name]
		if from := f.Front.TranslatedFrom; from != "" {
			if named, ok := english[f.Paper+"/"+base(from)]; ok {
				en = named
			}
		}
		out = append(out, pair{en: en, tr: f})
	}
	return out
}

func base(path string) string {
	if cut := strings.LastIndexByte(path, '/'); cut >= 0 {
		return path[cut+1:]
	}
	return path
}

// eachTranslation runs a check over every translated file that has an
// English to be compared with.
//
// The group stands down on a corpus with no translation in it, which is
// every corpus until M6 reaches it, and it stands down rather than passing,
// because a rule that passes on nothing is how a corpus acquires a check
// everybody believes is working.
//
// A translated file with no English at all is skipped by every rule but
// L04, which is the rule that says so. There is nothing for the others to
// compare it with, and eighteen findings about one file is eighteen ways of
// saying the same thing.
func eachTranslation(in *Input, check func(pair) []Finding) ([]Finding, error) {
	all := pairs(in)
	if len(all) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, p := range all {
		if p.en == nil {
			continue
		}
		out = append(out, check(p)...)
	}
	return out, nil
}

// translationRules is group L.
func translationRules() []Rule {
	return []Rule{
		{
			ID: "L01", Hard: true,
			What:  "the mathematics of a translation is the mathematics of its English.",
			Check: spanRule("L01", translate.Math),
		},
		{
			ID: "L02", Hard: true,
			What:  "the attribute blocks of a translation are its English ones.",
			Check: spanRule("L02", translate.Attribute),
		},
		{
			ID: "L03", Hard: true,
			What:  "the heading tree of a translation is its English one.",
			Check: ruleL03,
		},
		{
			ID: "L04", Hard: true,
			What:  "every translated file is a file of the English paper, with the same number and kind.",
			Check: ruleL04,
		},
		{
			ID: "L05", Hard: true,
			What:  "every translation records the English file and the hash it was made from.",
			Check: ruleL05,
		},
		{
			ID: "L06", Hard: false,
			What:  "a glossary term used in the English is rendered the glossary's way in the translation.",
			Check: ruleL06,
		},
		{
			ID: "L07", Hard: true,
			What:  "no paragraph came back in English.",
			Check: ruleL07,
		},
		{
			ID: "L08", Hard: false,
			What:  "no translation was written by a small model.",
			Check: ruleL08,
		},
		{
			ID: "L09", Hard: true,
			What:  "two files under one glossary version were translated against the same renderings.",
			Check: ruleL09,
		},
		{
			ID: "L10", Hard: true,
			What:  "no English glossary term is left standing in a translation with its rendering nowhere in the file.",
			Check: ruleL10,
		},
		{
			ID: "L11", Hard: true,
			What:  "no sentence came back in English.",
			Check: ruleL11,
		},
		{
			ID: "L12", Hard: true,
			What:  "the words set inside the mathematics are translated too.",
			Check: ruleL12,
		},
		{
			ID: "L13", Hard: true,
			What:  "no word of a translation is written in a script that language does not use.",
			Check: ruleL13,
		},
		{
			ID: "L14", Hard: true,
			What:  "a bibliography stands as printed in every language.",
			Check: ruleL14,
		},
		{
			ID: "L15", Hard: false,
			What:  "no translation was written on a free gateway.",
			Check: ruleL15,
		},
		{
			ID: "L16", Hard: true,
			What:  "the citations of a translation are its English ones.",
			Check: spanRule("L16", translate.Citation),
		},
		{
			ID: "L17", Hard: true,
			What:  "no translation is a provider's error message or an apology.",
			Check: ruleL17,
		},
		{
			ID: "L18", Hard: true,
			What:  "the listings of a translation are its English ones, byte for byte.",
			Check: spanRule("L18", translate.Code, translate.Inline),
		},
		{
			ID: "L19", Hard: false,
			What:  "the section title of a translation was translated too.",
			Check: ruleL19,
		},
	}
}

// spanRule is L01, L02, L16 and L18: one kind of protected span, compared
// with the English the way the translator compares it.
//
// Paragraph by paragraph, and as a bag inside a paragraph. Chinese and
// Japanese put a modifier in front of what it modifies, so two formulas in
// one English clause come back the other way round and the answer that did
// not swap them would be the wrong one. What is still caught is a span
// dropped, added, altered, or moved out of the paragraph it was in.
//
// The comparison is the translator's own function rather than a second
// implementation of it, so that the rule and the command cannot come to
// disagree about what a span is. That has a cost worth saying out loud: a
// bug in Protect is invisible to this rule, because the same bug is on both
// sides of the comparison. The spans themselves are group M's and group C's
// business and they check them against the PDF.
func spanRule(id string, kinds ...translate.Kind) func(*Input) ([]Finding, error) {
	want := map[translate.Kind]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	return func(in *Input) ([]Finding, error) {
		return eachTranslation(in, func(p pair) []Finding {
			var out []Finding
			for _, d := range translate.Compare(p.en.Body, p.tr.Body) {
				if !want[d.Want.Kind] && !want[d.Got.Kind] {
					continue
				}
				out = append(out, Finding{
					Rule: id, File: p.tr.Path,
					Message: d.String(),
				})
			}
			return out
		})
	}
}

// heading is a Markdown heading line, which in this corpus is always three
// hashes: papers split lifts the file's own heading into the front matter
// and everything left in a body is a subsection of it.
var headingLine = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.*)$`)

// ruleL03 wants the same headings at the same depths in the same order.
//
// Not the same words, obviously. What is checked is that a translation did
// not merge two subsections into one, promote a subsection to a section, or
// turn a heading into a paragraph, all three of which a model does when a
// passage is long and it starts summarising.
func ruleL03(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		want, got := depths(p.en.Body), depths(p.tr.Body)
		if equalInts(want, got) {
			return nil
		}
		return []Finding{{
			Rule: "L03", File: p.tr.Path,
			Message: fmt.Sprintf("the English has headings at depths %v and this has %v", want, got),
		}}
	})
}

func depths(body string) []int {
	var out []int
	for _, m := range headingLine.FindAllStringSubmatch(body, -1) {
		out = append(out, len(m[1]))
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ruleL04 is about the file, not about what is in it.
//
// A translated tree is the English tree with the words changed. A file in it
// with no English counterpart is a file nobody can review, because there is
// nothing to review it against, and the two ways one appears are a paper
// resplit after it was translated and a name typed by hand.
//
// The other direction, an English file with no translation, is coverage and
// not a fault, and coverage.md reports it.
func ruleL04(in *Input) ([]Finding, error) {
	all := pairs(in)
	if len(all) == 0 {
		return nil, ErrNotRun
	}
	var orphans []Finding
	for _, p := range all {
		if p.en == nil {
			orphans = append(orphans, Finding{
				Rule: "L04", File: p.tr.Path,
				Message: "there is no English file this is a translation of",
			})
		}
	}
	found, err := eachTranslation(in, func(p pair) []Finding {
		var out []Finding
		if p.tr.Front.Kind != p.en.Front.Kind {
			out = append(out, Finding{
				Rule: "L04", File: p.tr.Path,
				Message: fmt.Sprintf("this is kind %q and its English is kind %q", p.tr.Front.Kind, p.en.Front.Kind),
			})
		}
		if p.tr.Front.Section != p.en.Front.Section {
			out = append(out, Finding{
				Rule: "L04", File: p.tr.Path,
				Message: fmt.Sprintf("this is section %q and its English is section %q", p.tr.Front.Section, p.en.Front.Section),
			})
		}
		if p.tr.Front.Tag != p.en.Front.Tag {
			out = append(out, Finding{
				Rule: "L04", File: p.tr.Path,
				Message: fmt.Sprintf("this carries tag %q and its English carries %q", p.tr.Front.Tag, p.en.Front.Tag),
			})
		}
		return out
	})
	if err != nil {
		return nil, err
	}
	return append(orphans, found...), nil
}

// ruleL05 is the field that makes a stale translation findable.
//
// Without it nothing on disk says what question this file is the answer to,
// so nobody can tell a translation of the current English from a translation
// of a page that was re-extracted last month. A hash that does not match is
// not a failure here: the English moving is expected and papers translate
// queues the file again. A hash that is absent is.
func ruleL05(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		var out []Finding
		if p.tr.Front.TranslatedFrom == "" {
			out = append(out, Finding{
				Rule: "L05", File: p.tr.Path,
				Message: "no translated_from, so nothing says which English file this answers",
			})
		}
		if p.tr.Front.SourceContentSHA256 == "" {
			out = append(out, Finding{
				Rule: "L05", File: p.tr.Path,
				Message: "no source_content_sha256, so nothing can say whether this is still current",
			})
		}
		return out
	})
}

// ruleL06 checks the glossary was used where it applies.
//
// The English prose is searched for each term and the translation for its
// rendering. Prose and not the body: a term inside a formula or a listing is
// not prose and must not be translated, and searching the body would ask for
// the rendering of a variable name.
//
// Soft, and it has to be. A rendering is a phrase and a language inflects
// it: Vietnamese does not, Japanese attaches particles, and a Chinese
// rendering can legitimately appear with a measure word between its halves.
// A plain substring search over a real page finds most of what is wrong and
// some of what is not, which is a report worth reading and not a build worth
// failing.
func ruleL06(in *Input) ([]Finding, error) {
	if in.Glossary == nil || len(in.Glossary.Terms) == 0 {
		return nil, ErrNotRun
	}
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind == "references" {
			return nil
		}
		en := strings.ToLower(translate.Prose(p.en.Body))
		tr := strings.ToLower(translate.Prose(p.tr.Body))
		rs := []rune(en)
		terms := renderings(in.Glossary, p.en.Front.Field, p.tr.Lang)
		whole := handled(terms, rs, tr)
		var missed []string
		for _, t := range terms {
			if t.common || t.as == t.en || t.rendered(tr) || kept(tr, t.en) {
				continue
			}
			if !missing(rs, tr, t.en, whole) {
				continue
			}
			missed = append(missed, fmt.Sprintf("%q as %q", t.en, t.as))
		}
		if len(missed) == 0 {
			return nil
		}
		return []Finding{{
			Rule: "L06", File: p.tr.Path,
			Message: "the English uses these terms and the glossary's rendering is nowhere in the translation: " + strings.Join(cap5(missed), ", "),
		}}
	})
}

// a rendering is one glossary term in one language, flattened.
//
// Senses is the renderings of the other entries for the same English term,
// the ones this paper's field was not offered. The glossary holds two
// entries for "feature", one for the input variable of a model and one for
// the capability of a program, and the acknowledgements of a paper about
// generative models thank somebody for sharing a Theano feature. The field
// says which sense a paper is about and is right nearly everywhere, so it
// still decides which rendering the rule names; but a page that wrote the
// other sense did write the term, and calling that unrendered is a finding
// on a page that did nothing wrong.
type rendering struct {
	en, as string
	senses []string
	// common is the term's Common flag, and a term carrying it is offered
	// to the translator and not asked after here. The reasoning is on the
	// field in the glossary package.
	common bool
}

// rendered says whether a translation wrote this term, in the sense its
// field is about or in one of the others.
func (r rendering) rendered(tr string) bool {
	if strings.Contains(tr, strings.ToLower(r.as)) {
		return true
	}
	for _, as := range r.senses {
		if strings.Contains(tr, strings.ToLower(as)) {
			return true
		}
	}
	return false
}

func renderings(g *glossary.Glossary, f corpus.Field, l corpus.Lang) []rendering {
	senses := map[string][]string{}
	for _, t := range g.Terms {
		if t.Offered(f) {
			continue
		}
		if as, ok := t.Rendering(l); ok {
			en := strings.ToLower(strings.TrimSpace(t.En))
			senses[en] = append(senses[en], strings.TrimSpace(as))
		}
	}
	var out []rendering
	for _, t := range g.For(f) {
		as, ok := t.Rendering(l)
		if !ok {
			continue
		}
		en := strings.ToLower(strings.TrimSpace(t.En))
		out = append(out, rendering{en: en, as: strings.TrimSpace(as), senses: senses[en], common: t.Common})
	}
	return out
}

// cap5 keeps a finding to a line somebody will read. A page that missed
// forty terms has one problem and not forty.
func cap5(list []string) []string {
	const most = 5
	if len(list) <= most {
		return list
	}
	return append(list[:most:most], fmt.Sprintf("and %d more", len(list)-most))
}

// alone says whether a term stands by itself somewhere in text, rather than
// always inside a longer English name that other has a word for word copy
// of. Both sides are already lowercased.
//
// The two glossary rules ask it in opposite directions. L06 asks whether the
// English uses the term, with the translation as other. L10 asks whether the
// term is left standing in the translation, with the English as other. The
// question underneath is the same one: is this the word the glossary is
// about, or is it a syllable of somebody's name for a method.
//
// Two things make an occurrence not the word the glossary is about.
//
// A hyphen, first. The corpus writes "log-likelihood" and "auto-encoder" and
// neither of them is the glossary's "log" or its "encoder", so a hyphen
// counts as a letter when the boundary is worked out.
//
// A neighbour the other side wrote the same way, second. Japanese keeps
// "Markov chain" and "score matching" and "deep belief networks" in English
// because that is how a Japanese paper writes them, and reporting the
// "chain", the "score" and the "network" inside them as untranslated words
// gives the rule three findings on a page that did nothing wrong. So the
// term is taken together with the Latin word before it and with the Latin
// word after it, and an occurrence whose pair the other side wrote too is
// an English name that was kept on purpose. A whole English sentence left
// standing is not this rule's business: L11 has it, and L07 has a whole
// English paragraph.
//
// An open bracket after it, third. "hash(key) mod R" is the partition
// function in the MapReduce paper and it is set as running text rather than
// as code, so the Vietnamese carries it through as printed and should. A
// name with its argument list stuck to it is an identifier, and an
// identifier is the same in every language. Rendering it would be writing a
// call to a function nobody wrote.
func alone(text, other, term string) bool {
	rs := []rune(text)
	for _, s := range spans(rs, term) {
		if called(rs, s[1]) || copied(rs, other, s[0], s[1]) {
			continue
		}
		return true
	}
	return false
}

// called says whether what ends at end is a call rather than a word: the
// next character is an open bracket, with no space between.
func called(rs []rune, end int) bool {
	return end < len(rs) && (rs[end] == '(' || rs[end] == '[')
}

// spans is every place term stands as a word of rs, as half open rune
// indexes. A hyphen counts as a letter, so "log-likelihood" holds no span
// of "log".
func spans(rs []rune, term string) [][2]int {
	ts := []rune(term)
	var out [][2]int
	for at := 0; at+len(ts) <= len(rs); at++ {
		if string(rs[at:at+len(ts)]) != term {
			continue
		}
		if at > 0 && wordRune(rs[at-1]) {
			continue
		}
		end := at + len(ts)
		if end < len(rs) && wordRune(rs[end]) {
			continue
		}
		out = append(out, [2]int{at, end})
	}
	return out
}

// kept says whether the translation wrote the English term itself.
//
// A page that did that made a decision, and L10 is the rule that judges it:
// it asks whether an English term left standing was kept on purpose, and it
// is hard, so nothing gets past it by being reported here instead. L06
// reporting the same page as well is one mistake counted twice, and the two
// findings do not even agree about what is wrong with it.
//
// Japanese writes "Markov chain" in English, the way a Japanese paper does,
// and the glossary renders it マルコフ連鎖. Two pages of the first paper
// translated were reported by L06 for having no マルコフ連鎖 in them, on top
// of L10 having already looked at the same words and decided they were kept
// on purpose.
func kept(tr, term string) bool {
	return len(spans([]rune(tr), term)) > 0
}

// missing says whether the English uses this term somewhere the translation
// had to render it on its own and did not.
//
// This is alone with one more way out. An occurrence inside a longer
// glossary term that the translation did render is not a missed rendering:
// the translation handled the whole phrase, and the phrase is what the
// reader sees. Japanese writes "Markov chain" as マルコフ連鎖, which has no
// チェーン in it, and the glossary's "chain" is rendered チェーン, so every
// Japanese page that mentions a Markov chain was reported for a word it
// translated correctly. Three of the seven L06 findings on the first paper
// translated were that, in three different shapes: "chain" inside "Markov
// chain", and "objective" inside "training objective" in both Vietnamese
// and Chinese.
//
// It only works for a phrase the glossary knows, which is why the multiword
// entries were added at the same time. A rule that guessed at phrases would
// be guessing about the one thing the glossary exists to settle.
func missing(rs []rune, tr, term string, whole [][2]int) bool {
	for _, s := range spans(rs, term) {
		if copied(rs, tr, s[0], s[1]) {
			continue
		}
		if within(whole, s) {
			continue
		}
		return true
	}
	return false
}

// handled is where the English wrote a multiword glossary term that the
// translation rendered, which is the text a one word term inside it does
// not have to be rendered separately in.
func handled(terms []rendering, rs []rune, tr string) [][2]int {
	var out [][2]int
	for _, t := range terms {
		if !strings.Contains(t.en, " ") || !t.rendered(tr) {
			continue
		}
		out = append(out, spans(rs, t.en)...)
	}
	return out
}

// within says whether s sits inside one of the ranges.
func within(ranges [][2]int, s [2]int) bool {
	for _, r := range ranges {
		if r[0] <= s[0] && s[1] <= r[1] {
			return true
		}
	}
	return false
}

// copied says whether the term at [at,end) sits in a two word English phrase
// that other wrote as well.
func copied(rs []rune, other string, at, end int) bool {
	for _, p := range []string{
		string(rs[back(rs, at):end]),
		string(rs[at:forward(rs, end)]),
	} {
		if len([]rune(p)) > end-at && strings.Contains(other, p) {
			return true
		}
	}
	return false
}

// back is where the word before at starts, and at itself when the term
// opens the text or has punctuation rather than a word in front of it.
func back(rs []rune, at int) int {
	i := at
	for i > 0 && rs[i-1] == ' ' {
		i--
	}
	if i == at {
		return at
	}
	j := i
	for j > 0 && wordRune(rs[j-1]) {
		j--
	}
	if j == i {
		return at
	}
	return j
}

// forward is where the word after end finishes, and end itself when there
// is no word after it.
func forward(rs []rune, end int) int {
	i := end
	for i < len(rs) && rs[i] == ' ' {
		i++
	}
	if i == end {
		return end
	}
	j := i
	for j < len(rs) && wordRune(rs[j]) {
		j++
	}
	if j == i {
		return end
	}
	return j
}

// wordRune is what a term may not be joined to and still be that term. The
// hyphen is in it, and the reasoning is in alone.
func wordRune(r rune) bool { return letter(r) || r == '-' }

func letter(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// ruleL07 finds a paragraph that came back as its English.
//
// The comparison is on the prose with the protected spans taken out, so a
// paragraph that is nothing but a display equation, a table row of numbers
// or a citation is not a finding: there was nothing in it to translate and
// the English is the right answer.
//
// Short paragraphs are skipped, and the threshold is words of prose rather
// than characters. A line like "Figure 3" or "where" is a paragraph in the
// corpus and is the same in every language.
//
// The masthead of a front file is skipped too. See masthead.
func ruleL07(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind == "references" {
			return nil
		}
		var out []Finding
		for _, i := range untranslated(p) {
			out = append(out, Finding{
				Rule: "L07", File: p.tr.Path,
				Message: fmt.Sprintf("paragraph %d is the English paragraph, word for word", i+1),
			})
		}
		return out
	})
}

// prosePerParagraph is how long a paragraph has to be before "it is the same
// as the English" means anything.
//
// Eight words. Below that a paragraph is a caption, a heading, a table row
// or a line of the masthead, and those are regularly the same in two
// languages for good reasons: "Figure 3", an author's name, a venue.
const prosePerParagraph = 8

// proseWords is how many of them a paragraph holds, for the threshold above.
//
// A word has a letter in it. Splitting on spaces and counting what falls out
// is not the same thing, and the difference is a line of a program. The
// Floyd paper prints its algorithm as ALGOL, the extraction left four of the
// lines outside a fence as paragraphs of their own, and `else d := 1; e :=
// c; drop := op - 1;` splits into eleven pieces. Eleven is over the
// threshold, so the rule read the line as a paragraph of prose, found the
// Vietnamese identical, and reported a translator for not translating
// ALGOL. It holds six words, which is under it.
//
// Rule C08 is the one that wants those lines fenced and it reports them
// already. Until they are, they are still English text sitting in a
// translation and every one of them is supposed to be left alone.
func proseWords(s string) int {
	n := 0
	for _, f := range strings.Fields(s) {
		if strings.IndexFunc(f, unicode.IsLetter) >= 0 {
			n++
		}
	}
	return n
}

// untranslated is the indexes of the paragraphs of a pair that came back in
// English, and is empty when the two sides do not have the same number of
// paragraphs, which is rule L03's finding or the block count check's.
func untranslated(p pair) []int {
	want, got := blocksOf(p.en.Body), blocksOf(p.tr.Body)
	if len(want) != len(got) {
		return nil
	}
	var out []int
	for i := start(p); i < len(want); i++ {
		a, b := plainProse(want[i]), plainProse(got[i])
		if a == "" || proseWords(a) < prosePerParagraph {
			continue
		}
		if a == b {
			out = append(out, i)
		}
	}
	return out
}

// start is the first paragraph a translation of this page was meant to
// translate: the top of the file, or the abstract of a front page.
func start(p pair) int {
	if p.tr.Front.Kind != "front" {
		return 0
	}
	return masthead(blocksOf(p.en.Body))
}

// masthead is where the masthead of a front file ends: the index of the
// first paragraph long enough to be the abstract, or the end of the file if
// there is none.
//
// Everything above the abstract is the title, the byline, the affiliation
// and the arXiv stamp, and a translation is right to leave all of it. The
// byline of this corpus's first paper is eight names and runs well past the
// eight words L07 needs to speak up, and the affiliation under it is three
// lines of a Montréal address, so both were reported on every front file in
// all three languages. Names and the names of institutions stand as printed,
// the same way a bibliography entry does under rule 7 of the prompt.
//
// The abstract is found by length rather than by looking for the word
// Abstract, because the word is translated and the heading above it is
// sometimes a paragraph of its own and sometimes not there at all. Forty
// words is split.AbstractParagraph, which is the same measure the splitter
// used to decide this page was a front page in the first place.
func masthead(blocks []string) int {
	for i, b := range blocks {
		if corpus.Words(plainProse(b)) >= split.AbstractParagraph {
			return i
		}
	}
	return len(blocks)
}

// blocksOf cuts a body into the blocks the rest of the toolchain sees.
//
// This used to split on blank lines here, which is nearly right and is wrong
// in one place that matters: a listing with a blank line in it came apart
// into several blocks, and the pieces that were pure program text were then
// held up against the English as paragraphs. The MapReduce appendix is
// ninety lines of C++ in one fence and it produced nine findings against a
// translator for not translating C++.
//
// markdown.Blocks is what the emitter cuts with and package translate's
// chunker keeps a fence whole for the same reason, so this is the two of
// them agreeing rather than a third opinion. It also means the paragraph
// number in a finding is the block index the reading app shows, which is
// what somebody chasing the finding is going to be looking at.
func blocksOf(body string) []string { return markdown.Blocks(body) }

// plainProse is the prose of a passage, lowercased, with the protected spans
// and the runs of whitespace taken out, which is what two paragraphs have to
// share before one can be called a copy of the other.
func plainProse(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(translate.Prose(s)), " "))
}

// ruleL08 and ruleL15 are the honesty rules. A section written by a
// cut-down model or on somebody else's aggregator is flagged and kept, not
// rejected, so that coverage.md can say how much of the corpus is
// provisional and the reading app can say so on the page.
func ruleL08(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if !p.tr.Front.SmallModel && !llm.SmallModel(p.tr.Front.TranslationModel) {
			return nil
		}
		return []Finding{{
			Rule: "L08", File: p.tr.Path,
			Message: fmt.Sprintf("written by %s, which is a cut-down model, so this page is provisional", p.tr.Front.TranslationModel),
		}}
	})
}

func ruleL15(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if !p.tr.Front.Gateway {
			return nil
		}
		return []Finding{{
			Rule: "L15", File: p.tr.Path,
			Message: "written on a free gateway, where what answered is not what the route names, so this page is provisional",
		}}
	})
}

// ruleL09 is the rule that keeps the glossary version honest.
//
// The version is what makes a translation findable when a rendering
// changes, and it only works if it moves every time one does. Two files in
// the same field and language claiming the same version with different
// glossary_terms_sha256 mean somebody edited a rendering and left the
// version where it was, and every file written on either side of that edit
// now claims a provenance it does not have.
//
// It is checked between the files rather than against the glossary on disk,
// because the glossary on disk is version N and the corpus is full of files
// written under N-3. Comparing with it would fail the whole corpus for the
// one thing that is not wrong with it.
func ruleL09(in *Input) ([]Finding, error) {
	all := pairs(in)
	if len(all) == 0 {
		return nil, ErrNotRun
	}
	// key is field, language and version, because two papers in different
	// fields are offered different terms and honestly hash differently
	// under one version.
	type claim struct{ file, sha string }
	claims := map[string][]claim{}
	for _, p := range all {
		f := p.tr.Front
		if f.GlossaryVersion == 0 || f.GlossaryTermsSHA256 == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s/v%d", f.Field, f.Lang, f.GlossaryVersion)
		claims[key] = append(claims[key], claim{file: p.tr.Path, sha: f.GlossaryTermsSHA256})
	}
	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Finding
	for _, k := range keys {
		group := claims[k]
		first := group[0]
		for _, c := range group[1:] {
			if c.sha == first.sha {
				continue
			}
			out = append(out, Finding{
				Rule: "L09", File: c.file,
				Message: fmt.Sprintf("this and %s both say glossary version %s and they were translated against different renderings, so the version did not move when a rendering did",
					first.file, strings.TrimPrefix(k[strings.LastIndexByte(k, '/')+1:], "v")),
			})
		}
	}
	return out, nil
}

// ruleL10 finds an English glossary term standing in a translation.
//
// A gloss is not a finding. "mạng đối kháng sinh thành (generative
// adversarial network)" is good practice on a term's first appearance and
// the rendering is right there in the file, so what is looked for is the
// English term in the translated prose with the rendering nowhere in it at
// all. That is a term the translator did not translate rather than one it
// explained.
//
// Terms the glossary keeps in English are skipped, because keeping them is
// the decision the glossary recorded.
//
// The ACM classification on a front page is skipped too. See classification.
func ruleL10(in *Input) ([]Finding, error) {
	if in.Glossary == nil || len(in.Glossary.Terms) == 0 {
		return nil, ErrNotRun
	}
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind == "references" {
			return nil
		}
		en := strings.ToLower(translate.Prose(p.en.Body))
		// Looked for in the prose, and found anywhere in the file. A gloss
		// put on the term in a caption still explains it, so the rendering
		// is searched for in the whole of the translation and the English
		// term only in the part of it that was there to be translated.
		whole := strings.ToLower(translate.Prose(p.tr.Body))
		tr := strings.ToLower(translate.Prose(translatable(p)))
		var left []string
		for _, t := range renderings(in.Glossary, p.en.Front.Field, p.tr.Lang) {
			if t.as == t.en || !alone(tr, en, t.en) || t.rendered(whole) {
				continue
			}
			left = append(left, fmt.Sprintf("%q, which is %q", t.en, t.as))
		}
		if len(left) == 0 {
			return nil
		}
		return []Finding{{
			Rule: "L10", File: p.tr.Path,
			Message: "these terms stand in English with their rendering nowhere in the file: " + strings.Join(cap5(left), ", "),
		}}
	})
}

// translatable is the body of a translation with the blocks that were never
// anybody's to translate taken out of it.
//
// There is one kind so far and it is the ACM classification. See
// classification. The blocks are matched through the English, because the
// heading on them is translated and the content under it is not, which is
// the correct answer and is also exactly what makes them hard to recognise
// from the translation alone.
//
// A pair whose block counts disagree gets the body whole. That disagreement
// is rule L03's finding and guessing which block is which on top of it
// would turn one clear finding into two confusing ones.
func translatable(p pair) string {
	en, tr := blocksOf(p.en.Body), blocksOf(p.tr.Body)
	if len(en) != len(tr) {
		return p.tr.Body
	}
	var keep []string
	for i, b := range tr {
		if classification(en[i]) {
			continue
		}
		keep = append(keep, b)
	}
	return strings.Join(keep, "\n\n")
}

// acmHeadings are the two blocks of an ACM front page that carry a
// controlled vocabulary rather than a sentence.
var acmHeadings = []string{
	"categories and subject descriptors",
	"categories & subject descriptors",
	"general terms",
}

// classification says whether an English block is one of them.
//
// What is under those headings is not prose. "D.4.2 [Operating Systems]:
// Storage Management" is a code in the ACM Computing Classification System
// and "Algorithms, Management, Measurement, Performance, Design" is the
// fixed list of ACM general terms, and both of them are identifiers that
// happen to be spelled as English words. Rendering "Performance" into
// Vietnamese there does not translate anything, it breaks the code.
//
// The Dynamo paper is what found this. Its translator got it exactly right,
// translating the two headings and leaving the vocabulary under them
// standing, and L10 reported it for leaving "performance" in English.
//
// Matched on the heading and not on the shape of what follows, because the
// shape is a full stop away from an ordinary sentence and the heading is
// printed by the ACM template.
func classification(block string) bool {
	line, _, _ := strings.Cut(block, "\n")
	line = strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "#* ")))
	for _, h := range acmHeadings {
		if line == h {
			return true
		}
	}
	return false
}

// ruleL11 finds a sentence that came back as its English inside a paragraph
// that otherwise did not.
//
// A whole paragraph left in English is rule L07 and those paragraphs are
// skipped here, so the two rules do not report the same page twice. What is
// left is the case rule 11 of the prompt is about: a model that met a
// sentence it could not do, left it, and carried on. That is the right thing
// for it to do and the wrong thing to publish.
//
// The masthead of a front page is skipped, the same way L07 skips it and
// for the same reason. See masthead.
func ruleL11(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind == "references" {
			return nil
		}
		want, got := blocksOf(p.en.Body), blocksOf(p.tr.Body)
		if len(want) != len(got) {
			return nil
		}
		whole := map[int]bool{}
		for _, i := range untranslated(p) {
			whole[i] = true
		}
		var out []Finding
		for i := start(p); i < len(want); i++ {
			if whole[i] {
				continue
			}
			english := sentences(plainProse(want[i]))
			for s := range sentences(plainProse(got[i])) {
				if proseWords(s) < prosePerSentence || !english[s] {
					continue
				}
				out = append(out, Finding{
					Rule: "L11", File: p.tr.Path,
					Message: fmt.Sprintf("paragraph %d keeps an English sentence: %q", i+1, shorten(s)),
				})
			}
		}
		return out
	})
}

// prosePerSentence is how many words a sentence needs before finding it on
// both sides means it was left behind rather than that it is a phrase two
// languages spell the same.
//
// Ten, which is more than the paragraph threshold because a sentence is
// matched inside a paragraph that was otherwise translated, and the
// sentences that survive translation unchanged are things like "Here $n = 3$
// and $m = 4$." A paper's real sentences are longer than that.
//
// Counted by proseWords for the reason given there. The line of ALGOL that
// got past the paragraph threshold on eleven pieces of punctuation gets past
// this one too, and it is the same six words either way.
const prosePerSentence = 10

var sentenceEnd = regexp.MustCompile(`[.!?](?:\s|$)`)

// sentences cuts flattened prose into sentences, as a set.
func sentences(text string) map[string]bool {
	out := map[string]bool{}
	at := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(text, -1) {
		if s := strings.TrimSpace(text[at:loc[1]]); s != "" {
			out[s] = true
		}
		at = loc[1]
	}
	if s := strings.TrimSpace(text[at:]); s != "" {
		out[s] = true
	}
	return out
}

func shorten(s string) string {
	const most = 70
	if len([]rune(s)) <= most {
		return s
	}
	return string([]rune(s)[:most]) + "…"
}

// ruleL12 is about the prose that lives inside a formula.
//
// TeX has no way of putting a word in a formula except \text, so a paper
// writes "$p_{\text{data}}$" and "$(\text{not } A)$", and the second of
// those has a word in it that becomes Vietnamese. The span comparison lets
// it: it masks the argument of a \text before comparing, on purpose. This is
// the rule that checks the licence was used.
//
// Operator names are not prose and do not move, and the list of them is
// translate.Upright, which is the same list the comparison uses.
func ruleL12(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		want, got := textWords(p.en.Body), textWords(p.tr.Body)
		if len(want) == 0 || len(want) != len(got) {
			return nil
		}
		var same []string
		for i := range want {
			if want[i] == got[i] {
				same = append(same, want[i])
			}
		}
		if len(same) == 0 {
			return nil
		}
		return []Finding{{
			Rule: "L12", File: p.tr.Path,
			Message: "these words are set inside the mathematics and came back in English: " + strings.Join(cap5(same), ", "),
		}}
	})
}

// textWords is every \text argument of a body that is prose, in order.
//
// A single letter is a subscript label and not a word. An argument holding
// mathematics of its own is compared whole by the span check and is not
// prose. An operator name is not prose.
func textWords(body string) []string {
	var out []string
	for _, s := range translate.Protect(body) {
		if s.Kind != translate.Math {
			continue
		}
		for _, arg := range textArgs(s.Text) {
			word := strings.TrimSpace(arg)
			if len([]rune(word)) < 2 || strings.Contains(word, "$") || translate.Upright[word] {
				continue
			}
			if !strings.ContainsFunc(word, unicode.IsLetter) {
				continue
			}
			out = append(out, word)
		}
	}
	return out
}

var textOpen = regexp.MustCompile(`\\(?:text|textit|textbf|textrm|textnormal|mbox)\{`)

func textArgs(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		loc := textOpen.FindStringIndex(s[i:])
		if loc == nil {
			return out
		}
		open := i + loc[1]
		shut := brace(s, open)
		if shut < 0 {
			return out
		}
		out = append(out, s[open:shut])
		i = shut + 1
	}
	return out
}

// brace is the index of the brace closing the group opened just before open.
func brace(s string, open int) int {
	depth := 1
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// scripts is the writing systems each language of the corpus uses.
//
// This is the rule that needed most care and it is the one that is a table
// rather than a constant, because the four languages disagree about every
// entry. Han characters in a Vietnamese page are wrong and in a Japanese one
// are right. Kana in a Chinese page are wrong. Latin letters are right
// everywhere, because every one of these languages writes a model's name,
// an author's name and an acronym in Latin.
//
// What is checked is the prose, with the mathematics and the listings taken
// out. A Greek letter in a formula is a formula and is group M's business.
var scripts = map[corpus.Lang]map[string]bool{
	corpus.EN: {"Latin": true},
	corpus.VI: {"Latin": true},
	corpus.ZH: {"Latin": true, "Han": true},
	corpus.JA: {"Latin": true, "Han": true, "Hiragana": true, "Katakana": true},
}

// named is the scripts this rule can name. A rune in none of them, a
// mathematical symbol or an arrow, is not a word in another alphabet and is
// not this rule's business.
var named = map[string]*unicode.RangeTable{
	"Latin":      unicode.Latin,
	"Han":        unicode.Han,
	"Hiragana":   unicode.Hiragana,
	"Katakana":   unicode.Katakana,
	"Hangul":     unicode.Hangul,
	"Cyrillic":   unicode.Cyrillic,
	"Arabic":     unicode.Arabic,
	"Hebrew":     unicode.Hebrew,
	"Thai":       unicode.Thai,
	"Devanagari": unicode.Devanagari,
}

func ruleL13(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		allowed := scripts[p.tr.Lang]
		if allowed == nil {
			return nil
		}
		seen := map[string]rune{}
		for _, r := range translate.Prose(p.tr.Body) {
			for name, table := range named {
				if allowed[name] || !unicode.Is(table, r) {
					continue
				}
				if _, ok := seen[name]; !ok {
					seen[name] = r
				}
			}
		}
		if len(seen) == 0 {
			return nil
		}
		var out []Finding
		for _, name := range sorted(seen) {
			out = append(out, Finding{
				Rule: "L13", File: p.tr.Path,
				Message: fmt.Sprintf("%s is written in %s, which %s does not use", string(seen[name]), name, p.tr.Lang.Name()),
			})
		}
		return out
	})
}

func sorted(m map[string]rune) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ruleL14 is the one file of a paper that is the same in four languages.
//
// Author names, the titles of cited works and venue names are not
// translated, transliterated or reordered, so the bibliography is copied
// rather than asked for, and this is the rule that says the copy is a copy.
// A bibliography that differs from the English by a character is a
// bibliography somebody asked a model about.
func ruleL14(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind != "references" {
			return nil
		}
		if strings.TrimSpace(p.tr.Body) == strings.TrimSpace(p.en.Body) {
			return nil
		}
		return []Finding{{
			Rule: "L14", File: p.tr.Path,
			Message: "the bibliography is not the English bibliography, and a bibliography stands as printed",
		}}
	})
}

// ruleL19 is the title in the front matter, which is the one piece of a
// translated file that is not in its body.
//
// The section title is what the book prints as a chapter heading and what
// the reader prints in a table of contents, so a title left in English is
// the most visible line of the file and the least likely to be noticed by
// a rule that reads bodies. The translator does ask for it, as a heading at
// the top of the passage, and a model that answers the prose correctly
// still hands the heading back unchanged often enough to be worth a rule:
// on the first real run of this corpus it happened to Introduction twice
// out of three languages.
//
// It is soft because a title that is the same in both languages is a real
// answer. Plenty of section titles are a proper noun, an abbreviation or a
// formula, and refusing those would be refusing the truth.
//
// A front file and a bibliography are skipped. Neither has a title from the
// paper: what stands there is a label this toolchain wrote, and the book
// prints its own word for it in each language.
func ruleL19(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		switch p.tr.Front.Kind {
		case "front", "references":
			return nil
		}
		title := strings.TrimSpace(p.tr.Front.SectionTitle)
		if title == "" || title != strings.TrimSpace(p.en.Front.SectionTitle) {
			return nil
		}
		if !words(title) {
			return nil
		}
		return []Finding{{
			Rule: "L19", File: p.tr.Path,
			Message: fmt.Sprintf("the section title %q is the English one, and the book prints it as the chapter heading", title),
		}}
	})
}

// words says whether a title has anything in it a translator could have
// changed: two runs of letters, or one of more than three.
//
// A title of one short word is where this rule is wrong most often. "GAN",
// "MNIST", "Adam" and "TPU" stand in every language, and so does a section
// called "3.2". Asking for two words, or one long one, leaves those alone
// and still catches "Introduction", "Experiments" and "Related work", which
// are the titles that actually come back untranslated.
func words(title string) bool {
	n, longest, run := 0, 0, 0
	for _, r := range title + " " {
		if unicode.IsLetter(r) {
			run++
			continue
		}
		if run > 0 {
			n++
			if run > longest {
				longest = run
			}
			run = 0
		}
	}
	return n >= 2 || longest > 3
}

// apologies is what a model writes instead of a translation.
//
// Every one of these is a whole answer that was once written into a corpus
// somewhere as though it were prose, and the common shape is that it reads
// like a sentence, so nothing that looks for markup finds it. The list is
// matched against the start of the file and against a paragraph of its own,
// not anywhere at all: a paper about alignment contains the sentence "I
// cannot answer that" as a quoted example and it is prose of the paper.
var apologies = []string{
	"i'm sorry", "i am sorry", "i apologize", "i apologise",
	"as an ai", "as a language model", "i cannot assist",
	"i can't assist", "i cannot provide", "i can't provide",
	"i'm unable to", "i am unable to",
	"sorry, i", "unfortunately, i",
	"here is the translation", "here's the translation",
	"translation:", "note:",
}

// providerErrors is the other way a failure gets written to disk: the
// gateway's own error body, relayed as text.
var providerErrors = []string{
	"service unavailable", "rate limit", "internal server error",
	"bad gateway", "too many requests", "context length exceeded",
	"upstream error", "\"error\":", "'error':",
}

func ruleL17(in *Input) ([]Finding, error) {
	return eachTranslation(in, func(p pair) []Finding {
		if strings.TrimSpace(p.tr.Body) == "" {
			return []Finding{{
				Rule: "L17", File: p.tr.Path,
				Message: "the file has no body, and its English has " + fmt.Sprint(len(strings.Fields(p.en.Body))) + " words",
			}}
		}
		var out []Finding
		for _, b := range blocksOf(p.tr.Body) {
			low := strings.ToLower(strings.TrimSpace(b))
			for _, s := range apologies {
				if strings.HasPrefix(low, s) {
					out = append(out, Finding{
						Rule: "L17", File: p.tr.Path,
						Message: fmt.Sprintf("a paragraph opens %q, which is the model talking and not the paper", shorten(low)),
					})
					break
				}
			}
			for _, s := range providerErrors {
				if strings.Contains(low, s) {
					out = append(out, Finding{
						Rule: "L17", File: p.tr.Path,
						Message: fmt.Sprintf("a paragraph reads %q, which is a provider's error and not a translation", shorten(low)),
					})
					break
				}
			}
		}
		return out
	})
}
