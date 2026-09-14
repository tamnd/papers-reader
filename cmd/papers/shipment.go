package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	// held is the papers finished and not whole, for the end of the run to
	// report. A person reading the log wants to know that a paper was
	// deliberately left in the working tree rather than lost.
	held []string
}

// newShipment counts what the run owes each paper.
func newShipment(c *corpus.Corpus, langs []corpus.Lang, jobs []job) *shipment {
	s := &shipment{c: c, langs: langs, owed: map[string]int{}, broken: map[string]bool{}}
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
	case s.broken[paper], !s.whole(paper):
		s.held = append(s.held, paper)
	default:
		s.ready = append(s.ready, paper)
		s.files += s.count(paper)
	}
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
