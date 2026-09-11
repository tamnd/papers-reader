// Package polite holds the manners that every outbound request in this
// toolchain shares.
//
// Two things live here because two packages need them and a second copy of a
// rate limiter is a second thing to get wrong. The resolver asks four free
// APIs about a hundred papers, and the fetcher downloads from whatever host
// the resolver pointed it at. Both of them are guests.
package polite

import (
	"fmt"
	"sync"
	"time"
)

// Mailto is the address this toolchain identifies itself with.
//
// It is not decoration. Crossref, Unpaywall and OpenAlex run a polite pool and
// a shared pool, and a request that says who it is lands in the polite one. A
// client that stays anonymous gets the rate limit it deserves.
const Mailto = "tamnd87@gmail.com"

// MinInterval is the floor between two requests to the same host. One second
// is what the public APIs here ask for, and asking a free service for a
// hundred papers at once is how a free service stops being free.
const MinInterval = time.Second

// UserAgent is what this toolchain calls itself.
//
// The version is in it so that a service which needs to block one build of
// this tool can do that without blocking every build of it, and the contact
// address is in it so that somebody who wants it to stop has somewhere to
// write rather than only a firewall rule.
func UserAgent(version string) string {
	return fmt.Sprintf("papers-reader/%s (+https://github.com/tamnd/papers; %s)", version, Mailto)
}

// Gate holds requests back so that no host is asked twice inside Interval.
//
// The zero value works and does nothing, which is what a test wants. It is
// per host rather than global, because waiting a second before asking a
// different service helps nobody.
type Gate struct {
	// Interval is the floor between two requests to the same host. Zero or
	// less turns the gate off.
	Interval time.Duration
	// Now is the clock, so that a test does not have to spend real seconds
	// proving that the gate waits.
	Now func() time.Time

	mu   sync.Mutex
	last map[string]time.Time
}

// Wait blocks until this host may be asked again, and records that it is
// about to be.
func (g *Gate) Wait(host string) {
	if g == nil || g.Interval <= 0 {
		return
	}
	g.mu.Lock()
	if g.last == nil {
		g.last = map[string]time.Time{}
	}
	now := g.now()
	next := g.last[host].Add(g.Interval)
	if next.After(now) {
		g.last[host] = next
	} else {
		g.last[host] = now
	}
	g.mu.Unlock()

	if d := next.Sub(now); d > 0 {
		time.Sleep(d)
	}
}

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}
