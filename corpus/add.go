package corpus

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppendPaper writes one new entry into manifests/papers.yaml, at the end of
// the group its field belongs to.
//
// The entry is rendered through yaml so that the quoting is right, and then
// spliced into the file as text so that nothing else in it is touched. The
// obvious implementation is to decode the whole file into a yaml.Node, append
// to the sequence and encode it back, and it does not work. yaml.v3 carries
// comments through a node round trip and it does not carry the blank line
// between two entries, nor the places where a long author list was wrapped by
// hand. Encoding the file we have back out moves 158 lines and adds one paper,
// which is a diff nobody can review to make a change anybody could have made
// by hand.
//
// So the node is read for two things only: where the last entry of the group
// ends, and which ids are already taken. Everything above and below the
// insertion point comes out byte for byte the way it went in.
func AppendPaper(path string, p Paper) error {
	if p.ID == "" {
		return fmt.Errorf("a manifest entry needs an id")
	}
	if _, err := ParseID(p.ID); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	seq, err := paperNodes(b, path)
	if err != nil {
		return err
	}

	var group, last int
	for _, n := range seq {
		var had struct {
			ID    string `yaml:"id"`
			Field Field  `yaml:"field"`
		}
		if err := n.Decode(&had); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if had.ID == p.ID {
			return fmt.Errorf("%s is already in %s", p.ID, path)
		}
		last = endLine(n)
		if had.Field == p.Field {
			group = last
		}
	}
	// The end of the group it belongs to, or the end of the file for a field
	// the manifest has never held before. A new field starting at the bottom
	// is right: the groups are in catalogue order and the comment that names
	// each one is written by hand, so a program guessing where a new group
	// belongs would guess wrong as often as it guessed right.
	at := group
	if at == 0 {
		at = last
	}
	if at == 0 {
		return fmt.Errorf("%s has no papers in it", path)
	}

	text, err := EntryText(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, splice(b, at, text), 0o644)
}

// paperNodes is the sequence under the papers key, one node per entry.
func paperNodes(b []byte, path string) ([]*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a mapping", path)
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "papers" {
			continue
		}
		if top.Content[i+1].Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s: papers is not a list", path)
		}
		return top.Content[i+1].Content, nil
	}
	return nil, fmt.Errorf("%s has no papers key", path)
}

// endLine is the last line of the file that this node covers.
//
// A node reports the line it starts on and not the line it ends on, so the
// end is the furthest start of anything inside it. That is right for this
// manifest, where every value is a scalar or a flow sequence, and a block
// scalar would need its own newlines counted, so they are.
func endLine(n *yaml.Node) int {
	last := n.Line
	if n.Kind == yaml.ScalarNode {
		switch n.Style {
		case yaml.LiteralStyle, yaml.FoldedStyle:
			last += strings.Count(strings.TrimRight(n.Value, "\n"), "\n") + 1
		}
	}
	for _, c := range n.Content {
		last = max(last, endLine(c))
	}
	return last
}

// EntryText is one manifest entry as it should read in the file: a list item
// indented by two, with the author lists in the flow style the rest of the
// file uses.
//
// It is exported because papers add prints the entry before it writes it, and
// printing something other than what will be written would defeat the point
// of printing it.
func EntryText(p Paper) (string, error) {
	var n yaml.Node
	if err := n.Encode(p); err != nil {
		return "", err
	}
	flow(&n, "authors", "aka", "prerequisites")
	b, err := yaml.Marshal(&n)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = "  - " + line
			continue
		}
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// flow puts the named keys of a mapping on one line.
//
// The manifest writes an author list as [A, B, C] and yaml.Marshal writes it
// as a bullet per author. Both parse, and a file where the hundredth entry is
// laid out differently from the other ninety nine is a file that says a
// program wrote that one.
func flow(n *yaml.Node, keys ...string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		for _, k := range keys {
			if n.Content[i].Value == k && n.Content[i+1].Kind == yaml.SequenceNode {
				n.Content[i+1].Style = yaml.FlowStyle
			}
		}
	}
}

// splice puts text into b after line at, with a blank line in front of it,
// which is how the entries of this manifest are separated.
func splice(b []byte, at int, text string) []byte {
	lines := bytes.SplitAfter(b, []byte("\n"))
	var out bytes.Buffer
	for i, line := range lines {
		out.Write(line)
		if i+1 != at {
			continue
		}
		if !bytes.HasSuffix(line, []byte("\n")) {
			out.WriteByte('\n')
		}
		out.WriteByte('\n')
		out.WriteString(text)
	}
	return out.Bytes()
}
