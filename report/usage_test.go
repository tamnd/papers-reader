package report

import (
	"strings"
	"testing"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/ledger"
)

// run is a night of extraction: eight pages of one paper read by a vision
// model, one of which took two hosts to answer, and one page of another that
// nobody answered at all because the quota was spent.
func run() []ledger.Entry {
	day := time.Date(2026, 9, 10, 22, 0, 0, 0, time.UTC)
	var out []ledger.Entry
	for i := 1; i <= 8; i++ {
		out = append(out, ledger.Entry{
			TS: day.Add(time.Duration(i) * time.Minute), App: "papers",
			Stage: "extract", Target: "codd-1970-relational p" + itoa(i),
			Route: "server2", Model: "qwen2.5-vl", OK: true,
			ElapsedMS: 4000, Usage: llm.Usage{InputTokens: 2000, OutputTokens: 900},
		})
	}
	// The one that took two hosts. Two asks, one target, and the first of
	// them cost tokens the corpus has nothing to show for.
	out = append(out, ledger.Entry{
		TS: day.Add(9 * time.Minute), App: "papers",
		Stage: "extract", Target: "codd-1970-relational p3", Attempt: 1,
		Route: "server1", Model: "qwen2.5-vl", State: llm.StateBroken,
		Error: "the gateway hung up", ElapsedMS: 30000,
		Usage: llm.Usage{InputTokens: 2000},
	})
	out = append(out, ledger.Entry{
		TS: day.Add(10 * time.Minute), App: "papers",
		Stage: "extract", Target: "turing-1936-computable p1",
		Route: "server3", Model: "qwen2.5-vl", State: llm.StateQuota,
		Error: "out of turns until tomorrow",
	})
	return out
}

func TestAStageThatWasNeverAskedAnythingIsStillARow(t *testing.T) {
	// The row that matters on a machine where the work has not started. A
	// report that leaves the stage out reads as a report that lost it.
	u := BuildUsage(nil, UsageOptions{Stages: []string{"translate", "extract"}})
	if len(u.Stages) != 2 {
		t.Fatalf("%d rows, want the two stages that were named", len(u.Stages))
	}
	// In the order they were declared, which is the order of the pipeline,
	// and not in the order the alphabet happens to put them in.
	if u.Stages[0].Name != "translate" || u.Stages[0].Asks != 0 {
		t.Errorf("first row is %+v", u.Stages[0])
	}
	md := u.Markdown()
	if !strings.Contains(md, "Nothing has been asked of a model yet") {
		t.Errorf("an empty ledger renders as\n%s", md)
	}
	if !strings.Contains(md, "| extract |") {
		t.Errorf("the empty report has no extract row:\n%s", md)
	}
}

func TestARetriedPageIsTwoAsksAndOnePage(t *testing.T) {
	u := BuildUsage(run(), UsageOptions{Stages: []string{"extract", "translate"}})
	extract := stage(t, u, "extract")
	if extract.Asks != 10 {
		t.Errorf("%d asks, want 10", extract.Asks)
	}
	if extract.Answered != 8 {
		t.Errorf("%d answered, want 8", extract.Answered)
	}
	// Eight pages of one paper and the one page of the other that nobody
	// answered. A page that was asked about is a page that was worked on,
	// whether or not the work came to anything.
	if extract.Targets != 9 {
		t.Errorf("%d targets, want 9", extract.Targets)
	}
}

func TestASpentQuotaIsNotTheSameFailureAsAHostThatBroke(t *testing.T) {
	u := BuildUsage(run(), UsageOptions{})
	if u.Refused != 1 {
		t.Errorf("%d refused, want the one that was out of turns", u.Refused)
	}
	extract := stage(t, u, "extract")
	if extract.States[llm.StateQuota] != 1 || extract.States[llm.StateBroken] != 1 {
		t.Errorf("the failures are %v", extract.States)
	}
	md := u.Markdown()
	if !strings.Contains(md, "| extract | quota | 1 |") {
		t.Errorf("the failure table does not name the quota:\n%s", md)
	}
}

func TestThePaperIsTheFirstWordOfWhatWasAskedAbout(t *testing.T) {
	u := BuildUsage(run(), UsageOptions{})
	if len(u.Papers) != 2 {
		t.Fatalf("%d papers, want 2: %+v", len(u.Papers), u.Papers)
	}
	if u.Papers[0].ID != "codd-1970-relational" || u.Papers[0].Targets != 8 {
		t.Errorf("first paper is %+v, want 8 pages of codd", u.Papers[0])
	}
	if u.Papers[1].ID != "turing-1936-computable" {
		t.Errorf("second paper is %+v", u.Papers[1])
	}
}

func TestAnUnpricedModelIsADashAndNotAZero(t *testing.T) {
	// The fleet is a subscription and a set of free gateways. Nobody has
	// priced them, and a money column reading $0.00 would be this program
	// claiming that the night was free.
	u := BuildUsage(run(), UsageOptions{})
	if u.Unpriced != 10 {
		t.Errorf("%d unpriced asks, want all 10", u.Unpriced)
	}
	md := u.Markdown()
	if !strings.Contains(md, "| - |") {
		t.Errorf("the money column is not a dash:\n%s", md)
	}
	if !strings.Contains(md, "10 of the 10 asks went to a model with no price set") {
		t.Errorf("the report does not say why the money column is empty:\n%s", md)
	}
}

func TestAPricedModelIsCounted(t *testing.T) {
	prices := Prices{"qwen2.5-vl": {In: 1, Out: 5}}
	u := BuildUsage(run(), UsageOptions{Prices: prices})
	// Ten asks: 18,000 input tokens at a dollar the million and 7,200 output
	// at five, which is 1.8 cents plus 3.6 cents.
	if u.Unpriced != 0 {
		t.Errorf("%d unpriced asks, want none", u.Unpriced)
	}
	want := 0.054
	if u.Cost < want-1e-9 || u.Cost > want+1e-9 {
		t.Errorf("cost %v, want %v", u.Cost, want)
	}
	if !strings.Contains(u.Markdown(), "$0.05 at the prices given") {
		t.Errorf("the cost is not in the report:\n%s", u.Markdown())
	}
}

func TestAModelWithNoPriceInAPricedFleetStillReadsAsADash(t *testing.T) {
	// Half a fleet priced is not a bill. A stage that mixes a priced model
	// and an unpriced one has a cost nobody can put a number on, and the sum
	// of the half that was priced looks exactly like the whole.
	entries := append(run(), ledger.Entry{
		TS: time.Date(2026, 9, 10, 23, 0, 0, 0, time.UTC), App: "papers",
		Stage: "extract", Target: "codd-1970-relational p9", OK: true,
		Model: "a-model-nobody-priced", Usage: llm.Usage{InputTokens: 100, OutputTokens: 100},
	})
	u := BuildUsage(entries, UsageOptions{Prices: Prices{"qwen2.5-vl": {In: 1, Out: 5}}})
	if got := money(stage(t, u, "extract").Cost, u.Asks, u.Unpriced); got != "-" {
		t.Errorf("a half priced stage costs %q, want a dash", got)
	}
}

func TestTheReportNamesNoHost(t *testing.T) {
	// The ledger records which host answered, because an operator needs it.
	// This file is committed to a public repository and must not.
	u := BuildUsage(run(), UsageOptions{Stages: []string{"extract"}})
	md := u.Markdown()
	for _, host := range []string{"server1", "server2", "server3"} {
		if strings.Contains(md, host) {
			t.Errorf("the report names %s:\n%s", host, md)
		}
	}
	if strings.Contains(md, "/Users/") || strings.Contains(md, "/home/") {
		t.Errorf("the report has a home directory in it:\n%s", md)
	}
}

func TestTheWindowIsTheEntriesAndNotTheClock(t *testing.T) {
	u := BuildUsage(run(), UsageOptions{})
	if got := u.First.Format(time.DateOnly); got != "2026-09-10" {
		t.Errorf("the first entry is %s", got)
	}
	if !strings.Contains(u.Markdown(), "from 2026-09-10 to 2026-09-10") {
		t.Errorf("the report does not say what it covers:\n%s", u.Markdown())
	}
}

func TestAPriceTableThatIsNotThereIsNotAnError(t *testing.T) {
	p, err := LoadPrices(t.TempDir() + "/nothing.json")
	if err != nil {
		t.Fatalf("a missing price table is an error: %v", err)
	}
	if p != nil {
		t.Errorf("a missing price table loaded as %v", p)
	}
	if _, ok := p.Cost("anything", llm.Usage{InputTokens: 1000}); ok {
		t.Error("a nil price table priced something")
	}
}

func TestTheNumbersAreReadableAtAGlance(t *testing.T) {
	for _, c := range []struct {
		n int
		s string
	}{{0, "0"}, {999, "999"}, {1000, "1,000"}, {41234567, "41,234,567"}} {
		if got := count(c.n); got != c.s {
			t.Errorf("count(%d) is %q, want %q", c.n, got, c.s)
		}
	}
	for _, c := range []struct {
		d time.Duration
		s string
	}{
		{0, "0s"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m 30s"},
		{3*time.Hour + 4*time.Minute, "3h 4m"},
	} {
		if got := spent(c.d); got != c.s {
			t.Errorf("spent(%v) is %q, want %q", c.d, got, c.s)
		}
	}
}

func stage(t *testing.T, u *Usage, name string) UsageStage {
	t.Helper()
	for _, s := range u.Stages {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("there is no %s stage in %+v", name, u.Stages)
	return UsageStage{}
}

// itoa keeps the fixture readable without pulling strconv in for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
