package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/route"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/split"
	"github.com/tamnd/papers-reader/translate"
	"github.com/tamnd/papers-reader/work"
)

// Floor is the glossary coverage a language needs before a body is
// translated against it.
//
// Ninety per cent, and the reason it is not a hundred is that the last few
// terms of a glossary are the ones nobody can decide, and a corpus that
// cannot start until they are settled never starts. The reason it is not
// fifty is the arithmetic of the other direction: a body translated against
// half a glossary is a body that will have to be translated again once the
// other half lands, and the second run costs exactly what the first did.
const Floor = 90

func runTranslate(args []string) error {
	fs := flag.NewFlagSet("translate", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "translate these papers only, comma separated")
	field := fs.String("field", "", "translate one field only")
	all := fs.Bool("all", false, "translate every paper that has English content")
	langs := fs.String("lang", "", "which languages to write, comma separated, or every one of them")
	routes := fs.String("routes", "", "path to a routing table, instead of the one in the config directory")
	escalate := fs.String("escalate", "", "route names to send a refused chunk to, comma separated")
	tries := fs.Int("tries", translate.Tries, "how many times one chunk is asked before the file is given up on")
	floor := fs.Int("floor", Floor, "the glossary coverage a language needs, as a percentage")
	force := fs.Bool("force", false, "translate again even where the English has not changed")
	dry := fs.Bool("dry-run", false, "say what would be asked and ask nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers translate [flags]

Writes the Vietnamese, Chinese and Japanese of every English file, one
chunk at a time, and refuses an answer that did not come back as the same
document.

The mathematics, the listings, the citation markers and the tag attributes
are pulled out of the answer and compared with the source one at a time. An
answer whose spans differ anywhere is thrown away whole and the chunk is
asked again, because a translation with a quietly renamed variable is worse
than no translation: nothing further down the toolchain will catch it and a
reader has no way to know. A chunk that cannot be got right is a failure of
the whole file rather than a file with one English paragraph in it.

Every file records the English file it came from and the hash of that file
as it stood, so a run skips the files whose English has not changed and a
later pass can list exactly which translations are answers to a question
that has since moved. Use -force to translate one again anyway.

The glossary comes first. A language whose coverage is under the floor is
refused here rather than translated against a third of a vocabulary, which
is work that has to be done twice. Run papers glossary translate first.

The bibliography is copied through and not translated. Author names, the
titles of cited works and venue names stand as printed, which is rule L14,
and a references file has nothing else in it.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to translate: -id, -field or -all")
	}
	c, todo, err := chooseFrom(*root, *ids, *field)
	if err != nil {
		return err
	}
	want, err := languages(*langs)
	if err != nil {
		return err
	}
	g, err := glossary.Load(c.GlossaryManifest())
	if err != nil {
		return err
	}
	for _, l := range want {
		have, total := g.Coverage(l)
		if total == 0 {
			return fmt.Errorf("%s has no terms in it: build the glossary before anything is translated against it", c.GlossaryManifest())
		}
		if pct := have * 100 / total; pct < *floor {
			return fmt.Errorf("%s covers %d%% of the glossary and the floor is %d%%: run papers glossary translate -lang %s first",
				l.Name(), pct, *floor, l)
		}
	}

	jobs, err := plan(c, todo, want, *force)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("every translation is up to date with its English")
		return nil
	}
	chunks := 0
	for _, j := range jobs {
		chunks += j.chunks()
	}
	fmt.Printf("%d files to write, about %d chunks\n", len(jobs), chunks)
	if *dry {
		for _, j := range jobs {
			what := fmt.Sprintf("%d chunks", j.chunks())
			if j.copied() {
				what = "copied through, not translated"
			}
			fmt.Printf("  %s %s %s (%s)\n", j.lang, j.front.Paper, j.name, what)
		}
		fmt.Println("dry run, nothing asked and nothing written")
		return nil
	}

	logf := func(format string, args ...any) { fmt.Printf("    "+format+"\n", args...) }
	ask, from, err := fleet(*routes, *escalate, logf)
	if err != nil {
		return err
	}
	fmt.Printf("the chunks go to the hosts in the %s routing table\n", from)

	t := &translate.Translator{Ask: ask, Tries: *tries, Logf: logf}
	run := work.RunID()
	ctx := context.Background()
	var total llm.Usage
	written, asks, refused := 0, 0, 0
	for _, j := range jobs {
		res, err := translated(ctx, c, t, g, j, run)
		total = sum(total, res.Usage)
		asks += res.Asks
		refused += len(res.Refused)
		if err != nil {
			fmt.Printf("%d files written before the run stopped\n", written)
			return err
		}
		written++
		fmt.Printf("%s %s %s: %d chunks, %d asks, %s\n",
			j.lang, j.front.Paper, j.name, res.Chunks, res.Asks, strings.Join(res.Models, " and "))
	}
	fmt.Printf("%d files written, %d asks of which %d were refused, %d input and %d output tokens\n",
		written, asks, refused, total.InputTokens, total.OutputTokens)
	return nil
}

// A job is one English file to be written in one language.
type job struct {
	lang  corpus.Lang
	name  string
	front corpus.Front
	body  string
	// paper is the manifest entry, for the title the front matter of a
	// section file does not carry in full.
	paper corpus.Paper
	// abstract is the paper's own abstract, which goes into every chunk's
	// prompt as the context the chunker took away.
	abstract string
}

// copied says whether this file is put in the translated tree unchanged.
//
// A bibliography is. Author names, the titles of cited works and venue names
// stand as printed, which is rule L14, and a references file is nothing
// else. Asking a model to translate it and then checking that it did not is
// a way of paying for the same text four times.
func (j job) copied() bool { return j.front.Kind == "references" }

func (j job) chunks() int {
	if j.copied() {
		return 0
	}
	return len(translate.Chunks(j.body))
}

// plan reads the English side and works out what is owed, in paper then
// section then language order, which is the order somebody reading a
// half-finished corpus would want it done in.
func plan(c *corpus.Corpus, papers []corpus.Paper, langs []corpus.Lang, force bool) ([]job, error) {
	var out []job
	for _, p := range papers {
		dir := c.Content(corpus.EN, p.ID)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)

		abstract := ""
		var files []job
		for _, name := range names {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil, err
			}
			front, body, err := corpus.ParseFront(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", filepath.Join(dir, name), err)
			}
			if front.Kind == "front" {
				abstract = split.Abstract(string(body), abstractWords)
			}
			files = append(files, job{name: name, front: front, body: string(body), paper: p})
		}
		for _, f := range files {
			f.abstract = abstract
			for _, l := range langs {
				if !force && current(c, l, p.ID, f) {
					continue
				}
				f.lang = l
				out = append(out, f)
			}
		}
	}
	return out, nil
}

// abstractWords is how much of the abstract goes in the prompt.
//
// Two hundred and fifty, which is the whole abstract of nearly every paper
// in the corpus. It is the same number papers split uses for a restricted
// paper's stub, and the same number for the same reason: an abstract is
// about that long and one cut shorter stops mid-argument.
const abstractWords = 250

// current says whether a translation is already an answer to the English as
// it stands.
//
// A file with no source hash is not current whatever its contents, because
// nothing in it can say what it was made from. A file somebody edited by
// hand is left alone: the edit is the version of record and a run that
// overwrote it would throw away the review it came from.
func current(c *corpus.Corpus, l corpus.Lang, id string, f job) bool {
	b, err := os.ReadFile(filepath.Join(c.Content(l, id), f.name))
	if err != nil {
		return false
	}
	front, _, err := corpus.ParseFront(b)
	if err != nil {
		return false
	}
	if front.Edited {
		return true
	}
	was := f.front.ContentSHA256
	if was == "" {
		was = corpus.ContentSHA([]byte(f.body))
	}
	return front.SourceContentSHA256 == was
}

// translated translates one file and puts it on disk.
func translated(ctx context.Context, c *corpus.Corpus, t *translate.Translator, g *glossary.Glossary, j job, run string) (translate.Result, error) {
	var res translate.Result
	body := j.body
	if !j.copied() {
		var err error
		res, err = t.Body(ctx, paperOf(j), j.lang, terms(g, j.front.Field, j.lang), j.body)
		if err != nil {
			return res, err
		}
		body = res.Text
	}

	front := j.front
	front.Lang = j.lang
	front.Edited = false
	front.TranslatedFrom = filepath.Join("content", string(corpus.EN), j.front.Paper, j.name)
	front.SourceContentSHA256 = j.front.ContentSHA256
	if front.SourceContentSHA256 == "" {
		front.SourceContentSHA256 = corpus.ContentSHA([]byte(j.body))
	}
	front.TranslationModel = strings.Join(res.Models, ", ")
	front.TranslationRun = run
	front.GlossaryVersion = g.Version
	front.GlossaryTermsSHA256 = glossary.TermsSHA(g, j.front.Field, j.lang)
	front.ContentSHA256 = corpus.ContentSHA([]byte(body))
	p, err := prompt.Get(prompt.Translate)
	if err != nil {
		return res, err
	}
	front.PromptSHA256 = p.SHA

	out, err := corpus.Render(front, []byte(body))
	if err != nil {
		return res, err
	}
	dir := c.Content(j.lang, j.front.Paper)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	return res, os.WriteFile(filepath.Join(dir, j.name), out, 0o644)
}

func paperOf(j job) translate.Paper {
	note, _ := prompt.Note(j.front.Paper)
	return translate.Paper{
		ID:       j.front.Paper,
		Title:    j.front.Title,
		Field:    j.front.Field,
		Abstract: j.abstract,
		Note:     note,
	}
}

// terms is the glossary a paper in one field gets, in the language being
// written, with the terms that have no rendering yet left out.
func terms(g *glossary.Glossary, f corpus.Field, l corpus.Lang) []translate.Term {
	var out []translate.Term
	for _, t := range g.For(f) {
		as, ok := t.Rendering(l)
		if !ok {
			continue
		}
		out = append(out, translate.Term{En: t.En, As: as})
	}
	return out
}

// fleet builds the asker, and the second one a refused chunk escalates to.
//
// The escalation is the cheap-model-first policy and it lives here because
// this is what knows about routes. The first attempt goes to the pool in its
// own order, which is cheapest first; a chunk that comes back refused is
// asked again on the named routes only. Without -escalate the second attempt
// is the same pool, which is still worth having: a refusal is often the host
// having a bad minute rather than the model being unable to do the chunk.
func fleet(routes, names string, logf func(string, ...any)) (func(context.Context, string, llm.Request, int) (translate.Reply, error), string, error) {
	first, from, err := work.Fleet(work.Translate, routes, false, logf)
	if err != nil {
		return nil, from, err
	}
	second := first
	if strings.TrimSpace(names) != "" {
		registry, _, err := work.Routes(routes)
		if err != nil {
			return nil, from, err
		}
		only, err := registry.Select(commas(names))
		if err != nil {
			return nil, from, err
		}
		pool := work.Pool(only)
		if pool.Empty() {
			return nil, from, route.ErrNoRoutes()
		}
		up := *first
		up.Waiter = &work.Waiter{Pool: pool, Logf: logf}
		second = &up
	}
	return func(ctx context.Context, target string, req llm.Request, attempt int) (translate.Reply, error) {
		asker := first
		if attempt > 1 {
			asker = second
		}
		answer, err := asker.Do(ctx, target, req)
		return translate.Reply{Response: answer.Response, Model: answer.Model, Route: answer.Route}, err
	}, from, nil
}

// commas reads a comma separated flag.
func commas(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sum(a, b llm.Usage) llm.Usage {
	a.InputTokens += b.InputTokens
	a.CachedInputTokens += b.CachedInputTokens
	a.OutputTokens += b.OutputTokens
	a.ReasoningTokens += b.ReasoningTokens
	a.TotalTokens += b.TotalTokens
	return a
}
