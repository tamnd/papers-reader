// Package schema is the contract between the emitter and the reading app.
//
// site.schema.json says what papers emit writes, and this package is the Go
// side of it: the file embedded so a validator does not depend on a checkout
// having it, and a Validate that runs it. The TypeScript side generates its
// types from the same file, so an interface in the app cannot be hand edited
// out of agreement with the emitter without the generated types changing
// under it.
//
// Audit rule P05 runs this over a build of the corpus on every audit, which
// is what makes the file a contract rather than documentation.
package schema

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// english is the printer the validator writes its complaints with.
//
// The library takes one and panics on a nil, and the alternative of letting
// it pick the machine's locale would mean an audit finding in one language
// on a laptop and another in CI. The rules are written in English and so is
// everything they report.
var english = message.NewPrinter(language.English)

// Site is schema/site.schema.json as it sits in this repository.
//
// Embedded rather than read off the disk because the validator runs inside
// papers audit, which is run against a corpus from wherever the binary
// happens to be, and a schema found by walking up from the working directory
// is a schema that can be the wrong one.
//
//go:embed site.schema.json
var Site []byte

// The kinds of document a build writes, named as they are in the schema. A
// caller passes one of these to Validate rather than a pointer into the
// schema, so that moving a definition inside the file is not a change to
// this package's callers.
//
// Index and Graph are also the paths those two are written to, because there
// is one of each. A page is one per paper per language, so Page is the kind
// and Kind below works out which kind a path is.
const (
	Index = "index.json"
	Graph = "graph.json"
	Page  = "page"
)

// definitions maps a kind of document to the subschema it is held to.
var definitions = map[string]string{
	Index: "index",
	Graph: "graph",
	Page:  "page",
}

// pagePath is p/<id>/<lang>.json, which is the only shape of name a build
// writes other than the two fixed ones.
var pagePath = regexp.MustCompile(`^p/[a-z0-9]+-[0-9]{4}-[a-z0-9]+/[a-z]{2}\.json$`)

// Kind says which definition a path in a build is held to.
//
// A path this does not recognise is a file in a build that the schema says
// nothing about, and the caller is rule P05, which reports that rather than
// skipping it. A build that grew a file nobody wrote a definition for is
// exactly the drift this package exists to catch.
func Kind(path string) (string, bool) {
	switch {
	case path == Index || path == Graph:
		return path, true
	case pagePath.MatchString(path):
		return Page, true
	}
	return "", false
}

// compiled is the schema compiled once. Compiling it is a few milliseconds
// and the audit validates two documents, which is not enough to matter; what
// it is really for is that the rest of the package can then be a pure
// function of its argument.
var compiled struct {
	once sync.Once
	set  map[string]*jsonschema.Schema
	err  error
}

// Schema is the compiled subschema for one document.
func Schema(doc string) (*jsonschema.Schema, error) {
	compiled.once.Do(compile)
	if compiled.err != nil {
		return nil, compiled.err
	}
	s, ok := compiled.set[doc]
	if !ok {
		return nil, fmt.Errorf("%s is not a document of the site schema", doc)
	}
	return s, nil
}

func compile() {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(Site)))
	if err != nil {
		compiled.err = fmt.Errorf("site.schema.json: %w", err)
		return
	}
	const url = "site.schema.json"
	c := jsonschema.NewCompiler()
	// The format keywords are assertions here rather than annotations,
	// which is off by default in 2020-12. A date-time that is not one is
	// the kind of thing this schema exists to catch, and a corpus is not a
	// place where somebody is relying on the lenient reading.
	c.AssertFormat()
	if err := c.AddResource(url, doc); err != nil {
		compiled.err = err
		return
	}
	compiled.set = map[string]*jsonschema.Schema{}
	for name, def := range definitions {
		s, err := c.Compile(url + "#/$defs/" + def)
		if err != nil {
			compiled.err = err
			return
		}
		compiled.set[name] = s
	}
}

// Validate holds one emitted document to the schema.
//
// It takes the path the document is written to rather than the kind, so
// that a caller walking a build can hand it every file it finds without
// working out what each one is. A path the schema has no definition for is
// an error and not a pass.
//
// The errors come back as a list of sentences, one per place the document is
// wrong, rather than as the one nested error the validator returns. A
// caller here is an audit rule that reports findings, and a rule that
// reported a whole validation tree as a single finding would be a rule
// nobody reads the output of.
func Validate(doc string, body []byte) ([]string, error) {
	kind, ok := Kind(doc)
	if !ok {
		return nil, fmt.Errorf("%s is not a document of the site schema", doc)
	}
	s, err := Schema(kind)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", doc, err)
	}
	err = s.Validate(v)
	if err == nil {
		return nil, nil
	}
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return nil, err
	}
	return sentences(invalid), nil
}

// sentences flattens a validation error into one line per leaf.
//
// The validator returns a tree whose root says the document is invalid and
// whose leaves say what is actually wrong, and only the leaves are worth
// printing. Where a leaf has no location it is the document itself that is
// wrong, and the line says so rather than printing an empty path.
func sentences(e *jsonschema.ValidationError) []string {
	if len(e.Causes) == 0 {
		where := e.InstanceLocation
		if len(where) == 0 {
			return []string{"the document " + e.ErrorKind.LocalizedString(english)}
		}
		return []string{"/" + strings.Join(where, "/") + ": " + e.ErrorKind.LocalizedString(english)}
	}
	var out []string
	for _, c := range e.Causes {
		out = append(out, sentences(c)...)
	}
	return out
}
