package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// sealCorpus is a corpus with one English file in it, carrying whatever hash
// the caller wants and whatever body. Nothing here is copied from a paper.
func sealCorpus(t *testing.T, sha, body string) string {
	t.Helper()
	root := tagsCorpus(t, "")
	text := `---
paper: a-1970-paper
title: A Paper
section: "1"
section_title: The First Section
kind: section
lang: en
content_sha256: ` + sha + `
---

` + body
	writeSection(t, filepath.Join(root, "content/en/a-1970-paper/01_first.md"), text)
	return root
}

func writeSection(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSealPutsTheHashBackInStepAndSaysSo(t *testing.T) {
	body := "A paragraph somebody fixed by hand.\n"
	root := sealCorpus(t, strings.Repeat("0", 64), body)
	if err := runSeal([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "content/en/a-1970-paper/01_first.md"))
	if err != nil {
		t.Fatal(err)
	}
	front, got, err := corpus.ParseFront(b)
	if err != nil {
		t.Fatal(err)
	}
	if want := corpus.ContentSHA(got); front.ContentSHA256 != want {
		t.Errorf("content_sha256 is %s and the body hashes to %s", front.ContentSHA256, want)
	}
	if !front.Edited {
		t.Error("the file was sealed and does not say edited: true")
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(body) {
		t.Errorf("the body came back as %q and should not have been touched", string(got))
	}
}

func TestSealLeavesAFileThatAlreadyAgreesAlone(t *testing.T) {
	body := "A paragraph the pipeline wrote.\n"
	root := sealCorpus(t, corpus.ContentSHA([]byte(body)), body)
	path := filepath.Join(root, "content/en/a-1970-paper/01_first.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runSeal([]string{"-corpus", root, "-all"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a file whose hash already agrees was rewritten, so it now says a person edited it")
	}
}

func TestSealWritesNothingOnADryRun(t *testing.T) {
	root := sealCorpus(t, strings.Repeat("0", 64), "A paragraph somebody fixed by hand.\n")
	path := filepath.Join(root, "content/en/a-1970-paper/01_first.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := runSeal([]string{"-corpus", root, "-all", "-dry-run"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a dry run wrote the file")
	}
}

// Naming nothing seals nothing. A command that rewrites hashes across a
// whole corpus on a bare invocation is one keystroke from hiding damage.
func TestSealAsksToBeToldWhat(t *testing.T) {
	root := sealCorpus(t, strings.Repeat("0", 64), "A paragraph.\n")
	if err := runSeal([]string{"-corpus", root}); err == nil {
		t.Error("seal with no papers named sealed something")
	}
}
