package glossary

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/prompt"
)

// Reply is what one ask came back with.
type Reply struct {
	llm.Response
	Model string
}

// A Renderer fills in the renderings a glossary is missing, by asking a
// model and having somebody read the answer afterwards.
//
// Why a model at all, for a list of 139 terms that a person could look up.
// Because the looking up is the expensive part and the checking is not: a
// reviewer who is shown "throughput :: thông lượng" decides in a second, and
// the same reviewer facing an empty column has to go and find out what
// Vietnamese textbooks call it. The answer is a proposal in exactly the way
// the candidate list is a proposal, and nothing here writes a rendering that
// somebody has already written by hand.
type Renderer struct {
	// Ask puts one question to the fleet. The target it is given is the
	// language and the batch, so a ledger line says what the quota went on.
	Ask func(ctx context.Context, target string, req llm.Request) (Reply, error)
	// Batch is how many terms go in one question. Zero means Batch.
	Size int
	Logf func(string, ...any)
}

// Batch is how many terms go up at once.
//
// Forty, which is the balance between two failure modes. One term per ask is
// forty times the overhead and, worse, gives the model no way to be
// consistent between terms that share a word. The whole glossary in one ask
// is a long answer, and a long answer is where a model starts summarising:
// asked for 139 renderings in one go it returned 60 and a sentence saying the
// rest follow the same pattern.
const Batch = 40

// Fill asks for every rendering the glossary is missing in one language and
// writes the answers into it. It returns how many terms it filled.
//
// A term that already has a rendering is not asked about and is never
// overwritten, so a run can be repeated after a reviewer has fixed a few
// entries by hand without undoing the fixes. A term the model does not answer
// for is left empty and reported, which is a gap a person can see rather than
// a guess they cannot.
func (r *Renderer) Fill(ctx context.Context, g *Glossary, l corpus.Lang) (int, error) {
	rules, err := prompt.Lang(l)
	if err != nil {
		return 0, err
	}
	p, err := prompt.Get(prompt.GlossaryTerm)
	if err != nil {
		return 0, err
	}

	todo := g.Missing(l)
	if len(todo) == 0 {
		return 0, nil
	}
	filled := 0
	for _, batch := range batches(todo, r.size()) {
		text, err := p.Render(map[string]string{
			"LANGUAGE": l.Name(),
			"RULES":    strings.TrimSpace(rules.Text),
			"TERMS":    terms(batch),
		})
		if err != nil {
			return filled, err
		}
		target := fmt.Sprintf("glossary %s %s", l, batch[0].En)
		reply, err := r.Ask(ctx, target, llm.Request{
			Instructions: text,
			Input:        fmt.Sprintf("Give the %s rendering of each of the %d terms.", l.Name(), len(batch)),
		})
		if err != nil {
			return filled, fmt.Errorf("%s: %w", target, err)
		}
		got := Parse(reply.Text)
		for _, t := range batch {
			answer, ok := got[fold(t.En)]
			if !ok {
				r.logf("%s: %s came back without a rendering", l, t.En)
				continue
			}
			if !g.Set(t.En, l, answer) {
				continue
			}
			filled++
		}
	}
	return filled, nil
}

func (r *Renderer) size() int {
	if r.Size > 0 {
		return r.Size
	}
	return Batch
}

func (r *Renderer) logf(format string, args ...any) {
	if r.Logf != nil {
		r.Logf(format, args...)
	}
}

// batches cuts the terms into asks.
func batches(terms []Term, size int) [][]Term {
	var out [][]Term
	for i := 0; i < len(terms); i += size {
		end := min(i+size, len(terms))
		out = append(out, terms[i:end])
	}
	return out
}

// terms writes the batch the way the prompt asks for it: one term a line,
// with its note after a tab.
//
// The note goes up with the term because half of what makes this list hard is
// which sense of an English word the papers mean, and the notes are where
// that was decided. Without them "model" comes back as the machine learning
// sense every time, which is wrong for a third of this corpus.
func terms(batch []Term) string {
	var b strings.Builder
	for _, t := range batch {
		b.WriteString(t.En)
		if t.Note != "" {
			b.WriteString("\t")
			b.WriteString(t.Note)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// An Answer is one rendering as the model gave it.
type Answer struct {
	// Text is the rendering, or empty where the model said KEEP.
	Text string
	// Keep is the model saying the English stands in this language.
	Keep bool
	// Note is the alternative it was unsure about, where it said so. It goes
	// into the glossary next to the rendering, because a reviewer who is
	// shown the second best answer decides much faster than one who has to
	// think of it.
	Note string
}

// keep is the word the prompt asks for where the English stands.
const keep = "KEEP"

// sep is what separates the fields of an answer.
//
// Two colons rather than a tab or a comma. A tab does not survive a model
// that is formatting its answer, a comma is inside plenty of renderings, and
// a single colon is punctuation in Chinese and Japanese. Two colons with
// spaces round them is not something a rendering contains by accident.
const sep = "::"

// Parse reads the model's answer into renderings, keyed by the folded
// English.
//
// It is deliberately forgiving about the shape of a line and strict about
// what it takes from it. A model that has been asked for a bare list still
// sometimes numbers it, bullets it, or wraps the lot in a code fence, and
// refusing the whole batch over a leading hyphen would mean asking again for
// forty terms because of one character. What it will not do is guess: a line
// with no separator on it is not an answer and is dropped, and a term nobody
// answered for stays empty.
func Parse(text string) map[string]Answer {
	out := map[string]Answer{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimLeft(line, "0123456789.) \t")
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		parts := strings.Split(line, sep)
		if len(parts) < 2 {
			continue
		}
		en := strings.TrimSpace(parts[0])
		en = strings.Trim(en, "`*_")
		rendering := strings.TrimSpace(parts[1])
		rendering = strings.Trim(rendering, "`*_")
		if en == "" || rendering == "" {
			continue
		}
		a := Answer{Text: rendering}
		if strings.EqualFold(rendering, keep) {
			a = Answer{Keep: true}
		}
		if len(parts) > 2 {
			a.Note = strings.TrimSpace(strings.Join(parts[2:], sep))
		}
		out[fold(en)] = a
	}
	return out
}

// Set writes one rendering, and reports whether it wrote anything.
//
// It refuses to overwrite a rendering that is already there. The glossary is
// reviewed by hand and the review is the expensive part; a second run of the
// filler that quietly replaced a corrected entry would throw that away and
// leave no trace of having done it.
func (g *Glossary) Set(en string, l corpus.Lang, a Answer) bool {
	for i := range g.Terms {
		t := &g.Terms[i]
		if fold(t.En) != fold(en) {
			continue
		}
		if _, done := t.Rendering(l); done {
			return false
		}
		// KEEP is written in as the English rather than setting the Keep
		// flag, because the answer is about one language and the flag is
		// about all three. The first run of this got that wrong: the model
		// said cache and server stand in Vietnamese, which is true, the term
		// was flagged, and Chinese and Japanese were never asked. Chinese
		// says 缓存 and 服务器 and has done for thirty years.
		//
		// Written out, the file says zh: cache, which a reviewer can see is
		// wrong. Flagged, it said nothing at all in the Chinese column and
		// the coverage count called it finished.
		text := a.Text
		if a.Keep {
			text = t.En
		}
		switch l {
		case corpus.VI:
			t.Vi = text
		case corpus.ZH:
			t.Zh = text
		case corpus.JA:
			t.Ja = text
		default:
			return false
		}
		if a.Note != "" {
			t.Note = join(t.Note, string(l)+": "+a.Note)
		}
		return true
	}
	return false
}

// join puts a new note after an old one without losing either.
func join(old, add string) string {
	if old == "" {
		return add
	}
	if strings.Contains(old, add) {
		return old
	}
	return old + "; " + add
}

func fold(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Sorted is the terms in the order the file keeps them, which is
// alphabetical on the English.
//
// The file is read by people and written by a program, and a program that
// appends leaves a file nobody can find anything in. Sorting on write means
// a diff shows the terms that changed and not the terms that moved.
func (g *Glossary) Sorted() []Term {
	out := make([]Term, len(g.Terms))
	copy(out, g.Terms)
	sort.SliceStable(out, func(i, j int) bool { return fold(out[i].En) < fold(out[j].En) })
	return out
}
