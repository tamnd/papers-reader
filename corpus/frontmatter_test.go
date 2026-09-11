package corpus

import (
	"reflect"
	"strings"
	"testing"
)

const front = `---
paper: vaswani-2017-attention
title: Attention Is All You Need
authors:
  - Ashish Vaswani
year: 2017
field: ai-ml
section: "3.2"
section_title: Multi-Head Attention
kind: section
lang: en
extraction: native
content_sha256: deadbeef
---

### 3.2 Multi-Head Attention {#vaswani-2017-attention-s3-2 .section tag=0A3F}

Some text.
`

func TestParseFront(t *testing.T) {
	f, body, err := ParseFront([]byte(front))
	if err != nil {
		t.Fatal(err)
	}
	if f.Paper != "vaswani-2017-attention" || f.Lang != EN || f.Kind != "section" {
		t.Errorf("front matter is %+v", f)
	}
	if f.Section != "3.2" {
		t.Errorf("section is %q, want 3.2", f.Section)
	}
	if !strings.HasPrefix(string(body), "### 3.2") {
		t.Errorf("body starts with %q", firstLine(body))
	}
	if strings.Contains(string(body), "paper:") {
		t.Error("the front matter leaked into the body")
	}
}

func TestParseFrontRejects(t *testing.T) {
	cases := map[string]string{
		"no opening fence":       "paper: x\n---\nbody\n",
		"never closed":           "---\npaper: x\nbody\n",
		"front matter is broken": "---\npaper: [x\n---\nbody\n",
	}
	for name, in := range cases {
		if _, _, err := ParseFront([]byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRenderRoundTrip(t *testing.T) {
	f, body, err := ParseFront([]byte(front))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Render(f, body)
	if err != nil {
		t.Fatal(err)
	}
	f2, body2, err := ParseFront(out)
	if err != nil {
		t.Fatalf("what Render wrote does not parse: %v", err)
	}
	if !reflect.DeepEqual(f2, f) {
		t.Errorf("front matter changed:\n%+v\n%+v", f, f2)
	}
	if string(body2) != string(body) {
		t.Errorf("body changed:\n%q\n%q", body, body2)
	}
}

// Render leaves out the fields that are empty, so a file that has never been
// translated does not carry a row of blank translation provenance.
func TestRenderOmitsWhatIsNotThere(t *testing.T) {
	out, err := Render(Front{Paper: "turing-1936-computable", Kind: "front", Lang: EN}, []byte("text\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"translated_from", "glossary_version", "small_model", "authors"} {
		if strings.Contains(string(out), unwanted) {
			t.Errorf("%s was written out empty", unwanted)
		}
	}
}

// The hash is taken over the body alone. A change to the front matter, which
// includes writing the hash itself, must not make every file in the corpus
// look stale.
func TestContentSHAIgnoresFrontMatter(t *testing.T) {
	_, body, err := ParseFront([]byte(front))
	if err != nil {
		t.Fatal(err)
	}
	before := ContentSHA(body)

	f, _, _ := ParseFront([]byte(front))
	f.TranslationModel = "something-new"
	out, err := Render(f, body)
	if err != nil {
		t.Fatal(err)
	}
	_, body2, err := ParseFront(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := ContentSHA(body2); got != before {
		t.Error("editing the front matter changed the content hash")
	}

	if ContentSHA([]byte("a\n\n")) != ContentSHA([]byte("a")) {
		t.Error("trailing newlines changed the content hash")
	}
	if ContentSHA([]byte("a")) == ContentSHA([]byte("b")) {
		t.Error("two different bodies hash the same")
	}
}

func firstLine(b []byte) string {
	s := string(b)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
