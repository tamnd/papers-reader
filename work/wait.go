package work

import (
	"context"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/route"
)

// How long a run sits still.
const (
	// Slack is added to a route's reset time, because a host that says it is
	// back in thirty minutes and is asked at exactly thirty minutes answers
	// the same refusal and is cooled down for another half hour.
	Slack = 30 * time.Second
	// MaxWait is the longest single wait. It is not the longest total wait:
	// a run that has to sit out six hours sits out six hours, in hourly
	// pieces, and says so each time. An unattended run that prints nothing
	// between midnight and six is indistinguishable from one that has hung,
	// and somebody looking at the terminal in the morning should be able to
	// tell without reading the queue.
	MaxWait = time.Hour
)

// Waiter is route.Pool for a run that is meant to be left alone overnight.
//
// The difference is what happens when every host is out of turns. A pool
// returns an error, which is the right answer for a command somebody is
// watching: it says which routes are cold and when the first one comes back,
// and the person reading decides. It is the wrong answer for a rebuild of the
// corpus, where the fleet is browser sessions on three boxes and a
// subscription, and running out of turns is a routine part of a Tuesday
// rather than a failure. Exiting there means the run stops at nine in the
// evening, the quota clears at half past, and nothing happens until somebody
// notices in the morning.
//
// So this waits instead, and only for as long as the pool says is worth
// waiting. There is one case it will not wait for, and it matters: a fleet
// where no route is ever coming back. That is a person's problem, usually a
// session that logged itself out, and sleeping through it until morning would
// turn a five minute fix into a lost night.
type Waiter struct {
	Pool *route.Pool
	// Max is the longest single wait. Zero means MaxWait.
	Max time.Duration
	// Now and Sleep are the clock, replaceable because every test here is
	// about a deadline and no test should wait for one.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
	// Logf says what is being waited for. A run with nothing to say for an
	// hour should at least say that.
	Logf func(string, ...any)
}

// Pick is route.Pool.Pick, and waits rather than failing when every route is
// cold. The returned release must be called when the caller is done with the
// route, exactly as with the pool itself.
//
// It is Pick and not a separate Wait so that a caller cannot get it wrong.
// The two halves, asking whether anything is available and then waiting if
// nothing is, have to happen together or the wait is for a state that has
// already changed.
func (w *Waiter) Pick(ctx context.Context) (route.Route, llm.Completer, func(), error) {
	if w.Pool == nil || w.Pool.Empty() {
		return route.Route{}, nil, nil, route.ErrNoRoutes()
	}
	for attempt := 1; ; attempt++ {
		chosen, client, release, err := w.Pool.Pick(ctx)
		if err == nil {
			return chosen, client, release, nil
		}
		if ctx.Err() != nil {
			return route.Route{}, nil, nil, ctx.Err()
		}
		delay, who := w.delay(attempt)
		if delay <= 0 {
			// Nothing is counting down, so nothing is coming back. See delay.
			return route.Route{}, nil, nil, err
		}
		if who != "" {
			w.logf("every route is cold, waiting %s for %s", delay.Round(time.Second), who)
		} else {
			w.logf("no route is answering, waiting %s before asking again", delay.Round(time.Second))
		}
		if err := w.sleep(ctx, delay); err != nil {
			return route.Route{}, nil, nil, err
		}
	}
}

// delay is how long to wait and what for. A zero delay means do not wait.
//
// There are three answers because there are three situations. A route with a
// deadline on it is the easy one: wait for the deadline, a little past it, and
// no longer than Max. A fleet that is failing for reasons nothing has put a
// time on gets the library's backoff, which doubles from a second to thirty
// with jitter on it, because the cause is usually a tunnel that dropped or a
// box that is swapping and those come back on their own. And a fleet where
// every route has been retired for the life of the process gets nothing:
// there is no deadline anywhere because nothing is counting down.
func (w *Waiter) delay(attempt int) (time.Duration, string) {
	when, who := w.Pool.EarliestReset()
	if when.IsZero() {
		return 0, ""
	}
	wait := when.Sub(w.now()) + Slack
	if wait <= Slack {
		// The pool says a route is usable and Pick disagreed, which is a race
		// rather than a quota. Backing off is the safe reading of it: the
		// alternative is a loop that asks as fast as the machine can ask.
		wait = llm.Backoff(attempt)
		who = ""
	}
	return min(wait, w.max()), who
}

func (w *Waiter) max() time.Duration {
	if w.Max > 0 {
		return w.Max
	}
	return MaxWait
}

func (w *Waiter) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Waiter) sleep(ctx context.Context, d time.Duration) error {
	if w.Sleep != nil {
		return w.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (w *Waiter) logf(format string, args ...any) {
	if w.Logf != nil {
		w.Logf(format, args...)
	}
}
