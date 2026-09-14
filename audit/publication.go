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
//
// All six read one build, made once by Input.Site and shared, because
// building the corpus renders every formula in it through KaTeX.
func publicationRules() []Rule {
	return []Rule{
		{
			ID: "P01", Hard: true,
			What:  "every block of every page renders, and its HTML is on the allowlist.",
			Check: faultRule("P01", emit.FaultMath, emit.FaultMarkup),
		},
		{
			ID: "P02", Hard: true,
			What:  "every link a page carries has something at the other end.",
			Check: faultRule("P02", emit.FaultNote, emit.FaultPaper, emit.FaultCitation),
		},
		{
			ID: "P03", Hard: true,
			What:  "every figure a page shows is in the build.",
			Check: faultRule("P03", emit.FaultFigure),
		},
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
		{
			ID: "P06", Hard: true,
			What:  "every search result leads to a block that is in the build.",
			Check: ruleP06,
		},
	}
}

// faultRule is a rule that reports the faults of some kinds the build found.
//
// The three rules P01, P02 and P03 are one walk over one list, sorted into
// three piles by what went wrong, because that is how a person reads them:
// a formula that will not render is a different morning's work from a
// citation pointing at nothing. Writing them as three closures over one
// build rather than three functions keeps the pile each kind belongs in in
// one place, which is where somebody adding a fourth kind will look.
//
// All three are hard. A page that refers to something that is not there
// renders as a gap in a paper, and a corpus whose whole point is that the
// mathematics and the figures and the references came through intact cannot
// ship one of those as a warning.
func faultRule(id string, kinds ...string) func(*Input) ([]Finding, error) {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	return func(in *Input) ([]Finding, error) {
		site, err := in.Site()
		if err != nil {
			return nil, err
		}
		if len(site.Pages) == 0 {
			return nil, ErrNotRun
		}
		var out []Finding
		for _, f := range site.Faults {
			if !want[f.Kind] {
				continue
			}
			out = append(out, Finding{Rule: id, File: f.Page, Message: f.What})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].File != out[j].File {
				return out[i].File < out[j].File
			}
			return out[i].Message < out[j].Message
		})
		return out, nil
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
	site, err := in.Site()
	if err != nil {
		return nil, err
	}
	draft := map[string]bool{}
	for _, l := range site.Index.DraftLangs {
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
	site, err := in.Site()
	if err != nil {
		return nil, err
	}
	files, err := site.Files()
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, name := range emit.SortedNames(files) {
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

// ruleP06 checks that the search index agrees with the pages.
//
// The index is built from the pages, so in a correct build this rule can
// only fail if BuildSearch and BuildPages disagree about what a page holds.
// That is worth checking anyway, because the two are separate walks over the
// same structure and the failure they would produce is the worst kind: a
// search result that looks right in the list and scrolls to nothing when the
// reader clicks it. The schema pins a posting to a non-negative integer and
// nothing more, because a schema cannot count the posts.
//
// Hard, for the same reason the other build rules are. A result that goes
// nowhere is worse than no result.
func ruleP06(in *Input) ([]Finding, error) {
	site, err := in.Site()
	if err != nil {
		return nil, err
	}
	if len(site.Search) == 0 {
		return nil, ErrNotRun
	}
	blocks := map[string]bool{}
	for _, p := range site.Pages {
		at := string(p.Lang) + " " + p.ID
		for _, b := range p.Front.Blocks {
			blocks[fmt.Sprintf("%s  %d", at, b.I)] = true
		}
		for _, sec := range p.Sections {
			for _, b := range sec.Blocks {
				blocks[fmt.Sprintf("%s %s %d", at, sec.Anchor, b.I)] = true
			}
		}
	}
	var out []Finding
	for _, ix := range site.Search {
		file := emit.SearchPath(ix.Lang)
		say := func(format string, args ...any) {
			out = append(out, Finding{Rule: "P06", File: file, Message: fmt.Sprintf(format, args...)})
		}
		for _, post := range ix.Posts {
			key := fmt.Sprintf("%s %s %s %d", ix.Lang, post.Paper, post.Section, post.Block)
			if !blocks[key] {
				say("a result points at %s block %d of %s, which is not in the build",
					post.Section, post.Block, post.Paper)
			}
		}
		for _, name := range []string{"terms", "symbols"} {
			m := ix.Terms
			if name == "symbols" {
				m = ix.Symbols
			}
			for _, term := range ix.Sorted(m) {
				for _, at := range m[term] {
					if at < 0 || at >= len(ix.Posts) {
						say("the %s entry %q points at result %d of %d", name, term, at, len(ix.Posts))
					}
				}
			}
		}
	}
	return out, nil
}
