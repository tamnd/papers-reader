package mathtex

import "testing"

// The page prints a 2 by 2 and the text layer hands back the top row raised and
// the bottom row lowered, which is a matrix and reads as a pair of scripts.
func TestAMatrixTheLayerFlattenedIsCounted(t *testing.T) {
	for _, body := range []string{
		`une matrice X $=(_{C D}^{A B})$, avec $A\in \mathbf{M}_p(A_0)$`,
		`l'image de $(^{1 0}_{0 0})$ dans $A/\mathfrak{m}$`,
		`Soit A $= (^{a b}_{c d})$ un élément de`,
	} {
		if StackedMatrices(body) == 0 {
			t.Errorf("no matrix found in %q", body)
		}
	}
}

// A script with one symbol in it is a script. A script with a command in it is
// a subscript that carries a space, which every family indexed by a set has.
func TestAScriptIsNotAMatrix(t *testing.T) {
	for _, body := range []string{
		`$A^p_d\oplus A_d(1)^q$`,
		`une famille $(M_i)_{i\in I}$ de sous-modules`,
		`$\sum_{x\in E}Ax$`,
		`$x^2_i$ and $\mathbf{M}_{p,q}(A_1)$`,
		`$M_{\mathscr{B}}(u)$`,
	} {
		if n := StackedMatrices(body); n != 0 {
			t.Errorf("%d matrices found in %q, want none", n, body)
		}
	}
}

// A lone script with a space in it is a different fault. M_{I J} is the set
// difference M_{I \ J} with the backslash dropped by the layer, 21 times in
// chapter VIII, and counting it here would file it under the wrong heading.
func TestALoneScriptWithASpaceIsNotAMatrix(t *testing.T) {
	for _, body := range []string{
		`a submodule supplementary to $M_{I J}$`,
		`$M_{I J'}= M_{I J}\oplus M_i$`,
	} {
		if n := StackedMatrices(body); n != 0 {
			t.Errorf("%d matrices found in %q, want none", n, body)
		}
	}
}

// The two orders are the same matrix: which script the layer reports first
// depends on the run order in the page and not on the page.
func TestEitherOrderIsTheSameMatrix(t *testing.T) {
	if StackedMatrices(`$(^{A B}_{C D})$`) != 1 || StackedMatrices(`$(_{C D}^{A B})$`) != 1 {
		t.Error("the two orders did not both count as one matrix")
	}
}
