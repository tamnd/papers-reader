package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/ledger"
)

// LedgerPath is where the record of asks lives, written the way a person
// would type it rather than the way the machine resolved it.
//
// The resolved path has a home directory in it and a home directory has a
// user name in it, and this report is committed to a public repository. The
// same reasoning keeps the route names out of the tables below: a route is
// named after a host.
const LedgerPath = "~/.config/papers/ledger.jsonl"

// NoAsks is how a report with an empty ledger behind it opens.
//
// It is a constant because a caller needs to tell such a report from one
// with a night of work in it: the ledger is on the machine that did the
// work and the report is in the corpus, so a run somewhere else would
// otherwise quietly overwrite real numbers with zeroes.
const NoAsks = "Nothing has been asked of a model yet"

// A Price is what a model costs, in dollars per million tokens.
//
// Cached input is charged here at the full input price. Every vendor
// discounts it and none of them discount it the same way, and a report that
// overstates the bill is a better report than one that understates it.
//
// Note says where the number came from, and it is not decoration. Most of
// this fleet is priced at zero, and a zero with no sentence behind it reads
// like a model nobody got round to pricing. The report prints the note under
// the model table so that a reader can tell a rate of nothing from a rate
// nobody looked up.
type Price struct {
	In   float64 `json:"in"`
	Out  float64 `json:"out"`
	Note string  `json:"note,omitempty"`
}

// Prices is what each model costs, by the name the ledger recorded it under.
//
// A model that is not in the table has no price rather than a price of zero,
// and the report says how many asks went to one. Most of this corpus is built
// on a subscription and on free gateways, where the true marginal cost of an
// ask is not a number of dollars at all, and writing zero in the money column
// for those would be claiming to have measured something nobody measured.
type Prices map[string]Price

// LoadPrices reads a price table. A file that is not there is not an error:
// it is a fleet nobody has priced, which is the ordinary case.
func LoadPrices(path string) (Prices, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p Prices
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Cost is what one ask cost. The bool is false for a model with no price.
func (p Prices) Cost(model string, u llm.Usage) (float64, bool) {
	// An ask that never reached a model has no model name and no tokens on
	// it, and nothing was charged for it by anybody. Most of these are a
	// connection refused while a host was down. Calling them unpriced would
	// put a dash across a whole night's money on account of a call that did
	// not happen.
	if model == "" && u.InputTokens == 0 && u.OutputTokens == 0 {
		return 0, true
	}
	price, ok := p[model]
	if !ok {
		return 0, false
	}
	return (price.In*float64(u.InputTokens) + price.Out*float64(u.OutputTokens)) / 1e6, true
}

// UsageOptions is what a usage report is built with.
type UsageOptions struct {
	// Prices is the price table. Nil prices everything at nothing known.
	Prices Prices
	// Stages are the stages to show even when they were never asked
	// anything. A stage that has run nothing is the most useful row in the
	// table on a machine where the work has not started, and it only appears
	// if somebody says in advance which stages exist.
	Stages []string
	// Since is the start of the window, for the sentence that says what the
	// report covers. It does not filter; the caller filters.
	Since time.Time
	// Reads is what the corpus was read by, from ReadPages. It is here
	// because a report of what the work cost that counted only the asks
	// would leave out every page that cost nothing, and those are most of
	// them.
	Reads []Read
}

// A Usage is what the machine time cost, rolled up from the ledger.
//
// Grouped by stage, by model and by paper, and deliberately not by route.
// Every other rollup in the module names the host that answered, because
// that is the number an operator needs. This one is committed to a public
// repository, so it counts pages, tokens and seconds and names no hosts.
type Usage struct {
	Asks     int
	Answered int
	Refused  int
	Targets  int
	Tokens   llm.Usage
	Elapsed  time.Duration
	Cost     float64
	// Unpriced is the asks whose model is not in the price table, which is
	// what stops the money column from being read as a bill.
	Unpriced int
	Stages   []UsageStage
	Models   []UsageModel
	Papers   []UsagePaper
	Reads    []Read
	// First and Last are the ends of the window the entries actually cover,
	// which is not the window that was asked for.
	First, Last time.Time
	Since       time.Time
}

// A UsageStage is one stage of the pipeline.
type UsageStage struct {
	Name     string
	Asks     int
	Answered int
	Refused  int
	// Targets is how many distinct things were worked on, which is pages for
	// the extract stage and sections for translate. An ask that failed and
	// was asked again of another host is two asks and one target, and the
	// difference between those two numbers is what a bad night looks like.
	Targets  int
	Tokens   llm.Usage
	Elapsed  time.Duration
	Cost     float64
	Unpriced int
	States   map[llm.State]int
}

// A UsageModel is one model, by the name the ledger recorded.
type UsageModel struct {
	Name   string
	Asks   int
	Tokens llm.Usage
	Cost   float64
	Priced bool
	// Note is the price table's sentence about this model, copied here so
	// that the report can print it without carrying the table around.
	Note string
}

// A UsagePaper is one paper in one stage.
type UsagePaper struct {
	ID      string
	Stage   string
	Targets int
	Tokens  llm.Usage
	Elapsed time.Duration
}

// BuildUsage rolls the ledger up.
func BuildUsage(entries []ledger.Entry, opt UsageOptions) *Usage {
	u := &Usage{Since: opt.Since, Reads: opt.Reads}
	stages := map[string]*UsageStage{}
	models := map[string]*UsageModel{}
	papers := map[string]*UsagePaper{}
	// Distinct targets are counted per stage and per paper, and a target of
	// one stage is not a target of another, so the set is keyed by both.
	targets := map[string]bool{}

	// The declared stages keep the order they were declared in, which is
	// pipeline order, because the order of the pipeline is the order a person
	// reads the table in. A stage the ledger holds and nobody declared goes
	// after them, alphabetically.
	order := map[string]int{}
	for i, name := range opt.Stages {
		stages[name] = &UsageStage{Name: name, States: map[llm.State]int{}}
		order[name] = i
	}
	for _, e := range entries {
		stage := stages[e.Stage]
		if stage == nil {
			stage = &UsageStage{Name: e.Stage, States: map[llm.State]int{}}
			stages[e.Stage] = stage
		}
		cost, priced := opt.Prices.Cost(e.Model, e.Usage)

		u.Asks++
		u.Tokens = u.Tokens.Add(e.Usage)
		u.Elapsed += e.Elapsed()
		u.Cost += cost
		stage.Asks++
		stage.Tokens = stage.Tokens.Add(e.Usage)
		stage.Elapsed += e.Elapsed()
		stage.Cost += cost
		if !priced {
			u.Unpriced++
			stage.Unpriced++
		}
		switch {
		case e.OK:
			u.Answered++
			stage.Answered++
		default:
			state := e.State
			if state == "" {
				state = llm.StateUnknown
			}
			stage.States[state]++
			if e.Refused() {
				u.Refused++
				stage.Refused++
			}
		}
		if u.First.IsZero() || e.TS.Before(u.First) {
			u.First = e.TS
		}
		if e.TS.After(u.Last) {
			u.Last = e.TS
		}

		model := models[e.Model]
		if model == nil {
			model = &UsageModel{Name: e.Model, Priced: priced, Note: opt.Prices[e.Model].Note}
			models[e.Model] = model
		}
		model.Asks++
		model.Tokens = model.Tokens.Add(e.Usage)
		model.Cost += cost

		if e.Target == "" {
			continue
		}
		id := paperOf(e.Target)
		key := e.Stage + "\x00" + e.Target
		first := !targets[key]
		targets[key] = true
		if first {
			u.Targets++
			stage.Targets++
		}
		paper := papers[e.Stage+"\x00"+id]
		if paper == nil {
			paper = &UsagePaper{ID: id, Stage: e.Stage}
			papers[e.Stage+"\x00"+id] = paper
		}
		paper.Tokens = paper.Tokens.Add(e.Usage)
		paper.Elapsed += e.Elapsed()
		if first {
			paper.Targets++
		}
	}

	for _, s := range stages {
		u.Stages = append(u.Stages, *s)
	}
	sort.Slice(u.Stages, func(i, j int) bool {
		a, aok := order[u.Stages[i].Name]
		b, bok := order[u.Stages[j].Name]
		if aok != bok {
			return aok
		}
		if aok && a != b {
			return a < b
		}
		return u.Stages[i].Name < u.Stages[j].Name
	})
	for _, m := range models {
		u.Models = append(u.Models, *m)
	}
	sort.Slice(u.Models, func(i, j int) bool { return u.Models[i].Name < u.Models[j].Name })
	for _, p := range papers {
		u.Papers = append(u.Papers, *p)
	}
	sort.Slice(u.Papers, func(i, j int) bool {
		if u.Papers[i].ID != u.Papers[j].ID {
			return u.Papers[i].ID < u.Papers[j].ID
		}
		return u.Papers[i].Stage < u.Papers[j].Stage
	})
	return u
}

// paperOf is the paper an ask was about. A target is written in the words a
// person would use, a paper id and then what part of it, so the id is the
// first word of it.
func paperOf(target string) string {
	if i := strings.IndexAny(target, " \t"); i >= 0 {
		return target[:i]
	}
	return target
}

// Summary is the one line a run prints.
func (u *Usage) Summary() string {
	if u.Asks == 0 {
		return "no asks recorded"
	}
	return fmt.Sprintf("%s asks, %s answered, %s refused, %s tokens in, %s out, %s spent",
		count(u.Asks), count(u.Answered), count(u.Refused),
		count(u.Tokens.InputTokens), count(u.Tokens.OutputTokens), spent(u.Elapsed))
}

// Markdown is reports/usage.md.
func (u *Usage) Markdown() string {
	var b strings.Builder
	b.WriteString("# Usage\n\nWhat the corpus cost in machine time, by stage.\n\n")
	b.WriteString("This is read from the ledger, which is one line per ask and lives at `")
	b.WriteString(LedgerPath)
	b.WriteString("` on the machine that did the work. The ledger is not in this repository and will not be: it names the hosts that were asked. Nothing below names a host.\n\n")

	if u.Asks == 0 {
		b.WriteString(NoAsks + ", so there is nothing to count. A paper that came through the native extraction path never puts a question to a model, by design, and a corpus built entirely that way has an empty ledger and an honest zero here.\n")
		u.writeReads(&b)
		u.writeStages(&b)
		return b.String()
	}

	fmt.Fprintf(&b, "%s, from %s to %s.\n", sentence(u), u.First.Format(time.DateOnly), u.Last.Format(time.DateOnly))
	u.writeReads(&b)
	u.writeStages(&b)
	u.writeModels(&b)
	u.writePapers(&b)
	u.writeFailures(&b)
	return b.String()
}

// writeReads is the pages of the corpus by the path that read them, which
// is the part of this report that has numbers in it on a machine where no
// model has been asked anything.
func (u *Usage) writeReads(b *strings.Builder) {
	if len(u.Reads) == 0 {
		return
	}
	b.WriteString("\n## Pages read\n\n")
	b.WriteString("Counted off the committed English content, one page counted once per paper however many sections were cut from it. `native` is pdftotext on a born digital file with no model anywhere in the path, so those pages were never guessed. `layout` is a layout model reading the page geometry and `ocr` is a vision model reading a page image, and only those two cost anything in the tables below.\n\n")
	b.WriteString("| path | papers | pages | what read them |\n| --- | --: | --: | --- |\n")
	for _, r := range u.Reads {
		what := strings.Join(r.Models, ", ")
		if what == "" {
			what = "not recorded"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", r.Path, count(r.Papers), count(r.Pages), what)
	}
}

func (u *Usage) writeStages(b *strings.Builder) {
	b.WriteString("\n## Per stage\n\n")
	b.WriteString("A target is the thing one ask was about: a page for the extract stage, a section for translate. Asks are higher than targets when a host did not answer and the question went to the next one.\n\n")
	b.WriteString("| stage | asks | answered | refused | targets | tokens in | tokens out | time | cost |\n")
	b.WriteString("| --- | --: | --: | --: | --: | --: | --: | --: | --: |\n")
	for _, s := range u.Stages {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			name(s.Name), count(s.Asks), count(s.Answered), count(s.Refused), count(s.Targets),
			count(s.Tokens.InputTokens), count(s.Tokens.OutputTokens), spent(s.Elapsed),
			money(s.Cost, s.Asks, s.Unpriced))
	}
	if u.Unpriced == 0 && u.Asks > 0 && u.Cost == 0 {
		b.WriteString("\nEvery ask went to a model priced at nothing per token, so the money column is what this run added to a bill rather than what the fleet costs to have. A subscription is paid by the month whether or not a paper is read, and the GPU in the corner was paid for once. What each rate is and where it came from is under the model table.\n")
	}
	if u.Unpriced > 0 {
		fmt.Fprintf(b, "\n%s of the %s asks went to a model with no price set, so the money column is a dash for them. A price is dollars per million tokens by model, and the table is `manifests/prices.json`. A model that is missing from it is a model nobody has looked the rate up for, which is not the same thing as a model that costs nothing, so it gets a dash rather than a zero.\n",
			count(u.Unpriced), count(u.Asks))
	}
}

func (u *Usage) writeModels(b *strings.Builder) {
	if len(u.Models) == 0 {
		return
	}
	b.WriteString("\n## Per model\n\n")
	b.WriteString("| model | asks | tokens in | tokens out | cost |\n| --- | --: | --: | --: | --: |\n")
	for _, m := range u.Models {
		unpriced := 0
		if !m.Priced {
			unpriced = m.Asks
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", name(m.Name), count(m.Asks),
			count(m.Tokens.InputTokens), count(m.Tokens.OutputTokens), money(m.Cost, m.Asks, unpriced))
	}
	notes := 0
	for _, m := range u.Models {
		if m.Note == "" {
			continue
		}
		if notes == 0 {
			b.WriteString("\nWhat the rates are:\n\n")
		}
		notes++
		fmt.Fprintf(b, "- %s: %s\n", name(m.Name), m.Note)
	}
}

func (u *Usage) writePapers(b *strings.Builder) {
	if len(u.Papers) == 0 {
		return
	}
	b.WriteString("\n## Per paper\n\n")
	b.WriteString("In id order rather than in order of cost, so that two of these reports can be read side by side.\n\n")
	b.WriteString("| paper | stage | targets | tokens in | tokens out | time |\n| --- | --- | --: | --: | --: | --: |\n")
	for _, p := range u.Papers {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n", p.ID, name(p.Stage), count(p.Targets),
			count(p.Tokens.InputTokens), count(p.Tokens.OutputTokens), spent(p.Elapsed))
	}
}

// writeFailures says why the asks that failed failed, in the router's own
// vocabulary. A quota that was spent and a host that broke look the same in
// every other column and want different work from a person.
func (u *Usage) writeFailures(b *strings.Builder) {
	rows := 0
	for _, s := range u.Stages {
		rows += len(s.States)
	}
	if rows == 0 {
		return
	}
	b.WriteString("\n## What did not answer\n\n")
	b.WriteString("| stage | why | asks |\n| --- | --- | --: |\n")
	for _, s := range u.Stages {
		for _, st := range sorted(s.States) {
			fmt.Fprintf(b, "| %s | %s | %s |\n", name(s.Name), st, count(s.States[st]))
		}
	}
}

// sentence is the summary line in prose, for the top of the file.
func sentence(u *Usage) string {
	out := fmt.Sprintf("%s asks, %s of them answered", count(u.Asks), count(u.Answered))
	if u.Refused > 0 {
		out += fmt.Sprintf(" and %s refused before the model read the question", count(u.Refused))
	}
	out += fmt.Sprintf(". %s tokens in, %s out, %s of waiting over %s targets",
		count(u.Tokens.InputTokens), count(u.Tokens.OutputTokens), spent(u.Elapsed), count(u.Targets))
	if u.Unpriced == 0 && u.Asks > 0 {
		out += ", " + money(u.Cost, u.Asks, u.Unpriced) + " at the prices given"
	}
	return out
}

// sorted is the states of one stage, most asks first, so the row that wants
// doing something about is the row at the top.
func sorted(states map[llm.State]int) []llm.State {
	out := make([]llm.State, 0, len(states))
	for s := range states {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if states[out[i]] != states[out[j]] {
			return states[out[i]] > states[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// name is a stage or a model, printed as something a table can hold. A
// failed ask often has no model in it, because the name of the model comes
// back in the answer and there was no answer, and those asks are a row of
// their own rather than being quietly dropped.
func name(s string) string {
	if strings.TrimSpace(s) == "" {
		return "not recorded"
	}
	return s
}

// count is a number with thousands separators, because these run to eight
// digits and a reader should not have to count the digits to see which
// stage cost the most.
func count(n int) string {
	s := fmt.Sprint(n)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	return sign + strings.Join(append([]string{s}, parts...), ",")
}

// spent is a duration at the precision somebody reading a day of work wants,
// which is never milliseconds and never nine significant figures.
func spent(d time.Duration) string {
	switch {
	case d <= 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// money is a cost, or a dash for a row where any ask went to a model nobody
// priced. A partial sum printed as a total is the one number in this report
// that could be quoted at somebody.
func money(cost float64, asks, unpriced int) string {
	if asks == 0 || unpriced > 0 {
		return "-"
	}
	if cost > 0 && cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}
