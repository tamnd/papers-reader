package emit

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/markdown"
	"github.com/tamnd/papers-reader/mathtex"
)

// A Search is search-<lang>.json: an inverted index over one language of the
// corpus, built at emit time and searched in the browser.
//
// In the browser and not on a server. A hundred papers is a corpus small
// enough that the index fits in a download, and sending every query a reader
// types to a search service would mean the reading list of everybody who
// uses this site existing somewhere else. There is no server between the
// reader and the corpus anywhere else on this site and there is no reason
// for search to be the exception.
//
// One file per language, loaded on the first search. A reader reading the
// Vietnamese should not pay for the Japanese.
type Search struct {
	Version int         `json:"version"`
	Lang    corpus.Lang `json:"lang"`
	// Posts is every block worth finding, in a fixed order. A posting in
	// either index below is a position in this list, because a posting list
	// of integers is a fraction of the size of a posting list of
	// "paper:section:block" strings and the app has to do the lookup either
	// way.
	Posts []Post `json:"posts"`
	// Terms is the prose: an ordinary word index over the text of the
	// corpus.
	Terms map[string][]int `json:"terms"`
	// Symbols is the mathematics and the program text, indexed apart.
	//
	// Apart because they are searched differently and because mixing them
	// would bury the prose. A reader looking for softmax wants the formula
	// it appears in as well as the paragraph that names it, and a reader
	// looking for the word wants the paragraph first. Two indexes let the
	// app rank them; one index would have to guess.
	Symbols map[string][]int `json:"symbols"`
}

// A Post is one block a search can return.
//
// It carries enough to draw a result line without fetching the page: the
// paper, the section it is in and its title, and the opening of the block
// itself. The index would be a third of the size without the snippet, and
// then a page of ten results would be ten page fetches to show any of them.
type Post struct {
	Paper string `json:"p"`
	// Section is the anchor of the section the block is in, and is empty on
	// the front page, which is the paper itself rather than a section of it.
	Section string `json:"s,omitempty"`
	Block   int    `json:"i"`
	Title   string `json:"t,omitempty"`
	// Kind is the block's kind, so the app can mark a hit in a formula or a
	// listing as one.
	Kind string `json:"k"`
	Text string `json:"x"`
}

// snippet is how much of a block the index carries. Two lines of a result
// list, which is enough to tell whether the hit is the one you wanted and
// short enough that the whole index over four languages stays inside the
// budget in the specification.
const snippet = 180

// common is how much of one language a term may appear in before it stops
// being a search term.
//
// A word in a third of the blocks of the corpus cannot narrow a search down
// and its posting list is thousands of integers long, so it is the bulk of
// the index and none of the use of it. This is a stopword list that does
// not have to be written four times in four languages, and it drops "the"
// in the English and "của" in the Vietnamese without anybody deciding that
// it should.
const common = 3

// BuildSearch builds one index per language the corpus has pages in.
//
// Built from the pages and not from the corpus. What a reader searches has
// to be what the app shows: a block that degraded on the way into the build
// should not be findable, and a paper that emits nothing should not be in
// the index at all.
func BuildSearch(pages []*Page) []*Search {
	byLang := map[corpus.Lang]*Search{}
	for _, p := range pages {
		s := byLang[p.Lang]
		if s == nil {
			s = &Search{Version: Version, Lang: p.Lang,
				Terms: map[string][]int{}, Symbols: map[string][]int{}}
			byLang[p.Lang] = s
		}
		s.add(p)
	}
	out := make([]*Search, 0, len(byLang))
	for _, l := range corpus.Langs {
		if s := byLang[l]; s != nil {
			s.prune()
			out = append(out, s)
		}
	}
	return out
}

func (s *Search) add(p *Page) {
	s.index(p, "", "", p.Front.Blocks)
	for _, sec := range p.Sections {
		title := sec.Title
		if sec.Number != "" {
			title = sec.Number + ". " + sec.Title
		}
		s.index(p, sec.Anchor, title, sec.Blocks)
	}
}

func (s *Search) index(p *Page, anchor, title string, blocks []Block) {
	for _, b := range blocks {
		if b.plain == "" && b.symbols == "" {
			continue
		}
		text := b.plain
		if text == "" {
			text = b.symbols
		}
		at := len(s.Posts)
		s.Posts = append(s.Posts, Post{
			Paper: p.ID, Section: anchor, Block: b.I,
			Title: title, Kind: b.Kind, Text: cut(text, snippet),
		})
		// The section title is indexed against every block of the section
		// rather than once, so that a search for two words that fall one in
		// the title and one in a paragraph finds the paragraph. It is the
		// cheapest form of the field weighting a real engine would do.
		post(s.Terms, at, Tokens(b.plain), Tokens(title))
		post(s.Symbols, at, Tokens(b.symbols))
	}
}

// post files one posting under every distinct token of the runs given.
func post(into map[string][]int, at int, runs ...[]string) {
	seen := map[string]bool{}
	for _, run := range runs {
		for _, t := range run {
			if seen[t] {
				continue
			}
			seen[t] = true
			into[t] = append(into[t], at)
		}
	}
}

// prune drops the terms that are in too much of the corpus to narrow
// anything down. See common above.
func (s *Search) prune() {
	limit := len(s.Posts) / common
	for _, m := range []map[string][]int{s.Terms, s.Symbols} {
		for term, posts := range m {
			if len(posts) > limit {
				delete(m, term)
			}
		}
	}
}

// Sorted is the terms of one of the two indexes in order, which is the order
// anything walking it should walk it in: a rule that reported its findings in
// map order would report them differently every time.
func (s *Search) Sorted(m map[string][]int) []string {
	out := make([]string, 0, len(m))
	for t := range m {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// cut trims a snippet to a length without cutting a word in half, and puts
// an ellipsis on it where it cut.
func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)[:n]
	// Back up to a space, unless the text has none in the last stretch,
	// which is what a line of Chinese looks like.
	for i := len(r) - 1; i >= 0 && i > n-20; i-- {
		if r[i] == ' ' {
			return strings.TrimRight(string(r[:i]), " ,.;:") + "..."
		}
	}
	return string(r) + "..."
}

// Tokens cuts a run of text into the things a search matches against.
//
// By script and not by language, which is a deliberate difference from the
// specification. Latin runs split on anything that is not a letter or a
// digit; runs of Han, kana or Hangul come out as overlapping pairs, because
// there is no word segmenter for those that can be shipped to a browser and
// a pair of characters is what a query in those languages is made of.
//
// Doing it by script rather than by language means the same function serves
// all four files and a Chinese term quoted in an English paper is findable
// in the English index, which is the common case in this corpus and would
// not be if the tokeniser were chosen per file.
//
// Single Latin characters are dropped. A one letter token matches most of
// the corpus and is never what anybody searched for, except in mathematics,
// which is indexed as symbols and goes through here as its TeX.
func Tokens(s string) []string {
	var out []string
	var run []rune
	cjk := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		switch {
		case !cjk:
			if len(run) > 1 {
				out = append(out, string(run))
			}
		case len(run) == 1:
			out = append(out, string(run))
		default:
			for i := 0; i+1 < len(run); i++ {
				out = append(out, string(run[i:i+2]))
			}
		}
		run = run[:0]
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case wide(r) != cjk:
			flush()
			cjk = wide(r)
			run = append(run, r)
		default:
			run = append(run, r)
		}
	}
	flush()
	return out
}

// wide says a character belongs to a script written without spaces between
// its words.
//
// The two single characters are there because Unicode files them under the
// Common script rather than under Katakana or Han, and they are ordinary
// letters of Japanese: the prolonged sound mark is the second character of
// ネットワーク and the iteration mark is the second character of 人々.
// Without them a word is cut in half at the point a reader is most likely
// to type.
func wide(r rune) bool {
	return r == '\u30fc' || r == '\u3005' ||
		unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// SearchPath is where one language's index sits in a build.
func SearchPath(l corpus.Lang) string { return "search-" + string(l) + ".json" }

// strip cuts one block of Markdown into the prose a word search should match
// and the mathematics and program text a symbol search should match.
//
// The two are separated here rather than in the tokeniser because the
// decision is about what the span is and not about what the characters are.
// It runs on the Markdown the corpus holds and not on the HTML the page
// carries, because the HTML of a formula is KaTeX's, which holds the same
// expression three times over in three notations and would index as noise.
func strip(text string) (prose, symbols string) {
	var sym []string
	// A block with an unclosed dollar is left whole rather than cut at a
	// span that runs to the end of it. Rule M04 is what reports that block,
	// and indexing its prose with the dollars still in is a better wrong
	// answer than indexing half the paragraph as mathematics.
	if spans, unclosed := mathtex.Split(text); unclosed == nil && len(spans) > 0 {
		rs := []rune(text)
		var b strings.Builder
		at := 0
		for _, sp := range spans {
			delim := 1
			if sp.Display {
				delim = 2
			}
			b.WriteString(string(rs[at : sp.Start-delim]))
			b.WriteString(" ")
			sym = append(sym, sp.Text)
			at = sp.End + delim
		}
		b.WriteString(string(rs[at:]))
		text = b.String()
	}
	text = markdown.Code.ReplaceAllStringFunc(text, func(m string) string {
		sym = append(sym, strings.Trim(m, "`"))
		return " "
	})
	text = markdown.PaperCite.ReplaceAllString(text, " ")
	text = markdown.Note.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(plainer.Replace(text)), " "), strings.Join(sym, " ")
}

// plainer takes the Markdown punctuation out of a run of text. The
// tokeniser would drop all of it anyway; this is here so that the snippet a
// result line shows reads as a sentence and not as markup.
var plainer = strings.NewReplacer(
	"#", " ", "*", "", "_", " ", "|", " ", ">", " ",
	"[", " ", "]", " ", "\n", " ", "\\", " ",
)
