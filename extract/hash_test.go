package extract

import "testing"

func TestUnhashEscapesTheNumberSignInsideMathematics(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		// The line of the Codd page that sent this here.
		{"a column name",
			`so that $\Delta_t(manager#) \subset \Delta_t(serial#)$ holds.`,
			`so that $\Delta_t(manager\#) \subset \Delta_t(serial\#)$ holds.`},

		// A display is three lines and has to be seen as one span.
		{"a display over three lines",
			"$$\nR(part#, supplier#)\n$$",
			"$$\nR(part\\#, supplier\\#)\n$$"},

		// The page around the mathematics is Markdown and keeps its own
		// number signs.
		{"a heading above a formula",
			"# 3. Normal Form\n\nwhere $t(part#)$ is the column.",
			"# 3. Normal Form\n\nwhere $t(part\\#)$ is the column."},
		{"an attribute block after a caption",
			`Figure 2. The projection $\pi(job#)$. {#codd-1970-relational-fig-2 .figure}`,
			`Figure 2. The projection $\pi(job\#)$. {#codd-1970-relational-fig-2 .figure}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Unhash(c.in); got != c.want {
				t.Errorf("Unhash(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestUnhashLeavesAListingAlone(t *testing.T) {
	// A number sign in program text is a comment marker or a directive, and a
	// dollar sign in it is a shell prompt rather than a delimiter.
	in := "```\n#include <stdio.h>\nprintf(\"$%d#\\n\", n);\n```\n\nand $f(part#)$ after it."
	want := "```\n#include <stdio.h>\nprintf(\"$%d#\\n\", n);\n```\n\nand $f(part\\#)$ after it."
	if got := Unhash(in); got != want {
		t.Errorf("Unhash(%q) = %q, want %q", in, got, want)
	}
}

func TestTidyEscapesTheNumberSign(t *testing.T) {
	in := `The pair $\Delta_t(manager#) \subset \Delta_t(serial#)$ holds for every t.`
	want := `The pair $\Delta_t(manager\#) \subset \Delta_t(serial\#)$ holds for every t.`
	if got := Tidy(in); got != want {
		t.Errorf("Tidy(%q) = %q, want %q", in, got, want)
	}
}
