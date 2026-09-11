package main

import (
	"os"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/corpus"

	"gopkg.in/yaml.v3"
)

// writeSources rewrites manifests/sources.yaml from a set of records.
//
// Sorted by id, always, so that two runs in a different order produce the
// same file and a diff shows what changed rather than what moved. The header
// is rewritten with it, because a file a person is expected to read has to
// say what it is even though a program wrote it.
func writeSources(c *corpus.Corpus, byID map[string]corpus.Source) error {
	out := corpus.Sources{Sources: make([]corpus.Source, 0, len(byID))}
	for _, rec := range byID {
		out.Sources = append(out.Sources, rec)
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })

	var b strings.Builder
	b.WriteString("# Where each paper was found and what may be published from it.\n")
	b.WriteString("#\n")
	b.WriteString("# Written by `papers resolve`, `papers fetch` and `papers classify`.\n")
	b.WriteString("# A paper with no entry here, or with access: unknown, publishes nothing at all.\n")
	b.WriteString("#\n")
	b.WriteString("# The `by:` field says how much of a record came from a person. `pin` is a location somebody put in\n")
	b.WriteString("# papers.yaml, `seed` is the link the reading list arrived with, and no field at all means one of the\n")
	b.WriteString("# APIs found the paper on its own.\n")
	b.WriteString("#\n")
	b.WriteString("# To correct a record by hand, edit it and add a `by: hand` line. The resolver then leaves that paper\n")
	b.WriteString("# alone for good, including under --again, because somebody who read the publisher's own terms knows\n")
	b.WriteString("# more than the ladder does. Say in `note:` where you read them, so the next person does not repeat it.\n\n")

	enc, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	b.Write(enc)
	return os.WriteFile(c.SourcesManifest(), []byte(b.String()), 0o644)
}

// index is the records of a corpus keyed by id, which is how both resolve and
// fetch want them while they work.
func index(recorded *corpus.Sources) map[string]corpus.Source {
	byID := map[string]corpus.Source{}
	for _, rec := range recorded.Sources {
		byID[rec.ID] = rec
	}
	return byID
}
