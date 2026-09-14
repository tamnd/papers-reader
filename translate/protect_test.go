package translate

import (
	"strings"
	"testing"
)

// kinds is the shape of a body's protection: what was found and in what
// order. Most of what these tests want to say is a sentence about that shape
// rather than about any one span.
func kinds(spans []Span) string {
	var out []string
	for _, s := range spans {
		out = append(out, string(s.Kind))
	}
	return strings.Join(out, " ")
}

func texts(spans []Span) []string {
	var out []string
	for _, s := range spans {
		out = append(out, s.Text)
	}
	return out
}

func TestTheFourKindsAreFoundInTheOrderTheyAppear(t *testing.T) {
	body := "## The Objective {#paper-s2 .section tag=00C7}\n" +
		"\n" +
		"Following [[nash-1951-equilibrium]] and [4], the value is $V(G)$ for\n" +
		"the display\n" +
		"\n" +
		"$$V(G) = \\min_G \\max_D U(G, D)$$\n" +
		"\n" +
		"which the reference implementation computes as\n" +
		"\n" +
		"```python\n" +
		"v = minimax(g, d)\n" +
		"```\n"

	got := Protect(body)
	want := "attribute citation citation mathematics mathematics code"
	if kinds(got) != want {
		t.Fatalf("Protect found %q and the body has %q, in that order", kinds(got), want)
	}
	if got[0].Text != "{#paper-s2 .section tag=00C7}" {
		t.Errorf("the attribute block came back as %q", got[0].Text)
	}
	if got[3].Text != "$V(G)$" {
		t.Errorf("the inline span came back as %q and the dollars are part of it", got[3].Text)
	}
	if got[4].Text != "$$V(G) = \\min_G \\max_D U(G, D)$$" {
		t.Errorf("the display came back as %q", got[4].Text)
	}
	if got[5].Text != "```python\nv = minimax(g, d)\n```" {
		t.Errorf("the fence came back as %q and both fences are part of it", got[5].Text)
	}
}

func TestAFenceIsOneSpanAndNotWhateverIsInsideIt(t *testing.T) {
	body := "```sh\n" +
		"echo \"$HOME is [3] dollars\" {#not-an-anchor}\n" +
		"```\n"

	got := Protect(body)
	if kinds(got) != "code" {
		t.Fatalf("Protect found %q and a fence is protected whole", kinds(got))
	}
	if !strings.Contains(got[0].Text, "$HOME is [3] dollars") {
		t.Errorf("the fence came back as %q and its text is part of it", got[0].Text)
	}
}

func TestAnUnclosedFenceRunsToTheEndOfTheBody(t *testing.T) {
	body := "Here is the loop:\n\n```go\nfor i := range xs {\n"

	got := Protect(body)
	if kinds(got) != "code" {
		t.Fatalf("Protect found %q and an unclosed fence still protects what follows it", kinds(got))
	}
	if !strings.HasSuffix(got[0].Text, "for i := range xs {") {
		t.Errorf("the fence came back as %q and it runs to the last line", got[0].Text)
	}
}

func TestABracketInsideMathematicsIsNotACitation(t *testing.T) {
	body := "The output is squashed into $[0, 1]$ by the sigmoid.\n"

	got := Protect(body)
	if kinds(got) != "mathematics" {
		t.Fatalf("Protect found %q and the interval is part of the formula", kinds(got))
	}
}

func TestABracketThatIsAWordIsNotACitation(t *testing.T) {
	body := "Every sequence starts with the [CLS] token, and [sic] stays as printed.\n"

	if got := Protect(body); len(got) != 0 {
		t.Fatalf("Protect found %q and neither bracket is a reference", kinds(got))
	}
}

func TestSeveralNumbersInOneBracketAreOneCitation(t *testing.T) {
	body := "This was shown three times over [19, 9, 10].\n"

	got := Protect(body)
	if len(got) != 1 || got[0].Text != "[19, 9, 10]" {
		t.Fatalf("Protect found %q and the paper printed one reference", texts(got))
	}
}

func TestAnExampleSentenceInBackticksIsNotTranslated(t *testing.T) {
	source := "The input `my dog is hairy` becomes `my dog is [MASK]`.\n"
	answer := "Đầu vào `my dog is hairy` trở thành `my dog is [MASK]`.\n"

	got := Protect(source)
	if kinds(got) != "inline code inline code" {
		t.Fatalf("Protect found %q and both examples are the model's input", kinds(got))
	}
	if d := Compare(source, answer); len(d) != 0 {
		t.Fatalf("Compare refused a translation that kept both examples: %v", d)
	}
	if d := Compare(source, "Đầu vào `con chó của tôi có lông` trở thành `my dog is [MASK]`.\n"); len(d) != 1 {
		t.Fatalf("Compare found %v and the answer translated the sentence the model saw", d)
	}
}

func TestAFenceIsNotReadAsInlineCode(t *testing.T) {
	body := "```\nplain listing\n```\n"

	if got := Protect(body); kinds(got) != "code" {
		t.Fatalf("Protect found %q and the fence is one span", kinds(got))
	}
}

func TestAFootnoteMarkerIsACitation(t *testing.T) {
	body := "The hash is published widely[^1] and never revised.\n" +
		"\n" +
		"[^1]: A newspaper will do.\n"

	got := Protect(body)
	if len(got) != 2 || got[0].Text != "[^1]" || got[1].Text != "[^1]" {
		t.Fatalf("Protect found %q and both the marker and its definition carry the number", texts(got))
	}
	if got[0].Kind != Citation {
		t.Errorf("the marker came back as %s", got[0].Kind)
	}
}

func TestARangeOfNumbersIsOneCitation(t *testing.T) {
	body := "Publishing the hash is an old idea [2-5].\n"

	got := Protect(body)
	if len(got) != 1 || got[0].Text != "[2-5]" {
		t.Fatalf("Protect found %q and the range is one reference", texts(got))
	}
}

func TestACitationWithAPageLocatorIsProtectedWhole(t *testing.T) {
	body := "The same behaviour is plotted in [16, Figure 3(e)].\n"

	got := Protect(body)
	if len(got) != 1 || got[0].Text != "[16, Figure 3(e)]" {
		t.Fatalf("Protect found %q and the locator belongs to the reference", texts(got))
	}
}

func TestAnIntervalWrittenInProseIsNotACitation(t *testing.T) {
	body := "Averaged over thresholds [.5, .95] as the metric asks.\n"

	if got := Protect(body); len(got) != 0 {
		t.Fatalf("Protect found %q and there is no reference on that line", texts(got))
	}
}

func TestARenumberedFootnoteIsRefused(t *testing.T) {
	source := "published widely[^1]\n"
	answer := "công bố rộng rãi[^2]\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and the answer renumbered the footnote", d)
	}
}

func TestATranslationThatKeepsEverySpanIsAccepted(t *testing.T) {
	source := "The generator $G$ minimises [[goodfellow-2014-gan]] the loss\n" +
		"\n" +
		"$$\\log(1 - D(G(z)))$$\n" +
		"\n" +
		"over the noise prior $p_z(z)$.\n"
	answer := "Bộ sinh $G$ cực tiểu hoá [[goodfellow-2014-gan]] hàm mất mát\n" +
		"\n" +
		"$$\\log(1 - D(G(z)))$$\n" +
		"\n" +
		"trên phân phối nhiễu tiên nghiệm $p_z(z)$.\n"

	if d := Compare(source, answer); len(d) != 0 {
		t.Fatalf("Compare refused a faithful translation: %v", d)
	}
}

func TestARenamedVariableIsRefused(t *testing.T) {
	source := "The discriminator $D(x)$ is a probability.\n"
	answer := "Bộ phân biệt $P(x)$ là một xác suất.\n"

	d := Compare(source, answer)
	if len(d) != 1 {
		t.Fatalf("Compare found %d differences and the answer renamed one variable: %v", len(d), d)
	}
	if d[0].At != 1 || d[0].Want.Text != "$D(x)$" || d[0].Got.Text != "$P(x)$" {
		t.Errorf("Compare said %q", d[0])
	}
}

func TestADroppedSpanIsRefused(t *testing.T) {
	source := "We train $G$ and $D$ together.\n"
	answer := "Chúng tôi huấn luyện $G$ và bộ phân biệt cùng lúc.\n"

	d := Compare(source, answer)
	if len(d) != 1 || d[0].At != 2 {
		t.Fatalf("Compare found %v and the answer dropped the second span", d)
	}
	if !strings.Contains(d[0].String(), "nothing like it") {
		t.Errorf("Compare said %q and the answer dropped a span", d[0])
	}
}

func TestAnInventedSpanIsRefused(t *testing.T) {
	source := "We train $G$ and the discriminator together.\n"
	answer := "Chúng tôi huấn luyện $G$ và $D$ cùng lúc.\n"

	d := Compare(source, answer)
	if len(d) != 1 || d[0].At != 2 || d[0].Got.Text != "$D$" {
		t.Fatalf("Compare found %v and the answer invented a span", d)
	}
}

// Chinese and Japanese put a modifier in front of what it modifies, so two
// formulas in one English clause regularly come out the other way round, and
// a translation that kept the English order would be the wrong one. This is
// the sentence that made the point: the first Chinese run of the GAN paper
// gave up on it after three tries, all three of them correct.
func TestReorderingSpansInOneParagraphIsAllowed(t *testing.T) {
	source := "To learn the generator's distribution $p_g$ over data $x$, we define a prior.\n"
	answer := "为了学习生成器在数据 $x$ 上的分布 $p_g$，我们定义一个先验。\n"

	if d := Compare(source, answer); len(d) != 0 {
		t.Errorf("Compare found %v and the answer only put the two formulas in Chinese order", d)
	}
}

// Between paragraphs it is not word order, it is a formula that has moved to
// another paragraph, and the paper no longer says what it said.
func TestMovingASpanToAnotherParagraphIsRefused(t *testing.T) {
	source := "First $a$ is fixed.\n\nThen $b$ is fixed.\n"
	answer := "Trước hết $b$ được cố định.\n\nSau đó $a$ được cố định.\n"

	if d := Compare(source, answer); len(d) != 2 {
		t.Fatalf("Compare found %v and each paragraph has the other's formula", d)
	}
}

func TestAnInlineTurnedIntoADisplayIsRefused(t *testing.T) {
	source := "The value $V(G)$ is fixed.\n"
	answer := "Giá trị $$V(G)$$ là cố định.\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and the answer changed how the formula is set", d)
	}
}

func TestASpaceInsideTheDollarSignsIsNotAChange(t *testing.T) {
	source := "The generator distribution $p_g$ over $\\boldsymbol{x}$.\n"
	for _, answer := range []string{
		"$ p_g$ の上の $\\boldsymbol{x}$ 上の生成器分布。\n",
		"$p_g $ の上の $\\boldsymbol{x}$ 上の生成器分布。\n",
		"$ p_g $ の上の $ \\boldsymbol{x} $ 上の生成器分布。\n",
		"$p_g$ の上の $\\boldsymbol{x}$ 上の生成器分布。\n",
	} {
		if d := Compare(source, answer); len(d) != 0 {
			t.Errorf("Compare refused %q with %v and TeX ignores the space", answer, d)
		}
	}
}

func TestASpaceThatHoldsAControlWordApartStillCounts(t *testing.T) {
	source := "The angle $\\alpha x$ is small.\n"
	answer := "Góc $\\alphax$ nhỏ.\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and \\alphax is not a formula at all", d)
	}
}

func TestProseInsideATextCommandMayBeTranslated(t *testing.T) {
	source := "$p(y = 1 \\mid x) = \\text{probability that } x \\text{ is real}$\n"
	answer := "$p(y = 1 \\mid x) = \\text{xác suất rằng } x \\text{ là thật}$\n"

	if d := Compare(source, answer); len(d) != 0 {
		t.Fatalf("Compare refused a translated word inside a \\text: %v", d)
	}
}

func TestASpaceAtTheEdgeOfATextCommandMustSurvive(t *testing.T) {
	source := "$\\text{probability that } x$\n"
	answer := "$\\text{xác suất rằng} x$\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and the answer dropped the space that separates the word from the symbol", d)
	}
}

func TestAnUprightOperatorInsideATextCommandMayNotBeTranslated(t *testing.T) {
	source := "$G^* = \\text{argmin}_G V(G)$\n"
	answer := "$G^* = \\text{cực tiểu}_G V(G)$\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and argmin is an operator and not a word", d)
	}
}

func TestMathematicsInsideATextCommandIsComparedWhole(t *testing.T) {
	source := "$\\text{the $\\Gamma$ correspondence}$\n"
	answer := "$\\text{tương ứng $\\Lambda$}$\n"

	if d := Compare(source, answer); len(d) != 1 {
		t.Fatalf("Compare found %v and the answer rewrote a symbol inside the \\text", d)
	}
}

func TestARewrittenAnchorIsRefused(t *testing.T) {
	source := "## Background {#paper-s2 .section tag=00C7}\n"
	answer := "## Bối cảnh {#paper-boi-canh .section tag=00C7}\n"

	d := Compare(source, answer)
	if len(d) != 1 || d[0].Want.Kind != Attribute {
		t.Fatalf("Compare found %v and the anchor is what every link in the corpus points at", d)
	}
}

func TestARewrittenFenceIsRefused(t *testing.T) {
	source := "```python\nreturn x + 1\n```\n"
	answer := "```python\ntrả_về x + 1\n```\n"

	d := Compare(source, answer)
	if len(d) != 1 || d[0].Want.Kind != Code {
		t.Fatalf("Compare found %v and program text is not translated", d)
	}
}

func TestAPositionIsReportedSoTheSameFormulaTwiceCanBeToldApart(t *testing.T) {
	source := "We use $n$ layers and then $n$ heads.\n"
	answer := "Chúng tôi dùng $n$ lớp rồi $m$ đầu.\n"

	d := Compare(source, answer)
	if len(d) != 1 || d[0].At != 2 {
		t.Fatalf("Compare found %v and it is the second $n$ that changed", d)
	}
}

func TestAnEmptyBodyHasNothingToProtect(t *testing.T) {
	if got := Protect(""); len(got) != 0 {
		t.Fatalf("Protect found %q in an empty body", kinds(got))
	}
	if d := Compare("", ""); len(d) != 0 {
		t.Fatalf("Compare found %v between two empty bodies", d)
	}
}

func TestProseIsTheBodyWithTheSpansTakenOut(t *testing.T) {
	body := "The bound is $O(n \\log n)$ per query [12], as `sort()` shows."

	got := Prose(body)
	for _, gone := range []string{"$", "log", "[12]", "sort"} {
		if strings.Contains(got, gone) {
			t.Errorf("Prose kept %q: %q", gone, got)
		}
	}
	for _, kept := range []string{"bound", "per query", "shows"} {
		if !strings.Contains(got, kept) {
			t.Errorf("Prose dropped %q: %q", kept, got)
		}
	}
}

func TestProseDoesNotJoinTheWordsEitherSideOfASpan(t *testing.T) {
	// "each $x$ node" must not come back as "each node", which is a phrase
	// the paper never wrote and one the glossary extractor would count.
	got := Prose("each $x$ node")
	if strings.Contains(got, "each node") {
		t.Errorf("Prose closed the gap the formula left: %q", got)
	}
}

func TestOffsetsPointAtTheSpan(t *testing.T) {
	body := "chiều dài là $n$ đơn vị"

	got := Protect(body)
	if len(got) != 1 {
		t.Fatalf("Protect found %q", kinds(got))
	}
	rs := []rune(body)
	if cut := string(rs[got[0].Start:got[0].End]); cut != got[0].Text {
		t.Errorf("the offsets cut out %q and the span is %q, so they are counted in bytes and not runes", cut, got[0].Text)
	}
}

// A bare web address is protected, because a translator that touches one
// does not mistype it, it decorates it. All three translations of the GAN
// paper turned the footnote's address into a Markdown link with the address
// as both the text and the target.
func TestABareAddressIsProtected(t *testing.T) {
	for _, c := range []struct {
		what, body, want string
	}{
		{
			"an address at the end of a sentence",
			"All code is available at http://www.github.com/goodfeli/adversarial\n",
			"http://www.github.com/goodfeli/adversarial",
		},
		{
			"an address with the sentence's full stop after it",
			"See https://example.org/a/b.html. The rest follows.\n",
			"https://example.org/a/b.html",
		},
		{
			"an address written without a scheme",
			"Mirrored at www.example.org/papers, updated weekly.\n",
			"www.example.org/papers",
		},
	} {
		var got []string
		for _, s := range Protect(c.body) {
			if s.Kind == URL {
				got = append(got, s.Text)
			}
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: protected %q, want one URL %q", c.what, got, c.want)
		}
	}
}

// An address inside a fence or a backticked span belongs to that span and is
// not reported twice.
func TestAnAddressInsideAListingIsNotProtectedAgain(t *testing.T) {
	body := "```sh\ncurl http://example.org/x\n```\n\nAnd `http://example.org/y` in a sentence.\n"
	for _, s := range Protect(body) {
		if s.Kind == URL {
			t.Errorf("an address inside a %s was protected again: %q", s.Kind, s.Text)
		}
	}
}
