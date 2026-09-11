package katex

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
)

// The point of vendoring is that the bytes cannot change without somebody
// saying so. SHA256SUMS is what says so, and a sum nobody checks is a comment.
func TestTheVendoredBytesAreTheOnesRecorded(t *testing.T) {
	sums, err := os.Open("SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	defer sums.Close()

	read := func(name string) ([]byte, error) {
		if name == "katex.min.js" {
			return []byte(script), nil
		}
		return fs.ReadFile(Assets(), name)
	}

	n := 0
	sc := bufio.NewScanner(sums)
	for sc.Scan() {
		want, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			t.Fatalf("SHA256SUMS line is not a sum and a name: %q", sc.Text())
		}
		b, err := read(name)
		if err != nil {
			t.Errorf("%s is recorded and not embedded: %v", name, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s\n got %s\nwant %s", name, got, want)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	// One script, one stylesheet and the fonts. A file embedded and not
	// recorded is the case this catches, and it is the case that matters: it is
	// how an unchecked byte gets in.
	var embedded int
	err = fs.WalkDir(Assets(), ".", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			embedded++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != embedded+1 {
		t.Errorf("SHA256SUMS records %d files, %d are embedded", n, embedded+1)
	}
}

// The one thing this package is for.
func TestTheMathematicsOfThesePapersRenders(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// One from each corner of the hundred: scaled dot-product attention, an
	// asymptotic bound, a probability with a conditioning bar, an expectation
	// over a distribution, and a piecewise definition.
	for _, tex := range []string{
		`\mathrm{softmax}\left(\frac{QK^\top}{\sqrt{d_k}}\right)V`,
		`O(n \log n)`,
		`P(w_t \mid w_{1:t-1})`,
		`\mathbb{E}_{x \sim p}\left[\log q(x)\right]`,
		`\begin{cases} 1 & i = j \\ 0 & i \neq j \end{cases}`,
	} {
		got, err := r.Render(tex, false)
		if err != nil {
			t.Errorf("%s: %v", tex, err)
			continue
		}
		if !strings.HasPrefix(got, `<span class="katex">`) {
			t.Errorf("%s rendered as %.60s", tex, got)
		}
	}
}

// Display and inline are different markup, and the cache is keyed on both, so a
// span written first one way must not come back the other.
func TestDisplayIsNotInline(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	in, err := r.Render(`x = y`, false)
	if err != nil {
		t.Fatal(err)
	}
	disp, err := r.Render(`x = y`, true)
	if err != nil {
		t.Fatal(err)
	}
	if in == disp {
		t.Fatal("display mode changed nothing")
	}
	if !strings.Contains(disp, "katex-display") {
		t.Errorf("a display is not marked as one: %.80s", disp)
	}
	again, err := r.Render(`x = y`, false)
	if err != nil {
		t.Fatal(err)
	}
	if again != in {
		t.Error("the cache gave back the display for the inline")
	}
}

// A span KaTeX will not read is a fault in the extraction and has to arrive as
// an error. The alternative is red TeX on the page, which is the failure that
// gets shipped because nothing stops it.
func TestARefusedSpanIsAnError(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Render(`\frac{1`, false)
	if err == nil {
		t.Fatalf("a broken formula rendered: %s", got)
	}
	if !strings.Contains(err.Error(), "Unexpected end of input") {
		t.Errorf("the message does not say what is wrong: %v", err)
	}
	// KaTeX's own message names the offending input. The engine's position in
	// the evaluated script does not, and would be the only thing on the line
	// that means nothing to a reader of the corpus.
	if strings.Contains(err.Error(), "<eval>") {
		t.Errorf("the engine's position leaked into the message: %v", err)
	}
}

// emit renders every span of every section in four languages, and doing that
// on one goroutine is a choice the toolchain should be free to change without
// the renderer becoming the reason it cannot.
func TestConcurrentUse(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = r.Render(`\sum_{n=1}^{\infty} a_n`, i%2 == 0)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// The stylesheet asks for these by name and a font that is missing is a page of
// fallback glyphs at the wrong size, which looks like a rendering bug and is a
// packaging one.
func TestTheFontsTheStylesheetNeedsAreThere(t *testing.T) {
	css, err := fs.ReadFile(Assets(), "katex.min.css")
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := fs.ReadDir(Assets(), "fonts")
	if err != nil {
		t.Fatal(err)
	}
	if len(fonts) == 0 {
		t.Fatal("no fonts embedded")
	}
	for _, f := range fonts {
		if !strings.Contains(string(css), "fonts/"+f.Name()) {
			t.Errorf("%s is embedded and the stylesheet never asks for it", f.Name())
		}
	}
	// Every url() in the stylesheet is one of them. This is the direction that
	// leaves a reader looking at Times New Roman.
	for _, part := range strings.Split(string(css), "url(fonts/")[1:] {
		name := part[:strings.IndexAny(part, ")")]
		if !strings.HasSuffix(name, ".woff2") {
			continue
		}
		if _, err := fs.Stat(Assets(), "fonts/"+name); err != nil {
			t.Errorf("the stylesheet asks for %s: %v", name, err)
		}
	}
}
