package emit

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/katex"
)

func TestTheAllowlistTakesTheMarkupTheEmitterWrites(t *testing.T) {
	for _, s := range []string{
		`<p>Some prose.</p>`,
		`A paragraph with <em>emphasis</em> and <strong>weight</strong>.`,
		`Code in a <code>face</code>.`,
		`<ul><li>one</li><li>two</li></ul>`,
		`<table><thead><tr><th>n</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table>`,
		`<sup><a class="noteref" href="#fn-2">2</a></sup>`,
		`<a class="paper" href="/p/rumelhart-1986-backprop">rumelhart-1986-backprop</a>`,
		`See <a href="https://example.org/paper">the paper</a>.`,
	} {
		if bad := Check(s); len(bad) != 0 {
			t.Errorf("%q was refused: %v", s, bad)
		}
	}
}

func TestTheAllowlistRefusesWhatItIsFor(t *testing.T) {
	for _, c := range []struct{ html, want string }{
		{`<script>alert(1)</script>`, "allowlist"},
		{`<p>fine</p><iframe src="/"></iframe>`, "allowlist"},
		{`<img src="x.png">`, "allowlist"},
		{`<p onclick="steal()">prose</p>`, "event handler"},
		{`<a href="javascript:alert(1)">click</a>`, "will not follow"},
		{`<a href="data:text/html,x">click</a>`, "will not follow"},
	} {
		bad := Check(c.html)
		if len(bad) == 0 {
			t.Errorf("%q was allowed", c.html)
			continue
		}
		if !strings.Contains(strings.Join(bad, " "), c.want) {
			t.Errorf("%q was refused as %v, wanted something about %q", c.html, bad, c.want)
		}
	}
}

// KaTeX writes its own markup, including MathML for the screen readers, and
// listing thirty MathML elements here would be this package claiming to
// know what a future version of KaTeX emits. What makes the exemption safe
// is on the other side, so the thing to check is that it is real output of
// the real renderer that gets through it.
func TestWhatKaTeXWritesIsNotHeldToTheAllowlist(t *testing.T) {
	k, err := katex.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, display := range []bool{false, true} {
		out, err := k.Render(`\frac{\partial E}{\partial w_{ji}} = \sum_j \delta_j x_i`, display)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "<math") && !strings.Contains(out, "mathml") {
			t.Skip("this build of KaTeX writes no MathML, so there is nothing to exempt")
		}
		if bad := Check("<p>The rule is " + out + " and that is all.</p>"); len(bad) != 0 {
			t.Errorf("KaTeX output with display=%v was refused: %v", display, bad)
		}
	}
}

// The exemption is pinned to the two class names KaTeX opens with, so a
// span that merely has the word somewhere in a class cannot open one.
func TestOnlyARealKaTeXRootOpensTheExemption(t *testing.T) {
	if bad := Check(`<span class="not-katex"><script>x</script></span>`); len(bad) == 0 {
		t.Error("a span pretending to be KaTeX opened the exemption")
	}
	if bad := Check(`<span class="katex">`); len(bad) == 0 {
		t.Error("an unclosed KaTeX span was not reported")
	}
}

// The check runs over a whole page, and it names the block it found the
// trouble in, because a finding that said only which paper is a finding
// somebody has to go looking with.
func TestCheckPageNamesTheBlock(t *testing.T) {
	p := &Page{
		ID: "rumelhart-1986-backprop", Lang: corpus.EN,
		Sections: []Section{{
			Anchor: "rumelhart-1986-backprop-s1",
			Blocks: []Block{{Kind: "p", I: 0, HTML: "<p>fine</p>"}, {Kind: "p", I: 1, HTML: "<blink>no</blink>"}},
		}},
	}
	bad := CheckPage(p)
	if len(bad) != 1 {
		t.Fatalf("the faults are %+v", bad)
	}
	if bad[0].Kind != FaultMarkup || bad[0].Page != "p/rumelhart-1986-backprop/en.json" {
		t.Errorf("the fault is %+v", bad[0])
	}
	if !strings.Contains(bad[0].What, "rumelhart-1986-backprop-s1 block 1") {
		t.Errorf("the fault does not say where: %q", bad[0].What)
	}
}

// Everything the renderer writes has to be on the list, so the list and the
// renderer cannot drift apart without a build of the whole corpus saying so.
// This is the same check as rule P01, run here on a corpus small enough to
// be a unit test.
func TestABuildOfTheCorpusWritesNothingOffTheAllowlist(t *testing.T) {
	site, err := Build(whole(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range site.Faults {
		if f.Kind == FaultMarkup {
			t.Errorf("%s: %s", f.Page, f.What)
		}
	}
}

func TestTheAllowlistIsWhatItSaysItIs(t *testing.T) {
	if got := len(Allowed()); got != 17 {
		t.Errorf("%d elements on the allowlist, and the README says seventeen", got)
	}
	for _, tag := range []string{"div", "img", "script", "style", "iframe"} {
		if allowed[tag] {
			t.Errorf("<%s> is on the allowlist", tag)
		}
	}
}
