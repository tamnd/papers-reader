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

// vietnamese is the Vietnamese of section(""). The prose is different and the
// numbers are the same, because a section number is printed by the paper and
// a translator copies it.
func vietnamese() string {
	return `---
paper: a-1970-paper
title: Mot Bai Bao
section: "1"
section_title: Muc Thu Nhat
kind: section
lang: vi
---

Doan van thu nhat cua muc nay.

#### 1.1 Mot Tieu Muc

Doan van duoi tieu muc.
`
}

func TestATranslationGetsTheTagsItsEnglishWasGiven(t *testing.T) {
	root := tagsCorpus(t, section(""))
	vi := filepath.Join(root, "content/vi/a-1970-paper/01_first.md")
	if err := os.MkdirAll(filepath.Dir(vi), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vi, []byte(vietnamese()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}

	en := read(t, filepath.Join(root, "content/en/a-1970-paper/01_first.md"))
	got := read(t, vi)
	// Rule L04 compares the tag in the front matter and rule L02 compares
	// the attribute blocks span by span, so both have to match and the
	// anchor has to match with them.
	if want := "tag=" + tagOf(t, en, "tag="); !strings.Contains(got, want) {
		t.Errorf("the subsection is missing %q:\n%s", want, got)
	}
	if want := tagOf(t, en, "tag: "); !strings.Contains(got, "tag: "+want) && !strings.Contains(got, `tag: "`+want+`"`) {
		t.Errorf("the front matter is missing tag %s:\n%s", want, got)
	}
	if !strings.Contains(got, "a-1970-paper-s1-1") {
		t.Errorf("the translation did not get the English anchor:\n%s", got)
	}
}

func TestAHeadingOnlyTheTranslationHasIsGivenNothing(t *testing.T) {
	// A heading that is in a translation and not in its English is a
	// translation that went wrong. Handing it an identifier would write the
	// mistake into the register for good, where nothing is ever taken back
	// out, so the line is left as it was found and the audit reports it.
	root := tagsCorpus(t, section(""))
	vi := filepath.Join(root, "content/vi/a-1970-paper/01_first.md")
	if err := os.MkdirAll(filepath.Dir(vi), 0o755); err != nil {
		t.Fatal(err)
	}
	body := vietnamese() + "\n#### 1.2 Mot Muc Khong Co Trong Ban Goc\n\nDoan van.\n"
	if err := os.WriteFile(vi, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, vi); strings.Contains(got, "a-1970-paper-s1-2") {
		t.Errorf("a heading the English does not have was given an anchor:\n%s", got)
	}
	// Two anchors in the register, the section and its one subsection, and
	// nothing from the translation.
	if n := lines(t, filepath.Join(root, "tags", "tags")); n != 2 {
		t.Errorf("the register holds %d anchors, want 2", n)
	}
}

// tagOf is the first tag in a file, written either way the corpus writes one:
// `tag: 0001` in the front matter and `tag=0001` in an attribute block.
func tagOf(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		i := strings.Index(line, prefix)
		if i < 0 {
			continue
		}
		tag := line[i+len(prefix):]
		tag = strings.TrimRight(tag, "}")
		tag = strings.Trim(strings.TrimSpace(tag), `"`)
		if tag != "" {
			return tag
		}
	}
	t.Fatalf("no %q in:\n%s", prefix, text)
	return ""
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

// A run hands its tags out in reading order, which is the order of the files
// and then the order of the items down each file.
//
// The audit used to check this after the fact and cannot: a paper read again
// can come back with its items in a different order, their permanent tags
// come back with them, and the run no longer climbs with nothing wrong. This
// is the place the question can be settled, because here the order the
// assigner walked in is the order it walked in.
func TestARunHandsOutItsTagsInReadingOrder(t *testing.T) {
	root := tagsCorpus(t, section(""))
	write := func(path, text string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A second and a third file, so that the file order is tested as well as
	// the order within one file. The names sort the way a corpus sorts.
	write("content/en/a-1970-paper/02_second.md", `---
paper: a-1970-paper
title: A Paper
section: "2"
section_title: The Second Section
kind: section
lang: en
---

Text.

#### 2.1 A Subsection

Text.

#### 2.2 Another Subsection

Text.
`)
	write("content/en/a-1970-paper/03_third.md", `---
paper: a-1970-paper
title: A Paper
section: "3"
section_title: The Third Section
kind: section
lang: en
---

Text.
`)

	if err := runTagsAssign([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"a-1970-paper-s1",
		"a-1970-paper-s1-1",
		"a-1970-paper-s2",
		"a-1970-paper-s2-1",
		"a-1970-paper-s2-2",
		"a-1970-paper-s3",
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(read(t, filepath.Join(root, "tags", "tags"))), "\n") {
		_, anchor, ok := strings.Cut(line, ",")
		if !ok {
			t.Fatalf("the register has a line that is not tag,anchor: %q", line)
		}
		got = append(got, anchor)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the register reads\n  %s\nand reading order is\n  %s", strings.Join(got, " "), strings.Join(want, " "))
	}
}
