package audit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/split"
)

// Nothing in this file is a sentence of anybody's paper. The English is
// written here and the Vietnamese, Chinese and Japanese of it were written
// here too, so that a test that says "this came back in English" is about a
// paragraph nobody else owns.

// englishBody is the section every case translates, unless it says
// otherwise. It has one formula, one citation, one listing and one
// attribute block in it, which is one of everything group L compares.
const englishBody = `The bound is $n$ and the method of [3] reaches it. A longer paragraph
sits here so that the rules with a length floor have something to look at
and do not stand down on a two word section.

$$n = 3 \tag{1}$$
{#a-1970-paper-eq-1 .equation tag=00B1}

` + "```python" + `
def bound(n):
    return n
` + "```" + `
`

// viBody is the same section in Vietnamese, with the spans copied through
// and the prose actually translated.
const viBody = `Giới hạn là $n$ và phương pháp của [3] đạt tới nó. Một đoạn dài hơn
nằm ở đây để các quy tắc có ngưỡng độ dài có cái để xem xét và không đứng
xuống trước một mục chỉ có hai từ.

$$n = 3 \tag{1}$$
{#a-1970-paper-eq-1 .equation tag=00B1}

` + "```python" + `
def bound(n):
    return n
` + "```" + `
`

const bibliography = `[1] A. Author. A Paper. A Journal, 1970.

[3] B. Writer. Another Paper. Another Journal, 1972.
`

// pairOf writes one English section, its translation and both
// bibliographies, and runs the audit over them.
func pairOf(t *testing.T, lang corpus.Lang, en, tr string) *Report {
	t.Helper()
	return Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), en),
		"content/" + string(lang) + "/vaswani-2017-attention/01_section.md": file(
			answer(lang, "section", en), tr),
	}), false)
}

// answer is the front matter papers translate writes: the language, the
// English file it came from and the hash of that file as it stood.
func answer(lang corpus.Lang, kind, source string) string {
	return "paper: vaswani-2017-attention\ntitle: Attention Is All You Need\n" +
		"kind: " + kind + "\nlang: " + string(lang) + "\n" +
		"translated_from: content/en/vaswani-2017-attention/01_section.md\n" +
		"source_content_sha256: " + corpus.ContentSHA([]byte(source)) + "\n" +
		"glossary_version: 1\nglossary_terms_sha256: aaaa\n"
}

// The whole group stands down on a corpus with no translation in it, which
// is every corpus until M6 reaches the paper. Standing down is not passing.
func TestGroupLDoesNotRunOnAnUntranslatedCorpus(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
	}), false)

	for _, res := range rep.Results {
		if res.Rule.Group() != GroupTranslation {
			continue
		}
		if !res.NotRun {
			t.Errorf("%s ran on a corpus with no translation in it and said %v", res.Rule.ID, res.Findings)
		}
	}
}

// A translation the toolchain wrote passes every hard rule of the group.
// This is the case the other tests are departures from.
func TestAGoodTranslationPassesTheWholeGroup(t *testing.T) {
	rep := pairOf(t, corpus.VI, englishBody, viBody)

	for _, res := range rep.Results {
		if res.Rule.Group() != GroupTranslation || !res.Rule.Hard {
			continue
		}
		if res.Failed() {
			t.Errorf("%s failed a good translation: %v", res.Rule.ID, res.Findings)
		}
	}
}

func TestL01FindsAFormulaThatChanged(t *testing.T) {
	tr := strings.Replace(viBody, "$n$", "$m$", 1)
	res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L01")
	if !res.Failed() {
		t.Fatal("L01 passed a translation that renamed a variable")
	}
	if !strings.Contains(res.Findings[0].Message, "$m$") {
		t.Errorf("L01 does not say what it found: %q", res.Findings[0].Message)
	}
}

// A formula that moved inside its paragraph is not a finding. Chinese and
// Japanese put a modifier in front of what it modifies, so two formulas in
// one English clause come back the other way round, and a rule that refused
// that would refuse every correct translation into either.
func TestL01AllowsAFormulaToMoveInsideItsParagraph(t *testing.T) {
	const en = "We take $p_g$ over the data $x$, which is the distribution the generator learns and the one this section is about.\n"
	const zh = "为了学习生成器在数据 $x$ 上的分布 $p_g$，我们定义了一个先验，这是本节讨论的内容。\n"
	if res := result(t, pairOf(t, corpus.ZH, en, zh), "L01"); res.Failed() {
		t.Errorf("L01 refused a correct Chinese word order: %v", res.Findings)
	}
}

func TestL02FindsATagThatChanged(t *testing.T) {
	tr := strings.Replace(viBody, "tag=00B1", "tag=00B2", 1)
	if res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L02"); !res.Failed() {
		t.Fatal("L02 passed a translation that rewrote a tag")
	}
}

func TestL03FindsAHeadingThatBecameProse(t *testing.T) {
	en := englishBody + "\n### 1.1 The Narrow Case {#a-1970-paper-s1-1 .subsection tag=00D1}\n\nAnd a paragraph under it that is long enough for every other rule.\n"
	tr := viBody + "\nTrường hợp hẹp {#a-1970-paper-s1-1 .subsection tag=00D1}\n\nVà một đoạn bên dưới đủ dài cho mọi quy tắc khác trong nhóm này.\n"
	res := result(t, pairOf(t, corpus.VI, en, tr), "L03")
	if !res.Failed() {
		t.Fatal("L03 passed a translation that turned a heading into a paragraph")
	}
}

func TestL04FindsATranslationWithNoEnglish(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/07_extra.md": file(
			"paper: vaswani-2017-attention\ntitle: Attention Is All You Need\nkind: section\nlang: vi\n", viBody),
	}), false)

	res := result(t, rep, "L04")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "no English file") {
		t.Fatalf("L04 said %v about a Vietnamese file with no English", res.Findings)
	}
	// Every other rule of the group has to skip it rather than report its
	// own version of the same one problem.
	for _, other := range rep.Results {
		if other.Rule.Group() != GroupTranslation || other.Rule.ID == "L04" {
			continue
		}
		if other.Failed() {
			t.Errorf("%s also reported the orphan: %v", other.Rule.ID, other.Findings)
		}
	}
}

func TestL05WantsTheEnglishFileAndItsHash(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			"paper: vaswani-2017-attention\ntitle: Attention Is All You Need\nkind: section\nlang: vi\n", viBody),
	}), false)

	res := result(t, rep, "L05")
	if len(res.Findings) != 2 {
		t.Fatalf("L05 found %d and both fields are missing: %v", len(res.Findings), res.Findings)
	}
}

// The English hash not matching is not this rule's business. A page
// re-extracted after it was translated is expected and papers translate
// queues it again by itself.
func TestL05DoesNotMindAHashThatHasMoved(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			answer(corpus.VI, "section", "some older English entirely"), viBody),
	}), false)

	if res := result(t, rep, "L05"); res.Failed() {
		t.Errorf("L05 reported a translation of an English page that has since moved: %v", res.Findings)
	}
}

const glossaryYAML = `version: 1
terms:
  - en: attention
    vi: chú ý
  - en: encoder
    vi: bộ mã hóa
  - en: chain
    vi: chuỗi
  - en: Markov chain
    vi: xích Markov
  - en: library
    vi: thư viện
    common: true
  - en: hash
    vi: băm
  - en: performance
    vi: hiệu năng
`

// glossaryPair is pairOf with a glossary on disk, for L06 and L10.
func glossaryPair(t *testing.T, en, tr string) *Report {
	t.Helper()
	return Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"manifests/glossary.yaml":                         glossaryYAML,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), en),
		"content/vi/vaswani-2017-attention/01_section.md": file(answer(corpus.VI, "section", en), tr),
	}), false)
}

func TestL06SaysWhenARenderingIsNowhereInTheTranslation(t *testing.T) {
	en := "The attention mechanism is what this section is about and it is described at length below.\n"
	tr := "Cơ chế tập trung là nội dung của mục này và nó được mô tả chi tiết ở bên dưới đây.\n"
	res := result(t, glossaryPair(t, en, tr), "L06")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "attention") {
		t.Fatalf("L06 said %v about a page that ignored the glossary", res.Findings)
	}
}

// A page that wrote the English term itself is L10's, not L06's. L10 is
// hard and decides whether the term was kept on purpose, and two rules
// reporting one page do not even agree about what is wrong with it.
func TestL06LeavesATermTheTranslationKeptInEnglishToL10(t *testing.T) {
	en := "The attention mechanism is what this section is about and it is described at length below.\n"
	tr := "Cơ chế attention là nội dung của mục này và nó được mô tả chi tiết bên dưới đây.\n"
	if res := result(t, glossaryPair(t, en, tr), "L06"); res.Failed() {
		t.Errorf("L06 reported a term L10 is the judge of: %v", res.Findings)
	}
	if res := result(t, glossaryPair(t, en, tr), "L10"); !res.Failed() {
		t.Error("L10 did not pick up the term L06 handed to it")
	}
}

// A term marked common is one the English uses in its ordinary sense as
// often as its technical one, and the rule cannot tell the two apart. Aho
// and Corasick sped up "a library bibliographic search program", which is a
// building with books in it.
func TestL06DoesNotAskForATermMarkedCommon(t *testing.T) {
	en := "The algorithm sped up a library bibliographic search program by a factor of five to ten.\n"
	tr := "Thuật toán đã tăng tốc một chương trình tra cứu thư mục thư tịch lên gấp năm đến mười lần.\n"
	if res := result(t, glossaryPair(t, en, tr), "L06"); res.Failed() {
		t.Errorf("L06 asked for the rendering of a common word: %v", res.Findings)
	}
}

// Marking a term common says nothing about whether its English may be left
// standing in the translation. It may not, and L10 is the rule that says so.
func TestACommonTermIsStillL10sBusiness(t *testing.T) {
	en := "The algorithm sped up a library bibliographic search program by a factor of five to ten.\n"
	tr := "Thuật toán đã tăng tốc một chương trình tra cứu thư mục của library lên gấp năm đến mười lần.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); !res.Failed() {
		t.Error("L10 passed a common term the translation left in English")
	}
}

// What is under the ACM headings is a controlled vocabulary and not prose.
// Dynamo's translator translated the headings and left the codes under them
// standing, which is the right answer, and L10 reported it for it.
func TestL10LeavesTheACMClassificationAlone(t *testing.T) {
	en := "This section is about the storage layer and the paragraph is here so that the rules with a length floor have something to read.\n\n" +
		"Categories and Subject Descriptors\nD.4.2 [Operating Systems]: Storage Management; D.4.5 [Operating Systems]: Performance;\n\n" +
		"General Terms\nAlgorithms, Management, Measurement, Performance, Design, Reliability.\n"
	tr := "Mục này nói về tầng lưu trữ và đoạn văn này ở đây để các quy tắc có ngưỡng độ dài có cái để đọc.\n\n" +
		"Các danh mục và mô tả đối tượng\nD.4.2 [Operating Systems]: Storage Management; D.4.5 [Operating Systems]: Performance;\n\n" +
		"Các thuật ngữ chung\nAlgorithms, Management, Measurement, Performance, Design, Reliability.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 asked for the ACM classification to be translated: %v", res.Findings)
	}
}

// The same word in a sentence is still the rule's business, or the test
// above passes because the term went missing rather than because the block
// was skipped.
func TestL10StillReadsTheProseAroundTheClassification(t *testing.T) {
	en := "The storage layer was built for performance and this sentence is long enough for the rules to have something to read.\n\n" +
		"General Terms\nAlgorithms, Management, Measurement, Performance, Design, Reliability.\n"
	tr := "Tầng lưu trữ được xây dựng cho performance và câu này đủ dài để các quy tắc có cái để đọc ở đây.\n\n" +
		"Các thuật ngữ chung\nAlgorithms, Management, Measurement, Performance, Design, Reliability.\n"
	res := result(t, glossaryPair(t, en, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "performance") {
		t.Errorf("L10 said %v about a term left standing in a sentence", res.Findings)
	}
}

// A name with its arguments stuck to it is an identifier and an identifier
// is the same in every language. MapReduce sets its partition function as
// running text rather than as code, so this is what the Vietnamese sees.
func TestL10ReadsANameWithItsArgumentsAsAName(t *testing.T) {
	en := "The intermediate key space is partitioned into R pieces using a partitioning function, for example hash(key) mod R.\n"
	tr := "Không gian khóa trung gian được phân vùng thành R phần bằng một hàm phân vùng, ví dụ hash(key) mod R.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 asked for a function call to be translated: %v", res.Findings)
	}
}

func TestL06IsQuietWhenTheRenderingIsThere(t *testing.T) {
	en := "The attention mechanism is what this section is about and it is described at length below.\n"
	tr := "Cơ chế chú ý là nội dung của mục này và nó được mô tả chi tiết ở bên dưới đây.\n"
	if res := result(t, glossaryPair(t, en, tr), "L06"); res.Failed() {
		t.Errorf("L06 reported a page that used the glossary: %v", res.Findings)
	}
}

// A one word term inside a longer glossary term the translation did render
// is not a missed rendering. Japanese writes "Markov chain" as マルコフ連鎖,
// which has no チェーン in it, so every page that mentioned one was reported
// for a word it had translated correctly.
func TestL06AcceptsATermInsideALongerTermTheTranslationRendered(t *testing.T) {
	en := "A Markov chain is what this section is about and it is described at length below the figure.\n"
	tr := "Xích Markov là nội dung của mục này và nó được mô tả chi tiết ở bên dưới hình vẽ này.\n"
	if res := result(t, glossaryPair(t, en, tr), "L06"); res.Failed() {
		t.Errorf("L06 reported a word inside a phrase the translation handled whole: %v", res.Findings)
	}
}

// The way out is only for the occurrence inside the phrase. A page that
// wrote the phrase and then used the word on its own still has to render it.
func TestL06StillAsksForTheTermWhereItStandsAlone(t *testing.T) {
	en := "A Markov chain is what this section is about, and the chain is described at length below.\n"
	tr := "Xích Markov là nội dung của mục này, và cái đó được mô tả chi tiết ở bên dưới đây.\n"
	res := result(t, glossaryPair(t, en, tr), "L06")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "chain") {
		t.Fatalf("L06 said %v about a page that left a standalone term unrendered", res.Findings)
	}
}

// A term inside a formula or a listing is not prose and must not be
// translated, so it must not be asked for either.
func TestL06IgnoresATermInsideAProtectedSpan(t *testing.T) {
	en := "The value is `encoder` and the rest of this paragraph is here to give the length rules something to look at.\n"
	tr := "Giá trị là `encoder` và phần còn lại của đoạn này ở đây để các quy tắc độ dài có cái để xem xét kỹ.\n"
	if res := result(t, glossaryPair(t, en, tr), "L06"); res.Failed() {
		t.Errorf("L06 asked for the rendering of a word inside a listing: %v", res.Findings)
	}
}

// twoSenses is a term the glossary holds twice, once per sense, the way it
// holds "feature".
const twoSenses = `version: 1
terms:
  - en: feature
    vi: đặc trưng
    fields: [ai-ml]
  - en: feature
    vi: tính năng
    fields: [software]
`

// sensePair is glossaryPair with the two sense glossary and a paper whose
// field picks the first of them.
func sensePair(t *testing.T, en, tr string) *Report {
	t.Helper()
	front := "paper: vaswani-2017-attention\ntitle: Attention Is All You Need\nfield: ai-ml\nkind: section\nlang: en\n"
	return Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"manifests/glossary.yaml":                         twoSenses,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(front, en),
		"content/vi/vaswani-2017-attention/01_section.md": file(answer(corpus.VI, "section", en), tr),
	}), false)
}

// A paper about models says "feature" in the software sense once, in its
// acknowledgements, and the translation is right to write the software
// sense of it. The field picked the other entry, so the rule has to look at
// both before it says the term was not rendered.
func TestTheGlossaryRulesAcceptTheOtherSenseOfATerm(t *testing.T) {
	en := "We thank the developer who shared a compiler feature with us, and the rest of this sentence is here so the rules have something to read.\n"
	tr := "Chúng tôi cảm ơn nhà phát triển đã chia sẻ một tính năng của trình biên dịch, và phần còn lại của câu này ở đây để các quy tắc có cái để đọc.\n"
	rep := sensePair(t, en, tr)
	for _, id := range []string{"L06", "L10"} {
		if res := result(t, rep, id); res.Failed() {
			t.Errorf("%s reported the other sense of a term: %v", id, res.Findings)
		}
	}
}

func TestATermLeftInEnglishIsAFindingInEitherSense(t *testing.T) {
	en := "We thank the developer who shared a compiler feature with us, and the rest of this sentence is here so the rules have something to read.\n"
	tr := "Chúng tôi cảm ơn nhà phát triển đã chia sẻ một feature của trình biên dịch, và phần còn lại của câu này ở đây để các quy tắc có cái để đọc.\n"
	if res := result(t, sensePair(t, en, tr), "L10"); !res.Failed() {
		t.Error("L10 passed a term left standing in English")
	}
}

func TestL07FindsAParagraphThatCameBackInEnglish(t *testing.T) {
	tr := strings.Replace(viBody, "Giới hạn là $n$ và phương pháp của [3] đạt tới nó. Một đoạn dài hơn\nnằm ở đây để các quy tắc có ngưỡng độ dài có cái để xem xét và không đứng\nxuống trước một mục chỉ có hai từ.",
		"The bound is $n$ and the method of [3] reaches it. A longer paragraph\nsits here so that the rules with a length floor have something to look at\nand do not stand down on a two word section.", 1)
	res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L07")
	if !res.Failed() {
		t.Fatal("L07 passed a paragraph that is the English paragraph")
	}
	if !strings.Contains(res.Findings[0].Message, "paragraph 1") {
		t.Errorf("L07 does not say which paragraph: %q", res.Findings[0].Message)
	}
}

// A paragraph that is nothing but a formula is the same in every language
// and is the right answer, not a finding.
func TestL07LeavesAParagraphWithNoProseAlone(t *testing.T) {
	if res := result(t, pairOf(t, corpus.VI, englishBody, viBody), "L07"); res.Failed() {
		t.Errorf("L07 reported the display and the listing: %v", res.Findings)
	}
}

// algol is one line of a program that the extraction left outside a fence,
// which is how the older papers in this corpus come out. It splits into
// eleven pieces on spaces and holds six words.
const algol = "else d := 1; e := c; drop := op - 1;"

func TestL07LeavesALineOfAProgramAlone(t *testing.T) {
	if n := len(strings.Fields(algol)); n < prosePerParagraph {
		t.Fatalf("the line splits into %d pieces, which is already under the floor, so this test proves nothing", n)
	}
	en, tr := englishBody+"\n"+algol+"\n", viBody+"\n"+algol+"\n"
	rep := pairOf(t, corpus.VI, en, tr)
	// L11 reads the same line as a sentence and has its own floor, so both
	// rules have to be looking at words for the line to get through.
	for _, id := range []string{"L07", "L11"} {
		if res := result(t, rep, id); res.Failed() {
			t.Errorf("%s asked for a line of ALGOL to be translated: %v", id, res.Findings)
		}
	}
}

// listing is a fence with a blank line in the middle of it, which is what
// nearly every program in this corpus looks like. Both halves are over the
// word floor and both are the same in every language.
const listing = "```cpp\n" +
	"// Store the list of input files into the specification\n" +
	"for (int i = 1; i < argc; i++) {\n" +
	"  MapReduceInput* input = spec.add_input();\n" +
	"}\n" +
	"\n" +
	"// Optional: do partial sums within the map tasks to save bandwidth\n" +
	"out->set_combiner_class(\"Adder\");\n" +
	"```"

func TestL07LeavesAListingWithABlankLineInItAlone(t *testing.T) {
	en, tr := englishBody+"\n"+listing+"\n", viBody+"\n"+listing+"\n"
	// The halves have to be long enough to reach the rule, or this passes
	// for the wrong reason and goes on passing after the fence is broken.
	for _, half := range strings.Split(listing, "\n\n") {
		if n := proseWords(half); n < prosePerParagraph {
			t.Fatalf("half of the listing holds %d words, under the floor, so this test proves nothing", n)
		}
	}
	for _, id := range []string{"L07", "L11"} {
		if res := result(t, pairOf(t, corpus.VI, en, tr), id); res.Failed() {
			t.Errorf("%s asked for a listing to be translated: %v", id, res.Findings)
		}
	}
}

// display is a formula written with room around it, which is how eight of
// the English files in this corpus print theirs. Cut at blank lines it is
// three blocks and the middle one has no delimiter in it, so the block on
// its own reads as a paragraph of TeX.
const display = "$$\n" +
	"\nF(x) = \\operatorname{sign} \\left[ \\frac{1}{2} (x - m_1)^T \\Sigma_1^{-1} (x - m_1) + \\ln \\frac{|\\Sigma_2|}{|\\Sigma_1|} \\right].\n" +
	"\n$$"

func TestL07LeavesADisplayWrittenWithRoomAroundItAlone(t *testing.T) {
	middle := strings.Split(display, "\n\n")[1]
	if n := proseWords(middle); n < prosePerParagraph {
		t.Fatalf("the formula holds %d words, under the floor, so this test proves nothing", n)
	}
	en, tr := englishBody+"\n"+display+"\n", viBody+"\n"+display+"\n"
	for _, id := range []string{"L07", "L11"} {
		if res := result(t, pairOf(t, corpus.VI, en, tr), id); res.Failed() {
			t.Errorf("%s asked for a formula to be translated: %v", id, res.Findings)
		}
	}
}

func TestProseWordsCountsWordsAndNotPunctuation(t *testing.T) {
	for text, want := range map[string]int{
		algol:                          6,
		"for f := 1 step 1 until e do": 6,
		"The bound is $n$ and the method of [3] reaches it.": 10,
		"$$n = 3$$": 1,
	} {
		if got := proseWords(text); got != want {
			t.Errorf("proseWords(%q) = %d, want %d", text, got, want)
		}
	}
}

// enFront is the masthead of a front page and its abstract: a title, a
// byline, an affiliation and a paragraph long enough to be an abstract.
const enFront = `A Bound On Sorting

**Ada Lovelace, Grace Hopper, Edsger Dijkstra, Barbara Liskov, Alan Turing**

Department of Computing
The University of Somewhere
Somewhere, SW1 2AB

Abstract

We give a lower bound on the number of comparisons a sorting method needs,
and we show that the bound is reached by a method that is simple enough to
write out in full, which is what the rest of this paper does.
`

// viFront is the same page translated the way a front page is meant to
// come back: the title and the abstract in Vietnamese, the names and the
// address exactly as they were printed.
const viFront = `Một Cận Dưới Cho Sắp Xếp

**Ada Lovelace, Grace Hopper, Edsger Dijkstra, Barbara Liskov, Alan Turing**

Department of Computing
The University of Somewhere
Somewhere, SW1 2AB

Tóm tắt

Chúng tôi đưa ra một cận dưới cho số phép so sánh mà một phương pháp sắp xếp
cần đến, và chúng tôi chỉ ra rằng cận này đạt được bởi một phương pháp đơn
giản đến mức có thể viết ra đầy đủ, và đó là việc phần còn lại của bài báo
này làm.
`

// frontPair writes one English front page and its translation.
func frontPair(t *testing.T, en, tr string) *Report {
	t.Helper()
	front := "paper: vaswani-2017-attention\ntitle: Attention Is All You Need\n" +
		"kind: front\nlang: vi\ntranslated_from: content/en/vaswani-2017-attention/00_front.md\n" +
		"source_content_sha256: " + corpus.ContentSHA([]byte(en)) + "\n" +
		"glossary_version: 1\nglossary_terms_sha256: aaaa\n"
	return Run(build(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), en),
		"content/vi/vaswani-2017-attention/00_front.md": file(front, tr),
	}), false)
}

// The byline and the affiliation of a front page are the same in every
// language and a translation that leaves them is right, so the two rules
// that read a paragraph against its English have to read past them to the
// abstract.
func TestTheMastheadOfAFrontPageIsNotAFinding(t *testing.T) {
	rep := frontPair(t, enFront, viFront)
	for _, id := range []string{"L07", "L11"} {
		if res := result(t, rep, id); res.Failed() {
			t.Errorf("%s reported the names and the address: %v", id, res.Findings)
		}
	}
}

func TestL07StillReadsTheAbstractOfAFrontPage(t *testing.T) {
	tr := strings.Replace(viFront, "Chúng tôi đưa ra một cận dưới cho số phép so sánh mà một phương pháp sắp xếp\ncần đến, và chúng tôi chỉ ra rằng cận này đạt được bởi một phương pháp đơn\ngiản đến mức có thể viết ra đầy đủ, và đó là việc phần còn lại của bài báo\nnày làm.",
		"We give a lower bound on the number of comparisons a sorting method needs,\nand we show that the bound is reached by a method that is simple enough to\nwrite out in full, which is what the rest of this paper does.", 1)
	res := result(t, frontPair(t, enFront, tr), "L07")
	if len(res.Findings) != 1 {
		t.Fatalf("L07 found %d on an untranslated abstract, want 1: %v", len(res.Findings), res.Findings)
	}
}

// A byline long enough to be mistaken for the abstract, which is the shape
// the System R front page has: fourteen authors as initials and surnames,
// comfortably over the forty words that say a paragraph is the abstract.
// The masthead was declared to have ended above it and the byline was then
// the first thing L07 read.
const enManyAuthors = `A Bound On Sorting

A. LOVELACE, G. HOPPER, E. W. DIJKSTRA, B. LISKOV, A. M. TURING,
J. VON NEUMANN, K. ZUSE, C. E. SHANNON, D. E. KNUTH, J. MCCARTHY,
N. WIRTH, T. HOARE, R. MILNER, M. O. RABIN, D. S. SCOTT,
A. J. PERLIS, AND P. NAUR

Department of Computing

Abstract

We give a lower bound on the number of comparisons a sorting method needs,
and we show that the bound is reached by a method that is simple enough to
write out in full, which is what the rest of this paper does.
`

func TestL07LeavesALongBylineAlone(t *testing.T) {
	blocks := blocksOf(enManyAuthors)
	// The fixture only tests anything if the byline really is long enough
	// to be taken for the abstract, so check that before checking the rule.
	if n := corpus.Words(plainProse(blocks[1])); n < split.AbstractParagraph {
		t.Fatalf("the byline is %d words, which is under the %d that makes this the trap it is testing", n, split.AbstractParagraph)
	}
	tr := strings.Replace(enManyAuthors, "A Bound On Sorting", "Một Cận Dưới Cho Sắp Xếp", 1)
	tr = strings.Replace(tr, "Abstract\n\nWe give a lower bound on the number of comparisons a sorting method needs,\nand we show that the bound is reached by a method that is simple enough to\nwrite out in full, which is what the rest of this paper does.",
		"Tóm tắt\n\nChúng tôi đưa ra một cận dưới cho số phép so sánh mà một phương pháp sắp xếp\ncần đến, và chúng tôi chỉ ra rằng cận này đạt được bởi một phương pháp đơn\ngiản đến mức có thể viết ra đầy đủ, và đó là việc phần còn lại của bài báo\nnày làm.", 1)
	res := result(t, frontPair(t, enManyAuthors, tr), "L07")
	if res.Failed() {
		t.Errorf("L07 asked for the byline to be translated: %v", res.Findings)
	}
}

// And the abstract under a byline that long is still read, or the fix for
// the byline would be a hole the size of the whole front page.
func TestL07StillReadsTheAbstractUnderALongByline(t *testing.T) {
	tr := strings.Replace(enManyAuthors, "A Bound On Sorting", "Một Cận Dưới Cho Sắp Xếp", 1)
	res := result(t, frontPair(t, enManyAuthors, tr), "L07")
	if len(res.Findings) != 1 {
		t.Fatalf("L07 found %d on an untranslated abstract, want 1: %v", len(res.Findings), res.Findings)
	}
}

func TestABylineIsToldFromAnAbstract(t *testing.T) {
	for _, c := range []struct {
		text string
		want bool
	}{
		{"m. m. astrahan, m. w. blasgen, d. d. chamberlin, k. p. eswaran, j. n. gray, p. p. griffiths, w. f. king, r. a. lorie, p. r. mcjones, j. w. mehl, g. r. putzolu, i. l. traiger, b. w. wade, and v. watson", true},
		{"james c. corbett, jeffrey dean, michael epstein, andrew fikes, christopher frost, jj furman, sanjay ghemawat, andrey gubarev, christopher heiser, peter hochschild, wilson hsieh", true},
		// A funding note is the thinnest real paragraph in the corpus and
		// it is still clear of the line. It is only asked about paragraphs
		// of forty words or more, so the fixtures are that long.
		{"this research was partially supported by the defense advanced research projects agency under a contract with the office of naval research, and by a grant from the national science foundation that is administered through the university", false},
		{"we give a lower bound on the number of comparisons a sorting method needs, and we show that the bound is reached by a method that is simple enough to write out in full, which is what the rest of this paper does", false},
	} {
		if got := byline(c.text); got != c.want {
			t.Errorf("byline(%.40q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestL08AndL15MarkAProvisionalPage(t *testing.T) {
	front := answer(corpus.VI, "section", englishBody) +
		"translation_model: gpt-5.4-mini\nsmall_model: true\ngateway: true\n"
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(front, viBody),
	}), false)

	for _, id := range []string{"L08", "L15"} {
		res := result(t, rep, id)
		if !res.Failed() {
			t.Errorf("%s passed a page that says in its own front matter that it is provisional", id)
		}
		if res.Rule.Hard {
			t.Errorf("%s is hard, and a provisional page is published with a mark on it and not rejected", id)
		}
	}
}

// The rule reads the model name as well as the flag, because the flag is
// written by one command and the name is written by all of them.
func TestL08ReadsTheModelNameToo(t *testing.T) {
	front := answer(corpus.VI, "section", englishBody) + "translation_model: gemini-2.0-flash\n"
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(front, viBody),
	}), false)

	if res := result(t, rep, "L08"); !res.Failed() {
		t.Error("L08 passed a page whose recorded model has flash in the name")
	}
}

func TestL09FindsAVersionThatDidNotMove(t *testing.T) {
	first := answer(corpus.VI, "section", englishBody)
	second := strings.Replace(answer(corpus.VI, "section", englishBody),
		"glossary_terms_sha256: aaaa", "glossary_terms_sha256: bbbb", 1)
	second = strings.Replace(second, "01_section.md", "02_section.md", 1)
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/en/vaswani-2017-attention/02_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(first, viBody),
		"content/vi/vaswani-2017-attention/02_section.md": file(second, viBody),
	}), false)

	res := result(t, rep, "L09")
	if !res.Failed() {
		t.Fatal("L09 passed two files under one version translated against different renderings")
	}
}

func TestL10FindsAnEnglishTermLeftStanding(t *testing.T) {
	en := "The encoder is described here and the paragraph is long enough for every length floor in the group.\n"
	tr := "Phần encoder được mô tả ở đây và đoạn này đủ dài cho mọi ngưỡng độ dài trong nhóm quy tắc.\n"
	res := result(t, glossaryPair(t, en, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Fatalf("L10 said %v about a page that left a term in English", res.Findings)
	}
}

// A word that is a column heading in a table the run never translated is a
// word the caption under it has to keep. The TPU paper's Table 1 is a fenced
// block with a column headed "Vector" and a caption that reads "Vector is
// self-explanatory", and a caption that renamed the column would be
// describing a table that is not there.
func TestL10LeavesALabelFromAListingAlone(t *testing.T) {
	en := "```\nName  Layers  Encoder  Total\n```\n\nTable 1. The columns are the name, the layer count and the Encoder column, which is self-explanatory.\n"
	tr := "```\nName  Layers  Encoder  Total\n```\n\nBảng 1. Các cột là tên, số lớp và cột Encoder, vốn đã tự giải thích rõ ràng.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 asked for a column heading to be translated: %v", res.Findings)
	}
}

// And it is the listing that earns the exception, not the word. The same
// caption on a page with no such table is the finding it always was.
func TestL10StillReadsATermThatIsNotALabel(t *testing.T) {
	en := "Table 1. The columns are the name, the layer count and the Encoder column, which is self-explanatory.\n"
	tr := "Bảng 1. Các cột là tên, số lớp và cột Encoder, vốn đã tự giải thích rõ ràng.\n"
	res := result(t, glossaryPair(t, en, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Errorf("L10 said %v about a page that left a term in English", res.Findings)
	}
}

// A name the English hyphenates because it is being used as an adjective is
// the same name the translation writes with a space, and the word inside it
// is not a term left standing. Spanner writes "protocol-buffer-valued fields"
// and the Vietnamese writes "giá trị kiểu protocol buffer".
func TestL10ReadsAHyphenatedNameAsTheSameName(t *testing.T) {
	en := "The query language has extensions to support protocol-encoder-valued fields and nothing else here.\n"
	tr := "Ngôn ngữ truy vấn có phần mở rộng hỗ trợ các trường có giá trị kiểu protocol encoder và không gì khác.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 read the tail of a name as a term left standing: %v", res.Findings)
	}
}

// A pair with a hyphen of its own still matches. Spanner's Table 2 lists a
// "Read-Write Transaction" in both languages, and reading the hyphen as a
// space on one side only would have lost this to gain the case above.
func TestL10ReadsAHyphenInsideTheNameOnBothSides(t *testing.T) {
	en := "| Read-Write Encoder | leader |\n\nThe table above lists the kinds and there is nothing else on this page.\n"
	tr := "| Read-Write Encoder | leader |\n\nBảng ở trên liệt kê các loại và trên trang này không còn nội dung nào khác.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 read a row label both sides wrote the same way as a term: %v", res.Findings)
	}
}

// And it is the name that earns it. The same word with no such name around
// it on the English side is the finding it always was.
func TestL10StillReadsATermWithNoNameAroundIt(t *testing.T) {
	en := "The query language has extensions to support encoder fields and there is nothing else here.\n"
	tr := "Ngôn ngữ truy vấn có phần mở rộng hỗ trợ các trường encoder và ở đây không có gì khác nữa.\n"
	res := result(t, glossaryPair(t, en, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Errorf("L10 said %v about a page that left a term in English", res.Findings)
	}
}

// A gloss is good practice on a term's first appearance, and the rendering
// is right there in the file.
// Twenty of the twenty-three L06 and L10 findings on the first translated
// paper in the corpus were this, and every one of them was a page that had
// done exactly the right thing.
func TestTheGlossaryRulesLeaveALongerEnglishNameAlone(t *testing.T) {
	for _, tc := range []struct {
		name, en, tr string
	}{
		{
			name: "a hyphenated compound",
			en:   "The encoder-decoder bound is derived here and this paragraph is long enough for every length floor in the group.\n",
			tr:   "Cận encoder-decoder được suy ra ở đây và đoạn này đủ dài cho mọi ngưỡng độ dài trong nhóm quy tắc.\n",
		},
		{
			name: "an English name of a method the translation kept",
			en:   "Both have training rules that resemble encoder matching applied to the model, and this paragraph is long enough.\n",
			tr:   "Cả hai đều có quy tắc huấn luyện giống encoder matching được áp dụng cho mô hình, và đoạn này đủ dài rồi.\n",
		},
		{
			name: "the name with the term at the end of it",
			en:   "The work extends the denoising encoder of the earlier paper, and this paragraph is long enough for the floors.\n",
			tr:   "Công trình mở rộng denoising encoder của bài báo trước đó, và đoạn này đủ dài cho mọi ngưỡng độ dài.\n",
		},
	} {
		for _, rule := range []string{"L06", "L10"} {
			if res := result(t, glossaryPair(t, tc.en, tc.tr), rule); res.Failed() {
				t.Errorf("%s: %s reported %v", tc.name, rule, res.Findings)
			}
		}
	}
}

// The other side of it. A name the translation invented is not a name the
// paper wrote, and the term inside it is still standing in English.
func TestAnEnglishNameTheSourceNeverWroteIsStillAFinding(t *testing.T) {
	en := "The encoder is described here and the paragraph is long enough for every length floor in the group.\n"
	tr := "Phần encoder này được mô tả ở đây và đoạn này đủ dài cho mọi ngưỡng độ dài trong nhóm quy tắc.\n"
	res := result(t, glossaryPair(t, en, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Fatalf("L10 said %v about a term standing between two Vietnamese words", res.Findings)
	}
}

func TestL10AllowsAGloss(t *testing.T) {
	en := "The encoder is described here and the paragraph is long enough for every length floor in the group.\n"
	tr := "Bộ mã hóa (encoder) được mô tả ở đây và đoạn này đủ dài cho mọi ngưỡng độ dài trong nhóm.\n"
	if res := result(t, glossaryPair(t, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 reported a term glossed with its rendering beside it: %v", res.Findings)
	}
}

func TestL11FindsOneEnglishSentenceInATranslatedParagraph(t *testing.T) {
	en := "Giới hạn được mô tả ở đây một cách đầy đủ. The remaining sentence of this paragraph was never translated at all. Phần còn lại thì có.\n"
	source := "The bound is described here in full. The remaining sentence of this paragraph was never translated at all. The rest of it was.\n"
	res := result(t, pairOf(t, corpus.VI, source, en), "L11")
	if !res.Failed() {
		t.Fatal("L11 passed a paragraph with an English sentence left in it")
	}
}

// A paragraph left in English whole is L07's finding, and reporting it
// twice is two ways of saying the same thing.
func TestL11LeavesAWhollyEnglishParagraphToL07(t *testing.T) {
	source := "The bound is described here in full and this sentence is long enough for the floor.\n"
	res := result(t, pairOf(t, corpus.VI, source, source), "L11")
	if res.Failed() {
		t.Errorf("L11 reported a paragraph that L07 already reports: %v", res.Findings)
	}
}

func TestL12WantsTheWordsInsideTheMathematicsTranslated(t *testing.T) {
	source := `The distribution is $p_{\text{data}}(x)$ and the truth is $\text{true when } A$ holds here.` + "\n"
	tr := `Phân phối là $p_{\text{data}}(x)$ và điều kiện là $\text{true when } A$ đúng ở đây.` + "\n"
	res := result(t, pairOf(t, corpus.VI, source, tr), "L12")
	if !res.Failed() {
		t.Fatal("L12 passed a formula whose prose came back in English")
	}
	if !strings.Contains(res.Findings[0].Message, "true when") {
		t.Errorf("L12 does not say which words: %q", res.Findings[0].Message)
	}
}

// An operator name set upright is not prose and does not move, and a
// subscript label of one letter is not a word.
func TestL12LeavesAnOperatorNameAlone(t *testing.T) {
	source := `The value is $\text{argmax}_x f(x)$ and the label is $y_{\text{i}}$ in this paragraph here.` + "\n"
	tr := `Giá trị là $\text{argmax}_x f(x)$ và nhãn là $y_{\text{i}}$ trong đoạn văn này ở đây.` + "\n"
	if res := result(t, pairOf(t, corpus.VI, source, tr), "L12"); res.Failed() {
		t.Errorf("L12 asked for an operator name to be translated: %v", res.Findings)
	}
}

// A \text applied to an argument or carrying an index names something in
// the formula, and the prose around it goes on calling it by that name. The
// Transformer paper has six of them and the upright list had four.
func TestL12LeavesAnAppliedNameAlone(t *testing.T) {
	source := `The block is $\text{Sublayer}(x)$ and the ith of them is $\text{head}_i$ in this paragraph here.` + "\n"
	tr := `Khối là $\text{Sublayer}(x)$ và cái thứ i là $\text{head}_i$ trong đoạn văn này ở đây.` + "\n"
	if res := result(t, pairOf(t, corpus.VI, source, tr), "L12"); res.Failed() {
		t.Errorf("L12 asked for the name of a function to be translated: %v", res.Findings)
	}
}

// The journal's own line at the foot of a first page stands as printed, the
// same way a venue name in a bibliography does. It sits below the abstract,
// so the masthead rule does not reach it, and it runs past L07's eight word
// threshold. The Dennard paper was asked again and gave the same answer,
// which is the right answer.
func TestL07LeavesTheRunningHeadAlone(t *testing.T) {
	const head = "\nA JOURNAL OF INVENTED RESULTS, VOL. SC-9, NO. 5, OCTOBER 1974\n"
	const venued = `papers:
  - id: vaswani-2017-attention
    title: Attention Is All You Need
    authors: [Ashish Vaswani]
    year: 2017
    venue: A Journal of Invented Results
    field: ai-ml
    status: listed
`
	page := func(t *testing.T, en, tr string) *Report {
		t.Helper()
		return Run(build(t, map[string]string{
			"manifests/papers.yaml":                         venued,
			"manifests/sources.yaml":                        openSources,
			"content/en/vaswani-2017-attention/00_front.md": file(section("front"), en),
			"content/vi/vaswani-2017-attention/00_front.md": file(
				strings.Replace(answer(corpus.VI, "front", en), "01_section.md", "00_front.md", 1), tr),
		}), false)
	}
	if res := result(t, page(t, enFront+head, viFront+head), "L07"); res.Failed() {
		t.Errorf("L07 asked for the journal's own line to be translated: %v", res.Findings)
	}
	// And it is not a blanket pass on the page. The same page with the
	// abstract left in English is still a finding.
	if res := result(t, page(t, enFront+head, enFront+head), "L07"); !res.Failed() {
		t.Error("L07 passed a front page whose abstract came back in English")
	}
}

// A bibliography is the English file character for character, which is what
// rule L14 asks for, so every word inside every formula in it is in English
// and none of them is a finding. The Spanner bibliography cites a paper
// whose title sets "(by definition)" in \text, and this rule used to refuse
// the corpus over it.
func TestL12LeavesTheBibliographyAlone(t *testing.T) {
	body := "1. A. Author. A paper about $\\text{(by definition)}$ ordering. In Proc. 1978.\n"
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                        openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
		"content/en/vaswani-2017-attention/09_references.md": file(
			section("references"), body),
		"content/vi/vaswani-2017-attention/09_references.md": file(
			strings.Replace(answer(corpus.VI, "references", body),
				"01_section.md", "09_references.md", 1), body),
	}), false)
	if res := result(t, rep, "L12"); res.Failed() {
		t.Errorf("L12 asked for the bibliography to be translated: %v", res.Findings)
	}
}

// The rule that needed most care. Han characters in a Vietnamese page are
// wrong and in a Japanese one are right, and kana in a Chinese page are
// wrong. The allowed set is a table per language, not a constant.
func TestL13IsPerLanguage(t *testing.T) {
	cases := []struct {
		name  string
		lang  corpus.Lang
		body  string
		fails bool
	}{
		{"Han in Vietnamese", corpus.VI, "Giới hạn là 注意 và phần còn lại của đoạn này thì không có gì lạ cả.\n", true},
		{"Han in Chinese", corpus.ZH, "界限是注意力机制，这一段其余的部分没有什么特别的地方需要说明。\n", false},
		{"kana in Chinese", corpus.ZH, "界限是アテンション机制，这一段其余的部分没有什么特别的地方需要说明。\n", true},
		{"kana in Japanese", corpus.JA, "境界はアテンション機構であり、この段落の残りの部分に特別なことは何もない。\n", false},
		{"Latin in Chinese", corpus.ZH, "界限是 Transformer 模型，这一段其余的部分没有什么特别的地方要说。\n", false},
		{"Hangul in Japanese", corpus.JA, "境界は주의機構であり、この段落の残りの部分に特別なことは何もありません。\n", true},
	}
	source := "The bound is the attention mechanism and there is nothing else in this paragraph worth saying.\n"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := result(t, pairOf(t, c.lang, source, c.body), "L13")
			if res.Failed() != c.fails {
				t.Errorf("L13 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
		})
	}
}

// A Greek letter in a formula is a formula, and the rule reads the prose.
func TestL13LeavesTheMathematicsAlone(t *testing.T) {
	source := "The parameter is $\\alpha$ and the paragraph goes on for long enough to be looked at here.\n"
	tr := "Tham số là $\\alpha$ và đoạn văn tiếp tục đủ dài để được xem xét kỹ lưỡng ở đây.\n"
	if res := result(t, pairOf(t, corpus.VI, source, tr), "L13"); res.Failed() {
		t.Errorf("L13 reported a formula: %v", res.Findings)
	}
}

func TestL14WantsTheBibliographyCopied(t *testing.T) {
	for _, c := range []struct {
		name  string
		body  string
		fails bool
	}{
		{"copied", bibliography, false},
		{"asked for", strings.Replace(bibliography, "A Paper", "Một Bài Báo", 1), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			rep := Run(build(t, map[string]string{
				"manifests/sources.yaml":                        openSources,
				"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
				"content/en/vaswani-2017-attention/09_references.md": file(
					section("references"), bibliography),
				"content/vi/vaswani-2017-attention/09_references.md": file(
					strings.Replace(answer(corpus.VI, "references", bibliography),
						"01_section.md", "09_references.md", 1), c.body),
			}), false)
			if res := result(t, rep, "L14"); res.Failed() != c.fails {
				t.Errorf("L14 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
		})
	}
}

func TestL16FindsACitationThatChanged(t *testing.T) {
	tr := strings.Replace(viBody, "[3]", "[4]", 1)
	res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L16")
	if !res.Failed() {
		t.Fatal("L16 passed a translation that renumbered a citation")
	}
	if !strings.Contains(res.Findings[0].Message, "[4]") {
		t.Errorf("L16 does not say what it found: %q", res.Findings[0].Message)
	}
}

// A citation dropped altogether is the worse half of the same rule, and the
// message has to name the one that went missing rather than the one that
// stayed.
func TestL16FindsACitationThatWasDropped(t *testing.T) {
	tr := strings.Replace(viBody, " của [3]", "", 1)
	res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L16")
	if !res.Failed() {
		t.Fatal("L16 passed a translation that dropped a citation")
	}
	if !strings.Contains(res.Findings[0].Message, "[3]") {
		t.Errorf("L16 does not name the citation that went missing: %q", res.Findings[0].Message)
	}
}

// The title lives in the front matter and the body rules never see it, so
// this is the one line of a translated file that can stay in English with
// everything else correct. It is also the line the book prints biggest.
func TestL19ReadsTheTitleInTheFrontMatter(t *testing.T) {
	for _, c := range []struct {
		name  string
		en    string
		vi    string
		fails bool
	}{
		{"left in English", "Related Work", "Related Work", true},
		{"translated", "Related Work", "Công trình liên quan", false},
		{"an abbreviation that stands", "GAN", "GAN", false},
		{"a numbered title that stands", "3.2", "3.2", false},
		{"one long word left in English", "Experiments", "Experiments", true},
		{"a coined name that stands", "TrueTime", "TrueTime", false},
		{"a compound that is two words", "Related Work", "Related Work", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			rep := Run(build(t, map[string]string{
				"manifests/sources.yaml":                        openSources,
				"content/en/vaswani-2017-attention/00_front.md": file(section("front"), abstract),
				"content/en/vaswani-2017-attention/01_section.md": file(
					section("section")+"section_title: "+c.en+"\n", englishBody),
				"content/vi/vaswani-2017-attention/01_section.md": file(
					answer(corpus.VI, "section", englishBody)+"section_title: "+c.vi+"\n", viBody),
			}), false)

			res := result(t, rep, "L19")
			if res.Failed() != c.fails {
				t.Errorf("L19 failed=%v, want %v: %v", res.Failed(), c.fails, res.Findings)
			}
			if res.Rule.Hard {
				t.Error("L19 is hard, and a title that is the same in both languages is often the right answer")
			}
		})
	}
}

// The two files whose title this toolchain wrote rather than the paper. The
// book prints its own word for each of them per language, so there is
// nothing here for a translator to have got wrong.
func TestL19SkipsTheFrontMatterAndTheBibliography(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml": openSources,
		"content/en/vaswani-2017-attention/00_front.md": file(
			section("front")+"section_title: Front Matter\n", abstract),
		"content/en/vaswani-2017-attention/09_references.md": file(
			section("references")+"section_title: References\n", bibliography),
		"content/vi/vaswani-2017-attention/09_references.md": file(
			strings.Replace(answer(corpus.VI, "references", bibliography),
				"01_section.md", "09_references.md", 1)+"section_title: References\n", bibliography),
	}), false)

	if res := result(t, rep, "L19"); res.Failed() {
		t.Errorf("L19 asked for a label this toolchain wrote to be translated: %v", res.Findings)
	}
}

func TestL17FindsTheModelTalking(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
	}{
		{"an apology", "I'm sorry, but I cannot translate this passage for you today at all.\n"},
		{"a preamble", "Here is the translation of the passage you asked about, in Vietnamese below.\n"},
		{"a provider error", "Bản dịch bị lỗi vì máy chủ trả về Service Unavailable khi được hỏi lần nữa.\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if res := result(t, pairOf(t, corpus.VI, englishBody, c.body), "L17"); !res.Failed() {
				t.Errorf("L17 passed %s", c.name)
			}
		})
	}
}

func TestL17FindsAnEmptyTranslation(t *testing.T) {
	if res := result(t, pairOf(t, corpus.VI, englishBody, "\n"), "L17"); !res.Failed() {
		t.Error("L17 passed a translation with nothing in it")
	}
}

// A section that is a heading with its subsections under it carries no prose
// of its own, and the only right translation of no prose is no prose.
func TestL17LeavesAnEmptyTranslationOfAnEmptySection(t *testing.T) {
	if res := result(t, pairOf(t, corpus.VI, "\n", "\n"), "L17"); res.Failed() {
		t.Errorf("L17 asked for a translation of a section with nothing in it: %v", res.Findings)
	}
}

func TestL18FindsAListingThatMoved(t *testing.T) {
	tr := strings.Replace(viBody, "return n", "return m", 1)
	if res := result(t, pairOf(t, corpus.VI, englishBody, tr), "L18"); !res.Failed() {
		t.Fatal("L18 passed a listing whose program changed")
	}
}

// The English of a paper is re-read whenever the extraction improves, and
// until the translator catches up the file on disk answers an English that
// is no longer there. That is L20's finding and it is not the other rules'.
func TestL20FindsATranslationOfAnEnglishThatHasMoved(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			answer(corpus.VI, "section", "the English as it stood a month ago"), viBody),
	}), false)

	res := result(t, rep, "L20")
	if len(res.Findings) != 1 {
		t.Fatalf("L20 found %d on one stale translation: %v", len(res.Findings), res.Findings)
	}
}

// A translation of the English as it stands is not stale and L20 says
// nothing about it.
func TestL20SaysNothingAboutACurrentTranslation(t *testing.T) {
	if res := result(t, pairOf(t, corpus.VI, englishBody, viBody), "L20"); res.Failed() {
		t.Errorf("L20 reported a current translation: %v", res.Findings)
	}
}

// A verdict of differs-materially is work the corpus has already decided to
// do, and a file still carrying one is that work not yet done. Nothing said
// so until this rule: seven Chinese and Japanese files sat in the corpus
// with the verdict on them while the report a person reads named nine
// Vietnamese files that no longer carried it at all.
func TestL21FindsATranslationTheBackTranslationRefused(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			answer(corpus.VI, "section", englishBody)+"roundtrip: differs-materially\n", viBody),
	}), false)

	res := result(t, rep, "L21")
	if len(res.Findings) != 1 {
		t.Fatalf("L21 found %d on one refused translation: %v", len(res.Findings), res.Findings)
	}
}

// Differing in wording is the check saying the translation is right and
// says it differently, which is what a translation is. Only a material
// difference is work owed.
func TestL21LeavesAWordingDifferenceAlone(t *testing.T) {
	for _, verdict := range []string{"same", "differs-in-wording", ""} {
		front := answer(corpus.VI, "section", englishBody)
		if verdict != "" {
			front += "roundtrip: " + verdict + "\n"
		}
		rep := Run(build(t, map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
			"content/vi/vaswani-2017-attention/01_section.md": file(front, viBody),
		}), false)
		if res := result(t, rep, "L21"); res.Failed() {
			t.Errorf("L21 reported a translation the check passed as %q: %v", verdict, res.Findings)
		}
	}
}

// The Vietnamese of the Gamma paper said "các quan hệ lớn hơn 1 million
// bộ". The sentence is Vietnamese, the number is a number, and the word in
// the middle is English where triệu was meant. Nothing caught it: L13 reads
// the script and Vietnamese is written in Latin, L06 reads the glossary and
// million is not a term of art anybody would put in one, and the back
// translation put it into English and got the same number out.
func TestL22FindsAnEnglishScaleWordAfterANumber(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		fails bool
	}{
		{"the English word", "Không có cách nào để tạo ra các quan hệ lớn hơn 1 million bộ.\n", true},
		{"with a separator in the numeral", "Chúng tôi đã đo trên 1,000 thousand bản ghi.\n", true},
		{"the Vietnamese word", "Không có cách nào để tạo ra các quan hệ lớn hơn 1 triệu bộ.\n", false},
		{"a numeral on its own", "Không có cách nào để tạo ra các quan hệ lớn hơn 1000000 bộ.\n", false},
		{"the word with no number in front of it", "Một phần triệu, one in a million, là cách nói quen thuộc.\n", false},
	}
	for _, tc := range cases {
		rep := Run(build(t, map[string]string{
			"manifests/sources.yaml":                          openSources,
			"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
			"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
			"content/vi/vaswani-2017-attention/01_section.md": file(answer(corpus.VI, "section", englishBody), tc.body),
		}), false)
		if res := result(t, rep, "L22"); res.Failed() != tc.fails {
			t.Errorf("%s: L22 failed=%v, want %v (%v)", tc.name, res.Failed(), tc.fails, res.Findings)
		}
	}
}

// The titles of cited works stay in the language they were published in, so
// a paper called something about a billion rows is cited that way in all
// four languages.
func TestL22LeavesTheReferencesAlone(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                             openSources,
		"content/en/vaswani-2017-attention/00_front.md":      file(section("front"), abstract),
		"content/en/vaswani-2017-attention/09_references.md": file(section("references"), "[1] Sorting 1 billion records.\n"),
		"content/vi/vaswani-2017-attention/09_references.md": file(
			answer(corpus.VI, "references", "[1] Sorting 1 billion records.\n"), "[1] Sorting 1 billion records.\n"),
	}), false)
	if res := result(t, rep, "L22"); res.Failed() {
		t.Errorf("L22 reported a cited title: %v", res.Findings)
	}
}

// The Paxos front page grew from an abstract to six pages and the Japanese
// of the old abstract was reported as having dropped two citations, both
// headings and half the paper. One finding about a stale file, not a
// hundred.
func TestTheComparisonRulesStandDownOnAStaleTranslation(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			answer(corpus.VI, "section", "the English as it stood a month ago"),
			"Một đoạn văn không có gì chung với bản tiếng Anh.\n"),
	}), false)

	for _, id := range []string{"L01", "L03", "L07", "L16", "L18"} {
		if res := result(t, rep, id); res.Failed() {
			t.Errorf("%s reported a stale translation: %v", id, res.Findings)
		}
	}
}

// A stale file is still asked what wrote it and what script it is in,
// because those have the same answer whatever the English has done since.
func TestTheFileRulesStillRunOnAStaleTranslation(t *testing.T) {
	rep := Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), englishBody),
		"content/vi/vaswani-2017-attention/01_section.md": file(
			answer(corpus.VI, "section", "the English as it stood a month ago"),
			"I'm sorry, I cannot translate this page.\n"),
	}), false)

	if res := result(t, rep, "L17"); !res.Failed() {
		t.Error("L17 passed an apology because the file was out of date")
	}
}
