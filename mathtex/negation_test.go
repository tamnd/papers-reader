package mathtex

import "testing"

func TestNegationPutsTheStrokeBackOnTheSign(t *testing.T) {
	for _, c := range []struct {
		name, body, want string
		n                int
	}{
		// The two shapes the text layer hands back, one with the stroke after
		// the sign and one with it before.
		{"a stroke after the sign", `if $0\in /S$ and`, `if $0\notin S$ and`, 1},
		{"a stroke before the sign", `pour $\lambda  /\in$ Sp`, `pour $\lambda  \notin$ Sp`, 1},

		// The two singletons, one of each of the other signs.
		{"an inclusion", `es that $\mathfrak{g}^{\omega}\subset /$ (ad`, `es that $\mathfrak{g}^{\omega}\not\subset$ (ad`, 1},
		{"a congruence", `i.e. if $n\equiv / p$ (mod. 3)`, `i.e. if $n\not\equiv p$ (mod. 3)`, 1},

		// The space the stroke was standing in for. \notinS is no macro, so the
		// letter has to be held off the control word, and nothing else does.
		{"a letter after the stroke", `$0\in /S$`, `$0\notin S$`, 1},
		{"a macro after the stroke", `$x\in /\mathbf{R}$`, `$x\notin\mathbf{R}$`, 1},
		{"a brace after the stroke", `$a\in /\{b, c\}$`, `$a\notin\{b, c\}$`, 1},

		// A display is a span like any other, and a body can hold more than one.
		{"a display", "$$x \\in / A \\quad y \\in / B$$", `$$x \notin A \quad y \notin B$$`, 2},
		{"two spans", `$a\in /X$ and $b\in /Y$`, `$a\notin X$ and $b\notin Y$`, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, n := Negation(c.body)
			if got != c.want {
				t.Errorf("Negation(%q) = %q, want %q", c.body, got, c.want)
			}
			if n != c.n {
				t.Errorf("Negation(%q) counted %d, want %d", c.body, n, c.n)
			}
		})
	}
}

func TestNegationLeavesEverythingElseAlone(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		// A solidus whose other operand is not a relation sign is a quotient,
		// and quotients are on every other page of the library.
		{"a quotient of an integer", `the group $\mathbf{Z}/n\mathbf{Z}$ is`},
		{"a quotient by a subgroup", `the canonical map $G \to G/H$`},
		{"a quotient and a relation in the same span", `if $x/y \in A$ then`},

		// \int begins with the letters of \in and is not the relation.
		{"an integral over a quotient", `$\int_{0}^{1} f(x)/g(x)\,dx$`},

		// The corpus writes soliduses in its own sentences.
		{"a solidus in prose", `and/or, see TS I/II, § 3`},
		{"prose beside a span", `and/or $\in$ is the sign`},

		// A page already repaired must not be repaired again.
		{"a sign that is already struck", `if $0\notin S$ and $A\not\subset B$`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, n := Negation(c.body)
			if got != c.body || n != 0 {
				t.Errorf("Negation(%q) = %q, %d, want it untouched", c.body, got, n)
			}
		})
	}
}

// The spans are cut in runes and the body is spliced back together in runes, so
// a sentence with an accent or a guillemet in it has to come out whole.
func TestNegationKeepsItsPlaceInASentenceThatIsNotASCII(t *testing.T) {
	body := `Soit «$\lambda  /\in$» le spectre de $u$, où $u$ est réel.`
	want := `Soit «$\lambda  \notin$» le spectre de $u$, où $u$ est réel.`
	if got, n := Negation(body); got != want || n != 1 {
		t.Errorf("Negation(%q) = %q, %d, want %q, 1", body, got, n, want)
	}
}
