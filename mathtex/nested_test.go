package mathtex

import "testing"

// TestADollarInsideATextIsNestedAndNotTheEndOfTheSpan is the entry that stopped
// the French Théorie des ensembles building. Its index of notation names two
// correspondances inside a \text, each in dollars of its own, and reading those
// dollars flat gave three spans with \Gamma and \Gamma' left as prose between
// them. The build wraps prose mathematics in dollars, so the entry reached
// tectonic as \text{$$\Gamma$$, $$\Gamma$'$ correspondances} and it refused the
// page with "! Missing $ inserted."
func TestADollarInsideATextIsNestedAndNotTheEndOfTheSpan(t *testing.T) {
	body := `$G' \circ G\ (\text{G graphe}), \Gamma' \circ \Gamma\ (\text{$\Gamma$, $\Gamma'$ correspondances}) : \text{II, p. 12}$`

	spans, unclosed := Split(body)
	if unclosed != nil {
		t.Fatalf("the entry came back unclosed: %q", unclosed.Text)
	}
	if len(spans) != 1 {
		t.Fatalf("the entry split into %d spans, want 1: %q", len(spans), texts(spans))
	}
	if want := body[1 : len(body)-1]; spans[0].Text != want {
		t.Errorf("the span is %q, want %q", spans[0].Text, want)
	}
}

// TestABraceLeftOpenInMathStillEndsTheSpanAtItsDollar is the check that the
// nesting rule stayed narrow. A dollar inside any open brace at all would mean
// that $\mathbf{Q$, which is a brace somebody left open and which M04 exists to
// report, ran to the end of the file as one unclosed span instead, and the rule
// would have nothing to report it against.
func TestABraceLeftOpenInMathStillEndsTheSpanAtItsDollar(t *testing.T) {
	spans, unclosed := Split(`the field $\mathbf{Q$ is prime, and $x$ is in it`)
	if unclosed != nil {
		t.Fatalf("the body came back unclosed: %q", unclosed.Text)
	}
	if len(spans) != 2 || spans[0].Text != `\mathbf{Q` || spans[1].Text != "x" {
		t.Errorf("spans are %q, want [%q %q]", texts(spans), `\mathbf{Q`, "x")
	}
}

// TestATextGroupThatClosesReleasesItsDollars is the same entry read past its
// end: the \text is over by the time the entry's own closing dollar arrives, so
// that dollar has to close the span the way it always did.
func TestATextGroupThatClosesReleasesItsDollars(t *testing.T) {
	spans, unclosed := Split(`$a\ (\text{$b$ noted}) : c$ and then $d$`)
	if unclosed != nil {
		t.Fatalf("the body came back unclosed: %q", unclosed.Text)
	}
	if len(spans) != 2 || spans[0].Text != `a\ (\text{$b$ noted}) : c` || spans[1].Text != "d" {
		t.Errorf("spans are %q", texts(spans))
	}
}

func texts(spans []Span) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Text
	}
	return out
}
