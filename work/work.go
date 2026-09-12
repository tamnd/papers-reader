// Package work is the model side of the toolchain: which hosts a run may ask,
// what is left to do, and what an unattended run does when every host is out
// of turns.
//
// Almost none of that is implemented here. It is github.com/tamnd/llm, which
// is a separate library because none of it is about papers, and what this
// package holds is the handful of decisions that are this project's rather
// than that library's. Which application this is, since that name decides the
// config directory, the environment variables and the prompt cache prefix.
// Where the queue lives inside a corpus. Which stages exist. And how long a
// run should sit still before it gives up, which is the one thing a library
// cannot decide for a caller.
//
// Nothing here calls a model. The stages that do arrive with M2.
package work

import (
	"sync"

	"github.com/tamnd/llm"
	"github.com/tamnd/llm/exec"
	"github.com/tamnd/llm/queue"
	"github.com/tamnd/llm/route"
	"github.com/tamnd/papers-reader/corpus"
)

// App is what this program calls itself to the llm library.
//
// It is not cosmetic. The name is the config directory ~/.config/papers, the
// prefix on PAPERS_ROUTES and the rest of the environment, and the prompt
// cache key on a proxy that several projects share. Two projects that share a
// cache prefix read each other's answers, which is a bad afternoon to debug.
const App = "papers"

var configured sync.Once

// Configure tells the llm library which application this is. It is safe to
// call from anywhere and it is called by everything in this package that
// reads a path, so no command has to remember to do it first.
func Configure() {
	configured.Do(func() { llm.Configure(llm.Config{App: App}) })
}

// The stages a job can be in. They are the subcommands that put questions to
// a model, and nothing else is a stage: resolving, fetching and classifying
// are deterministic, cost nothing to repeat, and would only clutter a board
// that exists to show what is still owed to a model.
const (
	Extract   queue.Stage = "extract"
	Figures   queue.Stage = "figures"
	Refs      queue.Stage = "refs"
	Glossary  queue.Stage = "glossary"
	Translate queue.Stage = "translate"
)

// Stages is every stage, in pipeline order rather than alphabetical order,
// because the order is what a board should be read in.
var Stages = []queue.Stage{Extract, Figures, Refs, Glossary, Translate}

// Queue opens the corpus's work list.
//
// It lives under work/, which is gitignored, along with the page images and
// everything else a rebuild can produce again. A queue in a public repository
// would be a list of what a model was asked, committed by accident.
//
// Every stage is named at open, so an empty stage shows on the board as a row
// of zeroes rather than not showing at all. There is a difference between a
// stage with no work left and a stage nobody has started, and a board that
// cannot tell them apart is a board that hides the second one.
func Queue(c *corpus.Corpus) (*queue.Queue, error) {
	Configure()
	return queue.Open(c.Work("queue"), Stages...)
}

// Routes loads the routing table: the named file if there is one, else
// PAPERS_ROUTES, else ~/.config/papers/routes.json. It returns the path it
// read, because a command that prints a table of hosts should say where the
// table came from.
//
// None of those paths is in a repository, and that is deliberate. Host names,
// ports and ssh destinations are personal infrastructure, the registry the
// library ships is empty, and a route file has never been committed to any of
// these three repositories.
func Routes(path string) (route.Registry, string, error) {
	Configure()
	return route.LoadOrDefault(path)
}

// Pool is the set of hosts that can be asked a question, in the order they
// will be tried.
func Pool(registry route.Registry) *route.Pool { return wire(route.NewPool(registry)) }

// Vision is the subset that will take a page image. It is built separately so
// that a run with pages to read finds out at the start that there is nowhere
// to send them, rather than an hour in.
func Vision(registry route.Registry) *route.Pool { return wire(route.NewVisionPool(registry)) }

// wire registers the transport for an exec route, which is the subscription
// on this machine, reached by running its CLI rather than by calling an
// endpoint. The route package cannot build one itself without importing
// llm/exec, and llm/exec imports llm/route, so the caller closes the loop.
//
// Both pools get it. The CLI reads a page image as well as a hosted model
// does: llm/exec writes the image where the program can open it and puts the
// path on the command line, so an exec route that declares vision is a reader
// like any other and the vision pool is where it belongs.
func wire(p *route.Pool) *route.Pool {
	p.Build = exec.Build
	return p
}
