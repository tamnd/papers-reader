package emit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// indexOf is the index for one language of a built corpus.
func indexOf(t *testing.T, s *Site, l corpus.Lang) *Search {
	t.Helper()
	for _, ix := range s.Search {
		if ix.Lang == l {
			return ix
		}
	}
	t.Fatalf("there is no %s index", l)
	return nil
}

// hits is the posts a term stands for, which is the lookup the app does.
func hits(ix *Search, m map[string][]int, term string) []Post {
	var out []Post
	for _, at := range m[term] {
		out = append(out, ix.Posts[at])
	}
	return out
}

func TestTokensCutLatinOnPunctuationAndCJKIntoPairs(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"The back-propagation rule.", "the|back|propagation|rule"},
		{"GPT-3 has 175B parameters", "gpt|has|175b|parameters"},
		{"Mạng đối kháng sinh mẫu", "mạng|đối|kháng|sinh|mẫu"},
		{"生成对抗网络", "生成|成对|对抗|抗网|网络"},
		{"敵対的生成ネットワーク", "敵対|対的|的生|生成|成ネ|ネッ|ット|トワ|ワー|ーク"},
		// A script boundary is a token boundary, so a Chinese term quoted in
		// an English sentence is findable in the English index.
		{"the term 网络 means network", "the|term|网络|means|network"},
	} {
		if got := strings.Join(Tokens(c.in), "|"); got != c.want {
			t.Errorf("%q tokenised as %q, want %q", c.in, got, c.want)
		}
	}
}

// A one letter token matches most of the corpus and is never what anybody
// typed into a search box, and the case a query is typed in is not part of
// the query.
func TestTokensDropsTheSingleLettersAndFoldsTheCase(t *testing.T) {
	got := strings.Join(Tokens("A Network Of Units, x and y"), "|")
	if got != "network|of|units|and" {
		t.Errorf("the tokens are %q", got)
	}
}

// The prose and the mathematics go into separate piles, so that a search for
// a word does not match the middle of a formula and a search for a symbol
// does not have to compete with the prose around it.
func TestStripSeparatesTheProseFromTheSymbols(t *testing.T) {
	for _, c := range []struct{ in, prose, sym string }{
		{"The value $x_i$ is the input.", "The value is the input.", "x_i"},
		{"We minimise $$J(\\theta) = -\\log D(x)$$ over the batch.",
			"We minimise over the batch.", "J(\\theta) = -\\log D(x)"},
		{"Call `train(net, batch)` once per epoch.", "Call once per epoch.", "train(net, batch)"},
		{"This is the rule of [[rumelhart-1986-backprop]].", "This is the rule of .", ""},
		{"A claim with a note [^3] on it.", "A claim with a note on it.", ""},
		{"**Bold** and *slanted* prose.", "Bold and slanted prose.", ""},
	} {
		prose, sym := strip(c.in)
		if prose != c.prose {
			t.Errorf("%q gave prose %q, want %q", c.in, prose, c.prose)
		}
		if sym != c.sym {
			t.Errorf("%q gave symbols %q, want %q", c.in, sym, c.sym)
		}
	}
}

// A block with a dollar nobody closed is rule M04's business. Cutting it at
// a span that runs to the end of the block would index half a paragraph as
// mathematics, which is a worse wrong answer than leaving the dollars in.
func TestAnUnclosedFormulaLeavesTheBlockWhole(t *testing.T) {
	prose, sym := strip("The value $x is the input.")
	if !strings.Contains(prose, "is the input") || sym != "" {
		t.Errorf("the block came apart: %q and %q", prose, sym)
	}
}

func TestTheIndexFindsAWordOfTheProse(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	ix := indexOf(t, site, corpus.EN)
	got := hits(ix, ix.Terms, "prose")
	if len(got) != 1 {
		t.Fatalf("prose is in %d posts, want 1", len(got))
	}
	if got[0].Paper != "rumelhart-1986-backprop" || got[0].Kind != "p" {
		t.Errorf("the post is %+v", got[0])
	}
	if got[0].Section == "" || got[0].Title == "" {
		t.Errorf("the post cannot draw a result line: %+v", got[0])
	}
	if !strings.Contains(got[0].Text, "Some prose") {
		t.Errorf("the snippet is %q", got[0].Text)
	}
}

// The section title is indexed against every block of the section, so that a
// query whose words fall one in the title and one in a paragraph still finds
// the paragraph.
func TestTheSectionTitleIsFoundFromEveryBlockOfTheSection(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	ix := indexOf(t, site, corpus.EN)
	if len(hits(ix, ix.Terms, "section")) == 0 {
		t.Error("a word of the section title is in no post")
	}
}

// A search for softmax should find the formula it appears in as well as the
// paragraph that names it, which is what the second index is for.
func TestAFormulaIsFoundBySymbolAndNotByWord(t *testing.T) {
	c := corpusOf(t, map[string]string{
		"content/en/rumelhart-1986-backprop/00_front.md": front(
			"rumelhart-1986-backprop", "Learning Representations by Back-Propagating Errors", corpus.EN, 0, 1),
		"content/en/rumelhart-1986-backprop/01_method.md": body(
			"rumelhart-1986-backprop", corpus.EN, "1", "Method",
			"The output layer uses a softmax.\n\n$$y_i = \\frac{e^{z_i}}{\\sum_j e^{z_j}}\\tag{1}$$\n"),
	})
	site, err := Build(c)
	if err != nil {
		t.Fatal(err)
	}
	ix := indexOf(t, site, corpus.EN)
	if len(hits(ix, ix.Terms, "softmax")) != 1 {
		t.Error("the paragraph that names softmax is not in the word index")
	}
	sum := hits(ix, ix.Symbols, "frac")
	if len(sum) != 1 || sum[0].Kind != "math" {
		t.Errorf("the formula is in %v", sum)
	}
	if len(ix.Terms["frac"]) != 0 {
		t.Error("the TeX of a formula went into the word index")
	}
	// A formula has no prose, so the snippet the result line shows is the
	// TeX. Showing nothing would be worse.
	if !strings.Contains(sum[0].Text, "y_i") {
		t.Errorf("the snippet of a formula is %q", sum[0].Text)
	}
}

// One file per language, and a paper that exists in one language only is in
// that one index and no other.
func TestThereIsOneIndexPerLanguageOfTheCorpus(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Search) != 2 {
		t.Fatalf("%d indexes, want one for en and one for vi", len(site.Search))
	}
	if site.Search[0].Lang != corpus.EN || site.Search[1].Lang != corpus.VI {
		t.Errorf("the indexes are in the order %v %v", site.Search[0].Lang, site.Search[1].Lang)
	}
	for _, p := range indexOf(t, site, corpus.VI).Posts {
		if p.Paper == "hochreiter-1997-lstm" {
			t.Error("a paper with no Vietnamese is in the Vietnamese index")
		}
	}
}

// A word in a third of the corpus cannot narrow a search down and its
// posting list is most of the size of the index, so it is dropped. This is a
// stopword list nobody had to write in four languages.
func TestAWordInMostOfTheCorpusIsNotASearchTerm(t *testing.T) {
	s := &Search{
		Posts:   make([]Post, 9),
		Terms:   map[string][]int{"the": {0, 1, 2, 3}, "network": {2, 5, 8}, "softmax": {4}},
		Symbols: map[string][]int{"sum": {0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}
	s.prune()
	if _, ok := s.Terms["the"]; ok {
		t.Error("a word in four blocks of nine is still a search term")
	}
	if len(s.Terms["network"]) != 3 || len(s.Terms["softmax"]) != 1 {
		t.Errorf("the terms are %v", s.Sorted(s.Terms))
	}
	if len(s.Symbols) != 0 {
		t.Error("a symbol in every block is still a search term")
	}
}

func TestASnippetIsCutAtAWordBoundary(t *testing.T) {
	long := strings.Repeat("network of units ", 40)
	got := cut(long, 40)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("the snippet was not cut: %q", got)
	}
	if len([]rune(got)) > 43 {
		t.Errorf("the snippet is %d runes: %q", len([]rune(got)), got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "..."), " ") {
		t.Errorf("the snippet ends mid word: %q", got)
	}
	// A line of Chinese has no spaces to back up to, and a snippet of
	// nothing would be worse than one cut between two characters.
	if got := cut(strings.Repeat("生成对抗网络", 20), 10); len([]rune(got)) != 13 {
		t.Errorf("a line with no spaces came back as %q", got)
	}
	if got := cut("  Short  enough.\n", 40); got != "Short enough." {
		t.Errorf("a short block came back as %q", got)
	}
}

func TestTheIndexIsInTheBuild(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	files, err := site.Files()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"search-en.json", "search-vi.json"} {
		if len(files[want]) == 0 {
			t.Errorf("%s is not in the build: %v", want, SortedNames(files))
		}
	}
	if strings.Contains(string(files["search-en.json"]), "\n  ") {
		t.Error("the search index is indented, which is a megabyte of nothing")
	}
}
