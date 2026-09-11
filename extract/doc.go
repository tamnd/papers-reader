// Package extract turns PDF pages into Markdown.
//
// Three paths, and they are not interchangeable. The native path runs
// pdftotext over a born digital file, and its output is the only text in the
// corpus that no model ever guessed at. The layout path sends page geometry
// to a model. The ocr path sends a page image. Which one was used is recorded
// in the front matter of every file, because a reader deserves to know
// whether a sentence was read or inferred.
//
// Arrives in milestone M2, with the vision path in M3.
package extract
