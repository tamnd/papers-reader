package mathtex

import "testing"

func TestHashEscapesTheNumberSign(t *testing.T) {
	for _, c := range []struct {
		name, body, want string
		n                int
	}{
		// The two lines of the Codd page that sent this here, as they came
		// back from the reader.
		{"a column name in an inline span",
			`$\Delta_t(manager#) \subset \Delta_t(serial#)$`,
			`$\Delta_t(manager\#) \subset \Delta_t(serial\#)$`, 2},
		{"four of them in one span",
			`$P(s#, d#), \quad Q(s#, j#)$`,
			`$P(s\#, d\#), \quad Q(s\#, j\#)$`, 4},

		// A display is a span like any other, and a body can hold more than one.
		{"a display", "$$R(part#, supplier#)$$", `$$R(part\#, supplier\#)$$`, 2},
		{"two spans", `$a#$ and $b#$`, `$a\#$ and $b\#$`, 2},

		// A line break is two backslashes, so the sign after it is still bare.
		{"after a line break", `$$a \\ b#$$`, `$$a \\ b\#$$`, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, n := Hash(c.body)
			if got != c.want {
				t.Errorf("Hash(%q) = %q, want %q", c.body, got, c.want)
			}
			if n != c.n {
				t.Errorf("Hash(%q) counted %d, want %d", c.body, n, c.n)
			}
		})
	}
}

func TestHashLeavesEverythingElseAlone(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		// A number sign outside the mathematics is Markdown. All three of
		// these are on pages that also carry formulae.
		{"a heading", "# 3. Data Independence\n\nthe relation $R$ holds"},
		{"an attribute block", `Figure 2. The join of $R$ and $S$. {#codd-1970-relational-fig-2 .figure}`},
		{"a link to an anchor", `see [Section 1.3](01_model.md#normal-form) where $n$ is`},

		// A sign the answer already escaped needs nothing done to it, so a
		// second run over a repaired page is a no-op.
		{"a sign already escaped", `$\Delta_t(manager\#)$`},

		// The one place the sign is markup rather than a character.
		{"a macro definition", `$\def\R#1{\mathbf{R}^{#1}}$`},
		{"a new command", `$\newcommand{\proj}[1]{\pi_{#1}}$`},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, n := Hash(c.body)
			if got != c.body {
				t.Errorf("Hash(%q) = %q, want it unchanged", c.body, got)
			}
			if n != 0 {
				t.Errorf("Hash(%q) counted %d, want 0", c.body, n)
			}
		})
	}
}
