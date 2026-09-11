package work

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tamnd/llm/route"
)

// clock is the fake time these tests run on. Sleeping advances it, which is
// the whole trick: a run that waits half an hour for a quota to clear takes a
// microsecond to test and the pool still sees the half hour pass.
type clock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

// start is when every test begins, so that a test can write down a reset
// time without holding a clock to read it off.
var start = time.Date(2026, 9, 11, 21, 0, 0, 0, time.UTC)

func newClock() *clock {
	return &clock{now: start}
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	return nil
}

func (c *clock) waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.slept...)
}

// prober answers with whatever the test has queued up, and repeats the last
// answer for good. A run that keeps asking a host that is out of turns should
// keep being told so.
type prober struct {
	mu      sync.Mutex
	answers []route.Health
	clock   *clock
	asked   int
}

func (p *prober) Probe(_ context.Context, r route.Route) route.Health {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked++
	answer := p.answers[min(p.asked-1, len(p.answers)-1)]
	answer.Route = r.Name
	answer.CheckedAt = p.clock.Now()
	return answer
}

func gateway(name string, rank int) route.Route {
	return route.Route{
		Name:        name,
		Kind:        route.KindGateway,
		BaseURL:     "http://127.0.0.1:1/v1",
		Model:       "a-model",
		Rank:        rank,
		Concurrency: 1,
	}
}

// waiter builds a waiter over one gateway route whose probe answers are the
// ones given.
func waiter(t *testing.T, answers ...route.Health) (*Waiter, *clock, *[]string) {
	t.Helper()
	c := newClock()
	pool := Pool(route.Registry{Routes: []route.Route{gateway("one", 10)}})
	pool.Now = c.Now
	pool.Prober = &prober{answers: answers, clock: c}
	var said []string
	return &Waiter{
		Pool:  pool,
		Now:   c.Now,
		Sleep: c.Sleep,
		Logf:  func(format string, args ...any) { said = append(said, format) },
	}, c, &said
}

func TestARouteThatIsAnsweringIsPickedStraightAway(t *testing.T) {
	w, c, _ := waiter(t, route.Health{State: route.StateLive})

	chosen, client, release, err := w.Pick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if chosen.Name != "one" {
		t.Errorf("picked %q, want one", chosen.Name)
	}
	if client == nil {
		t.Error("no transport came back with the route")
	}
	if got := c.waits(); len(got) != 0 {
		t.Errorf("waited %v with a live route", got)
	}
}

func TestARunOutOfTurnsWaitsForThemRatherThanStopping(t *testing.T) {
	w, c, said := waiter(t,
		route.Health{State: route.StateQuota, Detail: "out of turns", ResetsAt: start.Add(20 * time.Minute)},
		route.Health{State: route.StateLive},
	)

	chosen, _, release, err := w.Pick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if chosen.Name != "one" {
		t.Errorf("picked %q, want one", chosen.Name)
	}
	waits := c.waits()
	if len(waits) != 1 {
		t.Fatalf("waited %v, want one wait", waits)
	}
	if waits[0] < 20*time.Minute || waits[0] > MaxWait {
		t.Errorf("waited %v, want about the twenty minutes the host asked for", waits[0])
	}
	if len(*said) != 1 || !strings.Contains((*said)[0], "cold") {
		t.Errorf("said %q, want one line about waiting", *said)
	}
}

func TestASingleWaitIsNeverLongerThanAnHour(t *testing.T) {
	// A session that logged itself out is cooled down for six hours. A run
	// that says nothing for six hours cannot be told from one that has hung.
	w, c, _ := waiter(t,
		route.Health{State: route.StateUnauthorized, Detail: "signed out", ResetsAt: start.Add(6 * time.Hour)},
		route.Health{State: route.StateLive},
	)

	if _, _, release, err := w.Pick(context.Background()); err != nil {
		t.Fatal(err)
	} else {
		release()
	}
	waits := c.waits()
	if len(waits) == 0 {
		t.Fatal("did not wait at all")
	}
	for _, d := range waits {
		if d > MaxWait {
			t.Errorf("one wait was %v, longer than the %v bound", d, MaxWait)
		}
	}
}

func TestAModelNobodyServesIsNotWaitedFor(t *testing.T) {
	// Gone is the one state no amount of waiting fixes: the host does not
	// serve that model and will not start serving it overnight. Sleeping
	// until morning over it turns an edit to the route file into a lost run.
	w, c, _ := waiter(t, route.Health{State: route.StateGone, Detail: "no such model"})

	if _, _, _, err := w.Pick(context.Background()); err == nil {
		t.Fatal("picked a route that is never coming back")
	}
	if got := c.waits(); len(got) != 0 {
		t.Errorf("waited %v for a route that is never coming back", got)
	}
}

func TestAFleetWithNoRoutesSaysSoRatherThanWaiting(t *testing.T) {
	c := newClock()
	w := &Waiter{Pool: Pool(route.Registry{}), Now: c.Now, Sleep: c.Sleep}

	_, _, _, err := w.Pick(context.Background())
	if err == nil {
		t.Fatal("picked a route out of an empty registry")
	}
	if !strings.Contains(err.Error(), "routes") {
		t.Errorf("error is %q, which does not point at the route file", err)
	}
	if got := c.waits(); len(got) != 0 {
		t.Errorf("waited %v with nothing configured", got)
	}
}

func TestStoppingTheRunStopsTheWaiting(t *testing.T) {
	w, _, _ := waiter(t, route.Health{State: route.StateQuota, ResetsAt: start.Add(45 * time.Minute)})
	ctx, cancel := context.WithCancel(context.Background())
	w.Sleep = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}

	if _, _, _, err := w.Pick(ctx); err != context.Canceled {
		t.Fatalf("got %v, want the run to stop", err)
	}
}

func TestTheSubscriptionOnThisMachineIsCallable(t *testing.T) {
	// An exec route has no HTTP transport, and a pool with no builder
	// registered reports that as a configuration mistake rather than trying
	// to call it. This is the test that says the builder is registered.
	c := newClock()
	pool := Pool(route.Registry{Routes: []route.Route{{
		Name: "codex", Kind: route.KindExec, Command: "codex", Model: "gpt-5", Rank: 100, Concurrency: 1,
	}}})
	pool.Now = c.Now
	pool.Prober = &prober{answers: []route.Health{{State: route.StateLive}}, clock: c}
	w := &Waiter{Pool: pool, Now: c.Now, Sleep: c.Sleep}

	chosen, client, release, err := w.Pick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if chosen.Name != "codex" {
		t.Errorf("picked %q, want codex", chosen.Name)
	}
	if client == nil {
		t.Error("the exec route came back with no way to run it")
	}
}
