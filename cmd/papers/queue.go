package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tamnd/llm/queue"
	"github.com/tamnd/papers-reader/work"
)

// runQueue shows what is still owed to a model, and is the one place a run
// that went wrong overnight can be put right.
//
// The queue is on disk because a round trip to one of these hosts is about
// two and a half minutes, so nothing worth doing fits in one process
// lifetime. A thousand pages is hours, laptops sleep, tunnels drop, and
// somebody will press Ctrl-C. What survives all of that is a directory of
// files, and this command is how a person reads it.
func runQueue(args []string) error {
	verb := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		verb, args = args[0], args[1:]
	}
	switch verb {
	case "show":
		return queueShow(args)
	case "list":
		return queueList(args)
	case "retry":
		return queueRetry(args)
	case "reap":
		return queueReap(args)
	case "drain":
		return queueDrain(args)
	}
	return fmt.Errorf("papers queue takes show, list, retry, reap or drain, not %q", verb)
}

// queueFlags is what every one of these needs: a corpus, and for most of them
// a stage.
func queueFlags(name, help string) (*flag.FlagSet, *string, *string) {
	fs := flag.NewFlagSet("queue "+name, flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	stage := fs.String("stage", "", "one stage only: "+stageNames())
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: papers queue %s [flags]\n\n%s\n\n", name, help)
		fs.PrintDefaults()
	}
	return fs, root, stage
}

func queueShow(args []string) error {
	fs, root, _ := queueFlags("show", `Prints the board: how many jobs are pending, leased, done, failed and
dead in each stage, and how many leases have expired.

An expired lease is the number that matters after a crash. It does not
mean work is in flight, it means a worker died holding it, and the job
comes back to pending the next time anything runs reap.

The stages are `+stageNames()+`. Each is a directory under
work/queue, which is gitignored: a queue is a list of what a model was
asked, and it has no business in a public repository.`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	q, err := work.Queue(c)
	if err != nil {
		return err
	}
	rows, err := q.StatsAll()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n\n", q.Root)
	fmt.Print(queue.Table(rows))

	total := 0
	for _, row := range rows {
		total += row.Total()
	}
	if total == 0 {
		fmt.Println("\nnothing has been queued yet. The stages that put questions to a model arrive in M2.")
	}
	return nil
}

func queueList(args []string) error {
	fs, root, stage := queueFlags("list", `Lists the jobs in one state, with what went wrong on each attempt.

Dead is the state worth reading. A dead job is a page or a chunk that
three attempts could not get, and the corpus has a hole where it should
be, so somebody has to look at the reason and decide.`)
	state := fs.String("state", "dead", "which state: "+stateNames())
	if err := fs.Parse(args); err != nil {
		return err
	}
	want, err := queue.ParseState(*state)
	if err != nil {
		return err
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	q, err := work.Queue(c)
	if err != nil {
		return err
	}
	stages, err := pick(q, *stage)
	if err != nil {
		return err
	}

	shown := 0
	for _, s := range stages {
		jobs, err := q.List(s, want)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			shown++
			fmt.Printf("%-10s %-40s %d attempts\n", job.Stage, job.Target, job.Attempts)
			for _, event := range job.History {
				if event.OK {
					continue
				}
				fmt.Printf("    %s  %-12s %s\n", event.TS.Format(time.DateOnly), event.Host, event.Reason)
			}
		}
	}
	fmt.Printf("%d %s\n", shown, want)
	return nil
}

func queueRetry(args []string) error {
	fs, root, stage := queueFlags("retry", `Puts failed and dead jobs back in pending, with their attempts cleared.

This is for after the thing that was breaking them has been fixed: a
session signed back in, a prompt corrected, a model swapped for one that
can count. It is not for running the same broken thing again, which is
what the attempt bound is there to stop.`)
	states := fs.String("state", "failed,dead", "which states to bring back")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var want []queue.State
	for _, name := range strings.Split(*states, ",") {
		if strings.TrimSpace(name) == "" {
			continue
		}
		state, err := queue.ParseState(name)
		if err != nil {
			return err
		}
		want = append(want, state)
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	q, err := work.Queue(c)
	if err != nil {
		return err
	}
	stages, err := pick(q, *stage)
	if err != nil {
		return err
	}
	moved := 0
	for _, s := range stages {
		n, err := q.Retry(s, want...)
		if err != nil {
			return err
		}
		moved += n
	}
	fmt.Printf("%d jobs are pending again\n", moved)
	return nil
}

func queueReap(args []string) error {
	fs, root, stage := queueFlags("reap", `Brings back jobs whose worker died holding them.

A worker claims a job by renaming it, with a deadline on the claim. A
worker that is killed, or a laptop that sleeps through the deadline,
leaves the job in leased with a time in the past and nothing coming back
for it. This is the whole of crash recovery, and any run does it at
startup, so it is only worth running by hand when a board shows expired
leases and nothing is scheduled to run.`)
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	q, err := work.Queue(c)
	if err != nil {
		return err
	}
	stages, err := pick(q, *stage)
	if err != nil {
		return err
	}
	for _, s := range stages {
		ids, err := q.Reap(s)
		if err != nil {
			return err
		}
		for _, id := range ids {
			fmt.Printf("  %-10s %s came back\n", s, id)
		}
	}
	return nil
}

func queueDrain(args []string) error {
	fs, root, stage := queueFlags("drain", `Removes the pending jobs in a stage.

Done, failed and dead stay where they are. They are the record of what
happened, and a queue that forgets its dead jobs is a queue that reports
a clean run.`)
	yes := fs.Bool("yes", false, "actually remove them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stage == "" {
		return fmt.Errorf("say which stage to drain with -stage: %s", stageNames())
	}
	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	q, err := work.Queue(c)
	if err != nil {
		return err
	}
	stats, err := q.Stats(queue.Stage(*stage))
	if err != nil {
		return err
	}
	pending := stats.Counts[queue.Pending]
	if !*yes {
		fmt.Printf("%d pending jobs in %s would be removed, pass -yes to do it\n", pending, *stage)
		return nil
	}
	n, err := q.Drain(queue.Stage(*stage))
	if err != nil {
		return err
	}
	fmt.Printf("%d pending jobs removed from %s\n", n, *stage)
	return nil
}

// pick turns the -stage flag into the stages to work on. An empty flag means
// every stage that exists on disk, rather than every stage this program knows
// about, because a stage nobody has run has nothing to report.
func pick(q *queue.Queue, stage string) ([]queue.Stage, error) {
	if stage == "" {
		return q.Stages()
	}
	return []queue.Stage{queue.Stage(stage)}, nil
}

func stageNames() string {
	names := make([]string, len(work.Stages))
	for i, s := range work.Stages {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

func stateNames() string {
	names := make([]string, len(queue.States))
	for i, s := range queue.States {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}
