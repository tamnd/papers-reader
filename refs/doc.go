// Package refs parses bibliographies and links the citations.
//
// A reference that resolves to a paper in the corpus becomes an edge in the
// citation graph. A reference that does not is kept as it was printed, with
// whatever identifiers were recoverable, because a bibliography with its
// unresolvable entries quietly dropped is not a bibliography.
//
// Arrives in milestone M4.
package refs
