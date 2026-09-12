package work

import (
	"context"
	"fmt"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/ledger"
	"github.com/tamnd/llm/queue"
	"github.com/tamnd/llm/route"
)

// An Ask puts one question to the fleet: pick a host, wait if every host is
// out of turns, send, record what it cost, and try the next host if this one
// did not answer.
//
// It exists so that the stages which talk to a model do not each write their
// own version of that. They differ in what they ask and in what they do with
// the answer, and they agree on everything in between, and the part they
// agree on is the part with the failover and the accounting in it.
//
// Nothing here judges an answer. A page that came back as an apology is a
// successful call that produced a bad page, and telling those apart is the
// business of whoever asked: the route answered, the route's quota was spent
// on it, and the record should say so.
type Ask struct {
	// Waiter is the pool, with the overnight behaviour. A run with somebody
	// watching it can set Max short; an unattended rebuild leaves it alone.
	Waiter *Waiter
	// Ledger is where the call is recorded. Nil records nothing, which is
	// what a test wants and what a dry run gets.
	Ledger *ledger.Log
	Stage  queue.Stage
	// Tries is how many hosts are asked before the question is given up on.
	// Zero means Tries.
	Tries int
	Logf  func(string, ...any)
}

// Tries is how many hosts one question is put to.
//
// Three, because the interesting failure is a host that is up, answers, and
// answers badly, and the way through that is a different host rather than the
// same one again. Beyond three the fleet is having a bad day and a run that
// keeps going is just spending everybody's quota on it.
const Tries = 3

// Answer is a response and the route that gave it, in the corpus's own words
// rather than the wire's.
//
// Model is Route.Model and not what came back in the response body. The two
// differ on purpose: the server answers to whatever its own shortlist entry
// is called, and the front matter has to name weights that will still mean
// something in a year.
type Answer struct {
	llm.Response
	Model string
	Route string
}

// Do puts one question and returns the answer.
//
// target is what the question was about, in words a person would use: a paper
// id and a page number. It goes in the ledger and it is what a usage report
// groups by, so "codd-1970-relational p3" is right and a hash is not.
func (a *Ask) Do(ctx context.Context, target string, req llm.Request) (Answer, error) {
	if a.Waiter == nil || a.Waiter.Pool == nil {
		return Answer{}, route.ErrNoRoutes()
	}
	var last error
	for attempt := 1; attempt <= a.tries(); attempt++ {
		chosen, client, release, err := a.Waiter.Pick(ctx)
		if err != nil {
			if last != nil {
				return Answer{}, fmt.Errorf("%w (the last host that answered said: %v)", err, last)
			}
			return Answer{}, err
		}

		// The wire name rather than the corpus name. Sending the corpus slug
		// gets a 404 from a server that has never heard of it.
		asked := req
		asked.Model = chosen.Wire()
		start := time.Now()
		res, err := client.Complete(ctx, asked)
		release()
		if res.Elapsed == 0 {
			res.Elapsed = time.Since(start)
		}
		res.Route = chosen.Name
		a.record(target, chosen.Name, attempt, res, err)

		if err == nil {
			a.Waiter.Pool.Succeed(chosen.Name)
			return Answer{Response: res, Model: chosen.Model, Route: chosen.Name}, nil
		}
		a.Waiter.Pool.Fail(chosen.Name, err)
		last = err
		if ctx.Err() != nil {
			return Answer{}, ctx.Err()
		}
		a.logf("%s: %s did not answer: %v", target, chosen.Name, llm.Condense(err.Error()))
	}
	return Answer{}, fmt.Errorf("%s: %d hosts were asked and none answered: %w", target, a.tries(), last)
}

// record writes the call down. A ledger that cannot be written to is not a
// reason to stop a run that is otherwise working, but it is a reason to say
// so, because a usage report with a hole in it looks like a quiet afternoon.
func (a *Ask) record(target, name string, attempt int, res llm.Response, err error) {
	if a.Ledger == nil {
		return
	}
	if werr := a.Ledger.Record(string(a.Stage), target, name, attempt, res, err); werr != nil {
		a.logf("the call to %s about %s was not recorded: %v", name, target, werr)
	}
}

func (a *Ask) tries() int {
	if a.Tries > 0 {
		return a.Tries
	}
	return Tries
}

func (a *Ask) logf(format string, args ...any) {
	if a.Logf != nil {
		a.Logf(format, args...)
	}
}

// Fleet assembles the asker for one stage: the routing table, the pool of
// hosts that can serve it, the overnight waiting and the ledger.
//
// It exists so that a command does not have to know the order those four go
// together in, and so that two commands cannot assemble them differently. It
// returns the path the routing table was read from, because a command that is
// about to spend somebody's quota should say whose table it is going by.
//
// vision picks the pool that will take a page image. A fleet with no such
// route fails here, at the start, rather than an hour into a run: the pages
// are the slow part and finding out at the end that there was nowhere to send
// them is the expensive way to learn it.
func Fleet(stage queue.Stage, path string, vision bool, logf func(string, ...any)) (*Ask, string, error) {
	registry, from, err := Routes(path)
	if err != nil {
		return nil, from, err
	}
	pool := Pool(registry)
	if vision {
		pool = Vision(registry)
	}
	if pool.Empty() {
		if vision {
			return nil, from, fmt.Errorf("no route can take a page image (the routing table is %s): add one with vision set, or run papers routes init", from)
		}
		return nil, from, route.ErrNoRoutes()
	}
	log, err := Ledger()
	if err != nil {
		// A run that cannot write the ledger is a run whose usage report will
		// have a hole in it, and that is worth saying and not worth stopping
		// for. The pages are the valuable part.
		if logf != nil {
			logf("the ledger will not open, so this run will not be in the usage report: %v", err)
		}
		log = nil
	}
	return &Ask{
		Waiter: &Waiter{Pool: pool, Logf: logf},
		Ledger: log,
		Stage:  stage,
		Logf:   logf,
	}, from, nil
}

// Ledger opens the record of what the fleet was asked.
//
// It lives beside the route file under ~/.config/papers rather than in the
// corpus, for the same reason the route file does: it names hosts. A usage
// report made from it is committed, and that report holds page counts, token
// counts and elapsed times and no host names at all.
func Ledger() (*ledger.Log, error) {
	Configure()
	return ledger.Open(ledger.DefaultPath())
}
