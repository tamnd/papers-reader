package extract

import "testing"

func TestAPriceDoesNotOpenAFormula(t *testing.T) {
	// Page 1 of the Unix paper, which was refused at three resolutions.
	in := "The hardware cost about $40,000, and less than two man years were\nspent on the system.\n"
	want := `The hardware cost about \$40,000, and less than two man years were` + "\nspent on the system.\n"
	if got := Money(in); got != want {
		t.Errorf("Money gave\n%q\nand the price is not a delimiter:\n%q", got, want)
	}
}

func TestAPageOfMathematicsIsNotTouched(t *testing.T) {
	for _, s := range []string{
		"The bound is $O(n \\log n)$ throughout.\n",
		"$$\nx = \\sum_{i=1}^{n} a_i\n$$\n",
		"Here $2^n$ is the number of subsets and $n$ the size.\n",
		"A run of $10$ trials cost $5$ hours.\n",
	} {
		if got := Money(s); got != s {
			t.Errorf("Money changed\n%q\ninto\n%q", s, got)
		}
	}
}

func TestAPriceBesideAFormulaLeavesTheFormula(t *testing.T) {
	in := "At $40,000 the machine runs $n$ jobs.\n"
	want := `At \$40,000 the machine runs $n$ jobs.` + "\n"
	if got := Money(in); got != want {
		t.Errorf("Money gave %q and wanted %q", got, want)
	}
}

func TestAFormulaLeftOpenIsStillLeftOpen(t *testing.T) {
	// Nothing on the line looks like a price, so this is a page that went
	// wrong and rule A2 has to see it.
	in := "The value of $x is not given here.\n"
	if got := Money(in); got != in {
		t.Errorf("Money repaired a page that should have been refused: %q", got)
	}
}

func TestAPriceInAListingIsPartOfTheListing(t *testing.T) {
	in := "```\necho $HOME\n```\n"
	if got := Money(in); got != in {
		t.Errorf("Money reached into a fenced block and gave %q", got)
	}
}

func TestAnEscapedPriceIsNotEscapedTwice(t *testing.T) {
	in := `The machine cost \$40,000 and the software $x$ nothing.` + "\n"
	if got := Money(in); got != in {
		t.Errorf("Money escaped an escape: %q", got)
	}
}

func TestTwoPricesOnALineAreBothPrices(t *testing.T) {
	// An even count, so the arithmetic says nothing. What says it is the
	// sentence the two of them would enclose.
	in := "It cost $40,000 to build and $5,000 a year to run.\n"
	want := `It cost \$40,000 to build and \$5,000 a year to run.` + "\n"
	if got := Money(in); got != want {
		t.Errorf("Money gave\n%q\nand wanted\n%q", got, want)
	}
}

func TestASpanOfMathematicsIsNotASentence(t *testing.T) {
	for _, s := range []string{
		"The bound $10 \\log n$ holds and $20 \\log n$ does not.\n",
		"Take $1 + x$ and $2 + y$ in turn.\n",
		"Write $3 \\mathrm{kg}$ for the mass.\n",
	} {
		if got := Money(s); got != s {
			t.Errorf("Money read mathematics as a sentence:\n%q\nbecame\n%q", s, got)
		}
	}
}

func TestAPriceSurvivesTheWholeTidier(t *testing.T) {
	in := "The hardware cost about $40,000 in all.\n"
	want := `The hardware cost about \$40,000 in all.`
	if got := Tidy(in); got != want {
		t.Errorf("Tidy gave %q and wanted %q", got, want)
	}
}
