package audit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// Every passage here is typeset in this file rather than taken off a paper.
// The names in them are invented for the same reason.

// glossedYAML renders one term as the English term with a Vietnamese word in
// front of it, which is what Vietnamese does with "attention", and renders
// another plainly so the tests have something to compare against.
const glossedYAML = `version: 1
terms:
  - en: attention
    vi: cơ chế attention
  - en: encoder
    vi: bộ mã hóa
  - en: benchmark
    vi: phép đo chuẩn
`

// glossaryPairWith is glossaryPair with a glossary of the test's choosing.
func glossaryPairWith(t *testing.T, yaml, en, tr string) *Report {
	t.Helper()
	return Run(build(t, map[string]string{
		"manifests/sources.yaml":                          openSources,
		"manifests/glossary.yaml":                         yaml,
		"content/en/vaswani-2017-attention/00_front.md":   file(section("front"), abstract),
		"content/en/vaswani-2017-attention/01_section.md": file(section("section"), en),
		"content/vi/vaswani-2017-attention/01_section.md": file(answer(corpus.VI, "section", en), tr),
	}), false)
}

// A rendering that keeps the English term is used by writing the English
// term, so a page that wrote it has used the glossary and not ignored it.
func TestL10DoesNotAskForARenderingThatKeepsTheEnglishTerm(t *testing.T) {
	en := "In the rows of the table we varied the number of attention heads and the width of the keys and values.\n"
	tr := "Trong các hàng của bảng, chúng tôi thay đổi số lượng đầu attention và chiều rộng của khóa cùng giá trị.\n"
	if res := result(t, glossaryPairWith(t, glossedYAML, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 asked for a rendering that already has the term in it: %v", res.Findings)
	}
	// The same page under a rendering that does not keep the English is a
	// finding, or the test above passes for some other reason entirely.
	plain := strings.Replace(glossedYAML, "cơ chế attention", "chú ý", 1)
	if res := result(t, glossaryPairWith(t, plain, en, tr), "L10"); !res.Failed() {
		t.Error("L10 passed a term left standing under a rendering that does not keep it")
	}
}

// quotedEn is a paragraph with what a model was asked quoted inside it, and
// quotedVi is it translated the way it should be, with the quotation as
// printed because the quotation is what was asked.
const (
	quotedEn = "The paper prints the question that was put to the model, which was “describe the encoder in one line”, and prints the answer that came back under it.\n"
	quotedVi = "Bài báo in ra câu hỏi đã đặt cho mô hình, vốn là “describe the encoder in one line”, và in câu trả lời nhận được ở bên dưới.\n"
)

func TestL10LeavesATermInsideAQuotation(t *testing.T) {
	if res := result(t, glossaryPairWith(t, glossedYAML, quotedEn, quotedVi), "L10"); res.Failed() {
		t.Errorf("L10 asked for a quotation to be translated: %v", res.Findings)
	}
}

// The words around the quotation are still the translator's own, or the test
// above passes because the whole paragraph went unread.
func TestL10StillReadsTheProseAroundAQuotation(t *testing.T) {
	tr := "Bài báo in ra câu hỏi đã đặt cho encoder, vốn là “describe the encoder in one line”, và in câu trả lời nhận được.\n"
	res := result(t, glossaryPairWith(t, glossedYAML, quotedEn, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Errorf("L10 said %v about a term left standing outside the quotation", res.Findings)
	}
}

// tableEn is a paragraph and a table of the words a model produced. A table
// of words is the result, so it stands in every language.
const (
	tableEn = "The table below lists the words the model produced for each of the two settings we tried in this experiment.\n\n" +
		"| Setting | Words |\n| --- | --- |\n| The first one | encoder, decoder, layer |\n| The second one | weight, gradient, step |\n"
	tableVi = "Bảng dưới đây liệt kê các từ mà mô hình sinh ra cho mỗi trong hai thiết lập mà chúng tôi đã thử.\n\n" +
		"| Thiết lập | Từ |\n| --- | --- |\n| Thiết lập thứ nhất | encoder, decoder, layer |\n| Thiết lập thứ hai | weight, gradient, step |\n"
)

func TestL10LeavesATermInATable(t *testing.T) {
	if res := result(t, glossaryPairWith(t, glossedYAML, tableEn, tableVi), "L10"); res.Failed() {
		t.Errorf("L10 asked for a table of results to be translated: %v", res.Findings)
	}
}

func TestL10StillReadsTheProseAboveATable(t *testing.T) {
	tr := strings.Replace(tableVi, "các từ mà mô hình", "các từ mà encoder", 1)
	res := result(t, glossaryPairWith(t, glossedYAML, tableEn, tr), "L10")
	if !res.Failed() || !strings.Contains(res.Findings[0].Message, "encoder") {
		t.Errorf("L10 said %v about a term left standing above the table", res.Findings)
	}
}

// A two word English name comes out of Vietnamese with the two words the
// other way round, because Vietnamese puts the qualifier second.
func TestL10ReadsANameTheTranslationPutInVietnameseOrder(t *testing.T) {
	en := "We thank the group that let us use their Hillside benchmark relation generator for the runs reported here.\n"
	tr := "Chúng tôi cảm ơn nhóm đã cho phép dùng bộ sinh quan hệ benchmark Hillside của họ cho các lần chạy ở đây.\n"
	if res := result(t, glossaryPairWith(t, glossedYAML, en, tr), "L10"); res.Failed() {
		t.Errorf("L10 asked for a name to be translated: %v", res.Findings)
	}
	// The same word with no name around it is a finding, so the exemption
	// is about the name and not about the word.
	bare := "Chúng tôi cảm ơn nhóm đã cho phép dùng bộ sinh quan hệ cho benchmark của họ cho các lần chạy ở đây.\n"
	if res := result(t, glossaryPairWith(t, glossedYAML, en, bare), "L10"); !res.Failed() {
		t.Error("L10 passed a term standing on its own next to no name at all")
	}
}
