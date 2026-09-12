package figures

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tamnd/papers-reader/extract"
	"github.com/tamnd/papers-reader/poppler"
)

// A Caption is one caption of a paper and where it sits.
type Caption struct {
	Page int
	Box  poppler.Box
	// Kind is the word the caption opens with, normalised: figure, table,
	// algorithm or listing. A table caption is here because a table is not a
	// figure and must not be committed as one, and the only way to refuse a
	// region that is really a table is to recognise the caption under it.
	Kind string
	// Number is what the paper called it, as the paper wrote it, so a paper
	// that numbers its figures 3-a keeps the a.
	Number string
	Text   string
}

// caption is the opening of a caption: the word, then optionally a number.
//
// Deliberately not anchored to a particular numbering, because the hundred
// write Figure 1, Fig. 1, FIG. 1, Figure 1., Figure 1:, Fig 3-a and Table
// I, and a pattern that insisted on one of them would lose every figure of
// the papers that use another.
//
// The abbreviating full stop is outside the first group and after the word
// boundary, so that Fig. 3 keeps its number. Inside the group it takes the
// boundary with it, the number is never reached, and every paper that
// abbreviates loses the one thing the entry is indexed by.
var caption = regexp.MustCompile(`^(Fig(?:ure)?|FIG(?:URE)?|Table|TABLE|Algorithm|ALGORITHM|Listing|LISTING|Chart)\b\.?[ \t]*([0-9]+(?:[.\-][0-9a-zA-Z]+)*|[IVXLC]+|[A-Z])?[ \t]*[.:)]?`)

// Captions is every caption on a page, in reading order.
//
// It reads the paragraphs rather than the lines, because a caption is three
// lines of prose and the second and third do not start with the word
// Figure. The paragraph is what the assembler would have written and it is
// what gets translated.
func Captions(page extract.Page) []Caption {
	var out []Caption
	for _, par := range page.Paragraphs {
		m := caption.FindStringSubmatch(par.Text)
		if m == nil {
			continue
		}
		// A sentence of the body that happens to begin "Figure 3 shows"
		// is a reference to a figure and not a caption. The test is the
		// punctuation the typesetter put after the number: a caption has
		// some, a sentence does not.
		if !separated(par.Text, m[0]) {
			continue
		}
		out = append(out, Caption{
			Page:   page.Number,
			Box:    par.Box,
			Kind:   kind(m[1]),
			Number: m[2],
			Text:   strings.TrimSpace(par.Text),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Box.YMin < out[j].Box.YMin })
	return out
}

// separated says whether what follows the number looks like a caption
// rather than a sentence carrying on.
//
// Two ways it can: the typesetter put a colon, a full stop, a dash or a
// parenthesis after the number, or the paper sets its captions with nothing
// but space, in which case the word after the number starts a sentence with
// a capital. "Figure 3 shows" is neither and is a cross reference in the
// body.
func separated(text, head string) bool {
	if strings.ContainsAny(last(head), ".:)") {
		return true
	}
	rest := strings.TrimSpace(text[len(head):])
	if rest == "" {
		return false
	}
	if strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, "—") {
		return true
	}
	word, _, _ := strings.Cut(rest, " ")
	return word != strings.ToLower(word)
}

func last(s string) string {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return ""
	}
	return s[len(s)-1:]
}

func kind(word string) string {
	switch w := strings.ToLower(strings.TrimRight(word, ".")); w {
	case "fig", "figure":
		return "figure"
	default:
		return w
	}
}

// Pair attaches the nearest caption to each candidate region of one page.
//
// Below for preference and above otherwise, because papers caption their
// figures below and their tables above and are not consistent about either.
// The caption has to be within a couple of lines of the region: a figure at
// the top of a column and the first paragraph of prose under it are not a
// figure and its caption, and there is nothing else on the page to tell
// them apart.
//
// A candidate with no caption gets none, and the caller does not commit it.
// In practice an uncaptioned hole in a column is a display equation, a
// table, a decorative rule or a logo far more often than it is a figure
// somebody forgot to caption.
func Pair(cands []Candidate, caps []Caption, pitch float64) []Found {
	out := make([]Found, len(cands))
	taken := make([]bool, len(caps))
	for i, c := range cands {
		out[i] = Found{Candidate: c}
		best, at := 0.0, -1
		for j, text := range caps {
			if taken[j] || text.Page != c.Page || !beside(c.Box, text.Box) {
				continue
			}
			d := distance(c.Box, text.Box)
			if d > reach*pitch {
				continue
			}
			// Below wins over above at the same distance, which is what
			// sorts out a figure with a caption under it sitting directly
			// beneath a table with its caption above.
			if text.Box.YMin >= c.Box.YMax {
				d -= 0.01
			}
			if at < 0 || d < best {
				best, at = d, j
			}
		}
		if at >= 0 {
			taken[at] = true
			out[i].Caption = &caps[at]
		}
	}
	return out
}

// A Found is a region with whatever caption belongs to it, which is nothing
// for most of them.
type Found struct {
	Candidate
	Caption *Caption
}

// reach is how far from the region a caption may be, in lines. Two lines of
// white space is what a journal leaves between a figure and its caption;
// more than that and the thing below the hole is the next paragraph.
const reach = 3

// beside says whether a caption is horizontally where the region is. A
// caption in the right column is not the caption of a figure in the left
// one, however close the two are vertically.
func beside(region, cap poppler.Box) bool {
	return cap.XMid() >= region.XMin && cap.XMid() <= region.XMax
}

func distance(region, cap poppler.Box) float64 {
	switch {
	case cap.YMin >= region.YMax:
		return cap.YMin - region.YMax
	case cap.YMax <= region.YMin:
		return region.YMin - cap.YMax
	}
	// Overlapping, which happens when the caption is set inside the figure's
	// own white space. Nothing is closer than that.
	return 0
}
