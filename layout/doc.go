// Package layout reads what a layout model made of a page and builds the
// corpus's own Markdown from it.
//
// Three programs do this job well enough to use and none of them is a
// dependency of this module. MinerU is the accuracy leader on dense academic
// layout and emits per-block bounding boxes, which is what the figure
// pipeline needs. Marker is several times faster. Docling is MIT licensed
// and runs on a CPU, which is the whole answer for a machine with no card.
// All three are Python with model downloads measured in gigabytes, so the Go
// side shells out to whichever the machine has, exactly as it shells out to
// poppler, and records in the paper's front matter which one read it.
//
// The tool's own Markdown is not used. Every one of them can emit Markdown
// and every one of them writes a different dialect of it: mathematics in
// \(..\) or in $..$ or in <math>, figures as links into a directory of its
// own naming, tables as HTML. The corpus has one dialect, and converting
// three into one after the fact means writing three fragile rewriters over
// text that has already thrown away what it knew. So this package reads the
// structured output instead, which is a list of blocks with a type, a page,
// a bounding box and some text, and writes the corpus dialect once.
//
// What comes out is page text in the same shape the native path produces, so
// that assemble, split and everything after them do not know or care which
// path a paper took. The difference a reader should see is in the front
// matter and nowhere else.
package layout
