package translate

import (
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

func TestAnAddressLinkedToItselfIsPutBack(t *testing.T) {
	// What all three translations of the GAN footnote did.
	source := "[^4]: All code and hyperparameters available at http://www.github.com/goodfeli/adversarial\n"
	answer := "[^4]: Toàn bộ mã nguồn có tại [http://www.github.com/goodfeli/adversarial](http://www.github.com/goodfeli/adversarial)\n"
	want := "[^4]: Toàn bộ mã nguồn có tại http://www.github.com/goodfeli/adversarial\n"
	if got := Unlink(source, answer); got != want {
		t.Errorf("Unlink gave\n%s\nand the address should have come back as it was printed:\n%s", got, want)
	}
}

func TestAnAnswerWithNothingWrappedComesBackUntouched(t *testing.T) {
	for _, s := range []string{
		"The rate is $10^{-4}$ throughout.\n",
		"See section [three](#three) for the proof.\n",
		"A footnote marker [^1] and a citation [[turing-1936-computable]].\n",
	} {
		if got := Unlink(s, s); got != s {
			t.Errorf("Unlink changed %q into %q and had no reason to", s, got)
		}
	}
}

func TestALinkThePassageAlreadyHadIsLeftAlone(t *testing.T) {
	// A source that carries the shape already is a source where the shape
	// is the paper's and not the model's.
	source := "Available at [http://x.test/a](http://x.test/a) in full.\n"
	answer := "Có sẵn đầy đủ tại [http://x.test/a](http://x.test/a).\n"
	if got := Unlink(source, answer); got != answer {
		t.Errorf("Unlink gave %q and the passage already read that way", got)
	}
}

func TestALinkThatSaysSomethingElseIsNotQuietlyRewritten(t *testing.T) {
	source := "The code is at http://x.test/a and the data is elsewhere.\n"
	answer := "Mã nguồn ở [đây](http://x.test/a).\n"
	if got := Unlink(source, answer); got != answer {
		t.Errorf("Unlink gave %q and should have left the decision to Verify", got)
	}
	if Verify(source, answer, corpus.VI) == nil {
		t.Error("Verify accepted a link the passage did not have")
	}
}

func TestALinkInAListingIsPartOfTheListing(t *testing.T) {
	source := "```\nfetch(\"[a](b)\")\n```\n"
	answer := source
	if got := Unlink(source, answer); got != answer {
		t.Errorf("Unlink reached into a fenced block and gave %q", got)
	}
	if len(Links(source)) != 0 {
		t.Errorf("Links found %v inside a listing", Links(source))
	}
}

func TestASecondLinkIsFoundInAPassageThatAlreadyHadOne(t *testing.T) {
	source := "Both [a](http://x.test/a) and plain http://x.test/b are given.\n"
	answer := "Cả [a](http://x.test/a) lẫn [b](http://x.test/b) đều được đưa ra.\n"
	if got := added(source, answer); got == "" {
		t.Error("the second link was not found because the first one covered for it")
	}
}

func TestAWrappedAddressIsAcceptedOnceItIsPutBack(t *testing.T) {
	source := "Code at http://x.test/a is provided.\n"
	answer := "Mã tại [http://x.test/a](http://x.test/a) được cung cấp.\n"
	if bad := Verify(source, Unlink(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// The two addresses that stopped a run, both of them three times.
func TestAnEscapedAddressIsPutBack(t *testing.T) {
	for _, c := range []struct{ why, source, answer, want string }{
		{
			"the AIMD paper's tilde, which is not Markdown in the first place",
			"A fuller account is at http://www.cse.wustl.edu/~jain and elsewhere.\n",
			"Trình bày đầy đủ hơn ở http://www.cse.wustl.edu/\\~jain và nơi khác.\n",
			"Trình bày đầy đủ hơn ở http://www.cse.wustl.edu/~jain và nơi khác.\n",
		},
		{
			"Milner's doi, whose parentheses the address pattern stops at",
			"See https://doi.org/10.1016/0022-0000(78)90014-4 for the proof.\n",
			"Xem https://doi.org/10.1016/0022-0000\\(78\\)90014-4 để biết chứng minh.\n",
			"Xem https://doi.org/10.1016/0022-0000(78)90014-4 để biết chứng minh.\n",
		},
	} {
		if got := Unescape(c.source, c.answer); got != c.want {
			t.Errorf("%s:\nUnescape gave %q\nand the address is written %q", c.why, got, c.want)
		}
	}
}

func TestARepairedAddressIsAccepted(t *testing.T) {
	source := "See https://doi.org/10.1016/0022-0000(78)90014-4 for the proof.\n"
	answer := "Xem https://doi.org/10.1016/0022-0000\\(78\\)90014-4 để biết chứng minh.\n"
	if bad := Verify(source, Unescape(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// A model that escapes its own escape writes two backslashes, and one
// round of unescaping leaves an address that is still not the one on the
// page. Both addresses below came back that way from a run in September.
func TestADoublyEscapedAddressIsPutBack(t *testing.T) {
	for _, c := range []struct {
		source, answer, want string
	}{
		{
			"See http://www.cse.wustl.edu/~jain for the note.\n",
			"Xem http://www.cse.wustl.edu/\\\\~jain de biet them.\n",
			"Xem http://www.cse.wustl.edu/~jain de biet them.\n",
		},
		{
			"See https://doi.org/10.1016/0022-0000(78)90014-4 for the proof.\n",
			"Xem https://doi.org/10.1016/0022-0000\\\\(78\\\\)90014-4 de biet chung minh.\n",
			"Xem https://doi.org/10.1016/0022-0000(78)90014-4 de biet chung minh.\n",
		},
	} {
		if got := Unescape(c.source, c.answer); got != c.want {
			t.Errorf("Unescape gave %q\nand the address is written %q", got, c.want)
		}
		if bad := Verify(c.source, Unescape(c.source, c.answer), corpus.VI); bad != nil {
			t.Errorf("a repaired answer was still refused: %s", bad[0])
		}
	}
}

func TestAnAddressTheSourceEscapedIsLeftAlone(t *testing.T) {
	// A paper that prints the backslash is a paper whose address has one in
	// it, and the answer is right to carry it.
	source := "The file is at http://x.test/a\\~b on the mirror.\n"
	answer := "Tệp nằm ở http://x.test/a\\~b trên bản sao.\n"
	if got := Unescape(source, answer); got != answer {
		t.Errorf("Unescape gave %q and the source is written that way too", got)
	}
}

func TestAnInventedAddressIsNotRepairedIntoAGoodOne(t *testing.T) {
	// Nothing unescapes to anything the source has, so there is nothing to
	// put back and Verify still gets to refuse it.
	source := "Code at http://x.test/a is provided.\n"
	answer := "Mã tại http://x.test/\\_elsewhere được cung cấp.\n"
	if got := Unescape(source, answer); got != answer {
		t.Errorf("Unescape gave %q and invented the repair", got)
	}
	if Verify(source, answer, corpus.VI) == nil {
		t.Error("Verify accepted an address the passage did not have")
	}
}

func TestAnEscapeOutsideAnAddressIsNotTouched(t *testing.T) {
	// A backslash in prose or in a formula is the author's, and a paper with
	// no address in it is not this repair's business at all.
	for _, s := range []string{
		"The cost is \\$5 per run.\n",
		"The set is $\\{x : x > 0\\}$ throughout.\n",
		"```\ncurl http://x.test/a\\?q=1\n```\n",
	} {
		if got := Unescape(s, s); got != s {
			t.Errorf("Unescape changed %q into %q and had no reason to", s, got)
		}
	}
}

// The subscript that stopped the BERT paper, three times on each of three
// passes of the run.
func TestAnEscapedFormulaIsPutBack(t *testing.T) {
	source := "The larger of the two is $\\mathrm{BERT}_{\\mathrm{BASE}}$ on every task.\n"
	answer := "Mô hình lớn hơn là $\\mathrm{BERT}\\_{\\mathrm{BASE}}$ trên mọi tác vụ.\n"
	want := "Mô hình lớn hơn là $\\mathrm{BERT}_{\\mathrm{BASE}}$ trên mọi tác vụ.\n"
	if got := UnescapeMath(source, answer); got != want {
		t.Errorf("UnescapeMath gave %q\nand the formula is written %q", got, want)
	}
	if bad := Verify(source, UnescapeMath(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// The other half of the tic: Markdown writes a literal backslash as two of
// them, and a model that is escaping its answer does it inside a formula
// too. In TeX a doubled backslash is a line break, so this one renders
// wrongly rather than merely comparing wrongly.
func TestADoubledBackslashIsPutBack(t *testing.T) {
	source := "We tried $s \\in \\{200, 400\\}$ and kept the larger.\n"
	answer := "Chung toi thu $s \\in \\\\{200, 400\\\\}$ va giu lai gia tri lon hon.\n"
	want := "Chung toi thu $s \\in \\{200, 400\\}$ va giu lai gia tri lon hon.\n"
	if got := UnescapeMath(source, answer); got != want {
		t.Errorf("UnescapeMath gave %q\nand the formula is written %q", got, want)
	}
	if bad := Verify(source, UnescapeMath(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// A formula with a real line break in it keeps the pair. Collapsing it gives
// something the source does not have, so the repair leaves it where it is.
func TestALineBreakInsideAFormulaIsLeftAlone(t *testing.T) {
	source := "The pair is $\\begin{cases} a \\\\ b \\end{cases}$ here.\n"
	answer := "Cap la $\\begin{cases} a \\\\ b \\end{cases}$ o day.\n"
	if got := UnescapeMath(source, answer); got != answer {
		t.Errorf("UnescapeMath gave %q and the source is written that way too", got)
	}
}

func TestAFormulaTheSourceEscapedIsLeftAlone(t *testing.T) {
	// A paper that prints a literal underscore in a formula is a paper
	// whose formula has a backslash in it, and the answer is right to
	// carry it.
	source := "The name is written $\\mathtt{a\\_b}$ in the table.\n"
	answer := "Tên được viết $\\mathtt{a\\_b}$ trong bảng.\n"
	if got := UnescapeMath(source, answer); got != answer {
		t.Errorf("UnescapeMath gave %q and the source is written that way too", got)
	}
}

func TestAnInventedFormulaIsNotRepairedIntoAGoodOne(t *testing.T) {
	source := "The larger of the two is $\\mathrm{BERT}_{\\mathrm{BASE}}$ on every task.\n"
	answer := "Mô hình lớn hơn là $\\mathrm{GPT}\\_{\\mathrm{LARGE}}$ trên mọi tác vụ.\n"
	if got := UnescapeMath(source, answer); got != answer {
		t.Errorf("UnescapeMath gave %q and invented the repair", got)
	}
	if Verify(source, answer, corpus.VI) == nil {
		t.Error("Verify accepted a formula the passage did not have")
	}
}

// The tic that held Shannon's appendix 6 back: a single letter formula
// reads as a word, so the answer gives the letter and drops the dollars.
func TestAOneLetterFormulaWrittenBareIsPutBack(t *testing.T) {
	source := "and similarly when $q$ is varied. Hence the conditions for a minimum are\n"
	answer := "và tương tự khi q được biến thiên. Do đó, các điều kiện cho một cực tiểu là\n"
	want := "và tương tự khi $q$ được biến thiên. Do đó, các điều kiện cho một cực tiểu là\n"
	if got := Redollar(source, answer); got != want {
		t.Errorf("Redollar gave %q\nand the formula is written %q", got, want)
	}
	if bad := Verify(source, Redollar(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// The arithmetic has to come out exactly. Two bare copies and only one
// formula short means one of the two is a word, and there is no telling
// which, so neither is touched.
func TestALetterTheAnswerAlsoUsesAsAWordIsLeftAlone(t *testing.T) {
	source := "The value $a$ is fixed.\n"
	answer := "Gia tri a la co dinh, a la mot chu.\n"
	if got := Redollar(source, answer); got != answer {
		t.Errorf("Redollar gave %q and there is no telling which a is the formula", got)
	}
}

func TestALetterInsideAWordIsNotWrapped(t *testing.T) {
	source := "The rate $q$ is fixed.\n"
	answer := "Tốc độ quy định là cố định.\n"
	if got := Redollar(source, answer); got != answer {
		t.Errorf("Redollar gave %q and the q is part of a word", got)
	}
}

func TestALetterInsideAListingIsNotWrapped(t *testing.T) {
	source := "The rate $q$ is fixed.\n\n```c\nint q;\n```\n"
	answer := "Tốc độ q là cố định.\n\n```c\nint q;\n```\n"
	want := "Tốc độ $q$ là cố định.\n\n```c\nint q;\n```\n"
	if got := Redollar(source, answer); got != want {
		t.Errorf("Redollar gave %q\nand the listing's q is not a formula", got)
	}
}

// A formula the answer dropped altogether is still refused. There is
// nothing to put the dollars around.
func TestAFormulaTheAnswerDroppedIsNotInvented(t *testing.T) {
	source := "The value $a$ is fixed.\n"
	answer := "Giá trị đó là cố định.\n"
	if got := Redollar(source, answer); got != answer {
		t.Errorf("Redollar gave %q and invented the formula", got)
	}
	if Verify(source, answer, corpus.VI) == nil {
		t.Error("Verify accepted an answer that dropped a formula")
	}
}

// Goldwasser, where the paragraph goes on calling the same object pair$_j$
// in prose four more times.
func TestANameThePaperPrintsIsPutBack(t *testing.T) {
	source := `Since $i'_j = 1$, $(v_j)^2 w^{-1} \mod x \in \text{pair}_j$ holds, and pair$_j$ is fixed.` + "\n"
	answer := `Vì $i'_j = 1$, $(v_j)^2 w^{-1} \mod x \in \text{cặp}_j$ đúng, và pair$_j$ là cố định.` + "\n"
	want := `Vì $i'_j = 1$, $(v_j)^2 w^{-1} \mod x \in \text{pair}_j$ đúng, và pair$_j$ là cố định.` + "\n"
	if got := Unrename(source, answer); got != want {
		t.Errorf("Unrename gave\n%q\nand the paper prints\n%q", got, want)
	}
	if bad := Verify(source, Unrename(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// Two names renamed in one paragraph and each of them could have been
// either, so neither is touched and the passage is refused.
func TestTwoRenamedNamesInOneParagraphAreLeftAlone(t *testing.T) {
	source := `The sets $\text{pair}_j$ and $\text{item}_j$ are built.` + "\n"
	answer := `Các tập $\text{cặp}_j$ và $\text{mục}_j$ được dựng.` + "\n"
	if got := Unrename(source, answer); got != answer {
		t.Errorf("Unrename gave %q and there is no telling which name is which", got)
	}
}

// A \text that says something about the formula is prose and is meant to be
// translated. Nothing here puts it back.
func TestATranslatedWordInsideAFormulaIsLeftTranslated(t *testing.T) {
	source := `The guard is $(\text{not } A) \text{ or } B$ throughout.` + "\n"
	answer := `Điều kiện là $(\text{không } A) \text{ hoặc } B$ xuyên suốt.` + "\n"
	if got := Unrename(source, answer); got != answer {
		t.Errorf("Unrename gave %q and the words in it are prose", got)
	}
}

// A formula that changed outside its \text is a changed formula, and the
// answer is refused rather than patched up.
func TestAFormulaThatChangedMoreThanItsNameIsNotPutBack(t *testing.T) {
	source := `The value $(v_j)^2 \in \text{pair}_j$ is fixed.` + "\n"
	answer := `Giá trị $(v_j)^3 \in \text{cặp}_j$ là cố định.` + "\n"
	if got := Unrename(source, answer); got != answer {
		t.Errorf("Unrename gave %q and the exponent moved", got)
	}
	if Verify(source, answer, corpus.VI) == nil {
		t.Error("Verify accepted an answer that changed a formula")
	}
}

// Repair is all five of them in one call, which is what the run makes.
func TestRepairPutsBackAnAddressAndAFormulaAtOnce(t *testing.T) {
	source := "See http://x.test/~a for $x_1$ and the rest.\n"
	answer := "Xem http://x.test/\\~a để biết $x\\_1$ và phần còn lại.\n"
	want := "Xem http://x.test/~a để biết $x_1$ và phần còn lại.\n"
	if got := Repair(source, answer); got != want {
		t.Errorf("Repair gave %q\nand the passage is written %q", got, want)
	}
}

// The AIMD front page: the address linked to itself with the label escaped,
// which is the linking tic and the escaping tic written on top of each
// other, and an author's email with the at sign escaped beside it.
func TestAnEscapedSelfLinkAndAnEscapedEmailArePutBack(t *testing.T) {
	source := "Write to jain@cse.wustl.edu, http://www.cse.wustl.edu/~jain for the note.\n"
	answer := "Gửi thư tới jain\\@cse.wustl.edu, [http://www.cse.wustl.edu/\\~jain](http://www.cse.wustl.edu/~jain) để biết thêm.\n"
	want := "Gửi thư tới jain@cse.wustl.edu, http://www.cse.wustl.edu/~jain để biết thêm.\n"
	if got := Repair(source, answer); got != want {
		t.Errorf("Repair gave\n%q\nand the passage is written\n%q", got, want)
	}
	if bad := Verify(source, Repair(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// Milner's doi, where the escapes are on the target rather than the label
// and the parentheses are inside the link. A pattern that stops at the
// first parenthesis sees half a link and repairs nothing, which is how the
// front of that paper failed four passes of the run.
func TestASelfLinkWhoseTargetIsEscapedIsPutBack(t *testing.T) {
	source := "The proof is at https://doi.org/10.1016/0022-0000(78)90014-4 in full.\n"
	answer := "Chứng minh đầy đủ ở [https://doi.org/10.1016/0022-0000(78)90014-4](https://doi.org/10.1016/0022-0000\\(78\\)90014-4).\n"
	want := "Chứng minh đầy đủ ở https://doi.org/10.1016/0022-0000(78)90014-4.\n"
	if got := Repair(source, answer); got != want {
		t.Errorf("Repair gave\n%q\nand the address is written\n%q", got, want)
	}
	if bad := Verify(source, Repair(source, answer), corpus.VI); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
	}
}

// A link that says something the target does not is prose the page did not
// have, and no amount of unescaping makes the two halves one address.
func TestALinkThatSaysSomethingElseIsLeftForVerify(t *testing.T) {
	source := "See http://x.test/a for the note.\n"
	answer := "Xem [trang chủ](http://x.test/a) để biết thêm.\n"
	if got := Unlink(source, answer); got != answer {
		t.Errorf("Unlink gave %q and should have left the link alone", got)
	}
	if bad := Verify(source, answer, corpus.VI); bad == nil {
		t.Error("an answer with an invented link in it was accepted")
	}
}

// An email is a protected span, so a model that translates the local part
// or the domain is refused rather than written into the corpus.
func TestAnEmailThatCameBackChangedIsRefused(t *testing.T) {
	source := "Write to jain@cse.wustl.edu for the note.\n"
	answer := "Gửi thư tới jain@cse.wustl.edu.vn để biết thêm.\n"
	if bad := Verify(source, answer, corpus.VI); bad == nil {
		t.Error("an answer that changed an email address was accepted")
	}
}
