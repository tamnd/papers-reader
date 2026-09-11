package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Detect says which tool wrote a JSON file, by looking for the field that
// only that tool writes.
//
// Sniffed rather than taken from a command line flag because the file on disk
// is the truth. A paper is often extracted once and read back months later, by
// a person who no longer remembers which tool was installed that week, and a
// wrong flag does not fail: it parses into an empty document and the paper
// quietly loses its body.
func Detect(b []byte) (Tool, error) {
	var probe struct {
		// Docling: a schema name, and the parallel lists.
		Schema string          `json:"schema_name"`
		Texts  json.RawMessage `json:"texts"`
		Body   json.RawMessage `json:"body"`
		// Marker: a block type at the root, and children under it.
		BlockType string          `json:"block_type"`
		Children  json.RawMessage `json:"children"`
		// MinerU middle.json: the page list.
		Info json.RawMessage `json:"pdf_info"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		// MinerU's content_list.json is a bare array, which is not an object
		// and so fails the probe above. Nothing else this reads is an array.
		if isArray(b) {
			return MinerU, nil
		}
		return "", fmt.Errorf("layout: this is not JSON any of %s wrote: %w", names(), err)
	}
	switch {
	case len(probe.Info) > 0:
		return MinerU, nil
	case strings.Contains(strings.ToLower(probe.Schema), "docling"),
		len(probe.Texts) > 0 && len(probe.Body) > 0:
		return Docling, nil
	case probe.BlockType != "" || len(probe.Children) > 0:
		return Marker, nil
	}
	if isArray(b) {
		return MinerU, nil
	}
	return "", fmt.Errorf("layout: this is not JSON any of %s wrote", names())
}

// Parse reads a tool's JSON, working out which tool wrote it.
func Parse(b []byte) (*Document, error) {
	tool, err := Detect(b)
	if err != nil {
		return nil, err
	}
	return ParseAs(tool, b)
}

// ParseAs reads a tool's JSON, having been told which tool wrote it.
func ParseAs(tool Tool, b []byte) (*Document, error) {
	switch tool {
	case MinerU:
		return parseMinerU(b)
	case Marker:
		return parseMarker(b)
	case Docling:
		return parseDocling(b)
	}
	return nil, fmt.Errorf("layout: %q is not a tool this reads, only %s", tool, names())
}

// Open reads one JSON file.
func Open(name string) (*Document, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	d, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}

// OpenDir reads whichever of a tool's output files is in a directory.
//
// Every one of the three writes a directory, not a file, and each has its own
// idea of what to call the JSON in it. Rather than make the caller learn three
// layouts, the known names are tried in order and then, failing those, every
// JSON file in the directory is sniffed. The preference order matters for
// MinerU, which writes both a middle.json with boxes and a content_list.json
// without: taking the richer one means the figure stage has boxes to crop from.
func OpenDir(dir string) (*Document, error) {
	known := []string{
		"middle.json",       // MinerU, with boxes
		"content_list.json", // MinerU, without
		"document.json",     // docling
		"docling.json",      // docling, older
	}
	for _, name := range known {
		matches, _ := filepath.Glob(filepath.Join(dir, "*"+name))
		sort.Strings(matches)
		for _, m := range matches {
			return Open(m)
		}
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(all)
	for _, name := range all {
		d, err := Open(name)
		if err == nil {
			return d, nil
		}
	}
	return nil, fmt.Errorf("%s: no file in here was written by any of %s", dir, names())
}

// isArray reports whether the first thing in the file is a [.
func isArray(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case '[':
			return true
		}
		return false
	}
	return false
}

// names is the three tools written out, for an error message.
func names() string {
	out := make([]string, 0, len(Tools))
	for _, t := range Tools {
		out = append(out, string(t))
	}
	return strings.Join(out[:len(out)-1], ", ") + " or " + out[len(out)-1]
}
