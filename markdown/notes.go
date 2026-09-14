package markdown

import (
	"regexp"
	"strings"
)

var noteDef = regexp.MustCompile(`^\[\^([^\]]+)\]:\s*(.*)$`)

// Notes lifts the footnote definitions out of a body and into a table of
// the paper's notes, and returns the body without them.
//
// They have to come out. Neither LaTeX nor a page of blocks has anywhere to
// put a free standing footnote definition, so a
// definition left in the prose would set as a paragraph reading "[^1]: Jean
// Pouget-Abadie is visiting Universite de Montreal", in the middle of section
// one, three pages after the marker that refers to it. The marker is on the
// author line of the front page and the definition is in the first section
// because that is where the page break fell, and no rearranging of the corpus
// would fix that: the corpus is right and it is the setting that has to
// gather them.
//
// A continuation line, indented under a definition, is joined to it. Nothing
// in this corpus has one yet and the format allows it.
func Notes(body string, into map[string]string) string {
	var out []string
	label := ""
	for _, line := range strings.Split(body, "\n") {
		if m := noteDef.FindStringSubmatch(line); m != nil {
			label = m[1]
			into[label] = m[2]
			continue
		}
		if label != "" {
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				into[label] = strings.TrimSpace(into[label] + " " + strings.TrimSpace(line))
				continue
			}
			if strings.TrimSpace(line) == "" {
				// A blank line after a definition ends it, and is dropped along
				// with it so the body does not grow a gap where it stood.
				label = ""
				continue
			}
			label = ""
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}
