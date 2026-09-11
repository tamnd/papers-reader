// Package refs parses bibliographies and links the citations.
//
// The hundred papers cite each other, and the graph of who cites whom is the
// best reason to read them as a set, so the bibliography is worth parsing
// well. It is also the part of a paper a parser is most likely to get wrong:
// every publisher has a house style, every house style has exceptions, and a
// reference that has been through OCR has lost some of them.
//
// So raw is kept for every entry and is never discarded. The reference
// section on the page renders raw, which means a reference the parser read
// badly still reads correctly, and the structured fields are used for one
// thing only: deciding whether the reference names a paper the corpus
// already has. That split is what makes it safe to parse aggressively.
//
// A reference that resolves to a paper in the corpus becomes an edge in the
// citation graph. A reference that does not is kept as it was printed, with
// whatever identifiers were recoverable, because a bibliography with its
// unresolvable entries quietly dropped is not a bibliography.
package refs
