package glossary

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/translate"
)

// A Candidate is a term the extractor proposes for the glossary.
//
// Papers is the count that matters and Uses is the tie-break. A phrase used
// forty times in one paper is that paper's notation and belongs in a note
// about that paper; a phrase used three times each in nine papers is the
// corpus's vocabulary and is what the glossary is for.
type Candidate struct {
	En     string         `yaml:"en"`
	Papers int            `yaml:"papers"`
	Uses   int            `yaml:"uses"`
	Fields []corpus.Field `yaml:"fields,omitempty"`
	// Where is a few of the papers it turns up in, so that somebody deciding
	// whether a term is scoped to a field can go and look without grepping.
	Where []string `yaml:"where,omitempty"`
}

// Candidates is what papers glossary extract writes, in the order it
// proposes them.
type Candidates struct {
	Candidates []Candidate `yaml:"candidates"`
}

// Options is how the extractor is tuned.
type Options struct {
	// Papers is the least number of papers a term must appear in. Three,
	// because a term in two papers is a coincidence between two authors and
	// a term in three is the field's word for the thing.
	Papers int
	// Top is how many candidates to keep. A person reviews this list by
	// hand, so a list nobody will finish reading is a list nobody will use.
	Top int
	// Words is the longest phrase to propose. Three, because "hash table"
	// and "bloom filter" are two, "recurrent neural network" is three, and
	// at four the list fills with sentence fragments.
	Words int
}

// Defaults are the extractor's settings, which are the ones every
// measurement in this package was made with.
var Defaults = Options{Papers: 3, Top: 300, Words: 3}

// A Text is one paper's English, as the extractor reads it.
type Text struct {
	ID    string
	Field corpus.Field
	Body  string
}

// Extract proposes candidates by how many papers use them.
//
// The prose is what is counted: the mathematics, the listings, the citation
// markers and the tag attributes are taken out first, because a corpus
// counted with them in proposes a glossary of variable names.
func Extract(texts []Text, have *Glossary, opt Options) *Candidates {
	if opt.Papers <= 0 {
		opt.Papers = Defaults.Papers
	}
	if opt.Words <= 0 {
		opt.Words = Defaults.Words
	}
	type tally struct {
		papers map[string]bool
		fields map[corpus.Field]bool
		uses   int
	}
	prose := make([]string, len(texts))
	for i, t := range texts {
		prose[i] = translate.Prose(t.Body)
	}
	one := singulars(prose)

	seen := map[string]*tally{}
	for i, t := range texts {
		for phrase, n := range phrases(prose[i], opt.Words, one) {
			e := seen[phrase]
			if e == nil {
				e = &tally{papers: map[string]bool{}, fields: map[corpus.Field]bool{}}
				seen[phrase] = e
			}
			e.papers[t.ID] = true
			e.fields[t.Field] = true
			e.uses += n
		}
	}

	var out []Candidate
	for phrase, e := range seen {
		if len(e.papers) < opt.Papers {
			continue
		}
		if have != nil && have.Has(phrase) {
			continue
		}
		c := Candidate{En: phrase, Papers: len(e.papers), Uses: e.uses}
		for f := range e.fields {
			if f != "" {
				c.Fields = append(c.Fields, f)
			}
		}
		sort.Slice(c.Fields, func(i, j int) bool { return c.Fields[i] < c.Fields[j] })
		for id := range e.papers {
			c.Where = append(c.Where, id)
		}
		sort.Strings(c.Where)
		if len(c.Where) > 4 {
			c.Where = c.Where[:4]
		}
		out = append(out, c)
	}
	out = whole(out)
	sort.Slice(out, func(i, j int) bool {
		if a, b := covers(out[i]), covers(out[j]); a != b {
			return a > b
		}
		if out[i].Papers != out[j].Papers {
			return out[i].Papers > out[j].Papers
		}
		return out[i].En < out[j].En
	})
	if opt.Top > 0 && len(out) > opt.Top {
		out = out[:opt.Top]
	}
	return &Candidates{Candidates: out}
}

// covers is how many words of the corpus a term accounts for, which is what
// the list is ranked on.
//
// Not the number of uses, and not the number of papers. Papers is the gate
// and a poor ranking: "model" is 351 uses in 20 papers and "problem" is 118
// in 31, and the paper count puts them the wrong way round. Uses on its own
// is better and still wrong, because every phrase loses to every word: with
// 300 candidates ranked on uses, five of them had two words in them. A term
// of two words is two words of the corpus each time it is written, it takes
// twice as long to get wrong, and this counts it that way.
func covers(c Candidate) int { return c.Uses * len(strings.Fields(c.En)) }

// whole drops a phrase that is nearly always part of a longer one.
//
// "neural" appears in as many papers as "neural network" does and means
// nothing on its own, and a list with both in it wastes the reviewer's
// attention twice. The threshold is nine tenths: a word that stands alone
// one time in ten is a word with a life of its own and stays.
func whole(cs []Candidate) []Candidate {
	byPhrase := make(map[string]Candidate, len(cs))
	for _, c := range cs {
		byPhrase[c.En] = c
	}
	var out []Candidate
	for _, c := range cs {
		// The most any one longer phrase accounts for, and not the sum of
		// them. "neural network" and "recurrent neural network" both hold
		// "neural", and every use of the longer is a use of the shorter, so
		// adding them up counts the same occurrences twice and would drop a
		// term that does stand on its own.
		covered := 0
		for phrase, longer := range byPhrase {
			if phrase == c.En || !contains(phrase, c.En) {
				continue
			}
			if longer.Uses > covered {
				covered = longer.Uses
			}
		}
		if covered*10 >= c.Uses*9 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// contains says whether the words of inner appear in a row in outer.
func contains(outer, inner string) bool {
	if outer == inner {
		return true
	}
	return strings.Contains(" "+outer+" ", " "+inner+" ")
}

// phrases counts every phrase of one to n words in a text.
//
// A phrase never crosses a line, a sentence or a comma, because a phrase
// that does is not a phrase: "the model. Attention is" is three words in a
// row and no part of the language. It never starts or ends on a stop word
// for the same reason, and it never holds a digit, a term with a number in
// it being a reference to something in the paper rather than a word.
func phrases(text string, n int, one map[string]string) map[string]int {
	out := map[string]int{}
	for _, run := range words(text) {
		for i, w := range run {
			if s, ok := one[w]; ok {
				run[i] = s
			}
		}
		for i := range run {
			for k := 1; k <= n && i+k <= len(run); k++ {
				w := run[i : i+k]
				if !phrase(w) {
					continue
				}
				out[strings.Join(w, " ")]++
			}
		}
	}
	return out
}

// singulars maps a plural to its singular, for every plural the corpus uses
// whose singular the corpus also uses.
//
// "problem" and "problems" are one term and a list with both in it wastes
// the reviewer's attention twice, worse than that, splits the count of a
// term across two rows so that neither passes the threshold. The rule is
// deliberately timid: a word is only folded when its singular is a word the
// corpus actually writes, so "address" does not become "addres" and "bits"
// folds only because "bit" is there to fold onto.
func singulars(texts []string) map[string]string {
	vocabulary := map[string]bool{}
	for _, text := range texts {
		for _, run := range words(text) {
			for _, w := range run {
				vocabulary[w] = true
			}
		}
	}
	out := map[string]string{}
	for w := range vocabulary {
		if s, ok := singular(w); ok && vocabulary[s] {
			out[w] = s
		}
	}
	return out
}

// singular takes the plural off a word, in the two cases English spells
// regularly enough to do without a dictionary.
func singular(w string) (string, bool) {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y", true
	case strings.HasSuffix(w, "sses"), strings.HasSuffix(w, "ches"), strings.HasSuffix(w, "shes"):
		return w[:len(w)-2], true
	case strings.HasSuffix(w, "ss"), strings.HasSuffix(w, "us"), strings.HasSuffix(w, "is"):
		return "", false
	case strings.HasSuffix(w, "s") && len(w) > 3:
		return w[:len(w)-1], true
	}
	return "", false
}

// words cuts a text into runs of words, one run per stretch of prose with
// no punctuation in it.
func words(text string) [][]string {
	var (
		out  [][]string
		run  []string
		word []rune
	)
	endWord := func() {
		if len(word) == 0 {
			return
		}
		w := strings.ToLower(string(word))
		word = word[:0]
		if !usable(w) {
			// A word the glossary will never want ends the run as surely as
			// a full stop does, so that the words on either side of it are
			// not read as a phrase.
			endRun(&out, &run)
			return
		}
		run = append(run, w)
	}
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word = append(word, r)
		case (r == '-' || r == '\'' || r == '’') && len(word) > 0:
			// A hyphen inside a word keeps it whole: "b-tree", "n-gram" and
			// "write-ahead" are one word each. A hyphen with a space beside
			// it is punctuation and falls through to the default.
			word = append(word, r)
		case r == ' ' || r == '\t':
			endWord()
		default:
			endWord()
			endRun(&out, &run)
		}
	}
	endWord()
	endRun(&out, &run)
	return out
}

func endRun(out *[][]string, run *[]string) {
	if len(*run) > 0 {
		*out = append(*out, *run)
		*run = nil
	}
}

// usable says whether a word can be part of a candidate at all.
//
// A word with a digit in it is a reference to something in the paper rather
// than a word of the language, and it breaks the run it is in: "figure 3
// shows" has three words in a row and no phrase in it.
func usable(w string) bool {
	for _, r := range w {
		if unicode.IsDigit(r) {
			return false
		}
	}
	// What the tokeniser kept on the end of a word, and a word that is
	// nothing but punctuation.
	return strings.Trim(w, "-'’") != ""
}

// phrase says whether a run of words can be proposed as a term.
func phrase(w []string) bool {
	if edge(w[0]) || edge(w[len(w)-1]) {
		return false
	}
	// A furniture word is a word of the paper rather than of the field, and
	// on its own it is never vocabulary. In a compound it usually is:
	// "critical section", "hash table", "floating point", "cache line",
	// "proof of work" and "worst case" are all terms a translator has to
	// render the same way every time, and all of them end on a word that
	// would be noise by itself.
	if len(w) == 1 && furniture[w[0]] {
		return false
	}
	// A furniture word may head a compound but not a phrase with a
	// preposition in it. "amount of data", "number of parameters" and "way
	// of doing" are ordinary English syntax with a word of the paper at the
	// front of it, and none of the three is a term. The same word at the
	// other end is fine, because what put it there is a real head: "proof
	// of work", "degrees of freedom".
	if furniture[w[0]] && joined(w) {
		return false
	}
	return middle(w)
}

// edge says whether a word may not begin or end a candidate.
//
// Short words are barred from the edges rather than from the phrase. A word
// of one or two letters is never a term and "of" is never the start of one,
// but "proof of work" and "degrees of freedom" are exactly the kind of term
// this glossary is for, and a rule that broke the run at "of" would lose
// both of them.
func edge(w string) bool { return stop[w] || len([]rune(w)) < 3 }

// middle says whether the words between the ends of a phrase can be there.
//
// A stop word in the middle of a phrase nearly always means the phrase is
// two phrases with a joint between them: "input and output" and "word and
// phrase" both came out of the corpus as candidates and neither is a term.
// The exception is the handful of prepositions that really do sit inside a
// term, and there are few enough of them to write down: "proof of work",
// "degrees of freedom", "out of order execution", "time to live".
func middle(w []string) bool {
	for i := 1; i < len(w)-1; i++ {
		if stop[w[i]] && !joiner[w[i]] {
			return false
		}
	}
	return true
}

// joined says whether a phrase has a preposition inside it.
func joined(w []string) bool {
	for i := 1; i < len(w)-1; i++ {
		if joiner[w[i]] {
			return true
		}
	}
	return false
}

// joiner is every function word allowed inside a term.
var joiner = map[string]bool{
	"of": true, "for": true, "in": true, "on": true, "to": true,
	"with": true, "per": true, "by": true, "the": true,
}

// stop is every word a candidate may not begin or end with.
//
// Three kinds of word are in here. The function words, which nobody would
// argue about. The general adjectives, adverbs and verbs of English, which
// are the ones that actually spoil the list: on this corpus "different",
// "single", "several", "possible", "particular" and "able" all appeared in
// every paper read and ranked above "network".
//
// The furniture of an academic paper is not in here. It is in furniture,
// which is a weaker bar, because half of those words are the head of a real
// compound term somewhere in this corpus.
//
// A word that is only ever a noun is not in here even when it is common.
// "model", "data", "system", "key" and "network" are exactly the terms a
// translator has to render the same way every time, and a corpus that let
// them go both ways would read as several corpora. They are what the
// glossary is for.
//
// The list is folded to its singular before it is looked up, so "systems"
// does not need a line of its own.
var stop = func() map[string]bool {
	const list = `
	a an the this that these those it its it's they them their there here
	i we you he she him her his our your my me us who whom whose which what
	and or but nor for yet so because since though although while whereas
	if then else unless until when whenever where wherever how however
	whether further existing obtain apply applies applied require requires
	required allow allows allowed consist consists contain contains include
	includes including provide provides provided describe describes
	described describing present presents presented propose proposes
	proposed introduce introduces introduced discuss discusses discussed
	define defines defined denote denotes denoted assume assumes assumed
	note notes noted report reports reported observe observes observed
	illustrate illustrates illustrated mention mentions mentioned
	http https www com org edu net gov
	of in on at to from by with without within into onto upon over under
	above below across through throughout between among against during
	before after behind beyond beside besides about around along toward
	towards per via than as like unlike up down off out
	is are was were be been being am do does did doing done have has had
	having will would shall should may might must can could ought need
	let make made makes making take takes taken taking get gets got give
	gives given giving use uses used using say says said see sees seen
	find finds found show shows shown showed know knows known knew
	call calls called calling want wants wanted try tries tried
	come comes came go goes went put puts keep keeps kept
	not no nor none nothing nobody never ever always often sometimes
	again also too very much many more most less least few fewer several
	some any all both each either neither every other others another same
	such own just only even still yet already soon later now then once
	first second third last next previous new old good better best bad
	worse worst large larger largest small smaller smallest big great
	long longer longest short shorter high higher highest low lower
	full empty single double whole entire complete partial simple complex
	easy hard difficult possible impossible likely unlikely similar
	different various particular specific general common typical usual
	important main major minor real true false right wrong able unable
	available present absent necessary sufficient enough certain clear
	actual actually really quite rather almost nearly about approximately
	typically usually generally normally commonly relatively significantly
	essentially effectively simply directly easily quickly slowly widely
	highly largely mainly partly fully finally initially currently
	previously recently similarly particularly especially specifically
	briefly clearly obviously roughly exactly far well better
	compared related associated based consider considered considering
	thus hence therefore however moreover furthermore nevertheless
	instead indeed perhaps maybe example examples eg ie cf etc et al resp
	following above below respectively namely
	one two three four five six seven eight nine ten
	`
	return set(list)
}()

// furniture is every word that is not a term on its own but can be part of
// one.
//
// These are the words a paper uses to talk about itself. Counted as terms
// they are frequent everywhere and vocabulary nowhere: "figure" and
// "section" and "table" came off this corpus in more papers than "model"
// did. Counted as the head of a compound they are half the vocabulary of
// computer science, and barring them outright lost "critical section",
// "hash table", "page table", "floating point", "cache line", "command
// line", "worst case", "time step", "sequence number", "return value" and
// "proof of work" in one go.
//
// So the bar is that a furniture word may not be a candidate by itself. It
// may sit anywhere in a phrase of two words or more, and what keeps the
// phrase honest there is the other end of it, which has to be a word that
// is neither furniture nor a stop word.
var furniture = set(`
	paper papers section sections subsection chapter appendix appendices
	figure figures table tables equation equations listing listings
	page pages line lines item items part parts step steps
	work works case cases way ways thing things point points
	number numbers value values amount amounts kind kinds sort sorts
	`)

func set(list string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(list) {
		out[w] = true
	}
	return out
}
