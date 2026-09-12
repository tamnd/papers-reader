package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tagsCorpus writes the smallest corpus that tags assign will look at: a
// manifest with one paper in it and one content file.
func tagsCorpus(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifests/papers.yaml", `papers:
  - id: a-1970-paper
    title: A Paper
    authors: [A. Author]
    year: 1970
    venue: A Journal
    field: theory
    status: listed
`)
	write("content/en/a-1970-paper/01_first.md", body)
	return root
}

// section is one content file. The front matter is the least the parser will
// take, and the body is typeset here rather than taken from any paper.
func section(tag string) string {
	front := `---
paper: a-1970-paper
title: A Paper
section: "1"
section_title: The First Section
kind: section
lang: en
`
	if tag != "" {
		front += "tag: \"" + tag + "\"\n"
	}
	return front + `---

The first paragraph of the section.

#### 1.1 A Subsection

The paragraph under the subsection.
`
}

func TestAssigningTagsToAFileThatHasNoneHandsThemOut(t *testing.T) {
	root := tagsCorpus(t, section(""))
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(root, "content/en/a-1970-paper/01_first.md"))
	if !strings.Contains(got, "tag: ") {
		t.Errorf("the section got no tag:\n%s", got)
	}
	if !strings.Contains(got, "a-1970-paper-s1-1") {
		t.Errorf("the subsection got no anchor:\n%s", got)
	}
	if n := lines(t, filepath.Join(root, "tags", "runs")); n != 1 {
		t.Errorf("a run that handed out tags wrote %d lines to runs, want 1", n)
	}
}

func TestTagsAreWrittenBackIntoAFileTheSplitterRewrote(t *testing.T) {
	// The splitter writes a content file from the assembled page text, which
	// has no attribute blocks in it, so every re-split drops the tags out of
	// the files while the register keeps them. A run that hands out nothing
	// new still has to put those back, and the bug this covers was that it
	// wrote nothing at all, quietly, and said so in a line nobody reads.
	root := tagsCorpus(t, section(""))
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "content/en/a-1970-paper/01_first.md")
	before := read(t, path)

	// What a re-split leaves behind.
	if err := os.WriteFile(path, []byte(section("")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != before {
		t.Errorf("the tags did not come back:\n%s\nwant\n%s", got, before)
	}
	// The register did not change and neither did the runs, because nothing
	// was handed out. A second line here would tell audit rule G06 that a
	// section was inserted when none was.
	if n := lines(t, filepath.Join(root, "tags", "runs")); n != 1 {
		t.Errorf("a run that put tags back wrote %d lines to runs, want 1", n)
	}
}

func TestAssigningTagsTwiceChangesNothing(t *testing.T) {
	root := tagsCorpus(t, section(""))
	path := filepath.Join(root, "content/en/a-1970-paper/01_first.md")
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	before := read(t, path)
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != before {
		t.Errorf("a second run rewrote the file:\n%s", got)
	}
}

func TestADryRunWritesNothing(t *testing.T) {
	root := tagsCorpus(t, section(""))
	path := filepath.Join(root, "content/en/a-1970-paper/01_first.md")
	before := read(t, path)
	if err := runTagsAssign([]string{"-corpus", root, "-all", "-dry-run"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != before {
		t.Errorf("a dry run rewrote the file:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "tags", "tags")); !os.IsNotExist(err) {
		t.Error("a dry run wrote the register")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if s := strings.TrimSpace(l); s != "" && !strings.HasPrefix(s, "#") {
			n++
		}
	}
	return n
}

// twice is one content file with the same unnumbered heading in it twice,
// which is what the ResNet appendix does: two sections headed MS COCO, one
// under Object Detection Baselines and one under Object Detection
// Improvements.
func twice() string {
	return `---
paper: a-1970-paper
title: A Paper
section: "1"
section_title: The First Section
kind: section
lang: en
---

#### A Baselines

#### Some Benchmark

The first measurement.

#### B Improvements

#### Some Benchmark

The second measurement.
`
}

func TestTwoHeadingsOfOneNameGetTwoAnchors(t *testing.T) {
	root := tagsCorpus(t, twice())
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(root, "content/en/a-1970-paper/01_first.md"))
	if n := strings.Count(got, "a-1970-paper-s-some-benchmark "); n != 1 {
		t.Errorf("the first heading of the name has %d anchors, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "a-1970-paper-s-some-benchmark-2") {
		t.Errorf("the second heading of the name did not get its own anchor:\n%s", got)
	}
}

// The tags in a file have to climb, which is what rule G06 checks, and two
// headings sharing an anchor is exactly how they stopped.
func TestTwoHeadingsOfOneNameGetTwoTags(t *testing.T) {
	root := tagsCorpus(t, twice())
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(root, "content/en/a-1970-paper/01_first.md"))
	seen := map[string]bool{}
	for _, line := range strings.Split(got, "\n") {
		i := strings.Index(line, "tag=")
		if i < 0 {
			continue
		}
		tag := strings.TrimRight(line[i+len("tag="):], "}")
		if seen[tag] {
			t.Errorf("tag %s is on two things:\n%s", tag, got)
		}
		seen[tag] = true
	}
}
