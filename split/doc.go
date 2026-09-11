// Package split cuts one assembled document into one file per top level
// section.
//
// The split has to be stable. A paper re-extracted by a better model must
// produce the same section boundaries wherever the sections did not change,
// or every tag in it moves and every translation goes stale at once. That is
// why the numbering scheme is detected once per paper and then required, and
// why a heading found only by its typography is reported rather than trusted
// on a paper that numbers its sections.
//
// The pages are joined into the document by package assemble first.
package split
