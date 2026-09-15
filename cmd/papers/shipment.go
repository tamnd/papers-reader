package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/audit"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/publish"
)

// A shipment is what a translation run may push and what it may not.
//
// The run writes one file at a time and used to push every twenty of them,
// whichever twenty they happened to be. That put the MapReduce paper into
// the corpus with two of its twelve sections in Vietnamese and three in
// Japanese and the rest missing, which is a paper nobody can read and which
// audit rules T04 and T06 both refuse. Half a paper is worse than none of
// it: none of it is a paper waiting its turn and half of it is a paper that
// looks done from the outside.
//
// So a batch carries whole papers only. A paper goes out when the run owes
// it nothing further and every English file of it has a counterpart in every
// language the run is writing. A paper with a file the run could not write
// is held back entire rather than pushed short: it stays in the working
// tree, the next run is owed the one file it is missing, and it goes out
// then.
//
// A batch also carries nothing a hard audit rule refuses. See audited.
//
// The count that decides when to push is still a count of files, because
// that is what the flag has always meant and because a batch measured in
// papers would be forty files for the GPT-3 paper and one for a stub.
type shipment struct {
	c     *corpus.Corpus
	langs []corpus.Lang

	// owed is how many files of a paper this run has still to write, and it
	// is what keeps a batch from going out down the middle of a paper that
	// was already whole on disk and is being written again.
	owed map[string]int
	// broken is a paper with a file the run could not write.
	broken map[string]bool
	// ready is the papers finished and whole since the last push, and files
	// is how many files stand behind them.
	ready []string
	files int
	// held is the papers finished and not published, in the order they were
	// held, and why is what to say about each. A person reading the log wants
	// to know that a paper was deliberately left in the working tree rather
	// than lost, and which of the two reasons it was.
	held []string
	why  map[string]string

	// findings is the hard audit of the corpus as it stands, and is what
	// decides whether a finished paper is one to publish. It is a field and
	// not a call because a shipment is worth testing on its own and the audit
	// wants a hundred papers to have anything to say. The run sets it to
	// hardFindings. Nothing set is nothing checked.
	findings func() ([]audit.Finding, error)
}

// newShipment counts what the run owes each paper.
func newShipment(c *corpus.Corpus, langs []corpus.Lang, jobs []job) *shipment {
	s := &shipment{c: c, langs: langs, owed: map[string]int{}, broken: map[string]bool{}, why: map[string]string{}}
	for _, j := range jobs {
		s.owed[j.front.Paper]++
	}
	return s
}

// done records one finished file and whether it was written.
func (s *shipment) done(paper string, ok bool) {
	if !ok {
		s.broken[paper] = true
	}
	s.owed[paper]--
	if s.owed[paper] > 0 {
		return
	}
	delete(s.owed, paper)
	switch {
	case s.broken[paper]:
		s.hold(paper, "a file of it could not be written")
	case !s.whole(paper):
		s.hold(paper, "it is not whole in every language the run is writing")
	default:
		s.ready = append(s.ready, paper)
		s.files += s.count(paper)
	}
}

// hold keeps a paper in the working tree and says why.
func (s *shipment) hold(paper, why string) {
	if _, was := s.why[paper]; !was {
		s.held = append(s.held, paper)
	}
	s.why[paper] = why
}

// pending is how many files are waiting to go out.
func (s *shipment) pending() int { return s.files }

// take is the paths the next batch stages, and empties the queue.
//
// The paper directories of the languages being written, and then the roots
// that are not a paper. Nothing under content is staged wholesale, because
// the whole point is that the half-written paper two lanes over stays where
// it is.
//
// Only paths that exist, because git add is given these verbatim and errors
// on a pathspec that matches nothing, which would stop a batch over a
// language a paper has no directory for.
func (s *shipment) take() []string {
	var paths []string
	s.audited()
	sort.Strings(s.ready)
	for _, id := range s.ready {
		for _, l := range s.langs {
			dir := s.c.Content(l, id)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			if rel, err := filepath.Rel(s.c.Root, dir); err == nil {
				paths = append(paths, rel)
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}
	for _, r := range publish.Roots {
		if r == "content" {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.c.Root, r)); err == nil {
			paths = append(paths, r)
		}
	}
	s.ready, s.files = nil, 0
	return paths
}

// audited holds back the papers a hard audit rule refuses.
//
// The run used to push whatever it had finished and merge it without waiting
// for anything, because the raw translations were going in first and the
// rules were still being written against what the models actually write. The
// corpus passed no rules then and there was nothing to gate on. It passes all
// seventy four of them now, CI runs them on every pull request, and a run
// that merges past a red check puts the corpus in a state a person has to go
// and fix by hand. Two batches did exactly that, in tamnd/papers#38 and #39:
// both merged with the audit failing, and main was red until somebody
// noticed.
//
// The whole hard audit runs, and the papers it names are held. Not the batch,
// because a rule that fires on one translation is no reason to keep the other
// four in the working tree, and not just this run's files either: a paper
// whose English has gone stale under a translation is the same broken paper
// whoever wrote it.
//
// A finding that names no file is printed and holds nothing. Those are the
// rules about the repository rather than about a paper, and there is no
// honest paper to blame one on. If one of them is failing then CI will say
// so on the pull request, which is the right place for a thing that is true
// of the whole corpus.
//
// It costs a few seconds a batch over a hundred papers, which against the
// minutes a chunk takes is nothing.
func (s *shipment) audited() {
	if s.findings == nil || len(s.ready) == 0 {
		return
	}
	found, err := s.findings()
	if err != nil {
		fmt.Printf("the batch could not be audited and is held back entire: %v\n", err)
		for _, id := range s.ready {
			s.hold(id, "the corpus could not be loaded to audit it")
		}
		s.ready, s.files = nil, 0
		return
	}
	bad := map[string]string{}
	for _, f := range found {
		id := blamed(f.File, s.ready)
		if id == "" {
			fmt.Printf("%s\n", f)
			continue
		}
		if _, was := bad[id]; !was {
			bad[id] = f.String()
		}
	}
	if len(bad) == 0 {
		return
	}
	keep := s.ready[:0]
	for _, id := range s.ready {
		if found, no := bad[id]; no {
			s.files -= s.count(id)
			s.hold(id, "a hard audit rule refuses it: "+found)
			continue
		}
		keep = append(keep, id)
	}
	s.ready = keep
}

// hardFindings runs the hard audit rules over the corpus and returns
// everything they found. It reloads each time, because the point of it is to
// read the files the run has just written.
func hardFindings(c *corpus.Corpus) func() ([]audit.Finding, error) {
	return func() ([]audit.Finding, error) {
		in, err := audit.Load(c)
		if err != nil {
			return nil, err
		}
		if err := pending(c, in); err != nil {
			return nil, err
		}
		var out []audit.Finding
		for _, res := range audit.Run(in, true).Results {
			if res.Err != nil {
				return nil, fmt.Errorf("rule %s: %w", res.Rule.ID, res.Err)
			}
			out = append(out, res.Findings...)
		}
		return out, nil
	}
}

// pending counts what the run has written and not yet committed as tracked,
// because the gate is asking whether the corpus is sound after this batch and
// the batch is what puts those files into git.
//
// Two rules read git's index rather than the disk, F04 for a figure and S03
// for a PDF, and they are right to: a figure that was rendered and never
// committed is a figure the site asks for and does not get. But the publish
// gate runs a few seconds before the commit that would hold it, so every
// paper with a picture in it was refused for a figure that was on its way in.
// The Paxos paper is two figures and it was held back by both of them.
//
// A nil tracked list means this is not a git checkout and the two rules stand
// down. It stays nil here, because a list built out of git status without a
// git to ask would be a list of nothing dressed up as an answer.
func pending(c *corpus.Corpus, in *audit.Input) error {
	if in.Tracked == nil {
		return nil
	}
	changes, err := publish.Changed(context.Background(), publish.Exec, c.Root, publish.Roots)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		in.Tracked = append(in.Tracked, ch.Path)
	}
	return nil
}

// blamed is the paper of the batch a finding is about, or the empty string
// for a finding about something else.
//
// By path segment rather than by prefix, because a finding names a file
// under content, under figures or under manifests and the identifier sits in
// a different place in each. The identifiers are long and hyphenated and
// nothing else in a path looks like one.
func blamed(file string, ready []string) string {
	if file == "" {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(file), "/")
	for _, id := range ready {
		for _, p := range parts {
			if p == id {
				return id
			}
		}
	}
	return ""
}

// whole says whether every English file of a paper has a counterpart in
// every language the run is writing.
//
// A paper with no English at all is not whole, because there is nothing for
// a translation of it to have come from. That case does not arise from plan,
// which reads the English to find the work, but it is the answer that keeps
// an empty directory from being published as a finished paper.
func (s *shipment) whole(id string) bool {
	want := names(s.c.Content(corpus.EN, id))
	if len(want) == 0 {
		return false
	}
	for _, l := range s.langs {
		if l == corpus.EN {
			continue
		}
		if strings.Join(names(s.c.Content(l, id)), " ") != strings.Join(want, " ") {
			return false
		}
	}
	return true
}

// count is how many files a whole paper puts in a batch.
func (s *shipment) count(id string) int {
	n := 0
	for _, l := range s.langs {
		if l == corpus.EN {
			continue
		}
		n += len(names(s.c.Content(l, id)))
	}
	return n
}

// names is the section files of one directory, sorted. A missing directory
// has none, which is not an error here: it is a paper that has not been
// written in that language yet.
func names(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
