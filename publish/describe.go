package publish

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// Describe writes the commit subject and the pull request body for a batch.
//
// A person reads these. The corpus takes a pull request every twenty minutes
// for a day and a half while a run goes, and the history of the repository
// is what is left afterwards, so a subject line of "automated update" would
// make that history worth nothing. What a reader wants from the subject is
// which papers moved and into which languages, and what they want from the
// body is the list, so that is what these are.
//
// The opening is the one sentence only the caller knows: which run this is,
// whether it is still going, what asked for it.
func Describe(changes []Change, opening string) (title, body string) {
	s := survey(changes)
	var b strings.Builder
	if opening != "" {
		fmt.Fprintf(&b, "%s\n", opening)
	}
	for _, l := range s.order {
		g := s.byLang[l]
		fmt.Fprintf(&b, "\n%s, %s in %s:\n\n", l.Name(), files(g.files), papers(len(g.order)))
		for _, id := range g.order {
			fmt.Fprintf(&b, "- %s, %s\n", id, files(g.byPaper[id]))
		}
	}
	if len(s.other) > 0 {
		fmt.Fprintf(&b, "\nAlso %s:\n\n", files(len(s.other)))
		for _, p := range s.other {
			fmt.Fprintf(&b, "- %s\n", p)
		}
	}
	return subject(s), strings.TrimRight(b.String(), "\n") + "\n"
}

// subject is the commit subject, which is also the pull request title.
//
// One physical line, always. A wrapped subject is truncated at the newline
// by the squash merge and the rest of it ends up in the body, which is how
// half a sentence gets into the history of the repository for good.
func subject(s *tally) string {
	if len(s.order) == 0 {
		return fmt.Sprintf("manifests: %s from the pipeline", files(len(s.other)))
	}
	var names []string
	for _, l := range s.order {
		names = append(names, s.byLang[l].Name())
	}
	into := list(names)
	if len(s.papers) == 1 {
		return fmt.Sprintf("content: %s in %s", s.papers[0], into)
	}
	return fmt.Sprintf("content: %s in %s", papers(len(s.papers)), into)
}

// A tally is the batch grouped the way the body prints it.
type tally struct {
	byLang map[corpus.Lang]*group
	order  []corpus.Lang
	papers []string
	other  []string
}

type group struct {
	corpus.Lang
	byPaper map[string]int
	order   []string
	files   int
}

// survey groups the changed paths.
//
// The languages come out in the order corpus.Langs has them and not in the
// order the run happened to finish them, because a reader comparing two
// batches should not have to work out that the Chinese moved above the
// Vietnamese for no reason. Everything that is not a content file goes in
// the other list under its own path: a batch usually has the glossary or a
// references manifest in it alongside the prose, and those are two lines
// rather than a section.
func survey(changes []Change) *tally {
	s := &tally{byLang: map[corpus.Lang]*group{}}
	seen := map[string]bool{}
	for _, c := range changes {
		lang, id, ok := content(c.Path)
		if !ok {
			s.other = append(s.other, c.Path)
			continue
		}
		g := s.byLang[lang]
		if g == nil {
			g = &group{Lang: lang, byPaper: map[string]int{}}
			s.byLang[lang] = g
		}
		if g.byPaper[id] == 0 {
			g.order = append(g.order, id)
		}
		g.byPaper[id]++
		g.files++
		if !seen[id] {
			seen[id] = true
			s.papers = append(s.papers, id)
		}
	}
	for _, l := range corpus.Langs {
		if s.byLang[l] != nil {
			s.order = append(s.order, l)
		}
	}
	for _, g := range s.byLang {
		sort.Strings(g.order)
	}
	sort.Strings(s.papers)
	sort.Strings(s.other)
	return s
}

// content pulls the language and the paper out of a content path, and says
// no for anything else.
func content(p string) (corpus.Lang, string, bool) {
	parts := strings.Split(p, "/")
	if len(parts) != 4 || parts[0] != "content" {
		return "", "", false
	}
	for _, l := range corpus.Langs {
		if string(l) == parts[1] {
			return l, parts[2], true
		}
	}
	return "", "", false
}

// files and papers are the counts as a person writes them.
func files(n int) string {
	if n == 1 {
		return "one file"
	}
	return fmt.Sprintf("%s files", count(n))
}

func papers(n int) string {
	if n == 1 {
		return "one paper"
	}
	return fmt.Sprintf("%s papers", count(n))
}

// count spells the small numbers out and prints the rest.
//
// Twelve is where a person switches over in ordinary prose and it is where
// the house style guide for this corpus switches over too.
func count(n int) string {
	words := []string{"no", "one", "two", "three", "four", "five", "six",
		"seven", "eight", "nine", "ten", "eleven", "twelve"}
	if n >= 0 && n < len(words) {
		return words[n]
	}
	return fmt.Sprint(n)
}

// list joins names the way a sentence does.
func list(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
