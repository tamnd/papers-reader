// Package emit builds the JSON the reading app consumes.
//
// The app reads emitted JSON rather than the corpus itself, so the corpus
// layout can change without breaking a deployed site, and the site can be
// served as static files with no server between it and the reader.
//
// Everything here is derived. Nothing in this package writes to the corpus,
// and a site directory can be deleted and built again from a checkout at any
// time. The shape of what it writes is pinned by schema/site.schema.json in
// this repository, which the Go emitter and the TypeScript reader both
// validate against, and which audit rule P05 runs over a build of the corpus
// on every audit.
package emit
