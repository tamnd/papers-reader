package extract

import (
	"regexp"
	"sort"
	"strings"
)

// edgeLines is how far in from the top and the bottom of a page a line has
// to be before this will count it as a candidate for the running head.
//
// The reading prompt asks for two pieces of furniture at an edge and no
// more: the running head transcribed verbatim, and the number the page
// prints on a line of its own, either at the head or at the foot. So a line
// three deep is body text, and a line two deep is only furniture if the line
// outside it is the folio. The McCabe paper has both and has them in either
// order, page 1 being the folio and then the journal line and page 2 being
// the running head and then the folio, which is why this is not one.
const edgeLines = 2

// Undress takes the furniture off pages that a model read.
//
// The reading prompt tells the model to transcribe the running head verbatim
// and to leave the printed page number on a line of its own, and says that
// both are furniture taken out later by a program that has to see them
// first. This is that program, and for a while it did not exist. The page
// map reads the folio out of the page text, so the number has to survive as
// far as here, and until now it survived all the way onto the page: the
// front matter of the McCabe paper was published with a line reading 308.
//
// Give it the whole paper, keyed by page number. It works on what it is
// given, and what comes back is the same pages with the furniture gone, the
// blank lines the furniture was sitting above or below gone with it, and
// nothing else touched.
//
// What counts as furniture is learned from the paper rather than matched
// against a list, which is what FindFurniture does on the native path and
// for the reason written there: every journal in the hundred does it
// differently. The difference is that the native path has the coordinates of
// every line and can ask which band a line sits in, and this has only the
// order of the lines, so the margin is the first and last few lines and
// nothing between them is ever looked at.
func Undress(pages map[int]string) map[int]string {
	seen := map[string]map[int]bool{}
	for number, text := range pages {
		for _, line := range append(head(text), foot(text)...) {
			k := fold(line)
			if seen[k] == nil {
				seen[k] = map[int]bool{}
			}
			seen[k][number] = true
		}
	}
	// The same third of the paper that a running head has to be on for
	// FindFurniture to believe in it, and never fewer than two pages, for the
	// same reasons: a journal that sets one head on the recto and another on
	// the verso puts each of them on half the pages and neither on more than
	// half, and two pages that share a line share it by coincidence as often
	// as not.
	least := max(len(pages)/3, 2)
	// A head that is established on enough pages carries its rotations with
	// it, whatever they are counted on. See bag for what a rotation is and
	// which paper wanted this.
	turned := map[string]bool{}
	for k, on := range seen {
		if len(on) < least {
			continue
		}
		if b := bag(k); b != "" {
			turned[b] = true
		}
	}
	repeated := func(line string) bool {
		if len(pages) < minPages {
			return false
		}
		k := fold(line)
		return len(seen[k]) >= least || turned[bag(k)]
	}
	out := make(map[int]string, len(pages))
	for number, text := range pages {
		lines := strings.Split(text, "\n")
		// The foot first. Stripping the head moves every line after it, and a
		// page of a single line is its own head and its own foot.
		lines = peel(lines, lastLine, repeated)
		lines = peel(lines, firstLine, repeated)
		if body := strings.Trim(strings.Join(lines, "\n"), "\n"); body != "" {
			out[number] = body + "\n"
		} else {
			out[number] = ""
		}
	}
	return out
}

// peel drops the furniture off one end of a page, the end being whichever of
// firstLine or lastLine it is handed.
//
// At most one folio and at most one running head come off, because that is
// all a page has. Peeling two of anything would be the same depth in lines
// and a good deal more dangerous: a paper set with a repeated line at the
// top and another repeated line under it is far likelier to be two lines of
// a heading than two running heads, and this has no coordinates to tell it
// otherwise.
func peel(lines []string, end func([]string, int) int, repeated func(string) bool) []string {
	number, head, past := false, false, 0
	for {
		at := end(lines, past)
		if at < 0 {
			return lines
		}
		line := strings.TrimSpace(lines[at])
		switch {
		case delimiter(line), equationNumber.MatchString(line):
			return lines
		// A footnote is not at the edge of the page, wherever it is written.
		// Markdown puts every definition at the foot of the document, so a
		// model that reads a footnote off the middle of the page writes it
		// last, under the folio, and the folio is then the second line up
		// rather than the first. Page 7 of the ResNet paper is one and it
		// published with its page number on it.
		case footnote.MatchString(line):
			past++
			continue
		// A bare number on its own line at the edge of a page is a page
		// number whether or not it repeats, and it never repeats, because it
		// counts. This is the same call Furniture.Is makes.
		case !number && folio.MatchString(line):
			number = true
		case !head && repeated(line):
			head = true
		default:
			return lines
		}
		lines = append(lines[:at], lines[at+1:]...)
	}
}

// bag is a folded line's words in one fixed order, so that the same running
// head set the other way round on the facing page comes out the same.
//
// The MapReduce paper wanted this. It prints "USENIX Association  OSDI '04:
// 6th Symposium on Operating Systems Design and Implementation" at the foot
// of a recto and the same words the other way round at the foot of a verso,
// which is two lines as far as fold is concerned. Thirteen pages means four,
// and the model transcribed the recto head on five pages and the verso head
// on two, so the recto came off and the verso stayed and published on two
// sections. Counting the words rather than the line makes it seven pages of
// one piece of furniture, which is what it is.
//
// Four words, because two lines of three short words each are a permutation
// of one another often enough to worry about and a running head is never
// that short. Over the corpus the shortest is six.
func bag(folded string) string {
	words := strings.Fields(folded)
	if len(words) < bagWords {
		return ""
	}
	sort.Strings(words)
	return strings.Join(words, " ")
}

const bagWords = 4

// footnote is a Markdown footnote definition, which is a label in brackets
// with a caret in front of it and a colon after it.
var footnote = regexp.MustCompile(`^\[\^[^\]]+\]:`)

// delimiter says whether a line is the edge of a block rather than a line of
// the page, and nothing here ever takes one off.
//
// A `$$` is the one that mattered. A page that ends inside a display equation
// ends on its closing delimiter, and the same delimiter is at the edge of
// enough pages of a paper full of mathematics for repeated to call it a
// running head. Taking it off does not lose a line, it leaves a display open,
// and the next thing to read the document runs the rest of the paper into the
// equation. Over the corpus this happened to five papers of sixty seven and
// broke the mathematics in all five: Cooley, Spanner, Cortes, Ford and the
// GAN paper.
//
// A fence has never been seen to do it and is here for the same reason, which
// is that the cost is the whole of the rest of the file either way.
func delimiter(line string) bool {
	if strings.HasPrefix(line, "$$") {
		return true
	}
	for _, c := range "`~" {
		if strings.HasPrefix(line, strings.Repeat(string(c), 3)) {
			return true
		}
	}
	return false
}

// equationNumber is a number in brackets on a line of its own, which is how a
// paper numbers a displayed equation and is not how a journal numbers a page.
//
// folio allows the brackets because on the native path a line is only offered
// to it once the coordinates say it is in the margin, and there "(4)" in the
// margin is a page number. Here there are no coordinates, only the order of
// the lines, so a bracketed number at the bottom of a page is whatever it
// most often is, and over the corpus exactly one page ends in one: page 4 of
// the GAN paper, where it numbers equation 4 and where "Eq. 4" three
// paragraphs later is what points at it. Eleven pages end in a bare number
// and all eleven are folios.
//
// It stops the peeling rather than being skipped by the folio test, because
// the other test would take it anyway. fold writes every number as one token
// so that a head reading "308 IEEE Transactions" is the same line as "309
// IEEE Transactions", and that makes "(1)" through "(6)" one line repeated on
// six pages, which is what repeated is looking for.
var equationNumber = regexp.MustCompile(`^\(\s*[0-9]{1,4}\s*\)$`)

// head is the lines at the top of a page that the furniture could be on,
// and foot is the ones at the bottom. Blank lines are passed over, because
// the model leaves one between the head and the text and one between the
// text and the folio. Footnote definitions are passed over for the reason
// peel passes over them, which is that Markdown moves them and the edge of
// the page is where they land. A page with nothing on it, which a full page
// plate is, has neither a head nor a foot.
func head(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines) && len(out) < edgeLines; i++ {
		if l := strings.TrimSpace(lines[i]); edge(l) {
			out = append(out, l)
		}
	}
	return out
}

func foot(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < edgeLines; i-- {
		if l := strings.TrimSpace(lines[i]); edge(l) {
			out = append(out, l)
		}
	}
	return out
}

func edge(line string) bool {
	return line != "" && !footnote.MatchString(line)
}

// firstLine and lastLine are the first and last lines of a page with
// anything on them, and -1 for a page with nothing on it. past is how many
// of those to step over first, which is how peel looks under a line it has
// decided is neither furniture nor a reason to stop.
func firstLine(lines []string, past int) int {
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if past == 0 {
			return i
		}
		past--
	}
	return -1
}

func lastLine(lines []string, past int) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if past == 0 {
			return i
		}
		past--
	}
	return -1
}
