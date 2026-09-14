package audit

import (
	"fmt"
	"sort"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/emit"
	"github.com/tamnd/papers-reader/glossary"
	"github.com/tamnd/papers-reader/schema"
)

// publicationRules is group P: the corpus as the reading app receives it.
//
// The other groups are about the corpus. These are about the build, and the
// distinction is worth keeping because a corpus can be entirely correct and
// still produce a site the app cannot render. The rules run over a build
// made in memory from the checkout rather than over a site directory
// somebody emitted earlier, so they say something about the corpus as it
// stands now and not about the last time anybody ran papers emit.
func publicationRules() []Rule {
	return []Rule{
		{
			ID: "P04", Hard: false,
			What:  "a language under the glossary coverage floor is emitted as a draft.",
			Check: ruleP04,
		},
		{
			ID: "P05", Hard: true,
			What:  "the emitted JSON validates against schema/site.schema.json.",
			Check: ruleP05,
		},
	}
}

// ruleP04 checks that a language the glossary does not cover is marked a
// draft in the emit.
//
// The reason it is a rule and not simply what the emitter does is that the
// emitter is one program and the honesty is the point. A corpus that is
// mostly machine translated against a glossary that covers two thirds of it,
// and that presents those pages as translations rather than as drafts, is
// making a claim it cannot support. This is the rule that says so, and it is
// soft because the answer is to finish the glossary rather than to stop
// emitting.
func ruleP04(in *Input) ([]Finding, error) {
	if len(in.Glossary.Terms) == 0 {
		return nil, ErrNotRun
	}
	ix, err := emit.BuildIndex(in.Corpus)
	if err != nil {
		return nil, err
	}
	draft := map[string]bool{}
	for _, l := range ix.DraftLangs {
		draft[string(l)] = true
	}
	var out []Finding
	for _, l := range corpus.Langs {
		if !l.Translated() || !in.Glossary.Under(l) {
			continue
		}
		if draft[string(l)] {
			continue
		}
		have, total := in.Glossary.Coverage(l)
		out = append(out, Finding{
			Rule: "P04", File: in.Corpus.GlossaryManifest(),
			Message: fmt.Sprintf("%s covers %d of %d glossary terms, which is under the floor of %d per cent, and the emit does not mark it a draft",
				l.Name(), have, total, glossary.Floor),
		})
	}
	return out, nil
}

// ruleP05 builds the site and holds every document of it to the schema.
//
// It is hard, because the schema is the contract with the reading app and a
// build that does not meet it is a deployed site that does not render. It is
// also the cheapest rule in the audit to satisfy and the most expensive one
// to discover the hard way: the alternative to failing here is a page that
// is blank in a browser on a Sunday.
func ruleP05(in *Input) ([]Finding, error) {
	site, err := emit.Build(in.Corpus)
	if err != nil {
		return nil, err
	}
	files, err := site.Files()
	if err != nil {
		return nil, err
	}
	var out []Finding
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bad, err := schema.Validate(name, files[name])
		if err != nil {
			return nil, err
		}
		for _, why := range bad {
			out = append(out, Finding{Rule: "P05", File: name, Message: why})
		}
	}
	return out, nil
}
