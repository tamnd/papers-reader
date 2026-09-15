package translate

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/llm"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/prompt"
)

// A Term is one line of the glossary as the translator needs it: one English
// term and the one rendering it has in the language being written.
//
// It is a local type and not glossary.Term for two reasons. The small one is
// that package glossary imports this one for Prose, so this one cannot import
// it back. The real one is that glossary.Term is three languages, a field, a
// note and a keep flag, all of which are a reviewer's business and none of
// which the translator has any use for. What goes in the prompt is two
// strings, and a prompt builder that took the whole record would have to
// decide again, here, which of its fields to print.
type Term struct{ En, As string }

// A Paper is what the prompt is told about the paper a passage came from.
//
// Abstract is the reason this type is not just an id. The model translating
// chunk four has not read chunk three, and fifty words of "this paper
// introduces a framework for estimating generative models via an adversarial
// process" is worth more to the translator of section 4.2 than any amount of
// glossary. It costs about eighty tokens a chunk and it is the cheapest thing
// in the prompt.
type Paper struct {
	ID       string
	Title    string
	Field    corpus.Field
	Abstract string
	// Note is what this paper adds to the shared rules, for the handful of
	// papers whose notation needs saying. Nearly always empty.
	Note string
}

// A Reply is what one ask came back with.
type Reply struct {
	llm.Response
	Model string
	Route string
}

// A Translator turns an English body into one other language, a chunk at a
// time, and refuses an answer that did not come back as the same document.
//
// Nothing here trusts the model and nothing here repairs it. An answer whose
// protected spans differ from the source's is thrown away whole and asked for
// again, which is harsh and is correct: a translation with a quietly renamed
// variable or a dropped citation is worse than no translation, because
// nothing further down the toolchain will catch it and a reader has no way to
// know.
type Translator struct {
	// Ask puts one question to the fleet. attempt counts from one, and the
	// caller is expected to send a second or third attempt somewhere better:
	// the cheap model answers most chunks correctly, the chunks it cannot do
	// come back refused, and the model that answers them next should be the
	// one above it. The escalation lives in the caller because the caller is
	// what knows about routes.
	Ask func(ctx context.Context, target string, req llm.Request, attempt int) (Reply, error)
	// Tries is how many times one chunk is asked before the file is given up
	// on. Zero means Tries.
	Tries int
	Logf  func(string, ...any)
}

// Tries is how many times one chunk is asked.
//
// Three. The first ask is the cheap route doing the work, the second is the
// same question somewhere better, and the third is that host having a bad
// minute. A fourth is a chunk the fleet cannot do today, and grinding on it
// spends the quota that the other thirty chunks of the paper need.
const Tries = 3

// A Result is one translated body and what it took.
//
// Models is every model that contributed a chunk, in the order they first
// did, because a body assembled from two routes is a fact the front matter
// should carry rather than round off to whichever answered last.
type Result struct {
	Text   string
	Models []string
	Routes []string
	Chunks int
	// Asks is how many questions were put, which is Chunks plus every refusal.
	// The difference between the two is the refusal rate, and the refusal rate
	// is how the prompt gets better.
	Asks int
	// Refused is the differences that made an answer be asked for again, in
	// the order they happened, for the run report.
	Refused []string
	Usage   llm.Usage
}

// Body translates one English body into one language.
//
// It goes chunk by chunk and joins the answers with a blank line, which
// reproduces the block structure exactly because a chunk boundary is a blank
// line the Markdown already had. A chunk that cannot be got right in Tries
// asks fails the whole file rather than being written half translated: a file
// with one English paragraph in the middle of it looks like a paper that did
// not translate that paragraph, and there is nothing in it to say otherwise.
func (t *Translator) Body(ctx context.Context, p Paper, l corpus.Lang, terms []Term, body string) (Result, error) {
	var out Result
	if strings.TrimSpace(body) == "" {
		return out, nil
	}
	tmpl, err := prompt.Get(prompt.Translate)
	if err != nil {
		return out, err
	}
	rules, err := prompt.Lang(l)
	if err != nil {
		return out, err
	}

	chunks := Chunks(body)
	out.Chunks = len(chunks)
	parts := make([]string, 0, len(chunks))
	for i, c := range chunks {
		text, err := tmpl.Render(map[string]string{
			"LANGUAGE": l.Name(),
			"SOURCE":   source(p),
			"FIELD":    field(p.Field),
			"ABSTRACT": abstract(p.Abstract),
			"GLOSSARY": Glossary(terms),
			"RULES":    strings.TrimSpace(rules.Text),
			"NOTE":     note(p.Note),
			"BODY":     c.Text,
		})
		if err != nil {
			return out, err
		}
		got, err := t.chunk(ctx, &out, fmt.Sprintf("%s %s chunk %d of %d", p.ID, l, i+1, len(chunks)), text, c)
		if err != nil {
			return out, err
		}
		parts = append(parts, got)
	}
	out.Text = strings.Join(parts, "\n\n") + "\n"
	return out, nil
}

// chunk asks for one chunk until it comes back as the same document.
func (t *Translator) chunk(ctx context.Context, out *Result, target, instructions string, c Chunk) (string, error) {
	var worst string
	for attempt := 1; attempt <= t.tries(); attempt++ {
		out.Asks++
		reply, err := t.Ask(ctx, target, llm.Request{
			Instructions: instructions,
			Input:        "Write the passage between the equals signs in the language asked for, and nothing else.",
		}, attempt)
		if err != nil {
			return "", fmt.Errorf("%s: %w", target, err)
		}
		out.Usage = add(out.Usage, reply.Usage)

		answer := Repair(c.Text, Clean(c.Text, reply.Text))
		bad := Verify(c.Text, answer)
		if len(bad) == 0 {
			out.Models = keep(out.Models, reply.Model)
			out.Routes = keep(out.Routes, reply.Route)
			return answer, nil
		}
		worst = bad[0].String()
		out.Refused = append(out.Refused, target+": "+worst)
		t.logf("%s: refused on attempt %d, %s", target, attempt, worst)
	}
	return "", fmt.Errorf("%s: %d answers were refused, the last because %s", target, t.tries(), worst)
}

func (t *Translator) tries() int {
	if t.Tries > 0 {
		return t.Tries
	}
	return Tries
}

func (t *Translator) logf(format string, args ...any) {
	if t.Logf != nil {
		t.Logf(format, args...)
	}
}

// Verify says every way an answer is not the source in another language.
//
// Four checks, and each one is a thing that happened. The spans are the
// spec's rule and the reason this package exists. The added link is the
// model that reads a web address printed as prose and writes it back as
// markup, which a span comparison passes because the address itself is
// unchanged; the harmless form of it, the address linked to itself, has
// already been undone by Unlink before this runs, so what is left to refuse
// is a link that says something the page did not. The block count is the
// preamble: a model that opens with "Here is the Vietnamese translation:"
// adds no span and would otherwise be written into the corpus as the first
// paragraph of the section. The echo is the model that returned the English
// unchanged, which a span comparison passes with nothing to say because the
// spans are, of course, identical.
//
// Nothing comes back for an answer that is right, and the first difference is
// enough to throw it away: the caller asks again rather than trying to repair
// it.
func Verify(source, answer string) []Difference {
	if strings.TrimSpace(answer) == "" {
		return []Difference{{At: 1, Why: "the answer is empty"}}
	}
	if bad := Compare(source, answer); len(bad) > 0 {
		return bad
	}
	if l := added(source, answer); l != "" {
		return []Difference{{At: 1, Why: fmt.Sprintf(
			"%s is a link the passage does not have, and the corpus has no links in it", short(l))}}
	}
	if want, got := len(blocks(source)), len(blocks(answer)); want != got {
		return []Difference{{At: 1, Why: fmt.Sprintf(
			"the passage is %d blocks and the answer is %d, so a paragraph was added, merged or dropped", want, got)}}
	}
	// Prose is the body with every protected span taken out of it, so two
	// bodies with the same prose are two bodies that differ only where nothing
	// was to be translated.
	if Prose(source) == Prose(answer) {
		return []Difference{{At: 1, Why: "the answer is the English, word for word"}}
	}
	return nil
}

// Clean takes off what a model wrapped its answer in.
//
// Only an outer fence, and only when the source was not itself one. A model
// handed a passage of markdown returns it inside a ```markdown block often
// enough to be worth undoing, and a model handed a listing returns the
// listing, which on the page looks exactly the same and must not be touched.
// The source settles which of the two an answer is and nothing else can: an
// answer that is one fenced block is both shapes at once.
//
// A preamble sentence is not stripped here, because a heuristic that ate the
// first line would eventually eat a first line that was the translation. The
// block count in Verify catches a preamble by asking again.
func Clean(source, text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(strings.TrimSpace(source), "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "```") {
		return text
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return text
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

// Glossary writes the terms the way the prompt asks for them.
//
// Longest first, which the caller has already done, because a prompt is read
// in order and "hash table" has to be decided before "table" is or the
// translator renders the two words of the phrase separately and the phrase
// comes out as neither.
func Glossary(terms []Term) string {
	if len(terms) == 0 {
		return "There is no glossary for this field yet. Whatever you choose for a\n" +
			"technical term, use the same choice everywhere in this passage."
	}
	var b strings.Builder
	for _, t := range terms {
		b.WriteString(t.En)
		b.WriteString(" ")
		b.WriteString(sep)
		b.WriteString(" ")
		b.WriteString(t.As)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// sep separates the English from its rendering in the glossary block. It is
// the same two colons the glossary prompt asks its answers in, so that a
// model that has seen one recognises the other.
const sep = "::"

func source(p Paper) string {
	switch {
	case p.Title != "" && p.ID != "":
		return fmt.Sprintf("%s (%s)", p.Title, p.ID)
	case p.Title != "":
		return p.Title
	case p.ID != "":
		return p.ID
	}
	return "a paper in this corpus"
}

func field(f corpus.Field) string {
	if f == "" {
		return "computer science"
	}
	return string(f)
}

// abstract is the context block, and nothing at all when there is no
// abstract. A heading over an empty section reads to a model as a section it
// was not given, and half of them say so in the answer.
func abstract(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "The abstract is not to hand. Translate the passage on its own terms."
	}
	return "What the paper says it is about, for context, not to be translated here:\n\n" + s
}

func note(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return "## About this paper in particular\n\n" + s
}

// keep adds a name to a list that holds each one once, in the order they
// first appeared.
func keep(list []string, name string) []string {
	if name == "" {
		return list
	}
	for _, have := range list {
		if have == name {
			return list
		}
	}
	return append(list, name)
}

// add sums what two asks cost. Every field of it, because a refused answer
// was paid for in full and a report that counted only the accepted ones would
// say the corpus was cheaper than it was.
func add(a, b llm.Usage) llm.Usage {
	b = b.Normalized()
	a.InputTokens += b.InputTokens
	a.CachedInputTokens += b.CachedInputTokens
	a.OutputTokens += b.OutputTokens
	a.ReasoningTokens += b.ReasoningTokens
	a.TotalTokens += b.TotalTokens
	return a
}
