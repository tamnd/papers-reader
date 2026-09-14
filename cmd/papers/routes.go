package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/route"
	"github.com/tamnd/papers-reader/work"
)

// runRoutes shows the hosts a run may ask, in the order it will try them.
//
// The table is not in this repository and will not be. Host names, ports, ssh
// destinations and the names of the environment variables holding keys are
// personal infrastructure, and a public repository is the wrong place for any
// of it. The registry the library ships is empty on purpose, so a fresh
// checkout asks nothing of anybody until a person writes the file.
func runRoutes(args []string) error {
	verb := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		verb, args = args[0], args[1:]
	}
	switch verb {
	case "show":
		return routesShow(args)
	case "init":
		return routesInit(args)
	}
	return fmt.Errorf("papers routes takes show or init, not %q", verb)
}

func routesShow(args []string) error {
	work.Configure()
	fs := flag.NewFlagSet("routes show", flag.ContinueOnError)
	file := fs.String("file", "", "read this route file rather than the usual one")
	probe := fs.Bool("probe", false, "ask each host whether it is up")
	deep := fs.Bool("deep", false, "with -probe, put one trivial question to each host as well")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers routes [show] [flags]

Prints the routing table: which hosts may be asked a question, in the order
they will be tried, and how many calls each will take at once.

The table is read from -file if there is one, else %s, else
%s. None of those is in a repository, and that is
deliberate: a host name is not something to commit to a public one.

With -probe each host is asked whether it is up, which is a GET and costs
nothing. With -deep it is also asked a one word question, which costs a
call and is the only way to find out that an account has been quietly moved
down to a smaller model than the route file names.

`, llm.EnvName("ROUTES"), route.DefaultPath())
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	registry, path, err := work.Routes(*file)
	if err != nil {
		// A file that does not validate is still a file worth looking at,
		// and this is the command for looking at it. The template routes
		// init writes fails validation on purpose, so this is the state
		// everybody's route file is in the first time they run this.
		shown, again := route.Read(path)
		if again != nil {
			return err
		}
		fmt.Printf("%d routes from %s\n\n", len(shown.Routes), path)
		fmt.Print(routeTable(shown.Routes))
		fmt.Println()
		return err
	}
	if len(registry.Routes) == 0 {
		fmt.Println("no routes are configured, so nothing can be asked anything yet")
		fmt.Printf("run papers routes init to write a template to %s\n", route.DefaultPath())
		return route.ErrNoRoutes()
	}
	if path == "" {
		path = "the library's own empty registry"
	}
	fmt.Printf("%d routes from %s\n\n", len(registry.Routes), path)
	fmt.Print(routeTable(registry.Enabled()))

	if disabled := len(registry.Routes) - len(registry.Enabled()); disabled > 0 {
		fmt.Printf("\n%d disabled\n", disabled)
		fmt.Print(routeTable(off(registry)))
	}
	if err := registry.Validate(); err != nil {
		fmt.Printf("\n%v\n", err)
	}
	// A job name that is not a stage is a route that will never be picked
	// for anything, and it looks exactly like a route that is configured.
	// The library cannot check this because the names are ours.
	for _, line := range strayJobs(registry) {
		fmt.Printf("\n%s\n", line)
	}
	if !*probe {
		return nil
	}

	pool := work.Pool(registry)
	pool.Prober = route.Prober{Deep: *deep}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fmt.Printf("\nprobing %d routes\n\n", len(pool.Routes()))
	fmt.Print(route.Table(pool.ProbeAll(ctx)))
	return nil
}

func routesInit(args []string) error {
	work.Configure()
	fs := flag.NewFlagSet("routes init", flag.ContinueOnError)
	file := fs.String("file", "", "write here rather than to the usual place")
	force := fs.Bool("force", false, "overwrite a route file that is already there")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers routes init [flags]

Writes a template route file, one entry per kind of host, with the
addresses, models and ranks left empty for you to fill in. Every entry is
incomplete on purpose: the file does not validate until somebody has said
what their own fleet is.

It goes to %s unless -file says otherwise, and an
existing file is never overwritten without -force.

`, route.DefaultPath())
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	path := *file
	if path == "" {
		path = route.DefaultPath()
	}
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s is already there, and a route file is somebody's own work: pass -force to overwrite it", path)
	}
	if err := route.Suggest().Write(path); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	fmt.Println("edit it, delete the kinds you do not have, then run papers routes -probe")
	return nil
}

// routeTable is the configured table, which is what the file says rather than
// what the hosts say. papers doctor and papers routes -probe report the
// second, and the two disagreeing is the interesting case.
func routeTable(routes []route.Route) string {
	width := len("route")
	kind := len("kind")
	model := len("model")
	jobs := len("jobs")
	for _, r := range routes {
		width = max(width, len(r.Name))
		kind = max(kind, len(string(r.Kind)))
		model = max(model, len(r.Model))
		jobs = max(jobs, len(jobsOf(r)))
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%-*s  %-*s  %-*s  %5s  %5s  %6s  %-*s  %s\n",
		width, "route", kind, "kind", model, "model", "rank", "lanes", "vision", jobs, "jobs", "note")
	for _, r := range routes {
		vision := "no"
		if r.Vision {
			vision = "yes"
		}
		fmt.Fprintf(&out, "%-*s  %-*s  %-*s  %5d  %5d  %6s  %-*s  %s\n",
			width, r.Name, kind, r.Kind, model, r.Model, r.Rank, r.Lanes(), vision, jobs, jobsOf(r), r.Note)
	}
	return out.String()
}

// jobsOf is the stages a route will serve, for the table. A route that names
// none serves all of them, and printing an empty cell for that reads as a
// route that serves nothing.
func jobsOf(r route.Route) string {
	if len(r.Jobs) == 0 {
		return "any"
	}
	return strings.Join(r.Jobs, ",")
}

// strayJobs is a line per route that names a job no stage answers to.
//
// The route file names stages by hand and a misspelled one is silent: the
// route is skipped by every pool and reads in the table as a route that is
// configured and is simply never chosen. work.Stages is the whole list, so
// this can say what was meant.
func strayJobs(registry route.Registry) []string {
	known := map[string]bool{}
	for _, s := range work.Stages {
		known[string(s)] = true
	}
	var out []string
	for _, r := range registry.Routes {
		var stray []string
		for _, j := range r.Jobs {
			if !known[strings.ToLower(strings.TrimSpace(j))] {
				stray = append(stray, j)
			}
		}
		if len(stray) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s names %s, which %s a stage, so no work will ever reach it under %s. the stages are %s",
			r.Name, strings.Join(stray, " and "), oneOrMore(len(stray), "is not", "are not"),
			oneOrMore(len(stray), "that name", "those names"), stageNames()))
	}
	return out
}

func oneOrMore(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// off is the routes the file has turned off. They are printed separately
// because a disabled route is a decision somebody made, and a table that
// hides it reads as though the route was never there.
func off(registry route.Registry) []route.Route {
	var out []route.Route
	for _, r := range registry.Routes {
		if r.Disabled {
			out = append(out, r)
		}
	}
	return out
}
