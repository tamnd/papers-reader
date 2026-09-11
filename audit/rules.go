package audit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/tags"
)

// Rules is every rule the toolchain implements today, in id order.
//
// The full set is nine groups and eighty-four rules. The ones here are the
// ones that can run before anything has been extracted: the licensing rules
// that decide what may be published at all, and the tag register rules. The
// rest arrive with the milestone that produces the files they read.
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
			ID: "S06", Hard: true,
			What:  "every open and permissive paper names a licence, not just a URL.",
			Check: ruleS06,
		},
	}
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
