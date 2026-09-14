package extract

import (
	"context"
	"fmt"
	"time"

	"github.com/tamnd/llm"
	"github.com/tamnd/papers-reader/prompt"
	"github.com/tamnd/papers-reader/render"
)

// A Reply is one answer from a model: what it said, and enough about where it
// came from to write the record.
//
// Model is the corpus's name for the weights and not the name the server
// answers to. The two differ on purpose: a host's shortlist entry is whatever
// its operator called it, and the front matter of a published file has to name
// something that will still mean something in a year.
type Reply struct {
	llm.Response
	Model string
}

// An Ask puts one question to a model and brings back what it said.
//
// It is a function and not an interface because there is one method and
// because the thing that implements it lives in package work, which knows
// about hosts, quotas and ledgers and which this package must not depend on.
// A test supplies four lines and no fleet.
//
// target is what the question is about, in words a person would use: a paper
// id and a page number. It is what the ledger records and what a usage report
// groups by.
type Ask func(ctx context.Context, target string, req llm.Request) (Reply, error)

// Vision reads the pages of one paper by showing a picture of each page to a
// model.
//
// It is the third extraction path and the only one with a retry in it. The
// other two have nothing to retry with: pdftotext over the same file reads the
// page the same way the second time, so a refused page is a page for a person.
// A model asked the same question twice gives two different answers, and a
// model asked the same question at a higher resolution sometimes gives a
// better one, so a refused page here goes back with more pixels in it.
//
// The zero Vision does not work. Ask, Prompt, Store and PDF are all required
// and Page says which one is missing rather than failing at the first call.
type Vision struct {
	Ask    Ask
	Prompt prompt.Prompt
	// Store is where the page rasters are. A page that is not rendered yet is
	// rendered here, so this command works on a machine that never ran papers
	// render; running it first is an optimisation and not a prerequisite.
	Store render.Store
	// PDF is the file the rasters are made from.
	PDF string
	// Paper is the id, for the ledger and for the log line.
	Paper string
	// Colour is the pages that have a colour image on them, from
	// render.Colour. Every other page is read in grey, because a colour raster
	// of a page of text is three times the upload for nothing and the upload
	// is what a browser session is rationed on.
	Colour map[int]bool
	// Detail is what the provider is told about how closely to look. Empty
	// means Detail.
	Detail string
	// Check is the acceptance rules, and nil accepts everything, which is what
	// a test wants. It is a function rather than a *Checker so that the caller
	// keeps its own checker: rule A5 is about how long this paper's pages
	// usually are, and two checkers over one paper would each have half the
	// evidence.
	Check func(page int, text string) []Fault
	// Repair is a last pass over a page before the rules see it, for the
	// mistakes that are worth correcting rather than asking about. It is
	// separate from Tidy because it needs the file the page came from, which
	// Tidy does not have and should not: Readdress is the one that wanted
	// it, and it is the file's own text layer that says what the address is.
	// nil is no repair, which is what a test wants.
	Repair func(page int, text string) string
	Logf   func(string, ...any)
}

// Detail is how closely a provider is asked to look at the image. High,
// because the whole question is what the small type says: a provider that
// downsamples a 300 dpi page to fit a thumbnail budget is being asked to read
// something nobody could read.
const Detail = "high"

// A Scan is one page as a model gave it back.
type Scan struct {
	Page int
	Text string
	// Profile is the raster the accepted answer was read from, or the last one
	// tried for a page that was never accepted.
	Profile  render.Profile
	Attempts int
	Model    string
	// Usage and Elapsed cover every attempt and not only the one that worked.
	// A page that took three tries cost three pages of quota, and a report
	// that counted the last one would say the corpus was cheaper than it was.
	Usage   llm.Usage
	Elapsed time.Duration
	// Faults is what the last attempt broke, and empty for a page that was
	// accepted. A page that arrives here with faults on it has been asked at
	// every resolution on the ladder and is out of attempts.
	Faults []Fault
}

// OK reports whether the page was accepted.
func (r Scan) OK() bool { return len(r.Faults) == 0 }

// How is the shape of the read, for the line printed under --v.
func (r Scan) How() string {
	s := fmt.Sprintf("%s at %s", r.Model, r.Profile)
	if r.Attempts > 1 {
		s += fmt.Sprintf(", %d attempts", r.Attempts)
	}
	return s
}

// Page reads one page, climbing the resolution ladder while the acceptance
// rules refuse what comes back.
//
// A page that is refused at every rung comes back with its faults on it and no
// error. That is not the same thing as a failure: the fleet answered, the
// quota was spent, and what came back is not good enough to publish. The
// caller reports it and goes on to the next page, because a paper that stops
// at its first bad page is a paper nobody ever finishes reading.
//
// An error means the question could not be put at all: the file will not
// rasterise, or no host answered. There is nothing to report about the page
// because nothing was learned about it.
//
// Every attempt starts again at the bottom of the ladder rather than at the
// best raster on disk. A page rendered at 600 is a page some earlier run had
// trouble with, and the trouble may well have been the model rather than the
// pixels; asking at 300 first costs one call and keeps the expensive rungs for
// the pages that need them.
func (v *Vision) Page(ctx context.Context, page int) (Scan, error) {
	out := Scan{Page: page}
	if err := v.ready(page); err != nil {
		return out, err
	}
	profile := render.Profile{DPI: render.Base, Gray: !v.Colour[page]}
	for {
		out.Attempts++
		out.Profile = profile
		if _, err := v.Store.Render(ctx, v.PDF, profile, page); err != nil {
			return out, err
		}
		image, err := v.Store.Read(profile, page)
		if err != nil {
			return out, err
		}

		reply, err := v.Ask(ctx, fmt.Sprintf("%s p%d", v.Paper, page), llm.Request{
			Instructions: v.Prompt.Text,
			Input:        "Transcribe this page.",
			Images:       []llm.Image{{MediaType: "image/png", Data: image, Detail: v.detail()}},
		})
		out.Usage = out.Usage.Add(reply.Usage.Normalized())
		out.Elapsed += reply.Elapsed
		if err != nil {
			return out, err
		}
		if reply.Model != "" {
			out.Model = reply.Model
		}

		text := Tidy(reply.Text)
		if v.Repair != nil {
			text = v.Repair(page, text)
		}
		faults := v.check(page, text)
		if len(faults) == 0 {
			out.Text = text
			out.Faults = nil
			return out, nil
		}
		out.Text = text
		out.Faults = faults

		next, ok := profile.Next()
		if !ok {
			// Out of rungs. The one thing left worth trying costs nothing
			// and asks nobody: a table the reader would not spell as a grid
			// is written as the fence the prompt asked for. See Fence.
			if text := Fence(text); len(v.check(page, text)) == 0 {
				out.Text = text
				out.Faults = nil
				v.logf("%s page %d was refused at %s (%s), and its tables are written as fences", v.Paper, page, profile, faults[0])
			}
			return out, nil
		}
		v.logf("%s page %d was refused at %s (%s), asking again at %s", v.Paper, page, profile, faults[0], next)
		profile = next
	}
}

// ready says which required field is missing, by name, before anything is
// rendered or asked for.
func (v *Vision) ready(page int) error {
	switch {
	case v.Ask == nil:
		return fmt.Errorf("there is nothing to ask: the vision path needs a model")
	case v.Prompt.Text == "":
		return fmt.Errorf("there is no reading prompt")
	case v.PDF == "":
		return fmt.Errorf("there is no file to rasterise")
	case v.Store.Dir == "":
		return fmt.Errorf("there is nowhere to keep the page images")
	case page < 1:
		return fmt.Errorf("page %d is not a page", page)
	}
	return nil
}

func (v *Vision) check(page int, text string) []Fault {
	if v.Check == nil {
		return nil
	}
	return v.Check(page, text)
}

func (v *Vision) detail() string {
	if v.Detail != "" {
		return v.Detail
	}
	return Detail
}

func (v *Vision) logf(format string, args ...any) {
	if v.Logf != nil {
		v.Logf(format, args...)
	}
}
