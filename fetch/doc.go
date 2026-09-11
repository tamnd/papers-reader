// Package fetch downloads the PDFs that the licence gate allows.
//
// It refuses anything that is not a real PDF, pins the SHA-256 of what it
// got, and writes nothing outside the gitignored pdf/ directory. A fetch is
// idempotent: a file whose hash already matches the record is not downloaded
// again.
//
// Arrives in milestone M1.
package fetch
