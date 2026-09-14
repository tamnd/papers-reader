package audit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/split"
)

// structureRules is group T, the rules about the shape of a content file
// rather than about what is written in it.
//
// They are the cheapest rules in the audit and they are the ones that catch a
// pipeline that has half run. A file whose recorded hash does not match its
// body is a file somebody edited by hand or a file a tool wrote twice; a
// paper missing section 4 is a splitter that dropped a heading. Neither shows
// up by reading the corpus, because both look perfectly ordinary on the page.
func structureRules() []Rule {
	return []Rule{
		{
			ID: "T01", Hard: true,
			What:  "every content file parses: front matter, then body.",
			Check: ruleT01,
		},
		{
			ID: "T02", Hard: true,
			What:  "every front matter field is known and typed.",
			Check: ruleT02,
		},
		{
			ID: "T03", Hard: true,
			What:  "content_sha256 matches the body as it stands.",
			Check: ruleT03,
		},
		{
			ID: "T04", Hard: true,
			What:  "section numbers within a paper are contiguous from 0.",
			Check: ruleT04,
		},
		{
			ID: "T05", Hard: true,
			What:  "the heading tree is well formed: no level skipped.",
			Check: ruleT05,
		},
		{
			ID: "T06", Hard: true,
			What:  "every paper has a 00_front.md with an abstract.",
			Check: ruleT06,
		},
		{
			ID: "T07", Hard: true,
			What:  "every paper with a reference section has it as the last file.",
			Check: ruleT07,
		},
		{
			ID:    "T08",
			What:  "no section body is under 200 characters.",
			Check: ruleT08,
		},
		{
			ID:    "T09",
			What:  "no section body is over 40,000 characters.",
			Check: ruleT09,
		},
		{
			ID: "T10", Hard: true,
			What:  "no page furniture is left in the body: running heads, bare folios.",
			Check: ruleT10,
		},
		{
			ID: "T11", Hard: true,
			What:  "no raw HTML markup is left in a body.",
			Check: ruleT11,
		},
		{
			ID: "T12", Hard: true,
			What:  "no Markdown link is left in a body.",
			Check: ruleT12,
		},
		{
			ID:    "T13",
			What:  "no word is left split at the hyphen the page broke it with.",
			Check: ruleT13,
		},
	}
}

// anyContent is what every rule in this group opens with. A corpus with no
// content in it yet is the normal state of a milestone in progress, and a
// group that passed on it would be claiming to have checked something.
func anyContent(in *Input) bool { return len(in.Content) > 0 }

// eachFile runs a check over every file that parsed, which is every rule in
// the group except T01 and T02.
func eachFile(in *Input, rule string, check func(*File) string) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		if msg := check(f); msg != "" {
			out = append(out, Finding{Rule: rule, File: f.Path, Message: msg})
		}
	}
	return out, nil
}

func ruleT01(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			out = append(out, Finding{Rule: "T01", File: f.Path, Message: f.Err.Error()})
		}
	}
	return out, nil
}

func ruleT02(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			// T01 has this one and says so in better words. Reporting it
			// twice would double the count of a corpus with one bad file.
			continue
		}
		if f.Strict != nil {
			out = append(out, Finding{
				Rule: "T02", File: f.Path,
				Message: strings.TrimPrefix(f.Strict.Error(), "yaml: "),
			})
			continue
		}
		if msg := typed(f.Front); msg != "" {
			out = append(out, Finding{Rule: "T02", File: f.Path, Message: msg})
		}
	}
	return out, nil
}

// kinds is what a content file may be. A file of some fifth kind is a file
// the reading app will not know how to lay out and the translator will not
// know whether to translate.
var kinds = map[string]bool{
	"front":      true,
	"section":    true,
	"references": true,
	"appendix":   true,
}

// typed checks the fields whose type YAML cannot check, which are the ones
// holding a word out of a fixed list. YAML will tell you that kind is a
// string. It will not tell you that "reference" is not "references".
func typed(f corpus.Front) string {
	switch {
	case strings.TrimSpace(f.Paper) == "":
		return "paper is empty, so nothing can say which paper this belongs to"
	case strings.TrimSpace(f.Title) == "":
		return "title is empty"
	case !kinds[f.Kind]:
		return fmt.Sprintf("kind is %q, and a content file is front, section, references or appendix", f.Kind)
	case !f.Lang.Valid():
		return fmt.Sprintf("lang is %q, which is not one of the four the corpus publishes", f.Lang)
	}
	return ""
}

// ruleT03 is the rule that says whether the corpus is what the pipeline
// produced. Every other rule in the audit reads the body and believes it; this
// one asks whether the body is still the body the tool that wrote it saw.
//
// A mismatch is not always wrong. Somebody who fixes a mangled formula by hand
// is doing the right thing and will trip this, which is the point: the fix has
// to be recorded, because a translation made from the old text is now a
// translation of something nobody can find.
func ruleT03(in *Input) ([]Finding, error) {
	return eachFile(in, "T03", func(f *File) string {
		want := f.Front.ContentSHA256
		if strings.TrimSpace(want) == "" {
			return "content_sha256 is missing, so nothing can say whether this file has been edited"
		}
		if got := corpus.ContentSHA([]byte(f.Body)); got != want {
			return fmt.Sprintf("content_sha256 is %s and the body hashes to %s", short12(want), short12(got))
		}
		return ""
	})
}

func short12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// ruleT04 counts the files rather than reading the section numbers in them.
// The file names are the order the reading app walks in and the order a
// translator works through, so a gap in them is a paper with a hole in the
// middle whatever the front matter claims.
func ruleT04(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	groups := byPaper(in.Content)
	for _, key := range keys(groups) {
		files := groups[key]
		seen := map[int]string{}
		for _, f := range files {
			if f.Ordinal < 0 {
				out = append(out, Finding{
					Rule: "T04", File: f.Path,
					Message: "the name does not open with a section number, and papers split writes them all",
				})
				continue
			}
			if was, ok := seen[f.Ordinal]; ok {
				out = append(out, Finding{
					Rule: "T04", File: f.Path,
					Message: fmt.Sprintf("section %02d is also %s", f.Ordinal, was),
				})
				continue
			}
			seen[f.Ordinal] = f.Name
		}
		for n := 0; n < len(seen); n++ {
			if _, ok := seen[n]; ok {
				continue
			}
			out = append(out, Finding{
				Rule: "T04", File: dirOf(files[0].Path),
				Message: fmt.Sprintf("there are %d sections and no section %02d, so the numbering has a hole in it", len(seen), n),
			})
		}
	}
	return out, nil
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return path
}

// heading matches an ATX heading, which is the only kind the corpus writes.
var heading = regexp.MustCompile(`^(#{1,6})\s+\S`)

// ruleT05 walks the headings of one file and refuses a jump. A file that goes
// from a level two heading to a level four has lost a heading somewhere, and
// the reading app builds its table of contents out of exactly this.
//
// The first heading in a file sets the level the file starts at rather than
// having to be a level one, because a content file is one section of a paper
// and the level it sits at in the whole is the splitter's business and not
// this rule's.
func ruleT05(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		last := 0
		for n, line := range prose(f.Body) {
			m := heading.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			level := len(m[1])
			if last > 0 && level > last+1 {
				out = append(out, Finding{
					Rule: "T05", File: f.Path, Line: n + 1,
					Message: fmt.Sprintf("a level %d heading under a level %d one, so a level is missing between them", level, last),
				})
			}
			last = level
		}
	}
	return out, nil
}

// fence matches the open or the close of a fenced code block.
var codeFence = regexp.MustCompile("^\\s{0,3}(```|~~~)")

// prose is the lines of a body with the fenced code blocks blanked out,
// keeping the line numbering. A hash at the start of a line inside a listing
// is a comment in somebody's Python and not a heading, and a rule that read
// it as one would report a heading tree the reader never sees.
//
// Blanked rather than dropped, because a finding names a line and a line
// number that counted only the prose would send a reader to the wrong place.
func prose(body string) []string {
	lines := strings.Split(body, "\n")
	out := make([]string, len(lines))
	in := false
	for i, line := range lines {
		if codeFence.MatchString(line) {
			in = !in
			continue
		}
		if !in {
			out[i] = line
		}
	}
	return out
}

// abstractWords is the shortest run of prose this will accept as an
// abstract, and it is the splitter's own number. Forty is well under the
// shortest abstract in the corpus and well over the longest title block,
// which is the gap the rule needs. A front file whose longest paragraph is
// seven words is a cover sheet that came out of the PDF where the abstract
// should have been.
//
// Shared with split rather than written twice, because the splitter uses it
// to decide whether a restricted paper's front block already holds an
// abstract, and two numbers would mean the splitter writing a file this rule
// then refuses with nobody able to say which of the two was wrong.
const abstractWords = split.AbstractParagraph

// ruleT06 is about the one file every paper has. The front matter file
// carries the title, the authors and the abstract, and the abstract is the
// part a reader reads before deciding whether to read the paper, so a corpus
// whose front file is a title and nothing else has published a catalogue
// entry rather than a paper.
//
// There is no heading to look for. Most papers head their abstract and the
// splitter folds that heading into this file, and the ones that do not head
// it are exactly the ones a heading test would fail. So the test is on the
// prose: an abstract is a paragraph, and a paragraph is longer than a title.
func ruleT06(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	groups := byPaper(in.Content)
	for _, key := range keys(groups) {
		files := groups[key]
		var front *File
		for _, f := range files {
			if f.Ordinal == 0 {
				front = f
			}
		}
		if front == nil {
			out = append(out, Finding{
				Rule: "T06", File: dirOf(files[0].Path),
				Message: "there is no 00_front.md, so this paper has no title block and no abstract",
			})
			continue
		}
		if front.Broken() {
			continue
		}
		if n := longestParagraph(front.Body); n < abstractWords {
			out = append(out, Finding{
				Rule: "T06", File: front.Path,
				Message: fmt.Sprintf("the longest paragraph is %s, which is a title block and not an abstract", plural(n, "word")),
			})
		}
	}
	return out, nil
}

// plural writes a count and its noun. A finding is a sentence somebody reads
// in a report and "1 words" is the mark of a program nobody proofread.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// longestParagraph is the word count of the longest paragraph of a body. The
// corpus writes one paragraph per line, so a line is a paragraph.
func longestParagraph(body string) int {
	most := 0
	for _, line := range prose(body) {
		if n := corpus.Words(line); n > most {
			most = n
		}
	}
	return most
}

// ruleT07 puts the bibliography at the end. A reference section in the middle
// of a paper is a splitter that cut an appendix out of the middle of it, and
// the reading app numbers the sections in file order, so the reader would be
// handed the references and then more paper.
func ruleT07(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	groups := byPaper(in.Content)
	for _, key := range keys(groups) {
		files := groups[key]
		for i, f := range files {
			if f.Broken() || f.Front.Kind != "references" {
				continue
			}
			// An appendix after the references is how a paper is printed, so
			// it is not this rule's business. Anything else is.
			for _, after := range files[i+1:] {
				if after.Broken() || after.Front.Kind == "appendix" {
					continue
				}
				out = append(out, Finding{
					Rule: "T07", File: f.Path,
					Message: fmt.Sprintf("the references are followed by %s, which is a %s", after.Name, after.Front.Kind),
				})
			}
		}
	}
	return out, nil
}

// minBody and maxBody are the two ends of a section that went wrong. Neither
// is hard: a paper really can have a one line section, and a paper with no
// headings at all really does come out as one long file. Both are worth a
// person's eye and neither is worth failing a build over.
const (
	minBody = 200
	maxBody = 40000
)

func ruleT08(in *Input) ([]Finding, error) {
	return eachFile(in, "T08", func(f *File) string {
		// The front file is a title, some authors and an abstract, and is
		// short in every paper that has one. T06 is the rule that reads it.
		if f.Ordinal == 0 {
			return ""
		}
		if n := len(strings.TrimSpace(f.Body)); n < minBody {
			return fmt.Sprintf("the body is %d characters, which is a split that landed in the wrong place", n)
		}
		return ""
	})
}

func ruleT09(in *Input) ([]Finding, error) {
	return eachFile(in, "T09", func(f *File) string {
		if n := len(f.Body); n > maxBody {
			return fmt.Sprintf("the body is %d characters, which is a section that was never split", n)
		}
		return ""
	})
}

// folio matches a line that is only a page number, with or without the rules
// and dashes a paper sets around one.
var folio = regexp.MustCompile(`(?i)^\s*[-–—|]*\s*(?:page\s+)?\d{1,4}\s*[-–—|]*\s*$`)

// ruleT10 is the last line of defence against page furniture, and it is where
// the running heads that the extractor's own detector missed turn up.
//
// The extractor learns the furniture from the whole paper: a line that sits
// in the same place on most pages is a running head and goes. What it cannot
// catch is a paper that prints its head on four pages out of forty, or one
// that prints the section title as the head so that the line differs on every
// page. Those arrive here, as a bare number on a line of its own or as a line
// that repeats.
//
// Only a bare folio is reported, and not a line of prose that happens to be
// short. The rule has to be one a person can act on: a finding that says
// "line 40 might be furniture" is a finding nobody checks twice.
func ruleT10(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		for n, line := range prose(f.Body) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if folio.MatchString(line) {
				out = append(out, Finding{
					Rule: "T10", File: f.Path, Line: n + 1,
					Message: fmt.Sprintf("%q is a page number on a line of its own", strings.TrimSpace(line)),
				})
			}
		}
	}
	return out, nil
}

var (
	// htmlTag is an opening, closing or self closing tag. The name has to be
	// letters and digits, which is what keeps `<satoshin@gmx.com>` and
	// `<n, k>` out of it before the list above is even consulted.
	htmlTag = regexp.MustCompile(`<\s*/?\s*([a-zA-Z][a-zA-Z0-9]*)\b[^>]*>`)
	// inlineCode is a span of backticks. A paper about the web writes
	// `<table>` in prose and means the word, not the markup.
	inlineCode = regexp.MustCompile("`[^`]*`")
)

// ruleT11 is the rule that keeps an HTML table from being silent.
//
// Three sections of the Transformer paper came back from the vision reader as
// raw `<table>` markup, and every other rule in the audit was happy with them.
// The T rules saw a body of the right length with a well formed heading tree.
// The M rules saw no mathematics, because `<td>1.0 \cdot 10^{20}</td>` is TeX
// with no dollars round it and mathtex cannot see it. The C rules saw no
// listings. The corpus was clean and three tables were unreadable.
//
// extract.Untable converts what it can and deliberately leaves alone what it
// cannot, so this rule is the other half of that decision: a table Untable
// refused is a table a person has to look at, and this is how they find out.
// It is hard because there is no version of a body with markup left in it that
// is correct.
//
// Acceptance rule A10 asks the same question of a page before the page is
// written, off the same list of tags, so a reading that comes back as HTML is
// now asked again rather than landing here. This rule stays because the two
// catch different things: A10 covers what the vision path reads today, and
// this covers the pages that were read before A10 existed, the ones a person
// corrected by hand, and the translations.
//
// Math and code are skipped, and so is inline code, because all three are
// places where a tag is content. Only the first tag in a file is reported,
// with a count, because one table is forty findings and a rule that produces
// forty findings for one defect is a rule people learn to scroll past.
func ruleT11(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		lines := strings.Split(f.Body, "\n")
		safe := protectedLines(f.Body, len(lines))
		first, found, count := 0, "", 0
		for i, line := range lines {
			if safe[i+1] {
				continue
			}
			for _, m := range htmlTag.FindAllStringSubmatch(inlineCode.ReplaceAllString(line, " "), -1) {
				if !extract.HTMLTags[strings.ToLower(m[1])] {
					continue
				}
				count++
				if first == 0 {
					first, found = i+1, strings.TrimSpace(m[0])
				}
			}
		}
		if count == 0 {
			continue
		}
		out = append(out, Finding{
			Rule: "T11", File: f.Path, Line: first,
			Message: fmt.Sprintf("%q is HTML, and there are %d tags in this file that the reading app will show as text", found, count),
		})
	}
	return out, nil
}

// ruleT12 is the same rule as T11 for the other markup a reader adds by
// habit, which is the link.
//
// The corpus has no links in it. A reference is a numbered entry, a cross
// reference is an anchor, a paper is [[an id]], and a figure is a manifest
// entry rather than an image. So a link in a body is always something a
// model wrote that was not on the page, and it always arrives the same way:
// the paper prints a web address as prose, "available at
// http://research.microsoft.com/barc/SortBenchmark/", and a model that has
// read a great deal of Markdown writes it back as a link.
//
// It was found three times over in one afternoon. All three translations of
// the GAN introduction did it to the footnote that gives the address of the
// code, and the second reading of the MapReduce bibliography did it to
// reference 10, where it came with a retyped address as well. So it is one
// rule over every file in every language rather than a comparison against
// the English, which is what it was when only the translations had done it.
//
// Hard, because a link is the one piece of markup the reading app and the
// book both render, so a wrong one is visible to a reader who has no way to
// check it.
func ruleT12(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		lines := strings.Split(f.Body, "\n")
		safe := protectedLines(f.Body, len(lines))
		for i, line := range lines {
			if safe[i+1] {
				continue
			}
			for _, m := range markdownLink.FindAllString(inlineCode.ReplaceAllString(line, " "), -1) {
				out = append(out, Finding{
					Rule: "T12", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("%s is a link, and the corpus has no links in it", m),
				})
			}
		}
	}
	return out, nil
}

// markdownLink is an inline link or an inline image. A reference style link
// is not matched and does not need to be: nothing writes one, and the
// definition it would need would be a finding of its own.
var markdownLink = regexp.MustCompile(`!?\[[^\]\n]*\]\([^)\n]*\)`)

// ruleT13 is the word the page broke across two lines and the reader put
// back together wrong.
//
// A printed column breaks a word at a hyphen to make the line fit, and the
// hyphen is the page's and not the word's. The reading prompt says so in as
// many words and asks for the word written whole, and a reader that has one
// of these to do in a page does it. The two in this corpus are a reader
// that had one to do in a long page and joined the two halves with a space
// instead of with nothing: "less than a mil- lisecond" in the MapReduce
// cluster description, and a surname in its bibliography.
//
// This is a rule and not a repair because there is no way to tell the two
// cases apart without a dictionary. "mil- lisecond" is one word with the
// page's hyphen in it and wants both the hyphen and the space taken out;
// "high- quality" is one word with its own hyphen in it and wants only the
// space taken out. A repair that guessed would turn the second into
// "highquality" across a hundred papers to save a person looking at two
// lines.
//
// Not hard, for the same reason. What it reports is sometimes neither of
// those: a paper writing "map- and reduce-style" means the hyphen and the
// space both, and that is why the conjunctions are skipped rather than
// reported and argued about.
//
// Math, code and inline code are skipped, where a hyphen is a minus sign
// and a flag.
func ruleT13(in *Input) ([]Finding, error) {
	if !anyContent(in) {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		lines := strings.Split(f.Body, "\n")
		safe := protectedLines(f.Body, len(lines))
		for i, line := range lines {
			if safe[i+1] {
				continue
			}
			for _, m := range splitWord.FindAllStringSubmatch(inlineCode.ReplaceAllString(line, " "), -1) {
				if suspended[m[2]] {
					continue
				}
				out = append(out, Finding{
					Rule: "T13", File: f.Path, Line: i + 1,
					Message: fmt.Sprintf("%q is one word the page broke across two lines", m[0]),
				})
			}
		}
	}
	return out, nil
}

// splitWord is two runs of lower case letters with a hyphen and a space
// between them. Lower case on both sides, because a capital after the space
// is a new sentence or a name and the hyphen before it is the page's dash.
// Two letters at least on each side, because a single letter either side of
// a hyphen is "e- mail" and "x- axis", which are their own repair.
var splitWord = regexp.MustCompile(`(\p{Ll}{2,})[-\x{2010}\x{2011}] (\p{Ll}{2,})`)

// suspended is the word that follows a hyphen the writer left hanging on
// purpose: "map- and reduce-style", "input- or output-bound". The hyphen is
// real, the space is real, and there is nothing to report.
var suspended = map[string]bool{
	"and": true, "or": true, "nor": true, "to": true, "but": true,
	"và": true, "hoặc": true,
}
