// Package fetch downloads the PDFs that the licence gate allows.
//
// It refuses anything that is not a real PDF, pins the SHA-256 of what it
// got, and writes nothing outside the gitignored pdf/ directory. A fetch is
// idempotent: a file whose hash already matches the record is not downloaded
// again.
//
// The two halves are deliberately separate. May is the licence gate and
// decides whether a paper may be downloaded at all, and Fetcher.Get is the
// download and decides whether what came back is a paper. Neither of them
// knows anything about the other, and both can be read on their own.
package fetch
