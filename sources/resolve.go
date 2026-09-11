package sources

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// Result is everything one resolution produced, including the parts that did
// not work.
//
// The near misses are kept deliberately. A paper that does not resolve leaves
// a person with two minutes of work, and the difference between two minutes
// and twenty is whether the report says "nothing found" or "Crossref offered
// this, and the year was four out".
type Result struct {
	ID string
	// Record is what would go into manifests/sources.yaml. It is only filled
	// in when a candidate was accepted.
	Record corpus.Source
	// Accepted is the candidate that won, and Rung is the step of the ladder
	// it came off.
	Accepted *Candidate
	Rung     string
	// Misses is every candidate that was looked at and refused, with the
	// scores that refused it.
	Misses []Miss
	// Notes are the things that went wrong on the way which are not a refused
	// candidate: a service that did not answer, a licence string nobody has a
	// rule for.
	Notes []string
}

// Miss is one refused candidate.
type Miss struct {
	Rung      string
	Candidate Candidate
	Verdict   Verdict
}

// OK reports whether the paper resolved.
func (r *Result) OK() bool { return r.Accepted != nil }

// Resolver runs the ladder. The fields are the rungs, so a test can replace
// one of them and every rung is reachable without a network.
type Resolver struct {
	Client *Client
	// Skip names rungs not to try, by the names used in Rungs. It exists for
	// --only and for debugging one service, and not for turning verification
	// off, which is not something this has a switch for.
	Skip map[string]bool
}

// Rungs, in the order they are tried.
var Rungs = []string{"pin", "arxiv", "crossref", "unpaywall", "openalex", "publisher", "seed"}

// Resolve walks the ladder for one paper and stops at the first candidate
// that is verified and has somewhere to fetch from.
//
// The order is not arbitrary. A pinned identifier wins because a person who
// has looked knows more than any API. arXiv comes next because it is the only
// rung where the identifier names exactly one thing and the licence is stated
// per submission. Crossref supplies the canonical DOI that the two rungs
// after it are keyed on. Unpaywall answers the actual question, which is
// whether a copy exists that anybody may fetch. OpenAlex is last of the
// services because its matching is the loosest. The publisher rules come
// after all of them because they are a rewrite applied to a landing page that
// the services found.
func (r *Resolver) Resolve(ctx context.Context, p corpus.Paper) *Result {
	res := &Result{ID: p.ID}
	want := Want{ID: p.ID, Title: p.Title, Authors: p.Authors, Year: p.Year, Seed: p.Seed}

	// Everything learned along the way, whichever rung learned it. A DOI from
	// Crossref is what Unpaywall is asked about, and a landing page from
	// Unpaywall is what the publisher rules rewrite.
	known := Candidate{DOI: CleanDOI(p.DOI), ArXiv: CleanArXivID(p.ArXiv), URL: p.URL}

	for _, rung := range Rungs {
		if r.Skip[rung] {
			continue
		}
		cands, err := r.try(ctx, rung, want, &known)
		if err != nil && !NotFound(err) {
			res.Notes = append(res.Notes, fmt.Sprintf("%s: %v", rung, err))
			continue
		}
		for _, cand := range cands {
			// A pin and a seed are not verified against the manifest,
			// because they came from the manifest. There is nothing to check
			// them against and no candidate metadata to check: a URL is a
			// URL. What still applies to both is the licence gate, and that
			// is the check that decides whether anything gets published.
			verdict := Verdict{OK: true, Why: "written in the manifest"}
			if rung != "pin" && rung != "seed" {
				verdict = Verify(want, cand)
			}
			if !verdict.OK {
				res.Misses = append(res.Misses, Miss{Rung: rung, Candidate: cand, Verdict: verdict})
				continue
			}
			absorb(&known, cand)
			if known.URL == "" {
				// Verified, and still nowhere to fetch from. Keep what it
				// taught us and carry on down the ladder.
				continue
			}
			r.licence(ctx, &known, res)
			res.Accepted, res.Rung = &known, rung
			res.Record = record(p, known, res)
			return res
		}
	}

	sort.SliceStable(res.Misses, func(i, j int) bool {
		return res.Misses[i].Verdict.TitleScore > res.Misses[j].Verdict.TitleScore
	})
	return res
}

// try runs one rung and returns what it offered, best first.
func (r *Resolver) try(ctx context.Context, rung string, want Want, known *Candidate) ([]Candidate, error) {
	switch rung {
	case "pin":
		return r.pin(ctx, *known)
	case "arxiv":
		if known.ArXiv != "" {
			cand, err := ArXivByID(ctx, r.Client, known.ArXiv)
			if err != nil {
				return nil, err
			}
			return []Candidate{cand}, nil
		}
		return ArXivByTitle(ctx, r.Client, want.Title)
	case "crossref":
		if known.DOI != "" {
			cand, err := CrossrefByDOI(ctx, r.Client, known.DOI)
			if err != nil {
				return nil, err
			}
			return []Candidate{cand}, nil
		}
		return CrossrefByTitle(ctx, r.Client, want.Title, want.Year)
	case "unpaywall":
		if known.DOI == "" {
			return nil, errNotFound
		}
		cand, err := Unpaywall(ctx, r.Client, known.DOI)
		if err != nil {
			return nil, err
		}
		return []Candidate{cand}, nil
	case "openalex":
		if known.DOI != "" {
			cand, err := OpenAlexByDOI(ctx, r.Client, known.DOI)
			if err != nil {
				return nil, err
			}
			return []Candidate{cand}, nil
		}
		return OpenAlexByTitle(ctx, r.Client, want.Title, want.Year)
	case "publisher":
		for _, landing := range []string{known.Landing, known.URL} {
			if cand, ok := Publisher(landing); ok {
				return []Candidate{cand}, nil
			}
		}
		return nil, errNotFound
	case "seed":
		if want.Seed == "" {
			return nil, errNotFound
		}
		// No licence, deliberately. A link on a reading list says where a
		// copy is and says nothing at all about what may be done with it.
		return []Candidate{{URL: want.Seed, Landing: want.Seed, Source: "seed"}}, nil
	}
	return nil, fmt.Errorf("there is no rung called %q", rung)
}

// pin turns the manifest's own identifiers into a candidate.
//
// A literal url is taken as given. An arXiv id is expanded, because the shape
// of an arXiv URL is known and there is no reason to make somebody type it.
// A bare DOI is not enough on its own, since a DOI is not a file, so it is
// left for the rungs below to turn into one.
func (r *Resolver) pin(ctx context.Context, known Candidate) ([]Candidate, error) {
	switch {
	case known.URL != "":
		cand := Candidate{URL: known.URL, Landing: known.URL, DOI: known.DOI, ArXiv: known.ArXiv, Source: "pin"}
		if known.ArXiv != "" {
			cand.Landing = "https://arxiv.org/abs/" + known.ArXiv
		}
		return []Candidate{cand}, nil
	case known.ArXiv != "":
		cand, err := ArXivByID(ctx, r.Client, known.ArXiv)
		if err != nil {
			return nil, err
		}
		cand.Source = "pin"
		return []Candidate{cand}, nil
	}
	return nil, errNotFound
}

// licence fills in a licence for a paper that has a location and no
// permission, which is the state a pinned arXiv id arrives in.
//
// Only arXiv can answer this, and only for arXiv, because it is the one
// service on the ladder that records a licence per submission rather than per
// journal. Anywhere else, a paper with a URL and no licence stays unknown and
// publishes nothing, which is the right answer.
func (r *Resolver) licence(ctx context.Context, known *Candidate, res *Result) {
	if known.Licence != "" || known.ArXiv == "" || r.Client == nil || r.Skip["arxiv"] {
		return
	}
	licence, stated, err := ArXivLicence(ctx, r.Client, known.ArXiv)
	if err != nil {
		if !NotFound(err) {
			res.Notes = append(res.Notes, fmt.Sprintf("arXiv would not say what licence this is under: %v", err))
		}
		return
	}
	known.Licence = licence
	if !stated {
		res.Notes = append(res.Notes, "the author chose no licence, so this is arXiv's default permission to redistribute and not the author's own")
	}
}

// absorb folds a candidate into what is known, without overwriting anything
// already established. A rung further down the ladder adds what the rungs
// above did not have and does not get to contradict them.
func absorb(known *Candidate, got Candidate) {
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&known.Title, got.Title},
		{&known.DOI, got.DOI},
		{&known.ArXiv, got.ArXiv},
		{&known.URL, got.URL},
		{&known.Landing, got.Landing},
		{&known.Abstract, got.Abstract},
		{&known.Source, got.Source},
	} {
		if *f.dst == "" {
			*f.dst = f.src
		}
	}
	if len(known.Authors) == 0 {
		known.Authors = got.Authors
	}
	if known.Year == 0 {
		known.Year = got.Year
	}
	// The licence is the exception: the most permissive one found wins rather
	// than the first, because a paper being on arXiv under one licence and in
	// a repository under another means both are true.
	if got.Licence != "" {
		_, have := Licence(known.Licence)
		_, incoming := Licence(got.Licence)
		if known.Licence == "" || Better(incoming, have) {
			known.Licence = got.Licence
		}
	}
}

// record builds the sources.yaml entry.
//
// Note that what may be published comes from the licence and nothing else.
// Not from the manifest's expect field, which is a prediction, and not from
// the paper having been found, which only means it exists. Being able to
// download something is not permission to republish it, so a paper with no
// licence lands on restricted and stays there until somebody finds one.
func record(p corpus.Paper, cand Candidate, res *Result) corpus.Source {
	name, access := Licence(cand.Licence)
	rec := corpus.Source{
		ID:      p.ID,
		Access:  access,
		Licence: name,
		URL:     cand.URL,
		Landing: cand.Landing,
	}
	// A site that publishes everything it hosts under one licence is a last
	// resort and only for a paper that has nothing better. USENIX and the RFC
	// editor are the two that really are that uniform.
	if rec.Access == corpus.AccessUnknown {
		if a, ok := PublisherAccess(cand.Source); ok {
			rec.Access = a
			if rec.Licence == "" {
				rec.Licence = publisherLicence(cand.Source)
			}
		}
	}
	// Then the host of whatever was accepted, which is a different question.
	// The rung says who found the file and the host says whose site it is
	// sitting on, and rights follow the second one. A paper on dl.acm.org
	// that a reading list linked to is under ACM's terms exactly as much as
	// one Crossref pointed at.
	if rec.Access == corpus.AccessUnknown {
		if r, ok := HostRule(cand.URL); ok && r.Access != corpus.AccessUnknown {
			rec.Access = r.Access
			if rec.Licence == "" {
				rec.Licence = r.Licence
			}
			res.Notes = append(res.Notes, fmt.Sprintf("classified from the host: %s publishes under one set of terms and this copy is on it", r.Host))
		}
	}
	// Last, a paper that was found and has no licence anybody can name.
	// Somebody holds the rights and it is not us, and that is exactly what
	// restricted means: the bibliographic record and a short abstract from the
	// metadata, no body text and no figures. All rights reserved is the
	// default for any work that does not say otherwise, so this is not a guess
	// about the paper, it is the law applied to the absence of a licence.
	//
	// It is also a far more useful answer than unknown, which would publish
	// nothing at all about a paper whose title, authors and year are not in
	// doubt. Unknown is kept for what it is actually for: a paper that did not
	// resolve to anything, so there is nothing to say about it and nothing to
	// publish.
	if rec.Access == corpus.AccessUnknown && rec.URL != "" {
		rec.Access = corpus.AccessRestricted
		if cand.Licence != "" {
			// A licence string nobody here has a rule for is a gap in the
			// rules. It gets the cautious class like everything else and it
			// gets said out loud, because the paper may well be open and a
			// person reading the report can add the rule.
			res.Notes = append(res.Notes, fmt.Sprintf("no rule for the licence %q, so this is treated as all rights reserved until somebody adds one", cand.Licence))
		} else {
			res.Notes = append(res.Notes, "no licence is recorded anywhere, so it is treated as all rights reserved and publishes front matter and a short abstract only")
		}
	}
	if p.Expect != "" && p.Expect != rec.Access {
		res.Notes = append(res.Notes, fmt.Sprintf("the manifest expected %s and the licence says %s", p.Expect, rec.Access))
	}
	return rec
}

// publishable is the paragraph the whole resolve stage exists to produce:
// how many of the papers may have their text published, how many get their
// front matter and an abstract, and how many produce nothing at all. The
// number that matters is the first one, and it is a good deal smaller than
// the number of papers that resolved, which is the point.
func publishable(results []*Result) string {
	counts := map[corpus.Access]int{}
	for _, r := range results {
		counts[r.Record.Access]++
	}
	var body, abstract int
	for a, n := range counts {
		switch {
		case a.Body():
			body += n
		case a.Abstract():
			abstract += n
		}
	}

	var b strings.Builder
	b.WriteString("## What may be published\n\n")
	fmt.Fprintf(&b, "%d of these may have their text published, %d get front matter and a short abstract only, and %d publish nothing at all.\n",
		body, abstract, len(results)-body-abstract)
	fmt.Fprintf(&b, "The count that the licence work is measured by, public domain plus open, is %d.\n\n",
		counts[corpus.AccessPublicDomain]+counts[corpus.AccessOpen])
	b.WriteString("| access | papers | what it publishes |\n| --- | --- | --- |\n")
	for _, a := range corpus.Accesses {
		fmt.Fprintf(&b, "| %s | %d | %s |\n", a, counts[a], publishes(a))
	}
	b.WriteString("\n")
	return b.String()
}

// publishes says in words what each class allows, so that the table is
// readable by somebody who has not read the specification.
func publishes(a corpus.Access) string {
	switch {
	case a.Body() && a.Figures():
		return "the full text, the mathematics and the figures"
	case a.Abstract():
		return "title, authors, year, links and an abstract under 250 words"
	}
	return "nothing"
}

// Markdown renders reports/resolve.md.
//
// The report is ordered by what needs a person: the papers that did not
// resolve come first, with their near misses, because that list is the work.
func Markdown(results []*Result) string {
	var b strings.Builder
	var ok, failed []*Result
	for _, r := range results {
		if r.OK() {
			ok = append(ok, r)
		} else {
			failed = append(failed, r)
		}
	}

	b.WriteString("# Resolve\n\n")
	fmt.Fprintf(&b, "%d papers, %d resolved, %d not.\n\n", len(results), len(ok), len(failed))
	b.WriteString(publishable(results))

	if len(failed) > 0 {
		b.WriteString("## Not resolved\n\n")
		b.WriteString("Each of these needs somebody to find the paper and put a url, doi or arxiv line in `manifests/papers.yaml`.\n")
		b.WriteString("The near misses are what the services offered and why it was refused.\n\n")
		for _, r := range failed {
			fmt.Fprintf(&b, "### %s\n\n", r.ID)
			for _, n := range r.Notes {
				fmt.Fprintf(&b, "- %s\n", n)
			}
			for _, m := range r.Misses {
				fmt.Fprintf(&b, "- %s offered %q (%d), title %.3f, year %+d, authors %v: %s\n",
					m.Rung, m.Candidate.Title, m.Candidate.Year, m.Verdict.TitleScore, m.Verdict.YearOff, m.Verdict.AuthorHit, m.Verdict.Why)
			}
			if len(r.Notes) == 0 && len(r.Misses) == 0 {
				b.WriteString("- no service had anything at all\n")
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Resolved\n\n")
	b.WriteString("| paper | rung | access | licence |\n| --- | --- | --- | --- |\n")
	for _, r := range ok {
		licence := r.Record.Licence
		if licence == "" {
			licence = "none recorded"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.ID, r.Rung, r.Record.Access, licence)
	}
	b.WriteString("\n")

	var notes int
	for _, r := range ok {
		notes += len(r.Notes)
	}
	if notes > 0 {
		b.WriteString("## Resolved, with something to look at\n\n")
		for _, r := range ok {
			for _, n := range r.Notes {
				fmt.Fprintf(&b, "- %s: %s\n", r.ID, n)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
