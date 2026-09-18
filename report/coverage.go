package report

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/classify"
	"github.com/tamnd/papers-reader/corpus"
)

// A State is how much of a paper the corpus publishes.
//
// Three states and not a percentage. A paper is not 60 per cent done: it
// either has its body in the corpus, or it has the front matter a
// restricted licence allows and nothing else, or it has nothing. A number
// between those would be a number nobody could act on.
type State string

const (
	// Full is a paper whose body is in the corpus, section files and all.
	Full State = "full"
	// Stub is the front matter and a short abstract, which is everything a
	// restricted paper may ever have here. A stub is not a failure.
	Stub State = "stub"
	// None is a paper the corpus publishes nothing of yet.
	None State = "none"
)

// A Coverage is how much of the corpus is done, by field and by language.
type Coverage struct {
	Papers  []CoveragePaper
	Fields  []CoverageField
	Total   CoverageField
	Waiting []CoverageWait
	Langs   []CoverageLang
	// Whole is the corpus policy: every paper published in full, whatever
	// its licence says. It changes what the report means rather than what it
	// counts. A stub is the finished state of a restricted paper when the
	// corpus publishes by licence, and it is a paper waiting to be read when
	// the corpus publishes everything.
	Whole bool
}

// A CoveragePaper is one paper and where it has got to.
type CoveragePaper struct {
	ID     string
	Field  corpus.Field
	Access corpus.Access
	State  State
	// Why is what the paper is waiting on, in words, and empty for a paper
	// that is as done as its licence allows. It is the column somebody reads
	// when they want to know what to go and do.
	Why string
	// Sections is how many files the English content is in, front matter
	// included.
	Sections int
}

// A CoverageField is one subject field counted up.
type CoverageField struct {
	Field corpus.Field
	// Papers is how many papers are in the field, which is the denominator
	// of the other three.
	Papers int
	Full   int
	Stub   int
	None   int
	// Whole is the corpus policy, copied onto every row so that Done can be
	// read off a field without the corpus being at hand.
	Whole bool
}

// Done is the share of the field that is as done as the corpus means it to
// be, from zero to one.
//
// Publishing by licence, a stub counts: a restricted paper with its abstract
// cut is finished, and counting it as a shortfall would be a report that can
// never reach the top of its own scale. Publishing every paper in full, a
// stub is a paper with three pages read out of thirty, so it counts for
// nothing and the report goes back to measuring work left to do.
func (f CoverageField) Done() float64 {
	if f.Papers == 0 {
		return 0
	}
	done := f.Full
	if !f.Whole {
		done += f.Stub
	}
	return float64(done) / float64(f.Papers)
}

// A CoverageWait is a reason papers are not done, and how many are waiting
// on it.
type CoverageWait struct {
	Why    string
	Papers int
}

// A CoverageLang is one language of the corpus.
type CoverageLang struct {
	Lang   corpus.Lang
	Papers int
	Files  int
	// Stale is the translated files whose English has changed since. It is
	// the field the whole provenance block earns its keep with: a stale file
	// is detectably stale rather than quietly wrong.
	Stale int
	// English is how many files the English has, copied onto every row so
	// that Done can be read off a language without the corpus being at hand.
	// It is the denominator of the whole table: a translation is of an
	// English file and there is nothing else for it to be of.
	English int
}

// Done is the share of the English this language has been translated into,
// from zero to one, counting a stale file as done.
//
// Files rather than papers, because a paper the translator has reached and
// not finished counts in the papers column from its first section and a
// corpus two thirds translated would read as fully translated. Files is the
// number that moves while a paper is being worked through.
//
// A stale file counts because it is a translation. What is wrong with it is
// that the English moved underneath it, which the stale column is for, and
// subtracting it here would make one file's worth of work show up twice and
// make the two columns hard to read together.
func (l CoverageLang) Done() float64 {
	if l.English == 0 {
		return 0
	}
	return float64(l.Files) / float64(l.English)
}

// BuildCoverage reads the corpus and counts it.
func BuildCoverage(c *corpus.Corpus) (*Coverage, error) {
	papers, err := c.LoadPapers()
	if err != nil {
		return nil, err
	}
	sources, err := corpus.LoadSources(c.SourcesManifest())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	cov := &Coverage{Whole: c.PublishesWhole()}
	fields := map[corpus.Field]*CoverageField{}
	waiting := map[string]int{}
	langs := map[corpus.Lang]*CoverageLang{}
	for _, lang := range corpus.Langs {
		langs[lang] = &CoverageLang{Lang: lang}
	}

	for _, p := range papers.Papers {
		var rec *corpus.Source
		if sources != nil {
			if found, ok := sources.ByID(p.ID); ok {
				rec = found
			}
		}
		english, err := Files(c, corpus.EN, p.ID)
		if err != nil {
			return nil, err
		}
		row := CoveragePaper{ID: p.ID, Field: p.Field, Sections: len(english)}
		if rec != nil {
			row.Access = rec.Access
		}
		row.State = StateOf(english)
		row.Why = why(c, p, rec, row.State)

		cov.Papers = append(cov.Papers, row)
		if fields[p.Field] == nil {
			fields[p.Field] = &CoverageField{Field: p.Field, Whole: cov.Whole}
		}
		tally := fields[p.Field]
		tally.Papers++
		switch row.State {
		case Full:
			tally.Full++
		case Stub:
			tally.Stub++
		default:
			tally.None++
		}
		if row.Why != "" {
			waiting[row.Why]++
		}

		for _, lang := range corpus.Langs {
			found, err := Files(c, lang, p.ID)
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				continue
			}
			langs[lang].Papers++
			langs[lang].Files += len(found)
			if lang == corpus.EN {
				continue
			}
			n, err := stale(c, p.ID, lang, found)
			if err != nil {
				return nil, err
			}
			langs[lang].Stale += n
		}
	}

	for _, f := range corpus.Fields {
		if fields[f] == nil {
			continue
		}
		cov.Fields = append(cov.Fields, *fields[f])
	}
	cov.Total.Whole = cov.Whole
	for _, f := range cov.Fields {
		cov.Total.Papers += f.Papers
		cov.Total.Full += f.Full
		cov.Total.Stub += f.Stub
		cov.Total.None += f.None
	}
	for reason, n := range waiting {
		cov.Waiting = append(cov.Waiting, CoverageWait{Why: reason, Papers: n})
	}
	sort.Slice(cov.Waiting, func(i, j int) bool {
		if cov.Waiting[i].Papers != cov.Waiting[j].Papers {
			return cov.Waiting[i].Papers > cov.Waiting[j].Papers
		}
		return cov.Waiting[i].Why < cov.Waiting[j].Why
	})
	for _, lang := range corpus.Langs {
		row := *langs[lang]
		row.English = langs[corpus.EN].Files
		cov.Langs = append(cov.Langs, row)
	}
	return cov, nil
}

// StateOf is how much of a paper is published, from what is on disk.
func StateOf(english []string) State {
	switch {
	case len(english) == 0:
		return None
	case len(english) == 1 && strings.HasPrefix(filepath.Base(english[0]), "00_"):
		return Stub
	default:
		return Full
	}
}

// why is what a paper is waiting on.
//
// It is deliberately a sentence and not a code. The list of them is short,
// they are counted by exact text, and somebody reading the report should be
// able to act on a row without going and looking anything up.
func why(c *corpus.Corpus, p corpus.Paper, rec *corpus.Source, s State) string {
	if rec == nil || rec.Access == "" || rec.Access == corpus.AccessUnknown {
		// Even under a policy that publishes everything, a paper nobody has
		// identified has no location to fetch from, so this is still the
		// first thing to go and do about it.
		return "nothing is known about what may be published from it"
	}
	if s == Stub && rec.Access == corpus.AccessRestricted && !c.PublishesWhole() {
		// As done as it will ever be. A restricted paper publishes its front
		// matter and an abstract and that is the whole of what it may have.
		return ""
	}
	if s == Full {
		return ""
	}
	if _, err := os.Stat(c.PDF(p.ID)); err != nil {
		return "not fetched yet"
	}
	switch classify.Layer(rec.TextLayer).Path() {
	case classify.PathNative:
		return "waiting on extraction, which needs no model"
	case classify.PathLayout:
		return "waiting on a layout tool"
	case classify.PathVision:
		return "waiting on a vision model"
	}
	return "not classified yet"
}

// Files is the content files of one paper in one language, in name order.
//
// A language the paper has nothing in is no files and no error. That is the
// ordinary case for most of the corpus in most of the languages, and a
// caller that had to tell a missing directory from a real failure at every
// call site would get it wrong somewhere.
func Files(c *corpus.Corpus, lang corpus.Lang, id string) ([]string, error) {
	dir := c.Content(lang, id)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// stale counts the translated files whose English has changed since they
// were made.
//
// A translation records the SHA-256 of the English file it was made from,
// so this is one hash compared against one hash. A file that records no
// source hash is not counted as stale: it is a file nobody can say anything
// about, and the audit is what objects to that.
func stale(c *corpus.Corpus, id string, lang corpus.Lang, found []string) (int, error) {
	n := 0
	for _, path := range found {
		b, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		front, _, err := corpus.ParseFront(b)
		if err != nil {
			return 0, err
		}
		if front.SourceContentSHA256 == "" {
			continue
		}
		english, err := os.ReadFile(filepath.Join(c.Content(corpus.EN, id), filepath.Base(path)))
		if os.IsNotExist(err) {
			// The English it was made from is gone, which is worse than
			// stale and is the audit's business rather than this report's.
			n++
			continue
		}
		if err != nil {
			return 0, err
		}
		was, body, err := corpus.ParseFront(english)
		if err != nil {
			return 0, err
		}
		now := was.ContentSHA256
		if now == "" {
			now = corpus.ContentSHA(body)
		}
		if now != front.SourceContentSHA256 {
			n++
		}
	}
	return n, nil
}

// Summary is the one line a run prints.
func (c *Coverage) Summary() string {
	return fmt.Sprintf("%d papers: %d full, %d stub, %d none",
		c.Total.Papers, c.Total.Full, c.Total.Stub, c.Total.None)
}

// Markdown is reports/coverage.md.
func (c *Coverage) Markdown() string {
	var b strings.Builder
	b.WriteString("# Coverage\n\nHow much of each paper the corpus publishes.\n\n")
	if c.Whole {
		b.WriteString("`full` is a paper whose body is here, section by section. `stub` is the front matter and a short abstract, which is where a paper stops until it is read in full. `none` is a paper the corpus publishes nothing of yet.\n\n")
		fmt.Fprintf(&b, "%s, which is %s of the corpus published in full.\n\n",
			c.Summary(), percent(c.Total.Done()))
	} else {
		b.WriteString("`full` is a paper whose body is here, section by section. `stub` is the front matter and a short abstract, which is the whole of what a restricted paper may ever have and is not a shortfall. `none` is a paper the corpus publishes nothing of yet.\n\n")
		fmt.Fprintf(&b, "%s, which is %s of what the licences allow.\n\n",
			c.Summary(), percent(c.Total.Done()))
	}

	b.WriteString("## Per field\n\n| field | papers | full | stub | none | done |\n| --- | --: | --: | --: | --: | --: |\n")
	for _, f := range c.Fields {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %s |\n",
			f.Field, f.Papers, f.Full, f.Stub, f.None, percent(f.Done()))
	}
	fmt.Fprintf(&b, "| **all** | %d | %d | %d | %d | %s |\n",
		c.Total.Papers, c.Total.Full, c.Total.Stub, c.Total.None, percent(c.Total.Done()))

	if len(c.Waiting) > 0 {
		b.WriteString("\n## What the rest is waiting on\n\n| waiting on | papers |\n| --- | --: |\n")
		for _, w := range c.Waiting {
			fmt.Fprintf(&b, "| %s | %d |\n", w.Why, w.Papers)
		}
	}

	b.WriteString("\n## Per language\n\n")
	b.WriteString("English is extracted and the other three are translated from it. `done` is the share of the English files this language has, which is what a reader in that language gets. `stale` is a translated file whose English has changed since, which is the one number here that means somebody has work to do rather than work to start.\n\n")
	b.WriteString("| language | papers | files | done | stale |\n| --- | --: | --: | --: | --: |\n")
	for _, l := range c.Langs {
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %d |\n", l.Lang, l.Papers, l.Files, percent(l.Done()), l.Stale)
	}

	left := 0
	for _, p := range c.Papers {
		if p.Why != "" {
			left++
		}
	}
	if left == 0 {
		b.WriteString("\nEvery paper is as done as its licence allows.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\n## What is left\n\nThe %d papers that are not yet as done as their licence allows, in id order.\n\n", left)
	b.WriteString("| paper | field | access | state | waiting on |\n| --- | --- | --- | --- | --- |\n")
	rows := append([]CoveragePaper(nil), c.Papers...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for _, p := range rows {
		if p.Why == "" {
			continue
		}
		access := string(p.Access)
		if access == "" {
			access = "none"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", p.ID, p.Field, access, p.State, p.Why)
	}
	return b.String()
}

// percent is a share as a whole number, because a tenth of a percent of a
// hundred papers is a tenth of a paper.
func percent(f float64) string {
	return fmt.Sprintf("%d%%", int(f*100+0.5))
}
