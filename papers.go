// Package papers builds the multilingual Markdown corpus in tamnd/papers out
// of the PDFs of a hundred computer science papers.
//
// The pipeline is: resolve where each paper can legally be fetched from, fetch
// it, measure what its text layer is worth, read it either natively or through
// a layout model or through vision OCR, cut out the figures, parse the
// bibliography, assemble the pages into sections, assign permanent tags,
// translate into Vietnamese, Chinese and Japanese, audit the result, and emit
// the JSON the reading app consumes.
//
// The subpackages carry the work. This root package holds the version string
// alone, so that an importer can depend on it without pulling in poppler, ssh
// or a model client.
package papers

// Version is set at build time with
// -ldflags "-X github.com/tamnd/papers-reader.Version=...".
var Version = "dev"
