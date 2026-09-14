package prompt

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// Every prompt in the directory is readable and hashes to something. A file
// added and not embedded is the failure this catches, and it is silent
// otherwise: the binary just carries yesterday's text.
func TestEveryPromptIsThere(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("the package embeds no prompts at all")
	}
	for _, p := range all {
		if strings.TrimSpace(p.Text) == "" {
			t.Errorf("%s is empty", p.Name)
		}
		if len(p.SHA) != 64 {
			t.Errorf("%s hashed to %q", p.Name, p.SHA)
		}
	}
}

// The reading prompt is fetched by a constant and a typo in it should be a
// panic at the first page and not a page read with an empty instruction.
func TestTheReadingPromptIsNamedCorrectly(t *testing.T) {
	p := MustGet(OCR)
	if p.Name != OCR {
		t.Errorf("the reading prompt is named %q", p.Name)
	}
	if _, err := Get("ocr_page"); err == nil {
		t.Error("a prompt that does not exist was found anyway")
	}
}

// A prompt with a placeholder nobody fills is a prompt sent to a model with
// {{SOMETHING}} still in it, and the model reads that as an instruction it
// cannot follow. Render refuses that, so the way it actually goes wrong is a
// caller filling LANGUAGE while the file asks for LANG, and a run that stops
// at the first ask with no clue which end the typo is at.
//
// So the variables are written down here as well as in the file, and the two
// have to agree. A prompt not in the table takes none, which is the common
// case and the one to keep common.
func TestEveryPlaceholderIsOneSomebodyFills(t *testing.T) {
	want := map[string][]string{
		GlossaryTerm:   {"LANGUAGE", "RULES", "TERMS"},
		Translate:      {"ABSTRACT", "BODY", "FIELD", "GLOSSARY", "LANGUAGE", "NOTE", "RULES", "SOURCE"},
		RoundtripBack:  {"BODY", "FIELD", "SOURCE"},
		RoundtripJudge: {"BACK", "ENGLISH", "LANGUAGE"},
	}
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		got := p.Vars()
		slices.Sort(got)
		if !slices.Equal(got, want[p.Name]) {
			t.Errorf("%s wants %v and this package fills %v", p.Name, got, want[p.Name])
		}
	}
}

// The language rules are looked up by building a file name out of a language
// code, so a language with no file is a missing file rather than a compile
// error, and the way to find out is to ask for all of them.
func TestEveryLanguageTheCorpusPublishesHasItsRules(t *testing.T) {
	for _, l := range corpus.Langs {
		p, err := Lang(l)
		if l == corpus.EN {
			if err == nil {
				t.Error("English has language rules, and nothing is translated into English here")
			}
			continue
		}
		if err != nil {
			t.Errorf("%s has no rules: %v", l, err)
			continue
		}
		if len(p.Text) < 200 {
			t.Errorf("the rules for %s are %d characters, which is not rules", l, len(p.Text))
		}
	}
}

// A paper with no note is read with the shared prompt byte for byte, which is
// the whole reason notes are separate files. A paper whose note changed must
// not take every other paper's pages down with it.
func TestAPaperWithNoNoteGetsTheSharedPromptExactly(t *testing.T) {
	shared := MustGet(OCR)
	plain, err := Page("a-paper-with-no-note")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Text != shared.Text || plain.SHA != shared.SHA {
		t.Error("a paper with no note was read with something other than the shared prompt")
	}
	if none, err := Page(""); err != nil || none.SHA != shared.SHA {
		t.Errorf("an empty paper id came back as %q, %v", none.SHA, err)
	}
}

// A note is added under the shared prompt and changes the hash, because the
// hash has to identify what the model was actually shown.
func TestANotedPaperIsAskedADifferentQuestion(t *testing.T) {
	noted := Noted()
	if len(noted) == 0 {
		t.Skip("no paper has a note yet")
	}
	shared := MustGet(OCR)
	for _, id := range noted {
		p, err := Page(id)
		if err != nil {
			t.Fatal(err)
		}
		if p.SHA == shared.SHA {
			t.Errorf("%s has a note and hashes the same as the shared prompt", id)
		}
		if !strings.HasPrefix(p.Text, strings.TrimSpace(shared.Text)) {
			t.Errorf("%s lost the shared rules", id)
		}
		note, ok := Note(id)
		if !ok {
			t.Fatalf("%s is listed as noted and has no note", id)
		}
		if !strings.HasSuffix(p.Text, note) {
			t.Errorf("%s does not end in its own note", id)
		}
		if p.Name != OCR {
			t.Errorf("%s is read under the name %q", id, p.Name)
		}
	}
}

// Noted lists the ids and not the file names, because the caller has a paper
// id in its hand and nothing else.
func TestNotedListsPaperIds(t *testing.T) {
	for _, id := range Noted() {
		if strings.HasSuffix(id, ".md") || strings.Contains(id, "/") {
			t.Errorf("%q is a file name and not a paper id", id)
		}
		if _, ok := Note(id); !ok {
			t.Errorf("%s is listed and cannot be read", id)
		}
	}
	if _, ok := Note("../ocr_paper"); ok {
		t.Error("a paper id climbed out of the notes directory")
	}
}

// The hash a translated page records has to move when either half of the
// prompt that produced it moves, and it has to be a different hash for each
// language, or a change to the Vietnamese rules would requeue the Japanese.
func TestTheTranslationHashCoversTheLanguageRules(t *testing.T) {
	body := MustGet(Translate)
	seen := map[string]corpus.Lang{}
	for _, l := range []corpus.Lang{corpus.VI, corpus.ZH, corpus.JA} {
		sha, err := TranslationSHA(l)
		if err != nil {
			t.Fatal(err)
		}
		if sha == body.SHA {
			t.Errorf("%s records the body prompt's own hash, so its language rules are not in it", l)
		}
		if was, ok := seen[sha]; ok {
			t.Errorf("%s and %s record the same hash", l, was)
		}
		seen[sha] = l
	}
	if _, err := TranslationSHA(corpus.EN); err == nil {
		t.Error("English has a translation hash and nothing is translated into English")
	}
}
