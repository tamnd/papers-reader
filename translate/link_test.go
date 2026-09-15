package translate

import "testing"

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
	if Verify(source, answer) == nil {
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
	if bad := Verify(source, Unlink(source, answer)); bad != nil {
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
	if bad := Verify(source, Unescape(source, answer)); bad != nil {
		t.Errorf("a repaired answer was still refused: %s", bad[0])
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
	if Verify(source, answer) == nil {
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
	if bad := Verify(source, UnescapeMath(source, answer)); bad != nil {
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
	if bad := Verify(source, UnescapeMath(source, answer)); bad != nil {
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
	if Verify(source, answer) == nil {
		t.Error("Verify accepted a formula the passage did not have")
	}
}

// Repair is the three of them in one call, which is what the run makes.
func TestRepairPutsBackAnAddressAndAFormulaAtOnce(t *testing.T) {
	source := "See http://x.test/~a for $x_1$ and the rest.\n"
	answer := "Xem http://x.test/\\~a để biết $x\\_1$ và phần còn lại.\n"
	want := "Xem http://x.test/~a để biết $x_1$ và phần còn lại.\n"
	if got := Repair(source, answer); got != want {
		t.Errorf("Repair gave %q\nand the passage is written %q", got, want)
	}
}
