// Package mathtex repairs and validates the LaTeX that comes out of
// extraction.
//
// A model reading an equation off a page produces LaTeX that is usually right
// and occasionally unbalanced. This package finds the unbalanced cases before
// they reach a browser, normalises the spellings that differ without meaning
// anything, and refuses to guess when a repair would change the mathematics.
//
// Arrives in milestone M2.
package mathtex
