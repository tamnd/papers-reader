package mathtex

import (
	"reflect"
	"testing"
)

// Every case here was run through KaTeX before it was written down, and the
// want is what KaTeX does with it: refused for a double script, or set.
func TestDoubleScripts(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		// The three faults the corpus carries, as the corpus writes them.
		{`\theta^-_E^1`, []string{`^-_E^1`}},
		{`\begin{pmatrix}^X_0^0_I\end{pmatrix}`, []string{`^X_0^0_I`}},
		{`\sum^p_{i=0}^{-1}`, []string{`^p_{i=0}^{-1}`}},
		{`\Gamma '_1^{\pi'_1}`, []string{`'_1^{\pi'_1}`}},

		// A base with one of each is what a formula looks like.
		{`x^2_i`, nil},
		{`\sum_{i=1}^n a_i`, nil},
		{`M_{I-J}`, nil},

		// Braces end a chain in TeX and they end one here, which is how
		// somebody writes a second superscript when they mean it.
		{`{x^a}^b`, nil},
		{`x^{a^b}`, nil},
		{`x^{a_b^c}`, nil},
		{`x^{a_b_c}`, []string{`_b_c`}},

		// A prime is the superscript, so what may follow it is a subscript and
		// nothing else. TeX gathers a superscript written straight after one
		// into the same superscript, and a space undoes that.
		{`f'_1`, nil},
		{`f''^2`, nil},
		{`f'^2_a`, nil},
		{`f' ^2`, []string{`' ^2`}},
		{`x^a'`, []string{`^a'`}},
		{`f_g'_{h}`, []string{`_g'_{h}`}},

		// \^ is an accent and \_ is an underscore, and neither is a mark.
		{`\hat^a_b`, nil},
		{`a\_b\^c`, nil},

		// Two bases, one chain each.
		{`x^a_b y^c_d`, nil},

		// More than one on a line, and more than two marks in a chain.
		{`^-_V^1 \circ ^-_W^1`, []string{`^-_V^1`, `^-_W^1`}},
		{`^0_0^{\leqslant}_{\leqslant}^j_j`, []string{`^0_0^{\leqslant}_{\leqslant}^j_j`}},
	} {
		if got := DoubleScripts(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("DoubleScripts(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A brace that never closes is M04's finding. Reading it as reaching to the end
// is what keeps this rule from reporting the same span for the second time and
// from running off the end of it.
func TestDoubleScriptsUnclosedBrace(t *testing.T) {
	if got := DoubleScripts(`x^{a_b_c`); !reflect.DeepEqual(got, []string{`_b_c`}) {
		t.Errorf("got %q, want the inner chain", got)
	}
	if got := DoubleScripts(`x^{a`); got != nil {
		t.Errorf("got %q, want nothing", got)
	}
}
