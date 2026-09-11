// Package figures crops the diagrams out of the pages.
//
// A figure is a cropped diagram. It is never a whole page, it is capped in
// bytes, and it is capped as a fraction of the page it came from. Those caps
// are what separate this corpus from a mirror of copyrighted PDFs, and they
// are enforced by audit rule F06 rather than by good intentions.
//
// Arrives in milestone M3.
package figures
