// Package split cuts one extracted document into one file per top level
// section, and assembles the pages into that document in the first place.
//
// The split has to be stable. A paper re-extracted by a better model must
// produce the same section boundaries wherever the sections did not change,
// or every tag in it moves and every translation goes stale at once.
//
// Arrives in milestone M5.
package split
