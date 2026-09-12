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
		var missed []string
		for _, t := range renderings(in.Glossary, p.en.Front.Field, p.tr.Lang) {
			if t.as == t.en {
				continue
			}
			if !word(en, t.en) || strings.Contains(tr, strings.ToLower(t.as)) {
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
type rendering struct{ en, as string }

func renderings(g *glossary.Glossary, f corpus.Field, l corpus.Lang) []rendering {
	var out []rendering
	for _, t := range g.For(f) {
		as, ok := t.Rendering(l)
		if !ok {
			continue
		}
		out = append(out, rendering{en: strings.ToLower(strings.TrimSpace(t.En)), as: strings.TrimSpace(as)})
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

// word says whether a term appears in a text on its own, rather than inside
// a longer word. Both sides are already lowercased.
func word(text, term string) bool {
	at := 0
	for {
		i := strings.Index(text[at:], term)
		if i < 0 {
			return false
		}
		i += at
		before := i == 0 || !letter(rune(text[i-1]))
		end := i + len(term)
		after := end == len(text) || !letter(rune(text[end]))
		if before && after {
			return true
		}
		at = i + 1
	}
}

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

// untranslated is the indexes of the paragraphs of a pair that came back in
// English, and is empty when the two sides do not have the same number of
// paragraphs, which is rule L03's finding or the block count check's.
func untranslated(p pair) []int {
	want, got := blocksOf(p.en.Body), blocksOf(p.tr.Body)
	if len(want) != len(got) {
		return nil
	}
	var out []int
	for i := range want {
		a, b := plainProse(want[i]), plainProse(got[i])
		if a == "" || len(strings.Fields(a)) < prosePerParagraph {
			continue
		}
		if a == b {
			out = append(out, i)
		}
	}
	return out
}

// blocksOf cuts a body at blank lines, which is the same cut the chunker
// makes and the same one the span comparison makes.
func blocksOf(body string) []string {
	var out []string
	for _, b := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(b) != "" {
			out = append(out, b)
		}
	}
	return out
}

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
func ruleL10(in *Input) ([]Finding, error) {
	if in.Glossary == nil || len(in.Glossary.Terms) == 0 {
		return nil, ErrNotRun
	}
	return eachTranslation(in, func(p pair) []Finding {
		if p.tr.Front.Kind == "references" {
			return nil
		}
		tr := strings.ToLower(translate.Prose(p.tr.Body))
		var left []string
		for _, t := range renderings(in.Glossary, p.en.Front.Field, p.tr.Lang) {
			if t.as == t.en || !word(tr, t.en) || strings.Contains(tr, strings.ToLower(t.as)) {
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

// ruleL11 finds a sentence that came back as its English inside a paragraph
// that otherwise did not.
//
// A whole paragraph left in English is rule L07 and those paragraphs are
// skipped here, so the two rules do not report the same page twice. What is
// left is the case rule 11 of the prompt is about: a model that met a
// sentence it could not do, left it, and carried on. That is the right thing
// for it to do and the wrong thing to publish.
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
		for i := range want {
			if whole[i] {
				continue
			}
			english := sentences(plainProse(want[i]))
			for s := range sentences(plainProse(got[i])) {
				if len(strings.Fields(s)) < prosePerSentence || !english[s] {
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
