// Package report writes what the corpus knows about itself: coverage per
// field and per language, the audit, what the machine time cost, and the
// citation graph over the corpus.
//
// These are committed files rather than a dashboard. A number that is in the
// repository can be read in a diff, and a number that changed for the worse
// is then visible in a review.
//
// Arrives in milestone M7.
package report
