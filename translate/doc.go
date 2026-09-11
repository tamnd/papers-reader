// Package translate produces the Vietnamese, Chinese and Japanese.
//
// The chunker never splits a sentence, an equation or a code block. The
// mathematics, the code and the tags pass through untouched, and the audit
// proves it rather than trusting it. Every translated file records the
// English file it came from and the hash of that file as it stood, so a later
// pass can list exactly which translations are answers to a question that has
// since changed.
//
// Arrives in milestone M6.
package translate
