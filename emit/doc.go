// Package emit builds the JSON the reading app consumes.
//
// The app reads emitted JSON rather than the corpus itself, so the corpus
// layout can change without breaking a deployed site, and the site can be
// served as static files with no server between it and the reader.
//
// Arrives in milestone M8.
package emit
