package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tamnd/papers-reader/corpus"
)

// A Site is everything a build of the corpus produces, held in memory.
//
// Built whole and then written, rather than written as it is built. The
// audit validates a build without writing anything, and a build that failed
// halfway would otherwise leave a site directory that is half of one corpus
// and half of the last, which is the kind of thing that is only noticed in a
// browser a week later.
type Site struct {
	Index *Index
	Graph *Graph
}

// Build reads the corpus and builds the site.
func Build(c *corpus.Corpus) (*Site, error) {
	ix, err := BuildIndex(c)
	if err != nil {
		return nil, err
	}
	g, err := BuildGraph(c)
	if err != nil {
		return nil, err
	}
	return &Site{Index: ix, Graph: g}, nil
}

// Files is the site as paths and bodies, in the order they are written.
//
// The JSON is indented with two spaces and ends in a newline. It is served
// gzipped, where the indentation costs almost nothing, and the difference it
// makes is that a diff of two builds is readable and a person can open the
// file and find out what the app is being told.
func (s *Site) Files() (map[string][]byte, error) {
	out := map[string][]byte{}
	for name, v := range map[string]any{"index.json": s.Index, "graph.json": s.Graph} {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
		out[name] = append(b, '\n')
	}
	return out, nil
}

// Write puts the site in a directory, making it if it is not there.
func (s *Site) Write(dir string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Summary is the one line a run prints.
func (s *Site) Summary() string {
	return fmt.Sprintf("version %d, %d papers, %d fields, %d reading lists, %d edges",
		s.Index.Version, len(s.Index.Papers), len(s.Index.Fields),
		len(s.Index.Collections), len(s.Graph.Edges))
}
