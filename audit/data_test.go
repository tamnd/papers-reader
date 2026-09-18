package audit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// Every passage here is typeset in this file rather than taken off a paper,
// which is the rule for fixtures in this repository. The shapes are the
// shapes the corpus was refused over and the words are not.

// table is a results table, English on both sides because the numbers and the
// row labels are the result.
const table = `| Setting | First task | Second task |
| --- | --- | --- |
| A system that was tuned for the task | 79.4 | 92.0 |
| A system that was given three examples | 80.5 | 68.8 |`

// listing is a figure showing what was asked of a model, which is four lines
// of pairs and not one sentence.
const examples = `Turn one language into another:
one animal => un animal
one plant => une plante
one stone =>`

// cloze is a question with the answer taken out of it.
const cloze = "Adams bought some equipment for the game, a ball, a glove, and a ______."

// sample is a passage a model wrote, which the paper prints to show what the
// model wrote.
const sample = `"This is a passage that was written by a machine. It is printed here word for word so that a reader can judge it for themselves."`

// dataBlocks are the four shapes, each of which was reported as a paragraph a
// translator had skipped.
func dataBlocks() map[string]string {
	return map[string]string{
		"a table":     table,
		"a listing":   examples,
		"a question":  cloze,
		"a quotation": sample,
	}
}

func TestTheRulesLeaveThePapersOwnDataAlone(t *testing.T) {
	for what, block := range dataBlocks() {
		// Over the floor, or the block never reaches the rule and this
		// test goes on passing after the exemption is taken out again.
		if n := proseWords(plainProse(block)); n < prosePerParagraph {
			t.Fatalf("%s holds %d words, under the floor, so this test proves nothing", what, n)
		}
		if !data(block) {
			t.Errorf("%s is not read as data", what)
		}
		en, tr := englishBody+"\n"+block+"\n", viBody+"\n"+block+"\n"
		rep := pairOf(t, corpus.VI, en, tr)
		for _, id := range []string{"L07", "L11"} {
			if res := result(t, rep, id); res.Failed() {
				t.Errorf("%s asked for %s to be translated: %v", id, what, res.Findings)
			}
		}
	}
}

// A question and a quotation both hold a finished sentence, so the two tests
// that are not about sentences have to be the ones doing the work.
func TestAQuestionAndAQuotationAreDataForTheirOwnReasons(t *testing.T) {
	if !sentential(cloze) || !blanked(cloze) {
		t.Errorf("the question is sentential %v and blanked %v, want both true", sentential(cloze), blanked(cloze))
	}
	if !sentential(sample) || !quotation(sample) {
		t.Errorf("the quotation is sentential %v and quoted %v, want both true", sentential(sample), quotation(sample))
	}
}

// A paragraph of prose is not data, whatever else it holds. Without this the
// exemption above could be spelled "skip everything" and every test in it
// would still pass.
func TestAParagraphOfProseIsNotData(t *testing.T) {
	for _, block := range []string{
		"The bound is reached by the method of the second paper and the proof of it is the whole of this section.",
		"A shorter sentence. The paper then goes on to say something that runs past the length a paragraph needs.",
	} {
		if data(block) {
			t.Errorf("a paragraph of prose is read as data: %q", block)
		}
	}
	if res := result(t, pairOf(t, corpus.VI, englishBody, englishBody), "L07"); !res.Failed() {
		t.Error("L07 passed a section that is the English section")
	}
}

// address is the note a front page carries under the byline. The lead-in is
// translated and the addresses stand, which is correct and was reported.
const (
	addressEn = "The present addresses of the authors are: A. Adams, A Company, Inc., 100 Some Street, Some Town, CA 90000; B. Brown, Another Company, 200 Other Road, Other Town, CA 90001."
	addressVi = "Địa chỉ hiện tại của các tác giả: A. Adams, A Company, Inc., 100 Some Street, Some Town, CA 90000; B. Brown, Another Company, 200 Other Road, Other Town, CA 90001."
)

// notice is the first footnote of a paper that was read at a conference. The
// sentence about the conference is translated and the notice is not, because
// a translated rights notice no longer says what the rightsholder wrote.
const (
	noticeEn = "This is a revised version of a paper that was read at a conference in 1981 and printed in the proceedings of it at pages 100 to 110. Copyright 1981 by The International Society of Some Engineers of America, Inc."
	noticeVi = "Đây là bản sửa đổi của một bài báo được trình bày tại một hội nghị năm 1981 và in trong kỷ yếu của hội nghị đó ở các trang 100 đến 110. Copyright 1981 by The International Society of Some Engineers of America, Inc."
)

// speech is a paragraph about a passage a machine wrote, with the passage
// quoted inside it. The paper's own words move and the quotation does not.
const (
	speechEn = "The machine was asked to carry on from the opening and this is the whole of what came back. \"I do not know whether you can change your coat, but you can change your mind about it.\""
	speechVi = "Máy được yêu cầu viết tiếp từ phần mở đầu và đây là toàn bộ những gì nó trả về. \"I do not know whether you can change your coat, but you can change your mind about it.\""
)

func TestL11LeavesASentenceThatStandsOnItsOwnAccount(t *testing.T) {
	for _, c := range []struct {
		what   string
		en, vi string
	}{
		{"an address", addressEn, addressVi},
		{"a rights notice", noticeEn, noticeVi},
		{"a quotation", speechEn, speechVi},
	} {
		// The paragraph has to be prose, or L11 skips the whole block and
		// the sentence test is never reached.
		if data(c.en) {
			t.Fatalf("%s is read as data, so this test proves nothing", c.what)
		}
		if len(spared(c.en)) == 0 {
			t.Errorf("%s spares nothing", c.what)
		}
		en, tr := englishBody+"\n"+c.en+"\n", viBody+"\n"+c.vi+"\n"
		if res := result(t, pairOf(t, corpus.VI, en, tr), "L11"); res.Failed() {
			t.Errorf("L11 asked for %s to be translated: %v", c.what, res.Findings)
		}
	}
}

// The paper's own sentences are still the paper's own sentences when there is
// a quotation in the paragraph with them.
func TestL11StillFindsTheSentenceOutsideAQuotation(t *testing.T) {
	vi := "The machine was asked to carry on from the opening and this is the whole of what came back. \"Tôi không biết liệu bạn có thay được áo khoác hay không, nhưng bạn có thể thay đổi ý mình.\""
	en, tr := englishBody+"\n"+speechEn+"\n", viBody+"\n"+vi+"\n"
	res := result(t, pairOf(t, corpus.VI, en, tr), "L11")
	if !res.Failed() {
		t.Fatal("L11 passed a paragraph whose own sentence came back in English")
	}
	if !strings.Contains(res.Findings[0].Message, "the machine was asked") {
		t.Errorf("L11 reported the wrong sentence: %q", res.Findings[0].Message)
	}
}

func TestNamesOnlyIsAboutNamesAndNothingElse(t *testing.T) {
	for _, s := range []string{
		"Adams, A Company, Inc., 100 Some Street, Some Town, CA 90000",
		"Second International Conference On Some Subject, Paris, France, April 8-10, 1981",
	} {
		if !namesOnly(s) {
			t.Errorf("a name is not read as one: %q", s)
		}
	}
	for _, s := range []string{
		"The Second International Conference was held in Paris and the paper was read there.",
		"Adams and Brown showed that the bound is reached.",
	} {
		if namesOnly(s) {
			t.Errorf("a sentence is read as a name: %q", s)
		}
	}
}

func TestRightsIsAboutRightsNotices(t *testing.T) {
	for _, s := range []string{
		"Copyright 1981 by The International Society of Some Engineers of America, Inc.",
		"© 1981 by a publisher.",
		"All rights reserved.",
	} {
		if !rights(s) {
			t.Errorf("a rights notice is not read as one: %q", s)
		}
	}
	if rights("The paper says something about the rights of a reader to copy it.") {
		t.Error("a sentence about rights is read as a rights notice")
	}
}
