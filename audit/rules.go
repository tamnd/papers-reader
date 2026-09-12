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

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/split"
	"github.com/tamnd/papers-reader/tags"
)

// Rules is every rule the toolchain implements today, in id order.
//
// The full set is nine groups and eighty-four rules. The ones here are the
// licensing rules that decide what may be published at all, the tag register
// rules, and the groups whose files the toolchain can already produce:
// figures and references. The rest arrive with the milestone that produces
// the files they read.
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
			What:  "no PDF is tracked by git, whatever the licence says.",
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
	}
	out = append(out, structureRules()...)
	out = append(out, mathematicsRules()...)
	out = append(out, figuresRules()...)
	out = append(out, refsRules()...)
	return append(out, []Rule{
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
		{
			ID: "G06", Hard: true,
			What:  "tags climb in reading order within a run.",
			Check: ruleG06,
		},
	}...)
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
func ruleS03(in *Input) ([]Finding, error) {
	if in.Tracked == nil {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, path := range in.Tracked {
		if strings.HasPrefix(path, "pdf/") || strings.HasSuffix(strings.ToLower(path), ".pdf") {
			out = append(out, Finding{
				Rule: "S03", File: path,
				Message: "a PDF is tracked in a repository that commits no PDFs",
			})
		}
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
func ruleS07(in *Input) ([]Finding, error) {
	restricted := 0
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() || in.Sources.Access(f.Paper) != corpus.AccessRestricted {
			continue
		}
		restricted++
		if n := len(strings.Fields(f.Body)); n > split.AbstractWords {
			out = append(out, Finding{
				Rule: "S07", File: f.Path,
				Message: fmt.Sprintf("%d words quoted from a restricted paper, and the limit is %d", n, split.AbstractWords),
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
// The densest paper in the corpus today is the Transformer paper at 2,656
// characters a page, so the cap is near twice anything real. It is set that
// wide because the rule is a tripwire for a model that answered about a
// paper instead of reading one, and that failure is out by a factor rather
// than by a fifth. A three column proceedings page from the 1960s is the one
// thing that could reach it honestly, and when one does somebody has to look
// at the paper and raise the number, which is the right amount of friction
// for a rule that decides whether the corpus is telling the truth.
const PageChars = 5000

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

// ruleG06 asks whether the tags in a file climb.
//
// Within one assignment they do, because the assigner walks the corpus in
// reading order and takes the next tag each time. Across assignments they do
// not, and must not be made to: a section added to a paper next year gets a
// tag from the top of the register and sits between two much lower ones, and
// that is what append only means. So the comparison is only made between two
// tags tags/runs says came out of the same run.
//
// What it catches is an attribute block copied from further down a file and
// pasted further up, which otherwise looks exactly like a correct tag.
func ruleG06(in *Input) ([]Finding, error) {
	runs, err := tags.LoadRuns(filepath.Join(in.Corpus.Tags(), "runs"))
	if err != nil {
		return []Finding{{Rule: "G06", File: "tags/runs", Message: err.Error()}}, nil
	}
	if len(runs) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Content {
		if f.Broken() {
			continue
		}
		attrs := fileTags(f)
		for i := 1; i < len(attrs); i++ {
			a, b := attrs[i-1], attrs[i]
			if !tags.Together(runs, a.Tag, b.Tag) || b.Tag.Value() > a.Tag.Value() {
				continue
			}
			out = append(out, Finding{
				Rule: "G06", File: f.Path,
				Message: fmt.Sprintf("%s carries %s and comes after %s, which carries %s, and one run hands tags out in reading order", b.Anchor, b.Tag, a.Anchor, a.Tag),
			})
		}
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
