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
// spans differ from the source's, paragraph by paragraph. An answer with a
// single difference is thrown away and asked for again, because a translation
// that has quietly renamed a variable or renumbered a footnote is worse than
// no translation at all: nothing further down the toolchain will ever catch
// it, and a reader has no way to know.
//
// Inside a paragraph the spans are a bag and not a list. That is not
// leniency, it is what the first Chinese and the first Japanese run taught:
// both stopped on section 3 of the GAN paper with three correct answers
// refused, because "the generator's distribution $p_g$ over data
// $\boldsymbol{x}$" has to come out with the two formulas the other way
// round in a language that puts a modifier in front of what it modifies.
// Compare says what that costs and why nothing cheaper will do.
//
// Chunks is the unit of work. A section is cut into pieces of about six
// thousand characters holding no more than sixty protected spans, on the
// blank lines the Markdown already has, and joining the answers back
// together with a blank line between them reproduces the section exactly. A
// block is never split, so a listing or a display equation that is over
// budget on its own goes alone rather than being cut in half. Over the
// hundred and forty nine English files in the corpus that is a hundred and
// eighty two chunks, a median of 1,771 characters, and none over budget.
//
// Translator.Body is the loop over the chunks. It refuses an answer three
// ways: on a span that differs, on a block count that differs, and on an
// answer that is the English handed back. The second and third are there
// because the first cannot see them. A model that opens with "Here is the
// Vietnamese translation:" has added no span, and a model that returns the
// source unchanged has spans that are, of course, identical.
//
// The measurement, on the first paper through it: nine sections of
// Generative Adversarial Nets into Vietnamese, eleven chunks, eleven asks,
// nothing refused, 32,931 input and 6,682 output tokens, and twenty four
// minutes on the subscription fleet. The mathematics of section 4 came back
// byte for byte including the two display equations of Algorithm 1.
//
// The first refusal on real data came on the Chinese of the same paper, and
// it was a real one: the source had the footnote marker "[^4]" and the
// answer had the citation "[4]", which is a footnote turned into a reference
// to the fourth paper in the bibliography. The retry was right. That is a
// corruption no reader would have spotted and no other stage would have
// caught, and it is the whole argument for asking again rather than
// repairing an answer.
package translate
