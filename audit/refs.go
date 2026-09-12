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
			ID:    "R06",
			What:  "the citation graph has no cycle among papers more than two years apart.",
			Check: ruleR06,
		},
		{
			ID:    "R07",
			What:  "a paper three or more corpus papers cite is in the corpus.",
			Check: ruleR07,
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

// ruleR06 looks for a citation that travels backwards in time.
//
// A paper cannot cite one published after it, so the graph the corpus builds
// out of resolves_to has to be acyclic, with one honest exception: two
// preprints of the same season cite each other's earlier drafts, and a
// journal version dated a year or two after the conference version reads as
// a loop that is really the same work twice. So a cycle is only reported
// when the papers in it are far enough apart that no revision explains it.
//
// This is the check the whole resolver is measured by. Every other rule here
// asks whether one entry is well formed; this one asks whether the graph as
// a whole could be true, and a cycle across a decade means the matcher
// joined two different papers with similar titles.
func ruleR06(in *Input) ([]Finding, error) {
	if len(in.Refs) == 0 {
		return nil, ErrNotRun
	}
	year := map[string]int{}
	for _, p := range in.Papers.Papers {
		year[p.ID] = p.Year
	}
	edges := map[string][]string{}
	for _, id := range sortedRefs(in) {
		seen := map[string]bool{}
		for _, e := range in.Refs[id].Entries {
			// A self loop is rule R05's finding and is not a cycle worth
			// reporting twice.
			if e.ResolvesTo == "" || e.ResolvesTo == id || seen[e.ResolvesTo] {
				continue
			}
			seen[e.ResolvesTo] = true
			edges[id] = append(edges[id], e.ResolvesTo)
		}
	}
	var out []Finding
	for _, group := range cycles(edges) {
		lo, hi := year[group[0]], year[group[0]]
		for _, id := range group {
			lo, hi = min(lo, year[id]), max(hi, year[id])
		}
		if hi-lo <= citeRevision {
			continue
		}
		out = append(out, Finding{
			Rule: "R06", File: refsPath(group[0]),
			Message: fmt.Sprintf("%s cite one another in a loop and were published %d years apart", strings.Join(group, ", "), hi-lo),
		})
	}
	return out, nil
}

// citeRevision is how many years apart two papers in a citation loop can be
// before the loop stops being a revision of the same work and starts being a
// mistake. Two, because a conference paper and the journal version of it are
// usually a year apart and occasionally two.
const citeRevision = 2

// cycles is every group of papers that can all reach one another, which is
// to say every strongly connected component with more than one paper in it.
// Each group comes back in id order, and the groups in the order of their
// first paper, so that a report reads the same way twice.
func cycles(edges map[string][]string) [][]string {
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var out [][]string
	next := 0

	var walk func(string)
	walk = func(v string) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range edges[v] {
			if _, seen := index[w]; !seen {
				walk(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var group []string
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			group = append(group, w)
			if w == v {
				break
			}
		}
		if len(group) > 1 {
			sort.Strings(group)
			out = append(out, group)
		}
	}
	roots := make([]string, 0, len(edges))
	for v := range edges {
		roots = append(roots, v)
	}
	sort.Strings(roots)
	for _, v := range roots {
		if _, seen := index[v]; !seen {
			walk(v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// ruleR07 is the one rule here that is about what the corpus does not have.
//
// A paper three of the hundred all cite is a paper the hundred are built on,
// and a reader who follows the citations arrives at it and finds nothing.
// That is not a defect in any file, which is why it is soft: it is the
// reading list telling the people who keep it what it is missing, and the
// answer is sometimes that the paper is a textbook or a technical report
// nobody would add.
//
// References are counted by paper and not by entry, so a paper that cites
// the same work in two of its sections counts once, and grouped by DOI or
// arXiv id where there is one and by the title otherwise. The title is
// matched loosely because the same work is printed three different ways in
// three different bibliographies.
func ruleR07(in *Input) ([]Finding, error) {
	if len(in.Refs) == 0 {
		return nil, ErrNotRun
	}
	type work struct {
		title string
		by    map[string]bool
	}
	works := map[string]*work{}
	for _, id := range sortedRefs(in) {
		for _, e := range in.Refs[id].Entries {
			if e.ResolvesTo != "" {
				continue
			}
			key := citedKey(e)
			if key == "" {
				continue
			}
			w := works[key]
			if w == nil {
				w = &work{title: citedName(e), by: map[string]bool{}}
				works[key] = w
			}
			w.by[id] = true
		}
	}
	keys := make([]string, 0, len(works))
	for k := range works {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []Finding
	for _, k := range keys {
		w := works[k]
		if len(w.by) < citedOften {
			continue
		}
		citing := make([]string, 0, len(w.by))
		for id := range w.by {
			citing = append(citing, id)
		}
		sort.Strings(citing)
		out = append(out, Finding{
			Rule: "R07", File: "manifests/papers.yaml",
			Message: fmt.Sprintf("%q is cited by %s and is not in the corpus", w.title, strings.Join(citing, ", ")),
		})
	}
	return out, nil
}

// citedOften is how many of the corpus's papers have to cite the same work
// before its absence is worth a line in the report. Three, because two
// papers in the same field citing the same thing is what a field is.
const citedOften = 3

// citedKey is what two references to the same work have in common: the
// identifier when the parser read one, and otherwise the title with
// everything a typesetter could have changed taken out of it.
func citedKey(e refs.Entry) string {
	switch {
	case e.DOI != "":
		return "doi:" + strings.ToLower(e.DOI)
	case e.ArXiv != "":
		return "arxiv:" + strings.ToLower(e.ArXiv)
	}
	var b strings.Builder
	for _, r := range strings.ToLower(e.Title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() < citedTitle {
		// A title too short to be sure of. Counting these would group every
		// reference the parser failed on under one key and report it as the
		// most cited work in the corpus.
		return ""
	}
	return "title:" + b.String()
}

// citedTitle is how many letters of a title are enough to tell one work from
// another. Twelve is about two words and is well under the shortest real
// title in the bibliographies here.
const citedTitle = 12

// citedName is a reference as a person would write it in a list of things to
// go and find.
func citedName(e refs.Entry) string {
	if e.Title != "" {
		return e.Title
	}
	return opening(e.Raw)
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
