package extract

import (
	"regexp"
	"strings"
)

// aligned is a gap wide enough that the page meant it: the two things on
// either side of it are set to the two margins and are not a phrase with
// something the matter with its spacing.
//
// Audit rule C09 fails a run of three spaces, and this deliberately asks for
// more than the rule does. Three is right for the rule, which reports and
// leaves the judgement to a person, and wrong for a repair, which rewrites
// the file. The Scheme memo prints its report number and its date with 65
// spaces between them. The Attention paper prints "Acknowledgements" and
// then, three spaces later, the sentence that follows it, which is a run-in
// heading and one line of prose. Breaking that one in two gave the splitter
// a heading it then made a section out of, and the paper published an
// acknowledgements section of one sentence and renumbered its references.
// So the repair takes the gaps it is sure of and leaves the rest to be
// reported.
var aligned = regexp.MustCompile(`\S {8,}\S`)

// gap is the run of spaces itself, for cutting a line at.
var gap = regexp.MustCompile(` {8,}`)

// Unalign breaks a line whose fields the page set at opposite margins.
//
// The cover of a tech report prints the report number against the left
// margin and the date against the right, on one line with a hand's width of
// space between them. Markdown collapses a run of spaces to one, so the
// Scheme memo published the line "AI Memo No. 349 December 1975", which is
// not a sentence and does not say what the page said. Audit rule C09 is
// there to catch exactly that.
//
// Each field goes on a paragraph of its own. Nothing is lost by it: the page
// put them apart on purpose, and one above the other says the same thing in
// a form that survives being rendered. They cannot go on consecutive lines,
// because Markdown would join those back into the line this started with.
//
// Only a line standing on its own is touched. Two or more in a row is a
// table, which the reading prompt asks for as a pipe table or inside a text
// fence, and breaking each row into its cells would finish destroying the
// grid rather than save it. A line that already has a pipe in it is that
// pipe table. A line inside a fence or inside a display is left alone, where
// the spaces are safe already and are usually the whole point: an aligned
// environment is built out of them.
//
// What is left of audit rule C09 after this is the narrower gaps, which is
// where the rule earns its keep: those are the ones somebody has to look at
// the page to settle.
func Unalign(s string) string {
	lines := strings.Split(s, "\n")
	code := codeLines(lines)
	math := displayLines(lines, code)
	loose := func(i int) bool {
		return i >= 0 && i < len(lines) && !code[i] && !math[i] &&
			!strings.Contains(lines[i], "|") && aligned.MatchString(lines[i])
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if !loose(i) || loose(i-1) || loose(i+1) {
			out = append(out, line)
			continue
		}
		fields := gap.Split(strings.TrimSpace(line), -1)
		out = append(out, strings.Join(fields, "\n\n"))
	}
	return strings.Join(out, "\n")
}

// displayLines marks the lines inside a $$ display, the other place a run of
// spaces is deliberate. It takes the fenced lines as given, because a $$ in
// a listing is program text and opens nothing.
func displayLines(lines []string, code []bool) []bool {
	out := make([]bool, len(lines))
	open := false
	for i, line := range lines {
		if code[i] {
			continue
		}
		if strings.TrimSpace(line) == "$$" {
			out[i], open = true, !open
			continue
		}
		out[i] = open
	}
	return out
}
