package mathtex

import "testing"

func TestPrimeBracesTheBaseUnderAPower(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		n    int
	}{
		{
			"a letter",
			`the dual $E'^*$ of it`,
			`the dual ${E'}^*$ of it`,
			1,
		},
		{
			"a braced power",
			`$S'^{-1}$`,
			`${S'}^{-1}$`,
			1,
		},
		{
			"a control sequence as the base",
			`$\lambda'^\#$`,
			`${\lambda'}^\#$`,
			1,
		},
		{
			"a subscript between the prime and the power",
			`$w = \prod_{\beta=1}^p x'_\beta^{m'(\beta)}$`,
			`$w = \prod_{\beta=1}^p {x'_\beta}^{m'(\beta)}$`,
			1,
		},
		{
			"two primes",
			`$x''^2$`,
			`${x''}^2$`,
			1,
		},
		{
			"twice in one span",
			`$\sigma(E'^*, F'^*)$`,
			`$\sigma({E'}^*, {F'}^*)$`,
			2,
		},
		{
			"already braced is left alone",
			`$\{x'\}^*$ and ${\psi'_g}^{-1}$`,
			`$\{x'\}^*$ and ${\psi'_g}^{-1}$`,
			0,
		},
		{
			"a prime with no power is not a fault",
			`$f'(x)$ and $g' = h'$`,
			`$f'(x)$ and $g' = h'$`,
			0,
		},
		{
			"an apostrophe in prose is not mathematics",
			`l'application de l'anneau, qu'on note`,
			`l'application de l'anneau, qu'on note`,
			0,
		},
		{
			"a display span",
			"$$\nA'^n = B\n$$",
			"$$\n{A'}^n = B\n$$",
			1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, n := Prime(c.in)
			if got != c.want {
				t.Errorf("Prime(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
			if n != c.n {
				t.Errorf("Prime(%q) counted %d, want %d", c.in, n, c.n)
			}
		})
	}
}

func TestPrimeIsIdempotent(t *testing.T) {
	in := `$E'^* \to \sigma(F'^{-1}, x'_\beta^{m'(\beta)})$`
	once, n := Prime(in)
	if n == 0 {
		t.Fatalf("Prime(%q) changed nothing", in)
	}
	twice, again := Prime(once)
	if again != 0 || twice != once {
		t.Errorf("running twice changed %q into %q, %d more", once, twice, again)
	}
}
