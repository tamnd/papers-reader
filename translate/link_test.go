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
