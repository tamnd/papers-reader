// Package glossary is the controlled vocabulary.
//
// One rendering per term per language, chosen once and used everywhere. This
// is the cheapest part of the pipeline and skipping it is what produces a
// corpus that reads as several corpora, because two papers that translate
// "throughput" differently read as two projects.
//
// The glossary itself is manifests/glossary.yaml in the corpus, written and
// reviewed by hand. Load reads it, For hands the terms of one field to the
// prompt builder longest first, and Coverage is what the translate command
// gates on.
//
// Extract is the other half. It counts the English corpus and proposes the
// terms worth deciding, into manifests/glossary-candidates.yaml, which is a
// proposal and never a glossary: nothing is a term until somebody says so.
// What it counts is the prose, with the mathematics, the listings and the
// citation markers taken out first, because a corpus counted with those in
// proposes a glossary of variable names.
package glossary
