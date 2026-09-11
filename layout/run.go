package layout

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Missing is the error for a layout tool that is not installed. It names the
// tool and how to get it, because all three are Python programs installed in
// their own way and "exec: mineru: not found" leaves a person guessing.
type Missing struct {
	Tool Tool
}

func (m *Missing) Error() string {
	return fmt.Sprintf("%s is not installed, and it is what the layout path runs: %s", m.Tool, install(m.Tool))
}

// install is how to get one of the tools. Kept as prose rather than as a
// command to copy, because all three are large Python programs with model
// weights and CUDA opinions and a one line install is a lie for at least one
// of them on at least one machine.
func install(t Tool) string {
	switch t {
	case MinerU:
		return "pip install mineru, then mineru-models-download for the weights"
	case Marker:
		return "pip install marker-pdf"
	case Docling:
		return "pip install docling"
	}
	return "it is not a tool this runs"
}

// command is the program name to look for on the path. Only docling calls its
// command the same thing the package is called.
func command(t Tool) string {
	switch t {
	case MinerU:
		return "mineru"
	case Marker:
		return "marker_single"
	case Docling:
		return "docling"
	}
	return ""
}

// Have reports whether a tool is on the path.
func Have(t Tool) bool {
	name := command(t)
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// Installed is every tool that is on the path, in preference order. It is
// what `papers doctor` reports and what Pick chooses from.
func Installed() []Tool {
	var out []Tool
	for _, t := range Tools {
		if Have(t) {
			out = append(out, t)
		}
	}
	return out
}

// Pick is the tool to use when the caller did not name one: the first
// installed, in the order of Tools.
func Pick() (Tool, error) {
	if have := Installed(); len(have) > 0 {
		return have[0], nil
	}
	return "", fmt.Errorf("none of %s is installed, so the layout path cannot run: %s", names(), install(MinerU))
}

// Version is what a tool says about itself, for doctor to print.
func Version(ctx context.Context, t Tool) (string, error) {
	name := command(t)
	if !Have(t) {
		return "", &Missing{Tool: t}
	}
	cmd := exec.CommandContext(ctx, name, "--version")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	for _, line := range strings.Split(out.String()+"\n"+errb.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("%s said nothing about its version", name)
}

// Run converts one PDF and reads the result.
//
// The output directory is the caller's and is not cleaned up, because these
// tools take minutes per paper on a GPU and rather longer without one. A
// second run over a directory that already holds the JSON should be free, so
// the check for that comes first: this is the resume point for the layout
// path in the same way the page store is the resume point for the native one.
func Run(ctx context.Context, t Tool, pdf, dir string) (*Document, error) {
	if t == "" {
		var err error
		if t, err = Pick(); err != nil {
			return nil, err
		}
	}
	if !Have(t) {
		return nil, &Missing{Tool: t}
	}
	if d, err := OpenDir(dir); err == nil {
		return d, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name, args, err := commandLine(t, pdf, dir)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.Stdout = &errb
	if err := cmd.Run(); err != nil {
		if msg := last(errb.String()); msg != "" {
			return nil, fmt.Errorf("%s: %s", name, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	d, err := OpenDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%s finished but wrote nothing this can read: %w", name, err)
	}
	return d, nil
}

// commandLine is the program and arguments for one tool.
//
// All three take a PDF and a directory and all three spell it differently,
// and two of them write into a subdirectory named after the file rather than
// into the directory they were given. That last part is why OpenDir walks
// rather than opening a fixed path.
func commandLine(t Tool, pdf, dir string) (string, []string, error) {
	switch t {
	case MinerU:
		// -m auto lets MinerU decide between its OCR and text pipelines,
		// which is the whole reason to run it over a scanned paper.
		return "mineru", []string{"-p", pdf, "-o", dir, "-m", "auto"}, nil
	case Marker:
		// Marker's JSON carries the boxes and the block types; its Markdown
		// does not, and the corpus writes its own Markdown anyway.
		return "marker_single", []string{pdf, "--output_format", "json", "--output_dir", dir}, nil
	case Docling:
		return "docling", []string{"--to", "json", "--output", dir, pdf}, nil
	}
	return "", nil, fmt.Errorf("layout: %q is not a tool this runs, only %s", t, names())
}

// Done reports whether a directory already holds output this can read, so a
// caller can skip a paper without paying for the parse.
func Done(dir string) bool {
	if _, err := os.Stat(dir); err != nil {
		return false
	}
	all, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(all) == 0 {
		// Two of the three write into a subdirectory named after the PDF.
		sub, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
		if err != nil {
			return false
		}
		all = sub
	}
	for _, name := range all {
		b, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		if _, err := Detect(b); err == nil {
			return true
		}
	}
	return false
}

// last is the final non-empty line of a tool's output, which is where a
// Python program puts the thing that actually went wrong. The first line of a
// traceback says "Traceback (most recent call last):" and nothing else.
func last(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
