package translate

import "testing"

func TestRebraceWritesOutWhatTeXWouldInfer(t *testing.T) {
	cases := []struct{ in, want string }{
		{`$\frac12$`, `$\frac{1}{2}$`},
		{`$\frac{1}{2}$`, `$\frac{1}{2}$`},
		{`$x^2$`, `$x^{2}$`},
		{`$x_i^2$`, `$x_{i}^{2}$`},
		{`$\hat\theta$`, `$\hat{\theta}$`},
		{`$\frac \alpha 2$`, `$\frac{\alpha}{2}$`},
		{`$\sqrt[3]{x}$`, `$\sqrt[3]{x}$`},
		{`$\sqrt2$`, `$\sqrt{2}$`},
		{`$\frac{\frac12}{3}$`, `$\frac{\frac{1}{2}}{3}$`},
		// Nothing to infer, so nothing changes.
		{`$p_g$`, `$p_{g}$`},
		{`$\mathbb{E}_{x}[\log D(x)]$`, `$\mathbb{E}_{x}[\log D(x)]$`},
		{`$\left(\frac{a}{b}\right)$`, `$\left(\frac{a}{b}\right)$`},
		// An escaped brace is a character and not a group.
		{`$\{1,2\}$`, `$\{1,2\}$`},
		// A star belongs to the command and not to the argument.
		{`$\operatorname*{argmax}_x$`, `$\operatorname*{argmax}_{x}$`},
	}
	for _, c := range cases {
		if got := rebrace(c.in); got != c.want {
			t.Errorf("rebrace(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestRebraceIsIdempotent(t *testing.T) {
	for _, s := range []string{
		`$\frac12$`, `$x^2_i$`, `$\sqrt[3]{\frac ab}$`, `$\hat\theta$`,
	} {
		once := rebrace(s)
		if twice := rebrace(once); twice != once {
			t.Errorf("rebrace(rebrace(%s)) = %s, want %s", s, twice, once)
		}
	}
}

// A formula that was never finished is left as it was written. Both sides of
// the comparison get the same treatment, so a broken formula still compares
// equal to itself and unequal to a different one.
func TestRebraceLeavesAnUnfinishedFormulaAlone(t *testing.T) {
	for _, s := range []string{`$\frac{1$`, `$\frac$`, `$x^$`, `${$`} {
		if got := rebrace(s); got != s {
			t.Errorf("rebrace(%s) = %s, want it unchanged", s, got)
		}
	}
}

func TestTwoSpellingsOfOneFractionAreOneFormula(t *testing.T) {
	en := `The value is $\frac{1}{2}$ at the optimum.`
	vi := `Giá trị là $\frac12$ tại điểm tối ưu.`
	if d := Compare(en, vi); len(d) != 0 {
		t.Fatalf("Compare reported %v, want nothing", d)
	}
}

func TestARenamedVariableIsStillADifference(t *testing.T) {
	en := `The value is $\frac{1}{2}$ at the optimum.`
	vi := `Giá trị là $\frac{1}{3}$ tại điểm tối ưu.`
	if d := Compare(en, vi); len(d) != 1 {
		t.Fatalf("Compare reported %v, want one difference", d)
	}
}
