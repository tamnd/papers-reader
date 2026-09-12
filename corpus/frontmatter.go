package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Front is the YAML block every content file opens with. It records what the
// paper is, where the text came from, and for a translation what it was made
// from.
//
// The provenance fields are the reason this block exists. A file that records
// the hash of the English it was translated from can be checked against the
// English as it stands now, so a corpus of four hundred translated files can
// say in one pass which of them are answers to a question that has since
// changed.
type Front struct {
	Paper        string   `yaml:"paper"`
	Title        string   `yaml:"title"`
	Authors      []string `yaml:"authors,omitempty"`
	Year         int      `yaml:"year,omitempty"`
	Venue        string   `yaml:"venue,omitempty"`
	Field        Field    `yaml:"field,omitempty"`
	Section      string   `yaml:"section,omitempty"`
	SectionTitle string   `yaml:"section_title,omitempty"`
	// Kind is front, section, references or appendix.
	Kind string `yaml:"kind"`
	Lang Lang   `yaml:"lang"`

	Source    string `yaml:"source,omitempty"`
	PDFSHA256 string `yaml:"pdf_sha256,omitempty"`
	PDFPages  string `yaml:"pdf_pages,omitempty"`
	// Extraction is native, layout or ocr, and the three are not
	// interchangeable. Native means pdftotext read a born digital file with no
	// model in the path, which makes those pages the only text in the corpus
	// that was never guessed.
	Extraction      string   `yaml:"extraction,omitempty"`
	ExtractionModel string   `yaml:"extraction_model,omitempty"`
	Figures         []string `yaml:"figures,omitempty"`
	Equations       int      `yaml:"equations,omitempty"`
	CodeBlocks      int      `yaml:"code_blocks,omitempty"`
	ContentSHA256   string   `yaml:"content_sha256,omitempty"`

	TranslatedFrom      string `yaml:"translated_from,omitempty"`
	SourceContentSHA256 string `yaml:"source_content_sha256,omitempty"`
	TranslationModel    string `yaml:"translation_model,omitempty"`
	TranslationRun      string `yaml:"translation_run,omitempty"`
	GlossaryVersion     int    `yaml:"glossary_version,omitempty"`
	GlossaryTermsSHA256 string `yaml:"glossary_terms_sha256,omitempty"`
	PromptSHA256        string `yaml:"prompt_sha256,omitempty"`
	// SmallModel and Gateway mark a section that is provisional because of
	// what wrote it. They are honesty fields: the audit flags such sections
	// rather than rejecting them, and the reading app says so on the page.
	SmallModel bool `yaml:"small_model,omitempty"`
	Gateway    bool `yaml:"gateway,omitempty"`
}

const fence = "---"

// ParseFront splits a content file into its front matter and its body.
func ParseFront(b []byte) (Front, []byte, error) {
	var f Front
	text := string(b)
	if !strings.HasPrefix(text, fence+"\n") {
		return f, nil, fmt.Errorf("the file does not open with a %s front matter fence", fence)
	}
	rest := text[len(fence)+1:]
	end := strings.Index(rest, "\n"+fence)
	if end < 0 {
		return f, nil, fmt.Errorf("the front matter is never closed with a %s line", fence)
	}
	head := rest[:end+1]
	body := rest[end+1+len(fence):]
	// Every blank line after the fence goes, not just the first, because
	// Render puts exactly one there and a parse that kept the rest would make
	// the round trip lossy for no reason anybody benefits from.
	body = strings.TrimLeft(body, "\n")
	if err := yaml.Unmarshal([]byte(head), &f); err != nil {
		return f, nil, fmt.Errorf("the front matter does not parse: %w", err)
	}
	return f, []byte(body), nil
}

// StrictFront reads the front matter again with a decoder that refuses a
// field it does not know, and returns what it made of it. It is what audit
// rule T02 asks.
//
// ParseFront is deliberately lenient: a corpus half way through a milestone
// has files written by an older version of this program, and a reader that
// refused them would make the toolchain unable to look at its own output. The
// audit is where that leniency stops. A field nobody reads is either a
// misspelling of one somebody does, which means the value it holds is being
// silently ignored, or a field somebody added without adding it here, which
// means the schema and the corpus have drifted apart.
func StrictFront(b []byte) error {
	_, _, err := ParseFront(b)
	if err != nil {
		return err
	}
	text := string(b)
	rest := text[len(fence)+1:]
	head := rest[:strings.Index(rest, "\n"+fence)+1]

	dec := yaml.NewDecoder(strings.NewReader(head))
	dec.KnownFields(true)
	var f Front
	if err := dec.Decode(&f); err != nil {
		return err
	}
	return nil
}

// Render writes a content file: the front matter, then the body.
func Render(f Front, body []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	out := []byte(fence + "\n")
	out = append(out, buf.Bytes()...)
	out = append(out, []byte(fence+"\n\n")...)
	out = append(out, bytes.TrimLeft(body, "\n")...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}

// ContentSHA is the hash recorded in content_sha256 and compared against by
// audit rule T03. It is taken over the body alone, so that rewriting the front
// matter does not make every file in the corpus look stale.
func ContentSHA(body []byte) string {
	sum := sha256.Sum256(bytes.TrimRight(body, "\n"))
	return hex.EncodeToString(sum[:])
}
