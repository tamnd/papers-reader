package glossary

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// Every fixture here is written for the test. Nothing in this file is text
// from a paper, because the corpus is public and not everything in it is
// freely licensed.

// corpusOf makes one Text per body, all in the same field.
func corpusOf(bodies ...string) []Text {
	out := make([]Text, len(bodies))
	for i, b := range bodies {
		out[i] = Text{ID: "paper-" + string(rune('a'+i)), Field: corpus.Systems, Body: b}
	}
	return out
}

// proposed says whether a term is in the list, and what it was counted at.
func proposed(cs *Candidates, en string) (Candidate, bool) {
	for _, c := range cs.Candidates {
		if c.En == en {
			return c, true
		}
	}
	return Candidate{}, false
}

func TestATermInThreePapersIsProposedAndInTwoIsNot(t *testing.T) {
	// Three, because a word two authors both reach for is a coincidence and
	// a word three reach for is the field's word for the thing.
	texts := corpusOf(
		"The write ahead log is flushed before the commit.",
		"A write ahead log makes the commit durable.",
		"Recovery replays the write ahead log.",
		"Nothing here but a lonely quorum.",
	)
	got := Extract(texts, nil, Defaults)
	if c, ok := proposed(got, "write ahead log"); !ok || c.Papers != 3 {
		t.Errorf("write ahead log is %+v, %v", c, ok)
	}
	if _, ok := proposed(got, "quorum"); ok {
		t.Error("a word used in one paper was proposed")
	}
}

func TestMathematicsAndCodeAreNotCounted(t *testing.T) {
	// A corpus counted with the formulas in proposes a glossary of variable
	// names. The prose is what a translator translates.
	body := "The bound is $O(n \\log n)$ for every $n$.\n\n```go\nfunc quicksort(xs []int) {}\n```\n"
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	for _, c := range got.Candidates {
		for _, bad := range []string{"log", "quicksort", "func", "int"} {
			if strings.Contains(c.En, bad) {
				t.Errorf("%q came out of the mathematics or the listing", c.En)
			}
		}
	}
	if _, ok := proposed(got, "bound"); !ok {
		t.Error("the prose around the formula was not counted")
	}
}

func TestAPhraseIsCountedAsOneTerm(t *testing.T) {
	body := "The hash table is resized when the hash table is full. A hash table is not a tree."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	c, ok := proposed(got, "hash table")
	if !ok {
		t.Fatalf("hash table was not proposed: %v", names(got))
	}
	if c.Uses != 9 {
		t.Errorf("hash table is used %d times, want three in each of three papers", c.Uses)
	}
}

func TestAPhraseNeverCrossesAFullStop(t *testing.T) {
	// "the model. Attention is" is three words in a row and no part of the
	// language.
	body := "We describe the model. Attention is the mechanism."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	for _, c := range got.Candidates {
		if strings.Contains(c.En, "model attention") {
			t.Errorf("a phrase was read across the full stop: %q", c.En)
		}
	}
}

func TestAPluralIsCountedAsItsSingular(t *testing.T) {
	// Otherwise the count of a term is split across two rows and neither
	// row passes the threshold.
	texts := corpusOf(
		"The buffer is pinned. The buffer is released.",
		"Two buffers are pinned and two buffers released.",
		"A buffer pool holds the buffers.",
	)
	got := Extract(texts, nil, Defaults)
	c, ok := proposed(got, "buffer")
	if !ok {
		t.Fatalf("buffer was not proposed: %v", names(got))
	}
	if c.Papers != 3 || c.Uses != 6 {
		t.Errorf("buffer is %d papers and %d uses, want 3 and 6", c.Papers, c.Uses)
	}
	if _, ok := proposed(got, "buffers"); ok {
		t.Error("the plural was proposed as a term of its own")
	}
}

func TestAPluralWhoseSingularIsNotAWordIsLeftAlone(t *testing.T) {
	// The folding is deliberately timid. "address" must not become
	// "addres", and it only folds a word whose singular the corpus writes.
	body := "The address is resolved. The address is cached. An address is a number."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	if _, ok := proposed(got, "address"); !ok {
		t.Errorf("address was folded away: %v", names(got))
	}
}

func TestAStopWordCannotStartOrEndATerm(t *testing.T) {
	body := "The system is the thing that the paper is about, and the system is general."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	for _, c := range got.Candidates {
		w := strings.Fields(c.En)
		if stop[w[0]] || stop[w[len(w)-1]] {
			t.Errorf("%q begins or ends on a stop word", c.En)
		}
	}
}

func TestAPrepositionIsAllowedInsideATerm(t *testing.T) {
	// "proof of work" is exactly the kind of term this glossary is for, and
	// a rule that broke the phrase at "of" would lose it.
	body := "The proof of work is the cost. A proof of work is checked cheaply. Proof of work again."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	if _, ok := proposed(got, "proof of work"); !ok {
		t.Errorf("proof of work was not proposed: %v", names(got))
	}
}

func TestAConjunctionInsideAPhraseRefusesIt(t *testing.T) {
	// "input and output" is two terms with a joint between them.
	body := "We measure the input and output of the stage. The input and output are logged. Input and output again."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	if _, ok := proposed(got, "input and output"); ok {
		t.Error("two terms joined by a conjunction were proposed as one")
	}
	if _, ok := proposed(got, "input"); !ok {
		t.Error("the terms either side of the conjunction were lost with it")
	}
}

func TestAWordThatIsAlwaysPartOfAPhraseIsDropped(t *testing.T) {
	// "neural" appears in as many papers as "neural network" and means
	// nothing on its own, and a list with both in it wastes the reviewer's
	// attention twice.
	body := "A neural network is trained. The neural network is deep. Every neural network here."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	if _, ok := proposed(got, "neural network"); !ok {
		t.Fatalf("neural network was not proposed: %v", names(got))
	}
	if _, ok := proposed(got, "neural"); ok {
		t.Error("neural was proposed on its own")
	}
}

func TestAWordWithALifeOfItsOwnSurvivesTheLongerPhrase(t *testing.T) {
	// The other side of the same rule. A word that stands alone more than
	// one time in ten is a word, whatever else it is part of.
	body := "The network is partitioned. A network of hosts. Every network here. " +
		"The neural network is one network among many networks in the network."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	if _, ok := proposed(got, "network"); !ok {
		t.Errorf("network was dropped for being inside neural network: %v", names(got))
	}
}

func TestATermAlreadyInTheGlossaryIsNotProposed(t *testing.T) {
	body := "The hash table is resized. A hash table is fast. Hash table again."
	have := &Glossary{Terms: []Term{{En: "Hash Table"}}}
	got := Extract(corpusOf(body, body, body), have, Defaults)
	if _, ok := proposed(got, "hash table"); ok {
		t.Error("a term already in the glossary was proposed again")
	}
}

func TestAPhraseOutranksAWordItCovers(t *testing.T) {
	// Ranked on uses alone, every phrase loses to every word: 300
	// candidates off this corpus had five phrases among them. A term of two
	// words is two words of the corpus each time it is written.
	body := "The learning rate is decayed. The learning rate warms up. A learning rate again. " +
		"The rate is a number. Another rate. A third rate."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	phrase, word := -1, -1
	for i, c := range got.Candidates {
		switch c.En {
		case "learning rate":
			phrase = i
		case "rate":
			word = i
		}
	}
	if phrase < 0 {
		t.Fatalf("learning rate was not proposed: %v", names(got))
	}
	if word >= 0 && word < phrase {
		t.Errorf("the word outranked the phrase: %v", names(got))
	}
}

func TestAWordWithADigitInItIsNotATerm(t *testing.T) {
	body := "The p4 language targets the switch. The p4 program is compiled. And p4 again."
	got := Extract(corpusOf(body, body, body), nil, Defaults)
	for _, c := range got.Candidates {
		if strings.Contains(c.En, "p4") {
			t.Errorf("%q has a number in it", c.En)
		}
	}
}

func TestTheFieldsATermAppearsInAreRecorded(t *testing.T) {
	// What somebody deciding whether to scope a term reads first.
	body := "The reduction is linear. A reduction of the problem. Reduction again."
	texts := []Text{
		{ID: "a", Field: corpus.Theory, Body: body},
		{ID: "b", Field: corpus.Theory, Body: body},
		{ID: "c", Field: corpus.Languages, Body: body},
	}
	got := Extract(texts, nil, Defaults)
	c, ok := proposed(got, "reduction")
	if !ok {
		t.Fatalf("reduction was not proposed: %v", names(got))
	}
	if len(c.Fields) != 2 || c.Fields[0] != corpus.Languages || c.Fields[1] != corpus.Theory {
		t.Errorf("the fields are %v", c.Fields)
	}
	if len(c.Where) != 3 {
		t.Errorf("the papers are %v", c.Where)
	}
}

func TestTheListIsCappedWhereSomebodyWillStopReading(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("The " + word(i) + " is here.\n\n")
	}
	body := b.String()
	got := Extract(corpusOf(body, body, body), nil, Options{Papers: 3, Top: 10, Words: 3})
	if len(got.Candidates) != 10 {
		t.Errorf("%d candidates, want the top ten", len(got.Candidates))
	}
}

func names(cs *Candidates) []string {
	out := make([]string, 0, len(cs.Candidates))
	for _, c := range cs.Candidates {
		out = append(out, c.En)
	}
	return out
}

// word is a made up term, distinct for each n and with no digit in it.
func word(n int) string {
	const letters = "abcdefgh"
	return "term" + string(letters[n/8%8]) + string(letters[n%8]) + "x"
}
