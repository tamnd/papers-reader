package main

import (
	"strings"
	"testing"

	"github.com/tamnd/llm/route"
)

// A job name in a route file is typed by hand and nothing on the library
// side can check it, because the names are ours. A misspelled one is the
// worst kind of wrong: the route is skipped by every pool and reads in the
// table as a route that is configured and is simply never chosen.
func TestAJobThatIsNotAStageIsReportedWithTheListOfStages(t *testing.T) {
	registry := route.Registry{Routes: []route.Route{{
		Name: "typo", Kind: route.KindPool, BaseURL: "http://127.0.0.1:1/v1",
		Model: "m", Rank: 1, Concurrency: 1, Jobs: []string{"translte", "extract"},
	}}}
	lines := strayJobs(registry)
	if len(lines) != 1 {
		t.Fatalf("%d lines, want 1: %v", len(lines), lines)
	}
	for _, want := range []string{"typo", "translte", "translate"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("the line does not say %q: %q", want, lines[0])
		}
	}
	// The job it spelled right is not a complaint, or every route with one
	// typo in it reads as a route with nothing right in it.
	if strings.Count(lines[0], "extract") != 1 {
		t.Errorf("the line reports the job that is a stage: %q", lines[0])
	}
}

func TestARouteThatNamesOnlyStagesIsQuiet(t *testing.T) {
	registry := route.Registry{Routes: []route.Route{
		{Name: "reader", Kind: route.KindDirect, BaseURL: "http://127.0.0.1:1/v1",
			Model: "m", Rank: 1, Concurrency: 1, Jobs: []string{"extract", "figures"}},
		{Name: "general", Kind: route.KindPool, BaseURL: "http://127.0.0.1:2/v1",
			Model: "m", Rank: 2, Concurrency: 1},
	}}
	if lines := strayJobs(registry); len(lines) != 0 {
		t.Errorf("strayJobs reported a route that named stages: %v", lines)
	}
}

// A route that names no jobs does all of them, so the table has to say so.
// An empty cell there reads as a route that does nothing.
func TestTheTableSaysAnyForARouteThatNamesNoJobs(t *testing.T) {
	if got := jobsOf(route.Route{Name: "general"}); got != "any" {
		t.Errorf("jobsOf = %q, want %q", got, "any")
	}
}
