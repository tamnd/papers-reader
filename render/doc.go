// Package render turns the pages of a PDF into pictures for the readers that
// cannot use a text layer.
//
// It is the one stage of the pipeline that is pure arithmetic. No model is
// asked anything, nothing leaves the machine, and the answer for a given page
// at a given resolution is the same every time, so it can run days ahead of
// the stage that needs it and on a machine with no network at all. That is
// worth keeping separate: the vision path is slow and rationed, and standing
// in a queue for a model with the rasteriser still to run wastes the scarce
// thing waiting on the plentiful one.
//
// Nothing here is committed. Page images live under images/<id>/, which is
// gitignored, and a page raster is a picture of a copyrighted paper whatever
// the audit rules say about crops.
package render
