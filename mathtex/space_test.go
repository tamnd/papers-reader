package mathtex

import "testing"

func TestSqueezeSpaceTakesOutTheSpacingTeXIgnores(t *testing.T) {
	for _, c := range []struct{ name, span, want string }{
		// The three shapes the Vietnamese run of Theory of Sets came back with.
		{"a space before a macro", `M \cap N`, `M\cap N`},
		{"the same formula written without it", `M\cap N`, `M\cap N`},
		{"a display wrapped over lines", "A' \\times B'\n\\subset A \\times B", `A'\times B'\subset A\times B`},
		{"an aligned block re-indented", "\\begin{aligned}\n  x &= y \\\\\n  y &= z\n\\end{aligned}",
			`\begin{aligned}x&=y\\y&=z\end{aligned}`},

		// The space that has to stay, because it is what ends a control word.
		{"a macro and the letter after it", `\cap N`, `\cap N`},
		{"a run of them reads as one", "\\cap   \n  N", `\cap N`},

		// Nothing at the ends, where a space sets nothing at all.
		{"space at the front and the back", `  x + y  `, `x+y`},

		// Left alone.
		{"a span with no space in it", `x^{2}+1`, `x^{2}+1`},
		{"a thin space, which is a macro and not whitespace", `x \, y`, `x\,y`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := SqueezeSpace(c.span); got != c.want {
				t.Errorf("SqueezeSpace(%q) = %q, want %q", c.span, got, c.want)
			}
		})
	}
}

// Two variables written side by side are not one variable with a longer name,
// and the space between them is the only thing saying so.
func TestSqueezeSpaceKeepsTwoLettersApart(t *testing.T) {
	if SqueezeSpace(`a b`) == SqueezeSpace(`ab`) {
		t.Error("a b and ab came out the same")
	}
}
