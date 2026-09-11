// Package glossary is the controlled vocabulary.
//
// One rendering per term per language, chosen once and used everywhere. This
// is the cheapest part of the pipeline and skipping it is what produces a
// corpus that reads as several corpora, because two papers that translate
// "throughput" differently read as two projects.
//
// Arrives in milestone M5.
package glossary
