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
	b.WriteString("# Written by `papers resolve` and `papers fetch`. Hand edits survive a re-run, so a record corrected by a person stays corrected.\n")
	b.WriteString("# A paper with no entry here, or with access: unknown, publishes nothing at all.\n\n")

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
