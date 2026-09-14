// Package markdown cuts the Markdown of the corpus into blocks and says what
// each one is.
//
// It holds no renderer. Three things render this corpus, the LaTeX of the
// books, the XHTML of the EPUBs and the JSON of the reading app, and they
// differ in every single case of the switch, so they are three renderers and
// not one with a flag. What they must not differ in is where one block ends
// and the next begins, and that is what lives here.
//
// The reading app is why this is a package and not three copies. A page of
// the site carries an index on every block, and the side by side view lays
// block i of the English against block i of the Vietnamese with no diffing
// and no heuristic. That index is only an alignment key if every language was
// cut the same way, and it is only worth trusting if the EPUB of the same
// paper was cut that way too.
//
// The patterns for the inline markup are here for the same reason and are
// used differently: each renderer holds its own spans and writes its own
// markup, but all three agree on what a span is.
package markdown
