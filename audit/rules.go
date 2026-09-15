package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/split"
	"github.com/tamnd/papers-reader/tags"
)

// Rules is every rule the toolchain implements today, in id order.
//
// Nine groups and ninety-three rules, which is all the ones designed and two
// more. The last to arrive was S10, which came out of a restricted paper
// that published the wrong paper's text: every other rule read it and found
// nothing, because a well formed publication of the wrong paper is well
// formed.
func Rules() []Rule {
	out := []Rule{
		{
			ID: "S01", Hard: true,
			What:  "no content file exists for a paper whose access is unknown or missing.",
			Check: ruleS01,
		},
		{
			ID: "S02", Hard: true,
			What:  "a restricted paper has only 00_front.md, and no figures.",
			Check: ruleS02,
		},
		{
			ID: "S03", Hard: true,
			What:  "no PDF and no EPUB is tracked by git, whatever the licence says.",
			Check: ruleS03,
		},
		{
			ID: "S04", Hard: true,
			What:  "every paper in papers.yaml has an entry in sources.yaml.",
			Check: ruleS04,
		},
		{
			ID: "S05", Hard: true,
			What:  "every fetched PDF hashes to what sources.yaml records.",
			Check: ruleS05,
		},
		{
			ID: "S06", Hard: true,
			What:  "every open and permissive paper names a licence, not just a URL.",
			Check: ruleS06,
		},
		{
			ID: "S07", Hard: true,
			What:  "a restricted paper quotes under 250 words.",
			Check: ruleS07,
		},
		{
			ID: "S08", Hard: true,
			What:  "no content is longer than the PDF it claims to come from could hold.",
			Check: ruleS08,
		},
		{
			ID: "S09", Hard: true,
			What:  "the pages that were read carry as much text as a paper's pages do.",
			Check: ruleS09,
		},
		{
			ID: "S10", Hard: true,
			What:  "the quotation published from a restricted paper is from that paper.",
			Check: ruleS10,
		},
	}
	out = append(out, structureRules()...)
	out = append(out, mathematicsRules()...)
	out = append(out, codeRules()...)
	out = append(out, figuresRules()...)
	out = append(out, refsRules()...)
	out = append(out, []Rule{
		{
			ID: "G01", Hard: true,
			What:  "every tag in tags/tags is four hex characters, and every line is tag,anchor.",
			Check: ruleG01,
		},
		{
			ID: "G02", Hard: true,
			What:  "no tag appears twice.",
			Check: ruleG02,
		},
		{
			ID: "G03", Hard: true,
			What:  "no anchor appears twice.",
			Check: ruleG03,
		},
		{
			ID: "G04", Hard: true,
			What:  "every tag in a body is in tags/tags against that anchor.",
			Check: ruleG04,
		},
		{
			ID: "G05", Hard: true,
			What:  "every anchored item in a body carries a tag.",
			Check: ruleG05,
		},
		// G06 was "tags climb in reading order within a run", and it is
		// retired. The identifier is not reused.
		//
		// It was not an invariant of the corpus, only of the moment the tags
		// were handed out. A paper read again can come back with its items in
		// a different order, and their tags are permanent and come back with
		// them, so the run no longer climbs and nothing is wrong. The BERT
		// paper is the case: Table 4 sits at the top of a column above the
		// heading of the section it belongs to, the earlier extraction had it
		// the other way round, and after the re-read the rule called a
		// faithful reading of the page a defect.
		//
		// What it was for is caught twice over. A block pasted with its
		// anchor is a duplicate anchor, which is G03. A block pasted and
		// given a new anchor carries a tag the register holds against
		// something else, which is G04. What is left is the assigner handing
		// out tags in the wrong order, which is pinned where it can be
		// decided for certain, in the assigner's own test.
	}...)
	out = append(out, translationRules()...)
	return append(out, publicationRules()...)
}

// contentFiles lists the Markdown a paper has in any language, relative to
// the corpus root.
func contentFiles(in *Input, id string) ([]string, error) {
	var out []string
	for _, lang := range corpus.Langs {
		dir := in.Corpus.Content(lang, id)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			out = append(out, filepath.ToSlash(filepath.Join("content", string(lang), id, e.Name())))
		}
	}
	return out, nil
}

// figureFiles lists the committed figures of a paper.
func figureFiles(in *Input, id string) ([]string, error) {
	entries, err := os.ReadDir(in.Corpus.Figures(id))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, "figures/"+id+"/"+e.Name())
		}
	}
	return out, nil
}

// ruleS01 is the rule that makes not having looked the same as not
// publishing. It runs whether or not anything has been resolved, because the
// state it guards against is exactly the unresolved one.
func ruleS01(in *Input) ([]Finding, error) {
	var out []Finding
	for _, p := range in.Papers.Papers {
		if in.Sources.Access(p.ID) != corpus.AccessUnknown {
			continue
		}
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			out = append(out, Finding{
				Rule: "S01", File: f,
				Message: fmt.Sprintf("%s has no access class, so it may not have content", p.ID),
			})
		}
	}
	return out, nil
}

// ruleS02 holds the line on a paper that is free to read and not free to
// redistribute: the bibliographic record and a short abstract, and nothing
// else.
func ruleS02(in *Input) ([]Finding, error) {
	var out []Finding
	restricted := 0
	for _, p := range in.Papers.Papers {
		if in.Sources.Access(p.ID) != corpus.AccessRestricted {
			continue
		}
		restricted++
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if filepath.Base(f) != "00_front.md" {
				out = append(out, Finding{
					Rule: "S02", File: f,
					Message: fmt.Sprintf("%s is restricted, so it gets 00_front.md and nothing else", p.ID),
				})
			}
		}
		figures, err := figureFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range figures {
			out = append(out, Finding{
				Rule: "S02", File: f,
				Message: fmt.Sprintf("%s is restricted, so none of its figures may be committed", p.ID),
			})
		}
	}
	if restricted == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// ruleS03 asks git what it is holding rather than reading .gitignore,
// because the difference between a corpus and a mirror is one `git add -f`
// and the rule has to catch the thing that happened rather than the
// configuration that should have prevented it.
//
// An EPUB is here beside the PDF because papers book writes one and it is
// the same object under another extension: the whole text of the paper and
// every figure, in one file, ready to read. The rule is about what leaves
// the repository and not about which format it left in.
func ruleS03(in *Input) ([]Finding, error) {
	if in.Tracked == nil {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, path := range in.Tracked {
		what := ""
		switch {
		case strings.HasPrefix(path, "pdf/"), strings.HasSuffix(strings.ToLower(path), ".pdf"):
			what = "PDF"
		case strings.HasSuffix(strings.ToLower(path), ".epub"):
			what = "EPUB"
		default:
			continue
		}
		out = append(out, Finding{
			Rule: "S03", File: path,
			Message: fmt.Sprintf("a %s is tracked in a repository that commits no %ss", what, what),
		})
	}
	return out, nil
}

// ruleS04 checks that resolution covered everything. It does not run on a
// corpus where resolution has never run, because before that there is no
// claim to check and failing every paper would say nothing anybody does not
// already know.
func ruleS04(in *Input) ([]Finding, error) {
	if len(in.Sources.Sources) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, p := range in.Papers.Papers {
		if _, ok := in.Sources.ByID(p.ID); !ok {
			out = append(out, Finding{
				Rule: "S04", File: "manifests/sources.yaml",
				Message: fmt.Sprintf("%s has no source record, so nothing can be fetched for it", p.ID),
			})
		}
	}
	return out, nil
}

// ruleS05 asks whether the file on the disk is still the file the record was
// written about. Everything downstream is an assertion about that PDF: the
// licence, the page count, the page map, and every content file's
// pdf_sha256. A paper that was re-fetched and came back different, or a
// working copy somebody replaced by hand, invalidates all of it silently.
//
// It only looks at the PDFs that are here. A corpus checked out in CI has
// none of them, because none of them are ever committed, and a rule that
// failed there would be a rule everybody learned to ignore.
func ruleS05(in *Input) ([]Finding, error) {
	var out []Finding
	seen := 0
	for _, rec := range in.Sources.Sources {
		if rec.SHA256 == "" {
			continue
		}
		path := in.Corpus.PDF(rec.ID)
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sum := sha256.New()
		_, err = io.Copy(sum, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		seen++
		if got := hex.EncodeToString(sum.Sum(nil)); got != rec.SHA256 {
			out = append(out, Finding{
				Rule: "S05", File: "manifests/sources.yaml",
				Message: fmt.Sprintf("%s on the disk hashes to %s and the record says %s, so everything extracted from it is about a different file",
					rec.ID, short12(got), short12(rec.SHA256)),
			})
		}
	}
	if seen == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// ruleS06 refuses "we could download it" as a licence. An open access paper
// states its terms somewhere, and if nobody wrote them down then nobody
// checked them.
func ruleS06(in *Input) ([]Finding, error) {
	if len(in.Sources.Sources) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, rec := range in.Sources.Sources {
		switch rec.Access {
		case corpus.AccessOpen, corpus.AccessPermissive:
			if strings.TrimSpace(rec.Licence) == "" {
				out = append(out, Finding{
					Rule: "S06", File: "manifests/sources.yaml",
					Message: fmt.Sprintf("%s is %s and names no licence", rec.ID, rec.Access),
				})
			}
		}
	}
	return out, nil
}

// ruleS07 is the one number in the corpus that is a legal position rather
// than an engineering one. A restricted paper publishes a quotation, and a
// quotation long enough to substitute for the paper is not a quotation.
//
// S02 says a restricted paper gets one file. This says how much may be in
// it, and the two together are the whole of what the corpus claims about a
// paper it may not redistribute.
//
// The limit travels with the language. A translation of the stub
// republishes the same quotation and not a longer one, and corpus.Words
// counts a Japanese sentence at more than twice what it counts the English
// of it, so holding every language to the English number failed the corpus
// for translating something it was allowed to publish. See corpus.Limit for
// where the numbers come from.
func ruleS07(in *Input) ([]Finding, error) {
	restricted := 0
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() || in.Sources.Access(f.Paper) != corpus.AccessRestricted {
			continue
		}
		restricted++
		limit := corpus.Limit(split.AbstractWords, f.Lang)
		if n := corpus.Words(f.Body); n > limit {
			out = append(out, Finding{
				Rule: "S07", File: f.Path,
				Message: fmt.Sprintf("%d words quoted from a restricted paper, and the limit in %s is %d", n, f.Lang.Name(), limit),
			})
		}
	}
	if restricted == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// PageChars is the most text one page of a PDF is taken to hold, and it is
// the whole of rule S08.
//
// It is near twice anything real, because the rule is a tripwire for a model
// that answered about a paper instead of reading one, and that failure is
// out by a factor rather than by a fifth.
//
// The number was 5,000, set against the Transformer paper at 2,656
// characters a page, which turned out to be an airy paper and not a dense
// one. The ResNet and Spanner papers then tripped the rule honestly. What
// settled it was pdftotext over the PDFs themselves: ResNet's own text layer
// holds 4,996 characters a page and Spanner's holds 5,030, against the 2,671
// the Transformer paper holds, so a dense two column page really does carry
// twice what the original measurement suggested and the Markdown of one is
// that plus its markup.
//
// So the cap is twice the densest page anybody has measured, which is what
// it was before. Raising it is meant to take this much work: the rule
// decides whether the corpus is telling the truth, and the only honest way
// past it is to go and read the paper.
const PageChars = 10000

// ruleS08 asks whether the text could have come out of the file it says it
// came out of. Nothing else in the audit asks this: every other rule reads
// the content and judges the content, and a fabricated paper is
// well-formed, parses, renders and passes all of them.
func ruleS08(in *Input) ([]Finding, error) {
	chars := map[string]int{}
	for _, f := range in.Content {
		if f.Broken() || f.Lang != corpus.EN {
			continue
		}
		chars[f.Paper] += len(f.Body)
	}
	if len(chars) == 0 {
		return nil, ErrNotRun
	}
	sized := 0
	var out []Finding
	for _, p := range in.Papers.Papers {
		rec, ok := in.Sources.ByID(p.ID)
		if !ok || rec.Pages <= 0 {
			continue
		}
		sized++
		most := rec.Pages * PageChars
		if n := chars[p.ID]; n > most {
			out = append(out, Finding{
				Rule: "S08", File: "content/en/" + p.ID,
				Message: fmt.Sprintf("%d characters from a %d page PDF, which is more than %d pages can hold", n, rec.Pages, rec.Pages),
			})
		}
	}
	if sized == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// SparsePage is the least text a page of a paper is taken to hold, averaged
// over the pages that were read, and it is the whole of rule S09.
//
// The sparsest paper in the corpus today averages 1,917 characters a page
// over its first three, title page included, and the densest 5,393. Six
// hundred is under a third of the sparsest and is several times what the two
// documents this rule was written for carried.
//
// It is an average and not a floor on every page, because a plate, a blank
// verso and a page that is one full width figure are all real pages of real
// papers and all of them are nearly empty.
const SparsePage = 600

// ruleS09 asks whether the file that was fetched is the paper at all.
//
// Nothing in the audit used to ask this and the corpus paid for it. The
// resolver's ladder found, for two of the first eight papers, a lecture slide
// deck about the paper rather than the paper: 38 slides "Presented by Manu
// Reddy, Sep 9 2015" for Rosenblatt's perceptron, and 12 slides on the
// sumcheck protocol for Shamir's IP = PSPACE. Both were plausible. Both had
// the right title on the first page, the right author, a native text layer
// and a hash that matched what was downloaded. Both passed every rule in
// this file, were extracted, were split, and were published as the corpus's
// account of those papers.
//
// What gives a deck away is that there is almost nothing on a slide. A page
// of a journal carries two thousand characters and a slide carries a heading
// and four bullets, so the text per page separates the two by a factor of ten
// and no threshold in between needs defending.
//
// It reads the pages under work/, which is where extraction puts them and
// which is not committed, so in CI it has nothing to look at and says so.
// That is the right place for it: a wrong document enters the corpus on the
// machine that fetched it, and this is the rule that should stop it there.
func ruleS09(in *Input) ([]Finding, error) {
	read := 0
	var out []Finding
	for _, p := range in.Papers.Papers {
		store := extract.Store{Dir: in.Corpus.Work(p.ID, "pages")}
		pages, err := store.Pages()
		if err != nil {
			return nil, err
		}
		if len(pages) == 0 {
			continue
		}
		read++
		chars := 0
		for _, page := range pages {
			text, err := store.Read(page)
			if err != nil {
				return nil, err
			}
			chars += len(text)
		}
		if per := chars / len(pages); per < SparsePage {
			out = append(out, Finding{
				Rule: "S09", File: "manifests/sources.yaml",
				Message: fmt.Sprintf("%s reads %d characters a page over %s, which is a slide deck or a scan and not a paper", p.ID, per, plural(len(pages), "page")),
			})
		}
	}
	if read == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// TitleShare is how much of a restricted paper's title has to turn up in the
// quotation published from it, and it is the whole of rule S10's threshold.
//
// Half, and the corpus is not close to it in either direction. Of the eighty
// eight restricted papers, every one that fails the author test sits at 0.71
// or above except the one that is wrong, which sits at 0.33. The abstract of
// a paper restates what the title says it is about, so the words come back
// whether or not the title line itself survived the reading.
const TitleShare = 0.5

// ruleS10 asks whether the quotation published from a restricted paper is
// out of that paper.
//
// S02 says such a paper gets one file and S07 says how long it may be.
// Neither of them reads it. The Floyd paper is Algorithm 97 in the
// Communications of the ACM Algorithms department, half a column on the
// second page of a five page scan of the whole department, and the pages
// that were read were the first three. What got published under Floyd's
// name was Algorithm 93, General Order Arithmetic, by Millard H. Perstein
// of Control Data, in full: a paper the corpus has no record of and no
// licence for, two hundred and seventy five words of it.
//
// Nothing else could have caught it. The file parses, the front matter is
// correct, the word count is under the cap, the mathematics closes and the
// Markdown is clean. It is a well formed publication of the wrong paper.
//
// The test is that the quotation shows some sign of being the paper it is
// filed under. Either it names one of the authors, or it uses the words the
// title uses. One or the other is enough because both fail honestly: a
// journal that sets the byline in a running head the reader dropped
// publishes an abstract with no author in it, and a title like "Go To
// Statement Considered Harmful" shares almost nothing with its own abstract.
// Together they are a test the corpus passes eighty seven times out of
// eighty eight.
//
// English only. A translated stub carries the title in the title's language
// and the quotation in the quotation's, and the two would have to be matched
// through the glossary to be compared at all. The English file is the one
// the translation was made from, so checking it checks both.
func ruleS10(in *Input) ([]Finding, error) {
	quoted := 0
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() || f.Lang != corpus.EN || in.Sources.Access(f.Paper) != corpus.AccessRestricted {
			continue
		}
		want := distinctive(f.Front.Title)
		if len(want) == 0 {
			continue
		}
		quoted++
		said := map[string]bool{}
		for _, w := range alnumWords(f.Body) {
			said[w] = true
		}
		for _, a := range f.Front.Authors {
			if n := surname(a); n != "" && said[n] {
				said = nil
				break
			}
		}
		if said == nil {
			continue
		}
		hit := 0
		for _, w := range want {
			if said[w] {
				hit++
			}
		}
		if share := float64(hit) / float64(len(want)); share < TitleShare {
			out = append(out, Finding{
				Rule: "S10", File: f.Path,
				Message: fmt.Sprintf("the quotation names no author of %s and uses %d of the %s in its title, so it may be another paper off the same pages", f.Paper, hit, plural(len(want), "distinctive word")),
			})
		}
	}
	if quoted == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// alnumWords is the text as lowercase runs of letters and digits, which is
// the only comparison a title and a page of OCR can be held to. They
// disagree about case, about the hyphen in Ion-Implanted, about the
// apostrophe in MOSFET's and about which dash was set.
func alnumWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// stopWords are the words of a title that say nothing about which paper it
// is. Two thirds of the corpus has a "the" in the title and one in the
// abstract, and counting those as a match would let any page match any
// paper.
var stopWords = map[string]bool{
	"and": true, "are": true, "for": true, "its": true, "new": true,
	"our": true, "that": true, "the": true, "this": true, "using": true,
	"via": true, "with": true,
}

// distinctive is the words of a title that are worth looking for: the ones
// that are not furniture and are longer than two characters.
//
// The length cut takes out the short words a stop list would never think to
// hold, and it takes out the number in "Algorithm 97" as well. That is the
// right call even on the paper the rule was written for. A page that says 93
// where the title says 97 is a difference of one digit in OCR of a 1962
// scan, and a rule that turned on it would be reporting the scanner as often
// as the corpus.
func distinctive(title string) []string {
	var out []string
	for _, w := range alnumWords(title) {
		if len(w) > 2 && !stopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// surname is the last word of a name, lowercased, which is the part of it a
// page is most likely to print the same way the manifest does. A manifest
// that says Robert W. Floyd meets a page that says R. W. Floyd, and the
// initials are the half that changes.
func surname(name string) string {
	w := alnumWords(name)
	if len(w) == 0 {
		return ""
	}
	return w[len(w)-1]
}

// register reads tags/tags for the G group. A register that does not parse is
// reported by G01 and the later rules stand down, because they would all
// report the same one broken line.
func register(in *Input) (*tags.Register, []Finding) {
	path := filepath.Join(in.Corpus.Tags(), "tags")
	reg, err := tags.Load(path)
	if err != nil {
		return nil, []Finding{{Rule: "G01", File: "tags/tags", Message: err.Error()}}
	}
	return reg, nil
}

func ruleG01(in *Input) ([]Finding, error) {
	reg, bad := register(in)
	if bad != nil {
		return bad, nil
	}
	if reg.Len() == 0 {
		return nil, ErrNotRun
	}
	return nil, nil
}

// ruleG02 and ruleG03 are enforced by the register as it is read, so the work
// here is to say which of the two refused the line. Reusing a tag and reusing
// an anchor are different mistakes: the first breaks every link that was ever
// written to that tag, the second means one paragraph is claimed twice.
func ruleG02(in *Input) ([]Finding, error) { return duplicate(in, "G02", "is already") }

func ruleG03(in *Input) ([]Finding, error) { return duplicate(in, "G03", "already carries") }

// ruleG04 asks whether a tag written into a body is the tag the register
// hands out for that anchor.
//
// The two files can disagree in three ways and all of them are real. A tag in
// a body that the register has never heard of is a tag somebody typed, and
// the next assignment will hand the same four characters to something else. A
// tag the register holds against a different anchor is a block copied from
// one paragraph to another, which is how one theorem comes to be two. An
// anchor the register holds under a different tag is an edit to the body that
// the register was never told about.
func ruleG04(in *Input) ([]Finding, error) {
	reg, bad := register(in)
	if bad != nil {
		return bad, nil
	}
	var out []Finding
	seen := 0
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		if f.Front.Tag != "" {
			if _, err := tags.ParseTag(f.Front.Tag); err != nil {
				out = append(out, Finding{Rule: "G04", File: f.Path, Message: err.Error()})
			}
		}
		for _, a := range fileTags(f) {
			seen++
			anchor, ok := reg.Anchor(a.Tag)
			switch {
			case !ok:
				out = append(out, Finding{
					Rule: "G04", File: f.Path,
					Message: fmt.Sprintf("%s carries tag %s, which is in no register", a.Anchor, a.Tag),
				})
			case anchor != a.Anchor:
				out = append(out, Finding{
					Rule: "G04", File: f.Path,
					Message: fmt.Sprintf("%s carries tag %s, which the register holds against %s", a.Anchor, a.Tag, anchor),
				})
			}
		}
	}
	if seen == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// fileTags is every tag a content file carries, in reading order.
//
// The file's own section comes first and comes out of the front matter,
// because papers split lifted its heading up there and left no line in the
// body for an attribute block to sit on. Everything else is a block in the
// body, in the order it appears.
func fileTags(f *File) []tags.Attr {
	var out []tags.Attr
	if f.Front.Tag != "" {
		if t, err := tags.ParseTag(f.Front.Tag); err == nil {
			key := tags.SectionKey(f.Front.Section, f.Front.Kind)
			out = append(out, tags.Attr{Anchor: tags.Anchor(f.Paper, key), Classes: []string{"section"}, Tag: t})
		}
	}
	return append(out, tags.ParseAttrs(f.Body)...)
}

// ruleG05 asks whether everything that should carry a tag does.
//
// It scans with the same function papers tags assign scans with, so the rule
// and the command cannot disagree about what an anchored item is. That is
// deliberate and it is also the rule's limit: a kind of item the scanner does
// not recognise is one neither of them will ever mention.
//
// A paper with no tagged item at all has not been through the assigner yet,
// and the rule stands down rather than printing a finding for every heading
// in it. Once one item in a file is tagged the file is in the register's
// world and the rest of it has to be too.
func ruleG05(in *Input) ([]Finding, error) {
	files := 0
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		items := tags.Scan(f.Body)
		tagged := 0
		for _, it := range items {
			if it.Tag != "" {
				tagged++
			}
		}
		if f.Front.Tag != "" {
			tagged++
		}
		if tagged == 0 {
			continue
		}
		files++
		if key := tags.SectionKey(f.Front.Section, f.Front.Kind); key != "" && f.Front.Tag == "" {
			out = append(out, Finding{
				Rule: "G05", File: f.Path,
				Message: fmt.Sprintf("the section the file is, %s, carries no tag in its front matter", tags.Anchor(f.Paper, key)),
			})
		}
		for _, it := range items {
			if it.Tag != "" {
				continue
			}
			out = append(out, Finding{
				Rule: "G05", File: f.Path, Line: it.Line + 1,
				Message: fmt.Sprintf("the %s %s carries no tag", it.Class, tags.Anchor(f.Paper, it.Key)),
			})
		}
	}
	if files == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

func duplicate(in *Input, rule, marker string) ([]Finding, error) {
	path := filepath.Join(in.Corpus.Tags(), "tags")
	reg, err := tags.Load(path)
	if err != nil {
		if strings.Contains(err.Error(), marker) {
			return []Finding{{Rule: rule, File: "tags/tags", Message: err.Error()}}, nil
		}
		return nil, ErrNotRun
	}
	if reg.Len() == 0 {
		return nil, ErrNotRun
	}
	return nil, nil
}

// trackedFiles asks git for its index. Outside a checkout there is no index
// and the rules that read it stand down rather than inventing a pass.
func trackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}
