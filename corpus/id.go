package corpus

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// idPattern is <surname>-<year>-<keyword>, lowercase ASCII with hyphens as the
// only separator. The surname is transliterated and carries no diacritics, so
// that an id can be typed on any keyboard and pasted into any filesystem.
var idPattern = regexp.MustCompile(`^([a-z][a-z0-9]*)-([0-9]{4})-([a-z0-9]+)$`)

// ID is a paper identifier, parsed into its three parts.
//
// An id is permanent. Renaming one would invalidate every tag line, every
// figure path, every citation edge and every translation's record of what it
// was made from, and the corpus has no mechanism to follow such a rename. A
// paper whose keyword turns out to be a poor choice keeps it, and the better
// name goes in the aka field of the manifest.
type ID struct {
	Full    string
	Surname string
	Year    int
	Keyword string
}

// ParseID reads an id and says what is wrong with it if it will not parse.
func ParseID(s string) (ID, error) {
	m := idPattern.FindStringSubmatch(s)
	if m == nil {
		switch {
		case s == "":
			return ID{}, fmt.Errorf("an empty string is not a paper id")
		case strings.ToLower(s) != s:
			return ID{}, fmt.Errorf("%q is not a paper id: ids are lowercase", s)
		case strings.Count(s, "-") < 2:
			return ID{}, fmt.Errorf("%q is not a paper id: want <surname>-<year>-<keyword>", s)
		}
		return ID{}, fmt.Errorf("%q is not a paper id: want <surname>-<year>-<keyword> in lowercase ASCII", s)
	}
	year, err := strconv.Atoi(m[2])
	if err != nil { // unreachable while the pattern says four digits
		return ID{}, fmt.Errorf("%q has no readable year: %w", s, err)
	}
	return ID{Full: s, Surname: m[1], Year: year, Keyword: m[3]}, nil
}

// ValidID reports whether s parses as a paper id.
func ValidID(s string) bool {
	_, err := ParseID(s)
	return err == nil
}

// String is the id as it is written everywhere else.
func (id ID) String() string { return id.Full }

// SectionAnchor is the anchor of a numbered section, with the dots of the
// number turned into hyphens: 3.2 of vaswani-2017-attention is
// vaswani-2017-attention-s3-2.
func SectionAnchor(paper, number string) string {
	return paper + "-s" + strings.ReplaceAll(number, ".", "-")
}

// ItemAnchor is the anchor of a numbered item other than a section: a
// statement, an equation, a figure, a table or a listing.
func ItemAnchor(paper, kind, number string) string {
	return paper + "-" + kind + "-" + strings.ReplaceAll(number, ".", "-")
}
