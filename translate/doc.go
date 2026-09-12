// Package translate produces the Vietnamese, Chinese and Japanese.
//
// The chunker never splits a sentence, an equation or a code block. The
// mathematics, the code and the tags pass through untouched, and the audit
// proves it rather than trusting it. Every translated file records the
// English file it came from and the hash of that file as it stood, so a later
// pass can list exactly which translations are answers to a question that has
// since changed.
//
// The proof is Protect and Compare. Protect finds every span of a body that a
// translation has to reproduce byte for byte, and Compare says how an answer's
// spans differ from the source's, position by position. An answer with a
// single difference is thrown away and asked for again, because a translation
// that has quietly renamed a variable or renumbered a footnote is worse than
// no translation at all: nothing further down the toolchain will ever catch
// it, and a reader has no way to know.
package translate
