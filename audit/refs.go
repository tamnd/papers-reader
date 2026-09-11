package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/refs"
	"github.com/tamnd/papers-reader/sources"
)

// refsRules is group R, the rules over the bibliographies and the citation
// graph they build.
//
// The graph is the part of the corpus a reader is most likely to take on
// trust, because checking one edge of it means finding two papers and
// reading both. So the rules here are about whether an edge is real, and
// three of them are hard.
func refsRules() []Rule {
	return []Rule{
		{
			ID: "R01", Hard: true,
			What:  "every [[id]] in a body names a paper in papers.yaml.",
			Check: ruleR01,
		},
		{
			ID: "R02", Hard: true,
			What:  "every in-text [n] has an entry n in that paper's bibliography.",
			Check: ruleR02,
		},
		{
			ID: "R03", Hard: true,
			What:  "every reference keeps the text the paper printed.",
			Check: ruleR03,
		},
		{
			ID:    "R04",
			What:  "every resolves_to passes the verified matcher again.",
			Check: ruleR04,
		},
		{
			ID: "R05", Hard: true,
			What:  "no resolves_to points at the citing paper itself.",
			Check: ruleR05,
		},
		{
			ID: "R08", Hard: true,
			What:  "a reference section renders the printed text and not only the parsed fields.",
			Check: ruleR08,
		},
	}
}

// ruleR01 catches a link to a paper that is not there. A dead [[id]] is a
// broken page in the reading app and the usual cause is a paper renamed in
// papers.yaml without the bibliographies being resolved again.
func ruleR01(in *Input) ([]Finding, error) {
	known := make(map[string]bool, len(in.Papers.Papers))
	for _, p := range in.Papers.Papers {
		known[p.ID] = true
	}
	var out []Finding
	ran := false
	for _, p := range in.Papers.Papers {
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			body, err := readBody(in, file)
			if err != nil {
				return nil, err
			}
			ran = true
			for _, id := range refs.Linked(string(body)) {
				if !known[id] {
					out = append(out, Finding{
						Rule: "R01", File: file,
						Message: fmt.Sprintf("the link to %s names no paper in papers.yaml", id),
					})
				}
			}
		}
	}
	if !ran {
		return nil, ErrNotRun
	}
	return out, nil
}

// ruleR02 catches a citation that points at nothing. A paper that says "as
// shown in [31]" and has thirty references has lost an entry somewhere in
// extraction, and the reader following the number finds nothing.
func ruleR02(in *Input) ([]Finding, error) {
	var out []Finding
	ran := false
	for _, p := range in.Papers.Papers {
		m := in.Refs[p.ID]
		if m == nil {
			continue
		}
		keys := make(map[string]bool, len(m.Entries))
		for _, e := range m.Entries {
			keys[e.Key] = true
		}
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			body, err := readBody(in, file)
			if err != nil {
				return nil, err
			}
			ran = true
			seen := map[string]bool{}
			for _, key := range refs.Citations(string(body)) {
				if keys[key] || seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, Finding{
					Rule: "R02", File: file,
					Message: fmt.Sprintf("the citation [%s] has no entry %s in the bibliography of %s", key, key, p.ID),
				})
			}
		}
	}
	if !ran {
		return nil, ErrNotRun
	}
	return out, nil
}

// ruleR03 is the one that keeps the whole design honest. The parser is
// allowed to be wrong about a reference because the page renders what the
// paper printed, and an entry that has lost its raw text has nothing left
// to render.
func ruleR03(in *Input) ([]Finding, error) {
	if len(in.Refs) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range sortedRefs(in) {
		m := in.Refs[id]
		for _, e := range m.Entries {
			if strings.TrimSpace(e.Raw) == "" {
				out = append(out, Finding{
					Rule: "R03", File: refsPath(id),
					Message: fmt.Sprintf("entry %s has no raw text", e.Key),
				})
			}
		}
	}
	return out, nil
}

// ruleR04 runs the matcher again over what it decided last time.
//
// It is soft because it can fail for an honest reason: a title corrected in
// papers.yaml moves the similarity and an edge that was right yesterday
// scores below the floor today. That is worth seeing and is not worth
// stopping a build for.
func ruleR04(in *Input) ([]Finding, error) {
	if len(in.Refs) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range sortedRefs(in) {
		m := in.Refs[id]
		for _, e := range m.Entries {
			if e.ResolvesTo == "" {
				continue
			}
			p, ok := paperByID(in, e.ResolvesTo)
			if !ok {
				// R01 and the rule below both have something to say about
				// this, and saying it three times helps nobody.
				continue
			}
			if e.ArXiv != "" || e.DOI != "" {
				// An identifier is not a similarity score and re-running the
				// matcher over one would fail every entry the parser could
				// read an id from and nothing else.
				continue
			}
			v := sources.Verify(
				sources.Want{Title: p.Title, Authors: p.Authors, Year: p.Year},
				sources.Candidate{Title: e.Title, Authors: e.Authors, Year: e.Year},
			)
			if !v.OK {
				out = append(out, Finding{
					Rule: "R04", File: refsPath(id),
					Message: fmt.Sprintf("entry %s resolves to %s and no longer passes the matcher: %s", e.Key, e.ResolvesTo, v.Why),
				})
			}
		}
	}
	return out, nil
}

// ruleR05 catches the self loop. A paper's own bibliography never names the
// paper, and the citation graph's one data-quality check is that it has no
// cycle among papers years apart, which a self loop would break before it
// started.
func ruleR05(in *Input) ([]Finding, error) {
	if len(in.Refs) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range sortedRefs(in) {
		m := in.Refs[id]
		for _, e := range m.Entries {
			if e.ResolvesTo == id {
				out = append(out, Finding{
					Rule: "R05", File: refsPath(id),
					Message: fmt.Sprintf("entry %s resolves to the paper it is printed in", e.Key),
				})
			}
		}
	}
	return out, nil
}

// ruleR08 checks that the reference section on the page is the paper's own
// text.
//
// A reference list rebuilt out of the parsed fields would read well and be
// subtly wrong about a hundred entries at once, and nobody proofreads a
// bibliography. So the rule looks for the opening of each entry as printed,
// in the file a reader sees.
func ruleR08(in *Input) ([]Finding, error) {
	var out []Finding
	ran := false
	for _, id := range sortedRefs(in) {
		m := in.Refs[id]
		body, file, err := referencesFile(in, id)
		if err != nil {
			return nil, err
		}
		if file == "" {
			continue
		}
		ran = true
		flat := flatten(body)
		missing := 0
		for _, e := range m.Entries {
			if opening := opening(e.Raw); opening != "" && !strings.Contains(flat, opening) {
				missing++
			}
		}
		if missing > 0 {
			out = append(out, Finding{
				Rule: "R08", File: file,
				Message: fmt.Sprintf("%d of the %d references are not on the page as the paper printed them", missing, len(m.Entries)),
			})
		}
	}
	if !ran {
		return nil, ErrNotRun
	}
	return out, nil
}

// opening is the start of a reference, long enough to be unmistakable and
// short enough to survive a citation inside the entry being rewritten into
// a link.
func opening(raw string) string {
	r := []rune(flatten(raw))
	if len(r) < 20 {
		return ""
	}
	if len(r) > 40 {
		r = r[:40]
	}
	return string(r)
}

func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

// referencesFile finds the file a paper's reference section was written to,
// by its kind rather than by its name, because the number in front of the
// name is the paper's own section number and is not the same twice.
func referencesFile(in *Input, id string) (body, file string, err error) {
	files, err := contentFiles(in, id)
	if err != nil {
		return "", "", err
	}
	for _, f := range files {
		if !strings.HasPrefix(f, "content/en/") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(in.Corpus.Root, filepath.FromSlash(f)))
		if err != nil {
			return "", "", err
		}
		front, text, err := corpus.ParseFront(b)
		if err != nil {
			continue
		}
		if front.Kind == "references" {
			return string(text), f, nil
		}
	}
	return "", "", nil
}

// readBody is one content file without its front matter. A file whose front
// matter will not parse is read whole, because the rules here are about
// what is in the text and a malformed header is somebody else's finding.
func readBody(in *Input, file string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(in.Corpus.Root, filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	_, body, err := corpus.ParseFront(b)
	if err != nil {
		return b, nil
	}
	return body, nil
}

func paperByID(in *Input, id string) (corpus.Paper, bool) {
	for _, p := range in.Papers.Papers {
		if p.ID == id {
			return p, true
		}
	}
	return corpus.Paper{}, false
}

func refsPath(id string) string { return "manifests/refs/" + id + ".yaml" }

// sortedRefs is every paper with a bibliography, in id order, so that a
// report reads the same way twice.
func sortedRefs(in *Input) []string {
	out := make([]string, 0, len(in.Refs))
	for id := range in.Refs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
