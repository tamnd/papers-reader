// Package tags is the register of permanent identifiers.
//
// A tag is four hex characters attached to a section, a numbered statement, a
// numbered equation, a figure, a table or a listing. Tags are append only,
// never reused and never edited. A tag is what lets the four languages point
// at the same paragraph, and what the citation graph joins on.
//
// The scheme is the Stacks Project's, by way of tamnd/bourbaki. The property
// that matters is that a tag survives re-extraction, re-splitting and
// renumbering: a link written today still resolves after the paper has been
// read again by a better model.
package tags

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Tag is four uppercase hex characters.
type Tag string

var tagPattern = regexp.MustCompile(`^[0-9A-F]{4}$`)

// ParseTag reads a tag and rejects anything that is not one. Lowercase is
// accepted on the way in and normalised, because a person typing a tag from a
// page should not have to hold shift.
func ParseTag(s string) (Tag, error) {
	up := strings.ToUpper(strings.TrimSpace(s))
	if !tagPattern.MatchString(up) {
		return "", fmt.Errorf("%q is not a tag: want four hex characters", s)
	}
	return Tag(up), nil
}

// Value is the tag as a number, which is how the register decides what comes
// next and whether tags climb in reading order.
func (t Tag) Value() int {
	n, err := strconv.ParseInt(string(t), 16, 32)
	if err != nil {
		return -1
	}
	return int(n)
}

// Entry is one line of tags/tags: a tag and the anchor it names.
type Entry struct {
	Tag    Tag
	Anchor string
}

// Register is tags/tags, in memory. It is append only and refuses anything
// that would break that, because the whole value of a tag is that it means
// one thing for ever.
type Register struct {
	entries []Entry
	byTag   map[Tag]string
	byAnchr map[string]Tag
}

// NewRegister returns an empty register.
func NewRegister() *Register {
	return &Register{byTag: map[Tag]string{}, byAnchr: map[string]Tag{}}
}

// Read parses a tag register, one "tag,anchor" line at a time. A blank line
// is skipped. Anything else is an error naming the line, because a register
// that silently dropped a line would hand out a tag that is already in use.
func Read(r io.Reader) (*Register, error) {
	reg := NewRegister()
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		tag, anchor, ok := strings.Cut(text, ",")
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not tag,anchor", line, text)
		}
		t, err := ParseTag(tag)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if err := reg.Add(t, strings.TrimSpace(anchor)); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
	}
	return reg, sc.Err()
}

// Load reads the register from a file. A register that does not exist yet is
// an empty one, which is what a corpus before its first tag run has.
func Load(path string) (*Register, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return NewRegister(), nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reg, err := Read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return reg, nil
}

// Add records a tag against an anchor.
func (r *Register) Add(t Tag, anchor string) error {
	if anchor == "" {
		return fmt.Errorf("tag %s names nothing", t)
	}
	if had, ok := r.byTag[t]; ok {
		if had == anchor {
			return nil
		}
		return fmt.Errorf("tag %s is already %s and tags are never reused", t, had)
	}
	if had, ok := r.byAnchr[anchor]; ok {
		return fmt.Errorf("%s already carries tag %s", anchor, had)
	}
	r.entries = append(r.entries, Entry{Tag: t, Anchor: anchor})
	r.byTag[t] = anchor
	r.byAnchr[anchor] = t
	return nil
}

// Anchor is what a tag names.
func (r *Register) Anchor(t Tag) (string, bool) { a, ok := r.byTag[t]; return a, ok }

// Tag is what an anchor carries.
func (r *Register) Tag(anchor string) (Tag, bool) { t, ok := r.byAnchr[anchor]; return t, ok }

// Len is how many tags have been handed out.
func (r *Register) Len() int { return len(r.entries) }

// Entries is the register in the order it was read or written.
func (r *Register) Entries() []Entry { return append([]Entry(nil), r.entries...) }

// Next is the tag after the highest one handed out so far.
//
// It climbs from the maximum rather than filling the lowest gap, because a
// gap in the register is a tag that was retired and reusing it is the one
// thing a permanent identifier may never do.
func (r *Register) Next() (Tag, error) {
	high := -1
	for t := range r.byTag {
		if v := t.Value(); v > high {
			high = v
		}
	}
	if high+1 >= 0x10000 {
		return "", fmt.Errorf("every one of the 65536 tags is taken")
	}
	return Tag(fmt.Sprintf("%04X", high+1)), nil
}

// Write writes the register out, lowest tag first, one line per entry. The
// file is sorted rather than appended to so that two runs on two machines
// produce the same bytes.
func (r *Register) Write(w io.Writer) error {
	entries := r.Entries()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Tag < entries[j].Tag })
	bw := bufio.NewWriter(w)
	for _, e := range entries {
		if _, err := fmt.Fprintf(bw, "%s,%s\n", e.Tag, e.Anchor); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// attrPattern matches the attribute block a tagged item carries:
//
//	{#vaswani-2017-attention-s3-2 .section tag=0A3F}
var attrPattern = regexp.MustCompile(`\{#([A-Za-z0-9][-A-Za-z0-9_.]*)((?:\s+\.[a-z]+)*)\s+tag=([0-9A-Fa-f]{4})\}`)

// Attr is a parsed attribute block.
type Attr struct {
	Anchor  string
	Classes []string
	Tag     Tag
}

// ParseAttrs finds every attribute block in a body, in the order they appear,
// which is reading order.
func ParseAttrs(body string) []Attr {
	var out []Attr
	for _, m := range attrPattern.FindAllStringSubmatch(body, -1) {
		tag, err := ParseTag(m[3])
		if err != nil {
			continue
		}
		var classes []string
		for _, c := range strings.Fields(m[2]) {
			classes = append(classes, strings.TrimPrefix(c, "."))
		}
		out = append(out, Attr{Anchor: m[1], Classes: classes, Tag: tag})
	}
	return out
}

// Format writes an attribute block the way the corpus spells it.
func Format(a Attr) string {
	var b strings.Builder
	b.WriteString("{#" + a.Anchor)
	for _, c := range a.Classes {
		b.WriteString(" ." + c)
	}
	b.WriteString(" tag=" + string(a.Tag) + "}")
	return b.String()
}
