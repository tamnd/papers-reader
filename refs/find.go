package refs

import (
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/assemble"
)

// Bibliography is the run of paragraphs holding the paper's reference list.
//
// It is found by its heading and not by position, because the reference list
// is very often not the last thing in the paper. Appendices come after it,
// and so do acknowledgements in about a third of what is on the list, and a
// parser that took everything after the heading would file an appendix full
// of proofs as forty malformed references.
//
// The heading is looked for from the end backwards. A paper that says "see
// the references" in its introduction has the word on page one, and the
// heading is the last one, not the first.
func Bibliography(d *assemble.Document) []assemble.Paragraph {
	if d == nil {
		return nil
	}
	start := -1
	for i := len(d.Paragraphs) - 1; i >= 0; i-- {
		if isBibliographyHeading(d.Paragraphs[i].Text) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	end := len(d.Paragraphs)
	for i := start; i < end; i++ {
		if endsBibliography(d.Paragraphs[i].Text) {
			end = i
			break
		}
	}
	if start >= end {
		return nil
	}
	return d.Paragraphs[start:end]
}

// bibliographyNames is every heading the reference list is printed under,
// casefolded and with the section number already taken off.
var bibliographyNames = map[string]bool{
	"references":           true,
	"reference":            true,
	"references cited":     true,
	"bibliography":         true,
	"literature cited":     true,
	"works cited":          true,
	"notes and refs":       true,
	"notes and references": true,
	"references and notes": true,
}

// afterNames is every heading that can follow the reference list. A heading
// that is not on this list does not end the bibliography, because the risk
// runs the other way: a reference beginning with a short institutional name
// would otherwise cut the list in half.
var afterNames = map[string]bool{
	"appendix":                true,
	"appendices":              true,
	"acknowledgement":         true,
	"acknowledgements":        true,
	"acknowledgment":          true,
	"acknowledgments":         true,
	"author contributions":    true,
	"supplementary material":  true,
	"supplementary materials": true,
	"supporting information":  true,
	"about the authors":       true,
	"biographies":             true,
}

func isBibliographyHeading(text string) bool {
	key, ok := headingKey(text)
	return ok && bibliographyNames[key]
}

func endsBibliography(text string) bool {
	key, ok := headingKey(text)
	if !ok {
		return false
	}
	return afterNames[key] || strings.HasPrefix(key, "appendix")
}

// headingKey reduces a paragraph to the words of its heading, or says it is
// not short enough to be one.
func headingKey(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	// The length cap is a guess at what a heading looks like, and it only has
	// to be guessed at for a paragraph with no marker on it. A line that
	// starts with an ATX marker is a heading whatever its length, and some of
	// them are long: BERT heads its appendix with the whole title of the
	// paper in quotation marks, a hundred and two characters of it, and under
	// the cap that heading did not end the bibliography and the parse ran on
	// through the appendix.
	if !strings.HasPrefix(text, "#") && len([]rune(text)) > 60 {
		return "", false
	}
	text = strings.TrimLeft(text, "# \t")
	text = strings.TrimLeft(text, "§ \t")
	// Drop a leading section number in any of the schemes package split
	// knows: "6", "6.", "A.", "VII.".
	if i := strings.IndexFunc(text, unicode.IsSpace); i > 0 && i <= 6 && isNumbering(text[:i]) {
		text = text[i:]
	}
	text = strings.Trim(text, " \t.:*#")
	if text == "" {
		return "", false
	}
	return strings.ToLower(strings.Join(strings.Fields(text), " ")), true
}

// isNumbering says whether a word is a section number rather than the first
// word of the heading.
func isNumbering(s string) bool {
	s = strings.TrimRight(s, ".")
	if s == "" {
		return false
	}
	digits, romans := true, true
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			digits = false
		}
		if !strings.ContainsRune("IVXL", r) {
			romans = false
		}
	}
	return digits || romans
}
