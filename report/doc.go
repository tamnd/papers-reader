// Package report writes what the corpus knows about itself: coverage per
// field and per language, the audit, what the machine time cost, and the
// citation graph over the corpus.
//
// These are committed files rather than a dashboard. A number that is in the
// repository can be read in a diff, and a number that changed for the worse
// is then visible in a review.
//
// A committed report is also a report anybody can read, which is the
// constraint the usage report is written under: it counts pages, tokens and
// seconds, and it names no host and no path with a user name in it.
//
// Coverage and usage are here. The citation graph arrives in milestone M7.
package report
