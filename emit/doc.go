// Package emit builds the JSON the reading app consumes.
//
// The app reads emitted JSON rather than the corpus itself, so the corpus
// layout can change without breaking a deployed site, and the site can be
// served as static files with no server between it and the reader.
//
// Four kinds of document. index.json is the catalogue, read on the first
// request of every visit. graph.json is the citation graph in the shape a
// layout wants. p/<id>/<lang>.json is one paper in one language, whole,
// which is one request to read a paper rather than one per section.
// search-<lang>.json is an inverted index over one language, fetched on the
// first search and not before.
//
// A page is a list of blocks and the block index is the alignment key.
// Every language of a paper has the same blocks in the same order with the
// same indices, which is what audit rules L03, L04, L16 and L18 enforce on
// the corpus, and it is why the splitter that cuts a body into blocks is
// package markdown and is shared with the books rather than written twice.
// Laying the English beside the Vietnamese is then a matter of putting
// block i against block i, with no diffing and no guessing.
//
// The mathematics is rendered here and not in the browser. KaTeX in the
// page would be three hundred kilobytes of JavaScript and a visible reflow
// on every paper, against markup that is already correct when the HTML
// arrives, and the TeX is carried beside the rendered form so a reader can
// copy the formula out and the search can index it as text.
//
// Everything here is derived. Nothing in this package writes to the corpus,
// and a site directory can be deleted and built again from a checkout at any
// time. The shape of what it writes is pinned by schema/site.schema.json in
// this repository, which the Go emitter and the TypeScript reader both
// validate against, and which audit rule P05 runs over a build of the corpus
// on every audit.
package emit
