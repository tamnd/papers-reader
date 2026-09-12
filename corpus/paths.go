package corpus

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvRoot names the checkout of tamnd/papers.
const EnvRoot = "PAPERS_CORPUS"

// Corpus is a checkout of tamnd/papers, and the root every other path in this
// package hangs off.
type Corpus struct {
	Root string
}

// Open finds the corpus. It takes root if one is given, else PAPERS_CORPUS,
// else it walks up from the working directory looking for manifests/papers.yaml.
//
// The walk is there because the natural place to run these commands from is
// inside the corpus, and asking for an environment variable to be set before
// the first command is a bad first five minutes.
func Open(root string) (*Corpus, error) {
	if root == "" {
		root = os.Getenv(EnvRoot)
	}
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		found, err := FindRoot(wd)
		if err != nil {
			return nil, err
		}
		root = found
	}
	c := &Corpus{Root: root}
	if _, err := os.Stat(c.PapersManifest()); err != nil {
		return nil, fmt.Errorf("%s does not look like a checkout of tamnd/papers: %w", root, err)
	}
	return c, nil
}

// FindRoot walks up from dir looking for the corpus manifest.
func FindRoot(dir string) (string, error) {
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, "manifests", "papers.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no corpus found at or above %s: set %s to a checkout of tamnd/papers", start, EnvRoot)
		}
		dir = parent
	}
}

// Manifests is the manifests directory.
func (c *Corpus) Manifests() string { return filepath.Join(c.Root, "manifests") }

// PapersManifest is manifests/papers.yaml.
func (c *Corpus) PapersManifest() string { return filepath.Join(c.Manifests(), "papers.yaml") }

// CollectionsManifest is manifests/collections.yaml.
func (c *Corpus) CollectionsManifest() string {
	return filepath.Join(c.Manifests(), "collections.yaml")
}

// SourcesManifest is manifests/sources.yaml.
func (c *Corpus) SourcesManifest() string { return filepath.Join(c.Manifests(), "sources.yaml") }

// FiguresManifest is manifests/figures.yaml.
func (c *Corpus) FiguresManifest() string { return filepath.Join(c.Manifests(), "figures.yaml") }

// GlossaryManifest is manifests/glossary.yaml.
func (c *Corpus) GlossaryManifest() string { return filepath.Join(c.Manifests(), "glossary.yaml") }

// Refs is manifests/refs/<id>.yaml, the parsed bibliography of one paper.
func (c *Corpus) Refs(id string) string {
	return filepath.Join(c.Manifests(), "refs", id+".yaml")
}

// Content is the directory holding one paper in one language.
func (c *Corpus) Content(lang Lang, id string) string {
	return filepath.Join(c.Root, "content", string(lang), id)
}

// Figures is the directory holding the cropped figures of one paper.
func (c *Corpus) Figures(id string) string {
	return filepath.Join(c.Root, "figures", id)
}

// Tags is the tag register directory.
func (c *Corpus) Tags() string { return filepath.Join(c.Root, "tags") }

// Reports is the generated reports directory.
func (c *Corpus) Reports() string { return filepath.Join(c.Root, "reports") }

// PDF is the source PDF of a paper. It lives under an ignored directory and
// is never committed, under any licence.
func (c *Corpus) PDF(id string) string {
	return filepath.Join(c.Root, "pdf", id+".pdf")
}

// Images is the page rasters of one paper. Like the PDF they came from they
// live under an ignored directory: a page image is a picture of somebody's
// copyrighted paper whatever the rules about crops say, and it is derived
// besides, so a checkout that wants one renders it again.
func (c *Corpus) Images(id string) string {
	return filepath.Join(c.Root, "images", id)
}

// Work is scratch space for the toolchain: queues, caches and page images.
// Ignored by git and safe to delete.
func (c *Corpus) Work(parts ...string) string {
	return filepath.Join(append([]string{c.Root, "work"}, parts...)...)
}
