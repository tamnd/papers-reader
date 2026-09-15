package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/route"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/publish"
	"github.com/tamnd/papers-reader/roundtrip"
	"github.com/tamnd/papers-reader/split"
	"github.com/tamnd/papers-reader/translate"
	"github.com/tamnd/papers-reader/work"
)

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
	floor := fs.Int("floor", glossary.Floor, "the glossary coverage a language needs, as a percentage")
	force := fs.Bool("force", false, "translate again even where the English has not changed")
	redo := fs.Bool("redo", false, "translate again the files a hard audit rule refuses, and nothing else")
	material := fs.Bool("material", false, "translate again the files the back translation calls materially different, and nothing else")
	atOnce := fs.Int("jobs", 0, "how many files to translate at once, or 0 for one per lane in the fleet")
	every := fs.Int("publish", 0, "push the papers the run has finished to the corpus and merge them, once so many files are ready")
	dry := fs.Bool("dry-run", false, "say what would be asked and ask nothing")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: papers translate [flags]

Writes the Vietnamese, Chinese and Japanese of every English file and
refuses an answer that did not come back as the same document.

Several files at once, one per lane the fleet has. A chunk takes the fleet
about two minutes and the corpus is five hundred of them, so a run that
asked one question at a time would take a day and leave eight of the nine
lanes idle for all of it. The chunks of one file still go one at a time,
because a chunk is given the paragraph before it for context.

The mathematics, the listings, the citation markers and the tag attributes
are pulled out of the answer and compared with the source paragraph by
paragraph. An answer that has lost one, gained one or altered one is thrown
away whole and the chunk is asked again, because a translation with a
quietly renamed variable is worse than no translation: nothing further down
the toolchain will catch it and a reader has no way to know. Inside a
paragraph the spans may move, because Chinese and Japanese put a modifier in
front of what it modifies and two formulas in one English clause come out
the other way round. A chunk that cannot be got right is a failure of
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

With -publish the run pushes as it goes, and it pushes whole papers only. A
paper goes out when every English file of it has a counterpart in every
language being written, so the paper the run is halfway through stays in the
working tree until it is finished. A paper with a section that could not be
written is held back entire and reported at the end, and the next run is
owed the one section it is missing.

A batch also carries nothing a hard audit rule refuses, and a paper one of
them names stays in the working tree. Use -redo to ask again for exactly
those files: the run audits the corpus, keeps the files the hard rules are
unhappy with and translates those and nothing else. Editing one by hand is
not the repair. The front matter carries the hash of the English the
translation answers, so an edit either breaks that hash or lies about what
produced the text.

An audit rule can see a formula go missing and cannot see a meaning change.
That is what papers roundtrip is for, and -material asks again for exactly
the pages it came away from believing something the paper does not say.

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

	jobs, err := plan(c, g, todo, want, *force || *redo || *material)
	if err != nil {
		return err
	}
	if *redo {
		found, err := hardFindings(c)()
		if err != nil {
			return err
		}
		if jobs = refusedOnly(found, jobs); len(jobs) == 0 {
			fmt.Println("no hard audit rule refuses a translation of these papers")
			return nil
		}
	}
	if *material {
		jobs, err = judgedMaterial(c, jobs)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("the back translation calls no translation of these papers materially different")
			return nil
		}
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
	ask, from, lanes, err := fleet(*routes, *escalate, logf)
	if err != nil {
		return err
	}
	if *atOnce > 0 {
		lanes = *atOnce
	}
	fmt.Printf("the chunks go to the hosts in the %s routing table, %d files at once\n", from, lanes)
	free, err := gateways(*routes)
	if err != nil {
		return err
	}

	t := &translate.Translator{Ask: ask, Tries: *tries, Logf: logf, Keep: keeper(c)}
	run := work.RunID()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var total llm.Usage
	written, asks, refused, skipped := 0, 0, 0, 0
	var failed []string
	inARow, stopping, batch := 0, false, 0
	ship := newShipment(c, want, jobs)
	ship.findings = hardFindings(c)
	for o := range spread(ctx, lanes, jobs, func(j job) (translate.Result, error) {
		return translated(ctx, c, t, g, j, run, free)
	}) {
		total = sum(total, o.res.Usage)
		asks += o.res.Asks
		refused += len(o.res.Refused)
		if o.err != nil {
			ship.done(o.job.front.Paper, false)
			// A file the cancellation killed is not a file that failed.
			// Saying so would report nine failures for one bad fleet and
			// hide the three that are the actual evidence.
			if stopping {
				skipped++
				continue
			}
			failed = append(failed, fmt.Sprintf("%s %s %s: %v", o.job.lang, o.job.front.Paper, o.job.name, o.err))
			fmt.Printf("%s %s %s: given up on, %v\n", o.job.lang, o.job.front.Paper, o.job.name, o.err)
			inARow++
			if inARow >= giveUp {
				fmt.Printf("%d files in a row could not be written, so the rest of the run is skipped\n", inARow)
				stopping = true
				stop()
			}
			continue
		}
		inARow = 0
		written++
		fmt.Printf("%s %s %s: %d chunks, %d asks%s, %s\n",
			o.job.lang, o.job.front.Paper, o.job.name, o.res.Chunks, o.res.Asks, kept(o.res), strings.Join(o.res.Models, " and "))
		ship.done(o.job.front.Paper, true)
		if *every > 0 && ship.pending() >= *every {
			batch++
			pushed(c, run, batch, false, ship.take())
		}
	}
	if *every > 0 {
		pushed(c, run, batch+1, true, ship.take())
	}
	if len(ship.held) > 0 {
		fmt.Printf("%d papers were left in the working tree rather than published:\n", len(ship.held))
		for _, id := range ship.held {
			fmt.Printf("  %s: %s\n", id, ship.why[id])
		}
	}
	fmt.Printf("%d files written, %d asks of which %d were refused, %d input and %d output tokens\n",
		written, asks, refused, total.InputTokens, total.OutputTokens)
	if skipped > 0 {
		fmt.Printf("%d files were still in the air when the run stopped and were left unwritten\n", skipped)
	}
	if len(failed) > 0 {
		fmt.Printf("%d files were not written:\n", len(failed))
		for _, f := range failed {
			fmt.Printf("  %s\n", f)
		}
		return fmt.Errorf("%d of %d files were not written", len(failed), len(jobs))
	}
	return nil
}

// pushed opens a pull request for what the run has written so far, merges
// it, and carries on.
//
// It never stops the run. A push fails because the network dropped or
// because github was busy, and neither of those is a reason to throw away
// the six hours of model time that is still in the queue. What was not
// pushed this time is picked up by the next batch, because a batch is
// whatever is in the working tree and not a list kept in memory.
//
// The opening says which run and whether it is over, because a person
// looking at a merged pull request three days later wants to know whether
// there is more coming and whether anybody has read it yet.
//
// paths is the paper directories the shipment says are whole, and is empty
// when nothing has finished, which is a batch that does not happen rather
// than a batch that pushes everything in the tree.
func pushed(c *corpus.Corpus, run string, batch int, last bool, paths []string) {
	if len(paths) == 0 {
		return
	}
	opening := fmt.Sprintf("This is translation run %s pushing out what it has finished so far.\n", run)
	if last {
		opening += "That is the run finished, so this is the last batch of it.\n"
	} else {
		opening += "The run is still going and the next batch will look like this one.\n"
	}
	opening += "Every paper here has been through the hard audit rules and passed them. A paper one of them refuses is held back in the working tree and named in the run log, so what is in this batch is what is ready rather than what happens to be finished.\n"
	res, err := push(context.Background(), c, "main", publish.Branch(run, batch), opening, paths, true, false)
	switch {
	case errors.Is(err, publish.Nothing):
		return
	case err != nil:
		fmt.Printf("batch %d could not be pushed, and the run carries on: %v\n", batch, err)
		return
	}
	fmt.Printf("batch %d: %d files in %s\n", batch, res.Files, res.URL)
}

// An outcome is one finished file: what was asked for, what came back, and
// whether it came back at all.
type outcome struct {
	job job
	res translate.Result
	err error
}

// spread runs write over the jobs, lanes of them at a time, and sends each
// outcome on as it finishes.
//
// Order is completion order and not the order of the list, which is the one
// thing a reader of the log has to know. A run of five hundred files prints
// them interleaved across the three hosts, and the paper a line is about is
// on the line.
//
// A cancelled context stops the run twice over: no further job is handed
// out, and the jobs already in the air fail on the next thing they ask the
// fleet. The channel still closes, because every worker returns and the
// collector is what closes it, so the caller's range ends rather than
// deadlocking on a run it just stopped.
func spread(ctx context.Context, lanes int, jobs []job, write func(job) (translate.Result, error)) <-chan outcome {
	if lanes < 1 {
		lanes = 1
	}
	queue := make(chan job)
	done := make(chan outcome)
	var wg sync.WaitGroup
	for range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				res, err := write(j)
				done <- outcome{job: j, res: res, err: err}
			}
		}()
	}
	go func() {
		defer close(queue)
		for _, j := range jobs {
			select {
			case queue <- j:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
	return done
}

// giveUp is how many files in a row may fail before the run stops.
//
// A file the model will not write correctly is not a reason to abandon the
// other three hundred, and it used to be. The first Japanese of the GAN
// paper stopped four files short because one section came back with a space
// inside a pair of dollar signs six times running, and the nine files after
// it in the queue were never asked for. A page that cannot be written is
// reported at the end and the run carries on.
//
// The count is here because the other thing that makes a file fail is the
// fleet being down, and then every file fails, slowly, and a run left alone
// overnight spends a subscription on nothing. Three in a row is a fleet and
// not a page.
const giveUp = 3

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
func plan(c *corpus.Corpus, g *glossary.Glossary, papers []corpus.Paper, langs []corpus.Lang, force bool) ([]job, error) {
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
				if !force && current(c, g, l, p.ID, f) {
					continue
				}
				f.lang = l
				out = append(out, f)
			}
		}
	}
	return out, nil
}

// refusedOnly keeps the jobs whose file a hard audit rule refuses.
//
// The run holds back a paper an audit rule names rather than publishing it,
// which is the right answer and leaves the paper sitting in the working tree
// with nobody to fix it. Fixing one by hand is not on: the front matter
// carries the hash of the English the translation answers, so an edit either
// breaks the hash or is a lie about what produced the text. The honest repair
// is to ask again, and until now asking again meant deleting the file by hand
// and remembering which ones they were.
//
// So the audit says which. It knows already, it is the thing that refused
// them, and a rule that can name a file can name a job. A run with -redo
// plans every file of the papers it was given and then keeps the handful the
// rules are unhappy with, which over the three papers that were red on
// tamnd/papers main was five files out of twenty five.
//
// Hard rules only, because a soft rule is a report worth reading and not a
// reason to spend four more questions on a page. A finding that names no
// file, or names an English file, or names something that is not a section
// of a paper, keeps nothing: there is no translation in it to ask for again.
func refusedOnly(found []audit.Finding, jobs []job) []job {
	bad := map[string]bool{}
	for _, f := range found {
		if l, id, name, ok := contentPath(f.File); ok && l != corpus.EN {
			bad[string(l)+"/"+id+"/"+name] = true
		}
	}
	var out []job
	for _, j := range jobs {
		if bad[string(j.lang)+"/"+j.front.Paper+"/"+j.name] {
			out = append(out, j)
		}
	}
	return out
}

// judgedMaterial keeps the jobs whose translation the back translation check
// came away from believing something the paper does not say.
//
// The same shape as refusedOnly and for the same reason, but the other half
// of the quality gate. An audit rule reads the file and can say a formula
// went missing. It cannot say that "much of the previous work" came back as
// "most of the previous work", which is what the back translation is for and
// what the L rules have no way to see.
//
// The verdict is read off the translated file's own front matter rather than
// out of reports/roundtrip.md. The report is prose for a person, one heading
// per page and the two texts under it, and a run that had to parse its own
// prose back would be a run that breaks when the headings are reworded. The
// front matter is where the check wrote the verdict down for a machine.
//
// A file with no verdict in it was never sampled and is not a file anything
// is known to be wrong with, so it keeps nothing.
func judgedMaterial(c *corpus.Corpus, jobs []job) ([]job, error) {
	var out []job
	for _, j := range jobs {
		b, err := os.ReadFile(filepath.Join(c.Content(j.lang, j.front.Paper), j.name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		front, _, err := corpus.ParseFront(b)
		if err != nil {
			return nil, err
		}
		if front.Roundtrip == string(roundtrip.Material) {
			out = append(out, j)
		}
	}
	return out, nil
}

// contentPath pulls the language, the paper and the file name out of a path
// under content, and says whether the path was one.
func contentPath(path string) (corpus.Lang, string, string, bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) != 4 || parts[0] != "content" {
		return "", "", "", false
	}
	return corpus.Lang(parts[1]), parts[2], parts[3], true
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
//
// The prompt counts as much as the English does. A page carries the hash of
// the prompt that produced it so that a change to the rules is visible as a
// corpus half written under each, and the way to make that visible is to
// owe the page again. It is not free and it is not meant to be: an edit to
// the translation prompt puts every translated page in the queue, which is
// the cost of the rules being one set of rules rather than whichever set
// happened to be in the build that ran.
func current(c *corpus.Corpus, g *glossary.Glossary, l corpus.Lang, id string, f job) bool {
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
	// A page the back translation check read and disagreed with is owed
	// again, whatever its hashes say. It is the one kind of staleness that
	// is about what the page means rather than about what produced it.
	if front.Roundtrip == string(roundtrip.Material) {
		return false
	}
	if sha, err := prompt.TranslationSHA(l); err == nil && front.PromptSHA256 != sha {
		return false
	}
	// A page translated against a rendering that has since changed is owed
	// again. The hash and not the version, which is the whole reason the
	// hash is written down: a version bump that added a databases term has
	// not moved anything under a paper about generative models, and queuing
	// the corpus every time the glossary is edited is how a corpus of four
	// hundred translated files never gets finished.
	if g != nil && front.GlossaryTermsSHA256 != glossary.TermsSHA(g, f.front.Field, l) {
		return false
	}
	was := f.front.ContentSHA256
	if was == "" {
		was = corpus.ContentSHA([]byte(f.body))
	}
	return front.SourceContentSHA256 == was
}

// translated translates one file and puts it on disk.
//
// free is the names of the routes that are gateways, for the provisional
// marks in the front matter.
func translated(ctx context.Context, c *corpus.Corpus, t *translate.Translator, g *glossary.Glossary, j job, run string, free map[string]bool) (translate.Result, error) {
	var res translate.Result
	front := j.front
	body := j.body
	if !j.copied() {
		// The section's own title goes into the passage as a heading and
		// comes back out of the answer. It is not in the body: papers split
		// lifted it into the front matter, so a translated file used to
		// carry an English section_title, and the table of contents of the
		// Vietnamese book was eight English headings over Vietnamese prose.
		//
		// As part of the body rather than as an ask of its own, because a
		// title of three words asked for on its own is a title translated
		// with nothing around it, and because the fleet answers in about two
		// minutes whether the question is a page or a phrase. The block
		// count check already guarantees the heading comes back as its own
		// block, so the answer can be cut at it.
		head := heading(j.front)
		ask := j.body
		if head != "" {
			ask = head + "\n\n" + j.body
		}
		var err error
		res, err = t.Body(ctx, paperOf(j), j.lang, terms(g, j.front.Field, j.lang), ask)
		if err != nil {
			return res, err
		}
		body = res.Text
		if head != "" {
			title, rest, ok := cutHeading(body)
			if !ok {
				return res, fmt.Errorf("%s %s: the answer does not open with the section heading it was given", j.front.Paper, j.name)
			}
			front.SectionTitle, body = title, rest
		}
	}

	front.Lang = j.lang
	front.Edited = false
	front.TranslatedFrom = filepath.Join("content", string(corpus.EN), j.front.Paper, j.name)
	front.SourceContentSHA256 = j.front.ContentSHA256
	if front.SourceContentSHA256 == "" {
		front.SourceContentSHA256 = corpus.ContentSHA([]byte(j.body))
	}
	front.TranslationModel = strings.Join(res.Models, ", ")
	front.TranslationRun = run
	front.SmallModel, front.Gateway = provisional(res, free)
	front.GlossaryVersion = g.Version
	front.GlossaryTermsSHA256 = glossary.TermsSHA(g, j.front.Field, j.lang)
	front.ContentSHA256 = corpus.ContentSHA([]byte(body))
	sha, err := prompt.TranslationSHA(j.lang)
	if err != nil {
		return res, err
	}
	front.PromptSHA256 = sha

	out, err := corpus.Render(front, []byte(body))
	if err != nil {
		return res, err
	}
	dir := c.Content(j.lang, j.front.Paper)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	return res, atomic(filepath.Join(dir, j.name), out)
}

// atomic puts a file in place in one step.
//
// Written and then renamed rather than written in place, because the run
// publishes as it goes and a git add that caught a file halfway through
// being written would commit half a translated section to a public
// repository and then mark it current in the front matter, which is a page
// nothing further down the toolchain would ever ask for again.
//
// The temporary name ends in .tmp rather than .md so that a run interrupted
// between the two steps leaves something the corpus reader skips rather than
// a section file with no front matter in it.
func atomic(name string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}

// provisional says whether a body is provisional, and why.
//
// Either mark is set if any one chunk of the file earned it. A body of
// twelve chunks where eleven came from the big model and one came from a
// mini is a body with a paragraph in it nobody has looked at as hard as the
// rest, and rounding that off to "written by the big model" is exactly the
// thing these two fields exist to stop.
//
// A file that was copied rather than asked for, which is the bibliography,
// earns neither: it has no models and no routes because no question was put.
func provisional(res translate.Result, free map[string]bool) (small, gateway bool) {
	for _, m := range res.Models {
		if llm.SmallModel(m) {
			small = true
		}
	}
	for _, r := range res.Routes {
		if free[r] {
			gateway = true
		}
	}
	return small, gateway
}

// heading is the section title as a line of Markdown, and empty for a file
// whose title is not the paper's own words.
//
// The front matter file and the bibliography are named by the toolchain and
// not by the paper: "Front Matter" is a label papers split invented and
// "References" is what the book prints from its own wordlist. Neither is
// prose of the paper and neither is worth an ask.
func heading(f corpus.Front) string {
	if f.SectionTitle == "" || f.Kind == "front" || f.Kind == "references" {
		return ""
	}
	return "### " + f.SectionTitle
}

// cutHeading takes the translated heading off the front of an answer.
func cutHeading(body string) (title, rest string, ok bool) {
	text := strings.TrimLeft(body, "\n")
	line, rest, _ := strings.Cut(text, "\n")
	title = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
	if !strings.HasPrefix(strings.TrimSpace(line), "#") || title == "" {
		return "", body, false
	}
	return title, strings.TrimLeft(rest, "\n"), true
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
func fleet(routes, names string, logf func(string, ...any)) (func(context.Context, string, llm.Request, int) (translate.Reply, error), string, int, error) {
	first, from, err := work.Fleet(work.Translate, routes, false, logf)
	if err != nil {
		return nil, from, 0, err
	}
	second := first
	if strings.TrimSpace(names) != "" {
		registry, _, err := work.Routes(routes)
		if err != nil {
			return nil, from, 0, err
		}
		only, err := registry.Select(commas(names))
		if err != nil {
			return nil, from, 0, err
		}
		pool := work.Pool(only)
		if pool.Empty() {
			return nil, from, 0, route.ErrNoRoutes()
		}
		up := *first
		up.Waiter = &work.Waiter{Pool: pool, Logf: logf}
		second = &up
	}
	// The first fleet and not the escalation, because the escalation is
	// where a refused chunk goes and not where the run lives. A pool of one
	// route named with -escalate would otherwise throttle the whole run to
	// that route's lanes.
	lanes := first.Waiter.Pool.Lanes()
	return func(ctx context.Context, target string, req llm.Request, attempt int) (translate.Reply, error) {
		asker := first
		if attempt > 1 {
			asker = second
		}
		answer, err := asker.Do(ctx, target, req)
		return translate.Reply{Response: answer.Response, Model: answer.Model, Route: answer.Route}, err
	}, from, lanes, nil
}

// gateways is the names of the routes in a table that are free gateways.
//
// A gateway is somebody else's aggregator in front of somebody else's model,
// and what answers a question there on Tuesday is not necessarily what
// answered it on Monday. The text is usable and the provenance is not, so a
// page written on one is marked and audit rule L15 counts them. The route
// file is read a second time for this, which costs a stat and a parse and
// saves threading a registry through the asker.
func gateways(path string) (map[string]bool, error) {
	registry, _, err := work.Routes(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range registry.Routes {
		if r.Kind == route.KindGateway {
			out[r.Name] = true
		}
	}
	return out, nil
}

// keeper writes the last refused answer of a chunk into the work directory,
// beside the source it was an answer to.
//
// A refusal prints sixty characters of each of the two spans that differ,
// which says which span and not what is wrong with it. Three files of the
// Vietnamese run were given up on over differences in a table of results
// and in a query string, and reading any of them meant guessing at what
// came back. The work directory is ignored by git and safe to delete, and
// a run that cannot write there carries on: keeping a copy for a person to
// read is not worth failing a translation over.
// kept is what the line for one file says about the parts of it that were
// not sent to a model: the chunks that had nothing written in a language in
// them, and the listings that were taken out of a chunk and put back after.
// Both are silent when there were none, which is most files.
func kept(r translate.Result) string {
	var parts []string
	if r.Copied > 0 {
		parts = append(parts, fmt.Sprintf("%d copied", r.Copied))
	}
	if r.Held > 0 {
		parts = append(parts, fmt.Sprintf("%d listings held back", r.Held))
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

func keeper(c *corpus.Corpus) func(target, source, answer string) {
	return func(target, source, answer string) {
		dir := c.Work("refused")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		name := strings.Map(func(r rune) rune {
			if r == ' ' || r == '/' {
				return '-'
			}
			return r
		}, target)
		text := "# " + target + "\n\n## asked\n\n" + source + "\n\n## answered\n\n" + answer + "\n"
		os.WriteFile(filepath.Join(dir, name+".md"), []byte(text), 0o644)
	}
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
