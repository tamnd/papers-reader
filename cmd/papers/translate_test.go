package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/roundtrip"
	"github.com/tamnd/papers-reader/translate"
)

// translateCorpus writes the smallest corpus papers translate will look at:
// one paper, a front file with an abstract in it, a section and a
// bibliography. Nothing here is text from a paper.
func translateCorpus(t *testing.T) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/papers.yaml", `papers:
  - id: a-1970-paper
    title: A Paper
    authors: [A. Author]
    year: 1970
    venue: A Journal
    field: theory
    status: listed
`)
	write("content/en/a-1970-paper/00_front.md", file("front", "Abstract\n\nThis paper is about a thing."))
	write("content/en/a-1970-paper/01_first.md", file("section", "The first paragraph."))
	write("content/en/a-1970-paper/02_references.md", file("references", "[1] A. Author. A Paper. A Journal, 1970."))

	c, err := corpus.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// file is one English content file, with the body's hash stamped in the way
// the splitter stamps it, so that the staleness check has something true to
// compare against.
func file(kind, body string) string {
	front := corpus.Front{
		Paper:         "a-1970-paper",
		Title:         "A Paper",
		Field:         corpus.Theory,
		Kind:          kind,
		Lang:          corpus.EN,
		ContentSHA256: corpus.ContentSHA([]byte(body)),
	}
	b, err := corpus.Render(front, []byte(body))
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestEveryEnglishFileIsPlannedForEveryLanguage(t *testing.T) {
	c := translateCorpus(t)
	papers, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI, corpus.ZH}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("%d jobs for three files in two languages", len(jobs))
	}
	for _, j := range jobs {
		if j.abstract == "" {
			t.Errorf("%s went up without the paper's abstract", j.name)
		}
	}
}

func TestTheBibliographyIsCopiedAndNotAsked(t *testing.T) {
	// Author names, the titles of cited works and venue names stand as
	// printed, which is rule L14, and a references file is nothing else.
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.front.Kind != "references" {
			continue
		}
		found = true
		if !j.copied() || j.chunks() != 0 {
			t.Errorf("the bibliography is planned as %d chunks", j.chunks())
		}
	}
	if !found {
		t.Error("the bibliography was left out of the plan altogether")
	}
}

func TestAFileWhoseEnglishHasNotMovedIsNotAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	// A translation that records the hash of the English as it stands.
	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        translatePrompt(t),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.name == "01_first.md" {
			t.Error("a translation that is already an answer to its English was planned again")
		}
	}
	again, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Errorf("-force planned %d of 3 files", len(again))
	}
}

// The prompt is half of what produced the file. A rule added to it is a rule
// the answers on disk were never held to, and the answer to that is to ask
// again, not to leave a corpus where some files followed the rule and some
// did not and nothing on disk says which.
func TestATranslationMadeWithAnOlderPromptIsAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        "0000000000000000000000000000000000000000000000000000000000000000",
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.name == "01_first.md" {
			found = true
		}
	}
	if !found {
		t.Error("a translation written to an older prompt was left as it was")
	}
}

// The glossary is the other half of what produced a file, and the terms
// hash was written into every translated file for months before anything
// read it. Editing a rendering left the pages made from the old one on disk
// and current.
func TestATranslationMadeAgainstARenderingThatMovedIsAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}

	// The paper is in theory, so the databases term is not offered to it
	// and the reduction is.
	before := &glossary.Glossary{Version: 1, Terms: []glossary.Term{
		{En: "reduction", Vi: "phép rút gọn"},
		{En: "commit", Vi: "xác nhận", Fields: []corpus.Field{corpus.Databases}},
	}}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        translatePrompt(t),
		GlossaryVersion:     1,
		GlossaryTermsSHA256: glossary.TermsSHA(before, corpus.Theory, corpus.VI),
	}, "Đoạn thứ nhất.")

	planned := func(g *glossary.Glossary) bool {
		t.Helper()
		jobs, err := plan(c, g, papers.Papers, []corpus.Lang{corpus.VI}, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range jobs {
			if j.name == "01_first.md" {
				return true
			}
		}
		return false
	}

	if planned(before) {
		t.Error("a page translated against this very glossary was planned again")
	}

	// A version bump that added a term the paper is not offered has not
	// moved anything under it.
	elsewhere := &glossary.Glossary{Version: 2, Terms: append(append([]glossary.Term{},
		before.Terms...), glossary.Term{En: "schema", Vi: "lược đồ", Fields: []corpus.Field{corpus.Databases}})}
	if planned(elsewhere) {
		t.Error("a databases term was added and a theory paper was queued for it")
	}

	// A rendering this paper was offered has changed, and the page is an
	// answer to a question that has moved.
	moved := &glossary.Glossary{Version: 2, Terms: []glossary.Term{
		{En: "reduction", Vi: "phép quy dẫn"},
		{En: "commit", Vi: "xác nhận", Fields: []corpus.Field{corpus.Databases}},
	}}
	if !planned(moved) {
		t.Error("a rendering the page was translated against changed and the page was left alone")
	}
}

// translatePrompt is the hash of the prompt this build carries, which is what
// a Vietnamese file has to record to count as current. The language rules
// are part of it, which is why it takes a language.
func translatePrompt(t *testing.T) string {
	t.Helper()
	sha, err := prompt.TranslationSHA(corpus.VI)
	if err != nil {
		t.Fatal(err)
	}
	return sha
}

func TestATranslationOfAnEnglishFileThatMovedIsPlannedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: corpus.ContentSHA([]byte("something else entirely")),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Errorf("%d of 3 files planned, and one of them is stale", len(jobs))
	}
}

func TestAHandEditedTranslationIsLeftAlone(t *testing.T) {
	// The edit is the version of record. A run that overwrote it would throw
	// away the review it came out of and leave no trace of having done so.
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: corpus.ContentSHA([]byte("something else entirely")),
		Edited:              true,
	}, "Đoạn thứ nhất, sửa bằng tay.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.name == "01_first.md" {
			t.Error("a hand edited translation was planned for overwriting")
		}
	}
}

// The back translation check is the only one that reads what a page means,
// and the verdict it writes on the page is the only kind of staleness that
// is about meaning rather than about what produced the file. Every hash
// matches here and the page is owed again anyway.
func TestAPageTheBackTranslationDisagreedWithIsAskedAgain(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        translatePrompt(t),
		Roundtrip:           string(roundtrip.Material),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.name == "01_first.md" {
			found = true
		}
	}
	if !found {
		t.Error("a page the judge said differs materially was left as it was")
	}
}

// A verdict that is not the bad one leaves the page alone. Differing in
// wording is what a literal back-translation of a good translation looks
// like, and a check that re-queued those would re-queue the corpus.
func TestAPageTheBackTranslationOnlyQuibbledWithIsLeftAlone(t *testing.T) {
	c := translateCorpus(t)
	papers, _ := c.LoadPapers()

	english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, "a-1970-paper"), "01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := corpus.ParseFront(english)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c, "01_first.md", corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Kind: "section", Lang: corpus.VI,
		SourceContentSHA256: was.ContentSHA256,
		PromptSHA256:        translatePrompt(t),
		Roundtrip:           string(roundtrip.Wording),
	}, "Đoạn thứ nhất.")

	jobs, err := plan(c, nil, papers.Papers, []corpus.Lang{corpus.VI}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.name == "01_first.md" {
			t.Error("a page the judge was content with was queued again")
		}
	}
}

// The two marks are about the weakest link and not the average one. A file
// is only as trustworthy as the least trustworthy question that went into
// it, and the whole reason these fields exist is so a page nobody looked at
// as hard cannot hide behind eleven pages that somebody did.
func TestOneWeakChunkMarksTheWholeFile(t *testing.T) {
	free := map[string]bool{"opencode": true}

	for _, c := range []struct {
		name    string
		res     translate.Result
		small   bool
		gateway bool
	}{
		{
			name: "the big model on a paid host",
			res:  translate.Result{Models: []string{"gpt-5", "gpt-5"}, Routes: []string{"server3", "server2"}},
		},
		{
			name:  "one chunk out of twelve from a mini",
			res:   translate.Result{Models: []string{"gpt-5", "gpt-5-mini"}, Routes: []string{"server3", "server3"}},
			small: true,
		},
		{
			name:    "one chunk through a free gateway",
			res:     translate.Result{Models: []string{"gpt-5", "gpt-5"}, Routes: []string{"server3", "opencode"}},
			gateway: true,
		},
		{
			name: "a file that was copied rather than asked for",
			res:  translate.Result{},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			small, gateway := provisional(c.res, free)
			if small != c.small || gateway != c.gateway {
				t.Errorf("small=%v gateway=%v, want small=%v gateway=%v", small, gateway, c.small, c.gateway)
			}
		})
	}
}

// put writes one translated file into the corpus.
func put(t *testing.T, c *corpus.Corpus, name string, front corpus.Front, body string) {
	t.Helper()
	front.ContentSHA256 = corpus.ContentSHA([]byte(body))
	b, err := corpus.Render(front, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	dir := c.Content(front.Lang, front.Paper)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A chunk takes the fleet about two minutes and the corpus is five hundred
// of them, so the run has to use every lane the fleet has rather than one.
func TestSpreadUsesEveryLane(t *testing.T) {
	const lanes = 4
	var mu sync.Mutex
	at, most := 0, 0
	start := make(chan struct{})
	var once sync.Once
	work := func(j job) (translate.Result, error) {
		mu.Lock()
		at++
		if at > most {
			most = at
		}
		full := at == lanes
		mu.Unlock()
		if full {
			// The last of the four is what lets the other three go, so
			// this only finishes if all four really are in the air.
			once.Do(func() { close(start) })
		}
		<-start
		mu.Lock()
		at--
		mu.Unlock()
		return translate.Result{Chunks: 1}, nil
	}
	jobs := make([]job, 12)
	for i := range jobs {
		jobs[i] = job{lang: corpus.VI, name: fmt.Sprintf("%02d_section.md", i)}
	}
	done := 0
	for o := range spread(context.Background(), lanes, jobs, work) {
		if o.err != nil {
			t.Errorf("%s: %v", o.job.name, o.err)
		}
		done++
	}
	if done != len(jobs) {
		t.Errorf("%d files came back, want %d", done, len(jobs))
	}
	if most != lanes {
		t.Errorf("%d files were in the air at once, want %d", most, lanes)
	}
}

// The run stops on a fleet that is down, and stopping must not hang the
// caller on a channel that never closes.
func TestSpreadStopsOnACancelledRun(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var mu sync.Mutex
	asked := 0
	work := func(j job) (translate.Result, error) {
		mu.Lock()
		asked++
		n := asked
		mu.Unlock()
		if n == 1 {
			stop()
		}
		return translate.Result{}, ctx.Err()
	}
	jobs := make([]job, 200)
	for i := range jobs {
		jobs[i] = job{lang: corpus.VI, name: fmt.Sprintf("%03d_section.md", i)}
	}
	done := 0
	for range spread(ctx, 2, jobs, work) {
		done++
	}
	if done == len(jobs) {
		t.Errorf("all %d files were handed out after the run was stopped", done)
	}
}

// One lane is one lane, and a routing table with nothing live in it must not
// ask for zero workers and hand out nothing at all.
func TestSpreadAlwaysHasALane(t *testing.T) {
	jobs := []job{{lang: corpus.VI, name: "00_front.md"}}
	done := 0
	for range spread(context.Background(), 0, jobs, func(job) (translate.Result, error) {
		return translate.Result{}, nil
	}) {
		done++
	}
	if done != 1 {
		t.Errorf("%d files came back, want 1", done)
	}
}

// planned is the jobs of a corpus in every language, which is what -redo
// narrows down.
func planned(t *testing.T, c *corpus.Corpus, langs ...corpus.Lang) []job {
	t.Helper()
	papers, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := plan(c, nil, papers.Papers, langs, true)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

func TestRedoKeepsOnlyTheFilesTheAuditRefuses(t *testing.T) {
	c := translateCorpus(t)
	jobs := planned(t, c, corpus.VI, corpus.ZH)
	got := refusedOnly([]audit.Finding{{
		Rule:    "L07",
		File:    "content/vi/a-1970-paper/01_first.md",
		Message: "paragraph 3 is the English paragraph, word for word",
	}}, jobs)
	if len(got) != 1 {
		t.Fatalf("%d of %d jobs kept for one finding, want 1", len(got), len(jobs))
	}
	if got[0].lang != corpus.VI || got[0].name != "01_first.md" {
		t.Errorf("the job kept is %s %s, want vi 01_first.md", got[0].lang, got[0].name)
	}
}

// Two rules on one file is one job. A file asked for twice is a file
// translated twice and the second answer overwrites the first.
func TestTwoFindingsOnOneFileAreOneJob(t *testing.T) {
	c := translateCorpus(t)
	got := refusedOnly([]audit.Finding{
		{Rule: "L07", File: "content/vi/a-1970-paper/01_first.md"},
		{Rule: "L10", File: "content/vi/a-1970-paper/01_first.md"},
	}, planned(t, c, corpus.VI))
	if len(got) != 1 {
		t.Errorf("%d jobs for two findings on one file, want 1", len(got))
	}
}

// A finding that names no translation keeps nothing. There is no file to
// ask for again in any of these.
func TestRedoIgnoresAFindingThatIsNotATranslation(t *testing.T) {
	c := translateCorpus(t)
	jobs := planned(t, c, corpus.VI)
	for _, f := range []audit.Finding{
		{Rule: "S03", Message: "a pdf is in the index"},
		{Rule: "M02", File: "manifests/papers.yaml"},
		{Rule: "T01", File: "content/en/a-1970-paper/01_first.md"},
		{Rule: "F06", File: "figures/a-1970-paper/fig-1.png"},
		{Rule: "L07", File: "content/vi/b-1980-paper/01_first.md"},
	} {
		if got := refusedOnly([]audit.Finding{f}, jobs); len(got) != 0 {
			t.Errorf("%s on %q kept %d jobs, want 0", f.Rule, f.File, len(got))
		}
	}
}

// judged writes a translated file carrying the verdict the back
// translation left on it, which is where -material reads from.
func judged(t *testing.T, c *corpus.Corpus, l corpus.Lang, name, verdict string) {
	t.Helper()
	body := []byte("Đoạn thứ nhất.")
	b, err := corpus.Render(corpus.Front{
		Paper: "a-1970-paper", Title: "A Paper", Field: corpus.Theory,
		Kind: "section", Lang: l, ContentSHA256: corpus.ContentSHA(body),
		Roundtrip: verdict, RoundtripRun: "20260101T000000Z",
	}, body)
	if err != nil {
		t.Fatal(err)
	}
	dir := c.Content(l, "a-1970-paper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMaterialKeepsOnlyThePagesTheBackTranslationFailed(t *testing.T) {
	c := translateCorpus(t)
	judged(t, c, corpus.VI, "01_first.md", "differs-materially")
	judged(t, c, corpus.VI, "00_front.md", "differs-in-wording")
	judged(t, c, corpus.ZH, "01_first.md", "the-same")
	got, err := judgedMaterial(c, planned(t, c, corpus.VI, corpus.ZH))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d jobs kept, want 1: %v", len(got), got)
	}
	if got[0].lang != corpus.VI || got[0].name != "01_first.md" {
		t.Errorf("the job kept is %s %s, want vi 01_first.md", got[0].lang, got[0].name)
	}
}

// A page nothing has looked at is not a page anything is wrong with. Most
// of the corpus is never sampled and asking for all of it again would be
// the whole run under another name.
func TestMaterialKeepsNothingWhereTheCheckHasNotRun(t *testing.T) {
	c := translateCorpus(t)
	judged(t, c, corpus.VI, "01_first.md", "")
	got, err := judgedMaterial(c, planned(t, c, corpus.VI))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("%d jobs kept for a corpus with no verdicts in it, want 0", len(got))
	}
}

func TestContentPathReadsAPathUnderContent(t *testing.T) {
	for _, c := range []struct {
		path     string
		lang     corpus.Lang
		id, name string
		ok       bool
	}{
		{"content/vi/a-1970-paper/01_first.md", corpus.VI, "a-1970-paper", "01_first.md", true},
		{"content/en/a-1970-paper/01_first.md", corpus.EN, "a-1970-paper", "01_first.md", true},
		{"content/vi/a-1970-paper", "", "", "", false},
		{"manifests/papers.yaml", "", "", "", false},
		{"", "", "", "", false},
	} {
		l, id, name, ok := contentPath(c.path)
		if ok != c.ok || l != c.lang || id != c.id || name != c.name {
			t.Errorf("contentPath(%q) is %q %q %q %v, want %q %q %q %v",
				c.path, l, id, name, ok, c.lang, c.id, c.name, c.ok)
		}
	}
}

// Which rules -redo acts on. Every hard rule, because a file one of them
// refuses never ships until it is asked for again, and L19 on its own out
// of the soft ones, because a section title left in English cannot be
// repaired any other way.
func TestWhichRulesRedoActsOn(t *testing.T) {
	soft := 0
	for _, r := range audit.Rules() {
		want := r.Hard || r.ID == "L19"
		if got := reask(r); got != want {
			t.Errorf("reask(%s) is %v, want %v", r.ID, got, want)
		}
		if !r.Hard && !reask(r) {
			soft++
		}
	}
	if soft == 0 {
		t.Error("every soft rule is asked again, and that is not the bargain")
	}
}

// A run stops when the fleet is down and carries on when a page is refused,
// and the two look the same from the outside: a file that was not written.
// The Vietnamese pass over the corpus stopped after 86 files of 862 because
// three theory papers in a row lost a chunk to their mathematics while the
// fleet was up the whole time.
func TestARefusedPageIsNotEvidenceTheFleetIsDown(t *testing.T) {
	refused := &translate.Refusal{Target: "razborov-1997-naturalproofs vi chunk 1 of 3", Tries: 5, Worst: "span 1"}
	for _, c := range []struct {
		name string
		err  error
		want bool
	}{
		{"a page every answer was refused for", refused, false},
		{"the same, reported by the file it failed", fmt.Errorf("04_inherent_limitations.md: %w", refused), false},
		{"no host would answer", errors.New("3 hosts were asked and none of them answered"), true},
		{"the corpus could not be written to", errors.New("open content/vi: permission denied"), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := downed(c.err); got != c.want {
				t.Errorf("downed(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}
