// Package book turns one paper in the corpus back into something to read away
// from a browser: a LaTeX source, a PDF, and an EPUB.
//
// The corpus is Markdown because Markdown is what a model can be asked for and
// what a person can correct by hand. It is not what anybody wants to read a
// paper in. This package is the other end of the pipeline from extract: the
// pages went in as pictures and come back out as a typeset document, in any of
// the four languages, with the mathematics set rather than printed as dollar
// signs and with the figures back in place under their captions.
//
// The three outputs share a model and split at the last moment. Load reads the
// content directory, the figures manifest and the refs manifest into a Book.
// LaTeX writes that Book as a document. Build runs tectonic over the document.
// EPUB writes the same Book as XHTML with the mathematics rendered to MathML
// by KaTeX, which is the same renderer the reading app will use, so the two
// agree on what a formula looks like by construction rather than by care.
//
// Nothing here fetches anything and nothing here asks a model. A book is built
// from committed files only, which is what makes it reproducible and what
// makes it safe: a paper the corpus may not republish has no body in the
// corpus, so a book of it comes out as its front matter and its abstract and
// stops, without this package needing to know the licence rule.
package book
