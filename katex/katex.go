// Package katex renders TeX to HTML at build time, with no Node and no
// network, by running the real katex.min.js inside a JavaScript engine.
//
// It has two jobs and the second is the one that runs most often. The reading
// app is served the finished markup for every span, so a reader never waits
// for a renderer and never sees a flash of raw TeX. And extraction validates
// every span it writes by rendering it here, which is audit rule M04: a model
// that hands back a formula KaTeX cannot parse has misread the page, and the
// cheapest place to find that out is the minute the page comes back rather
// than in a browser after the corpus is published.
//
// Three ways to reach KaTeX from Go. Shelling out to Node puts a second
// toolchain in CI and on the laptop for a project that is otherwise Go and
// poppler. A Go implementation that handles what a hundred papers write does
// not exist, and writing one is not a trade worth making. Embedding a
// JavaScript engine and running the real file costs one vendored dependency
// and keeps the build pure Go, which is what this does.
//
// The file is vendored rather than fetched, and its SHA-256 is recorded in
// SHA256SUMS and checked by a test, for the same reason every source PDF has
// its hash written down: a dependency that can change under us without saying
// so is not a dependency, it is a risk.
//
// Carried over from bourbaki-solver, where it rendered six volumes.
package katex

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// Version is the KaTeX release vendored here. It is written down because the
// error messages below are its error messages and they move between releases.
const Version = "0.18.4"

//go:embed katex.min.js
var script string

//go:embed assets
var assetsFS embed.FS

// Assets is the stylesheet and the fonts a page needs to display what Render
// writes: assets/katex.min.css and assets/fonts/*.woff2.
//
// Only woff2 is vendored, of the three formats KaTeX ships. The stylesheet
// names all three in one src list and every browser that has shipped since 2016
// takes the first it understands, so the other two are bytes nobody would ever
// fetch.
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err) // the directory is embedded above; this cannot fail at run time
	}
	return sub
}

// Renderer holds one JavaScript engine with KaTeX loaded in it.
//
// Loading the script costs about a tenth of a second and rendering a span costs
// well under a millisecond, so the engine is built once and kept. A Renderer is
// safe to use from several goroutines: goja is not, and the lock is what makes
// up the difference.
type Renderer struct {
	mu    sync.Mutex
	vm    *goja.Runtime
	call  goja.Callable
	cache map[string]string
}

// New loads KaTeX into a fresh engine.
func New() (*Renderer, error) {
	vm := goja.New()
	if _, err := vm.RunString(script); err != nil {
		return nil, fmt.Errorf("loading katex %s: %w", Version, err)
	}
	fn, ok := goja.AssertFunction(vm.Get("katex").ToObject(vm).Get("renderToString"))
	if !ok {
		return nil, fmt.Errorf("katex %s has no renderToString", Version)
	}
	return &Renderer{vm: vm, call: fn, cache: map[string]string{}}, nil
}

// Render returns the HTML for one span of TeX.
//
// An error is a refusal by KaTeX and it is returned rather than swallowed. A
// span that does not parse is a fault in the extraction, and falling back to
// printing the raw TeX would put the fault on the page in a form that looks
// deliberate. The message is KaTeX's own, which names the character it stopped
// at.
func (r *Renderer) Render(tex string, display bool) (string, error) {
	return r.render(tex, display)
}

// Valid reports whether KaTeX will read a span, and is what extraction and
// audit rule M04 ask. It is Render with the HTML thrown away, deliberately:
// two ways of deciding whether a formula parses would eventually disagree,
// and the one that said yes would be the one nobody had looked at.
func (r *Renderer) Valid(tex string, display bool) error {
	_, err := r.render(tex, display)
	return err
}

func (r *Renderer) render(tex string, display bool) (string, error) {
	key := tex
	if display {
		key = "$$" + tex
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if html, ok := r.cache[key]; ok {
		return html, nil
	}

	opts := r.vm.NewObject()
	opts.Set("displayMode", display)
	// throwOnError is the default and is set anyway, because the alternative is
	// KaTeX writing the error into the page in red, which is the one behaviour
	// this must not have.
	opts.Set("throwOnError", true)
	// No macros, and it is meant to stay that way.
	//
	// A paper that uses a macro its own preamble defined is a paper whose
	// extraction has to expand the macro, because the corpus is read by four
	// languages and a reading app and none of them has that preamble. Teaching
	// this renderer the macro would make the span render here and nowhere
	// else, which is the worst of the three outcomes: it would look correct in
	// the audit and be broken on the page.
	//
	// KaTeX already knows \mathbb, \mathcal, \mathbf, \mathrm, \operatorname
	// and the rest of what papers actually write, so the table stays empty.
	opts.Set("macros", r.vm.NewObject())
	opts.Set("strict", false)

	v, err := r.call(goja.Undefined(), r.vm.ToValue(tex), opts)
	if err != nil {
		return "", errors.New(clean(err.Error()))
	}
	html := v.String()
	r.cache[key] = html
	return html, nil
}

// clean takes the engine's position off the end of a KaTeX error. The position
// is inside the one line of JavaScript this package evaluates and says nothing
// about the corpus, and the caller knows the file and the line that matter.
func clean(msg string) string {
	if i := strings.Index(msg, " at <eval>:"); i > 0 {
		msg = msg[:i]
	}
	msg = strings.TrimPrefix(msg, "ParseError: ")
	return strings.TrimPrefix(msg, "KaTeX parse error: ")
}
