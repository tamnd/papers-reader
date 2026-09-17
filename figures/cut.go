package figures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// A Run is one paper's figures: find the regions, pair the captions, render
// what is left and write the ones that fit the budget.
//
// The zero Run does not work. Set the Budget with Default, so that a caller
// who forgot gets the caps rather than none.
type Run struct {
	// Paper is the id, which is what the manifest records.
	Paper string
	// PDF is the file to read. It is never committed and never leaves the
	// ignored directory it sits in.
	PDF string
	// Dir is figures/<id>, where the PNGs go. The entries go to the caller,
	// which puts them in the one manifest the corpus keeps.
	Dir    string
	Budget Budget
	// Dry says render everything and write nothing, which is how a person
	// sees what a paper would commit before it commits it.
	Dry bool
}

// A Result is what a run came to.
type Result struct {
	// Figures is this paper's entries, ready to go into the corpus
	// manifest. They are in the order they were found, which is page order.
	Figures []Figure
	// Written is figures committed, Skipped is regions that were looked at
	// and refused.
	Written, Skipped int
	// Notes is everything a person should see: the regions with no caption,
	// the tables that came back as pictures, the figures that broke the
	// budget. It is a report and not an error, because a paper with one
	// unreadable region and nine good figures should commit the nine.
	Notes []string
}

func (r *Result) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// Do runs the whole thing over the pages of one paper.
//
// The pages are the geometry, from poppler.Layouts, and are the same read
// the native extraction path makes. Passing them in rather than reading
// them here is what lets a caller do one pdftotext run for the text and the
// figures both.
func (r *Run) Do(ctx context.Context, pages []poppler.Layout) (*Result, error) {
	out := &Result{}
	if len(pages) == 0 {
		return out, nil
	}
	// The furniture is learned from the whole paper, exactly as extraction
	// learns it. A running head left in the text is a line in a column, and
	// a line in a column is a page with no hole in it.
	furniture := extract.FindFurniture(pages)
	frame := FindFrame(pages, furniture)
	images, err := poppler.ImageList(ctx, r.PDF, pages[0].Number, pages[len(pages)-1].Number)
	if err != nil {
		// Not fatal. Without the list every region is rendered as a drawing
		// at 300 dpi, which is right for most of them and is never wrong
		// enough to be worth failing the paper over.
		out.note("pdfimages could not list the images, so every region is rendered as a drawing: %v", err)
	}

	for _, page := range pages {
		if err := r.page(ctx, page, furniture, frame, images, out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (r *Run) page(ctx context.Context, page poppler.Layout, f *extract.Furniture, frame Frame, images []poppler.Image, out *Result) error {
	text := extract.Read(page, f)
	caps := Captions(text, f.Lines(page))
	pitch := pitchOf(f.Lines(page))

	// Two passes, because a figure can be found two ways. Find looks for the
	// hole a picture leaves in a column, and Anchor grows a region up from a
	// caption whose figure left no hole because it is drawn with type. The
	// second runs over what the first came back with, so it knows which
	// captions are already spoken for.
	regions := Anchor(page, f, frame, caps, Pair(Find(page, f, frame), caps, pitch))

	for _, found := range regions {
		switch {
		case found.Caption == nil:
			// Far more often a decorative rule, a logo, a signature block
			// or a display equation that was too tall than a figure
			// somebody forgot to caption. Reported, never committed.
			out.Skipped++
			out.note("page %d has an uncaptioned region %.0f by %.0f points, which is not committed",
				page.Number, found.Box.Width(), found.Box.Height())
			continue
		case found.Caption.Kind == "table":
			// A table is not a figure and must not be committed as one.
			// The right answer is a Markdown table, and a table that only
			// exists here as a picture is a hole in the extraction that
			// somebody has to fill.
			out.Skipped++
			out.note("page %d has a table that came back as a region: %s", page.Number, short(found.Caption.Text))
			continue
		}
		// The candidate goes down as it was found, a hole in a column. What
		// is under it is decided in Fit, once the white has come off and the
		// box is the picture rather than the hole.
		if err := r.one(ctx, page, found.Candidate, images, found.Caption, out); err != nil {
			return err
		}
	}
	return nil
}

func (r *Run) one(ctx context.Context, page poppler.Layout, c Candidate, images []poppler.Image, text *Caption, out *Result) error {
	// The fraction is checked before the render and not only after it.
	// Rendering a region that covers most of the page costs seconds and
	// produces a file that must not be written, and the check is cheap.
	fraction := Fraction(c.Box, page)
	if fraction > r.Budget.MaxFraction {
		out.Skipped++
		out.note("page %d has a region covering %.0f%% of the page, which is a page image and not a figure",
			page.Number, fraction*100)
		return nil
	}

	got, err := Fit(ctx, r.PDF, c, page, images, r.Budget)
	if err != nil {
		out.Skipped++
		out.note("page %d could not be rendered: %v", page.Number, err)
		return nil
	}
	if Blank(got.Data) {
		out.Skipped++
		out.note("page %d has a region that renders as nothing, so the hole in the column was white space", page.Number)
		return nil
	}

	sha := SHA256(got.Data)
	if already, ok := Has(out.Figures, sha); ok {
		out.note("page %d repeats the picture already committed as %s", page.Number, already.Name())
		return nil
	}

	fig := Figure{
		Paper:   r.Paper,
		ID:      Next(out.Figures),
		Number:  text.Number,
		Page:    page.Number,
		Box:     pdfBox(got.Box, page.Height),
		Caption: text.Text,
		SHA256:  sha,
		Method:  got.Method,
		// Measured on what was trimmed and committed, not on the hole it
		// was found in. The rule is about the picture.
		Fraction: roundFraction(Fraction(got.Box, page)),
		Width:    got.Width,
		Height:   got.Height,
		Bytes:    len(got.Data),
	}
	if got.Scale < 1 {
		fig.Scale = round(got.Scale)
	}
	if err := r.Budget.Check(fig); err != nil {
		out.Skipped++
		out.note("page %d has a figure that does not fit the budget: %v", page.Number, err)
		return nil
	}
	if text.Number == "" {
		out.note("page %d has a figure whose caption carries no number, so it is filed as %s", page.Number, fig.ID)
	}

	out.Figures = append(out.Figures, fig)
	out.Written++
	if r.Dry {
		return nil
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.Dir, fig.Name()), got.Data, 0o644)
}

// short is a caption cut down to something that fits on one line of a
// report.
func short(s string) string {
	const n = 60
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
