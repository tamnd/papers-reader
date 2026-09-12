package book

import (
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// set renders the fixture body the way Document does, and gives back both the
// LaTeX and the renderer, because half of what a renderer produces is the
// list of what it could not do.
func set(t *testing.T) (string, *Renderer) {
	t.Helper()
	b := testBook(t)
	r := &Renderer{Book: b}
	return r.LaTeX(b.Sections[0].Body), r
}

func TestTheMathematicsGoesThroughUntouched(t *testing.T) {
	tex, _ := set(t)

	if !strings.Contains(tex, `$n$`) || !strings.Contains(tex, `$w$`) {
		t.Error("an inline span did not survive")
	}
	// A display with a \tag is numbered, because the number is the paper's
	// and a reference in the prose says it.
	if !strings.Contains(tex, `\begin{equation}`) || !strings.Contains(tex, `a^n + b^n \ne c^n`) {
		t.Errorf("the display did not set as a numbered equation:\n%s", tex)
	}
	if !strings.Contains(tex, `\label{fermat-1637-margin-eq-1}`) {
		t.Error("the equation lost the anchor the prose refers to")
	}
	// The \tag stays. It is amsmath's way of printing a number that is not
	// the one LaTeX counted, and the number is the paper's.
	if !strings.Contains(tex, `\tag{1}`) {
		t.Error("the equation lost the number the paper printed")
	}
}

// Everything outside a protected span is escaped, and everything inside one
// is not. The order is the whole design of the renderer and this is the test
// of it.
func TestProseIsEscapedAndMathematicsIsNot(t *testing.T) {
	r := &Renderer{Book: testBook(t)}

	s := r.Inline(`50% of $50\%$ is 25% & rising_fast`)
	if !strings.Contains(s, `50\% of`) || !strings.Contains(s, `25\% \& rising\_fast`) {
		t.Errorf("the prose is not escaped: %s", s)
	}
	if !strings.Contains(s, `$50\%$`) {
		t.Errorf("the mathematics was escaped: %s", s)
	}
}

// LaTeX stops dead on a text mode macro in math mode and takes the whole book
// with it. The corpus has one and it is correct: the reader wrote $\LaTeX$
// because the logo is not a word.
func TestATextModeMacroInAMathSpanIsWrapped(t *testing.T) {
	r := &Renderer{Book: testBook(t)}

	s := r.Inline(`typeset with $\LaTeX$ by hand`)
	if !strings.Contains(s, `$\text{\LaTeX}$`) {
		t.Errorf("the macro was left in math mode, which is the error that lost the first build: %s", s)
	}
}

func TestACitationLinksToTheEntryItNames(t *testing.T) {
	tex, r := set(t)

	if !strings.Contains(tex, `[\hyperlink{bib-1}{1}]`) {
		t.Errorf("the numeric citation is not a link:\n%s", tex)
	}
	// A citation of another paper of the corpus prints the number this
	// paper's own bibliography gave it, because a reader of a book has no
	// use for a corpus identifier in the middle of a sentence.
	if !strings.Contains(tex, `[\hyperlink{bib-2}{2}]`) {
		t.Errorf("the corpus citation did not become the number the paper printed:\n%s", tex)
	}
	if strings.Contains(tex, "euler-1770-algebra") {
		t.Error("the corpus identifier is in the prose")
	}
	if len(r.Unlinked) != 0 {
		t.Errorf("the renderer says %v is unlinked and the bibliography resolves it", r.Unlinked)
	}
}

func TestACorpusCitationWithNoEntryIsReported(t *testing.T) {
	b := testBook(t)
	r := &Renderer{Book: b}

	s := r.Inline("as [[knuth-1968-art]] has it")
	if !strings.Contains(s, `\textsc{knuth-1968-art}`) {
		t.Errorf("an unresolved citation was dropped rather than shown: %s", s)
	}
	if len(r.Unlinked) != 1 || r.Unlinked[0] != "knuth-1968-art" {
		t.Errorf("the renderer says %v and one citation has no entry", r.Unlinked)
	}
}

func TestAFigureIsSetWithThePaperSNumber(t *testing.T) {
	tex, r := set(t)

	if len(r.Missing) != 0 {
		t.Errorf("the renderer could not find %v and the manifest has the figure", r.Missing)
	}
	if !strings.Contains(tex, `\setcounter{figure}{0}`) {
		t.Error("the figure counter was not set, so LaTeX will number the figure itself")
	}
	if !strings.Contains(tex, `\includegraphics`) || !strings.Contains(tex, "f01.png") {
		t.Errorf("the picture is not in the figure:\n%s", tex)
	}
	if !strings.Contains(tex, `\caption{the margin, to scale.}`) {
		t.Errorf("the caption kept the word and the number LaTeX is about to print again:\n%s", tex)
	}
	if !strings.Contains(tex, `\label{fermat-1637-margin-fig-1}`) {
		t.Error("the figure lost its anchor")
	}
}

// The caption says what the word in front of a figure number is, in whatever
// language the book is in. A wordlist would be a second opinion about
// something the corpus already knows.
func TestTheFigureNameIsReadOffTheCaption(t *testing.T) {
	if got := testBook(t).FigureName(); got != "Figure" {
		t.Errorf("the figure name is %q", got)
	}
}

func TestAnAttributeBlockNeverReachesThePage(t *testing.T) {
	tex, _ := set(t)

	for _, s := range []string{"tag=00B1", "tag=00C1", "tag=00D1", `\{\#`} {
		if strings.Contains(tex, s) {
			t.Errorf("%q is in the prose:\n%s", s, tex)
		}
	}
}

func TestAHeadingIsNumberedByThePaperAndNotByLaTeX(t *testing.T) {
	tex, _ := set(t)

	if !strings.Contains(tex, `\subsection*{1.1 The Narrow Case}`) {
		t.Errorf("the subsection did not keep the number the paper printed:\n%s", tex)
	}
	if !strings.Contains(tex, `\addcontentsline{toc}{subsection}{\texorpdfstring{`) {
		t.Error("the heading is starred and not in the contents")
	}
}

// hyperref writes the contents line into the PDF outline, which holds no TeX
// at all. The first build of this warned eight times and printed \hskip in
// the outline.
func TestABookmarkIsPlainText(t *testing.T) {
	if got := bookmark(`4.1 Optimality of $p_g = p_{\text{data}}$`); got != "4.1 Optimality of" {
		t.Errorf("the bookmark is %q", got)
	}
	if got := bookmark("**Theorem** 1"); got != "Theorem 1" {
		t.Errorf("the bookmark kept the markup: %q", got)
	}
	// A heading that is nothing but a formula would come back empty, and an
	// empty outline entry is worse than an approximate one.
	if got := bookmark("$p_g$"); got != "p_g" {
		t.Errorf("the bookmark is %q and the heading was all mathematics", got)
	}
}

func TestAListAndATableAndAListingAreSet(t *testing.T) {
	tex, r := set(t)

	if !strings.Contains(tex, `\begin{itemize}`) || strings.Count(tex, `\item `) != 2 {
		t.Errorf("the list did not set:\n%s", tex)
	}
	if !strings.Contains(tex, `\begin{tabular}`) || !strings.Contains(tex, `\toprule`) {
		t.Errorf("the table did not set:\n%s", tex)
	}
	if !strings.Contains(tex, `\begin{Verbatim}`) || !strings.Contains(tex, "def margin(n):") {
		t.Errorf("the listing did not set:\n%s", tex)
	}
	if len(r.Wide) != 0 {
		t.Errorf("a table of two columns was called wide: %v", r.Wide)
	}
}

// Up to three columns a table is set on the natural width of its cells. Above
// that the cells are paragraphs of an equal share of the page, because five
// columns of prose set on natural widths came out eleven hundred points wider
// than the page.
func TestAWideTableIsSetInColumnsThatWrap(t *testing.T) {
	if got := columns(3); got != "lll" {
		t.Errorf("a narrow table is set with %q", got)
	}
	got := columns(5)
	if strings.Count(got, `p{`) != 5 || !strings.Contains(got, `\raggedright`) {
		t.Errorf("a wide table is set with %q", got)
	}
}

func TestAFootnoteIsSetWhereItsMarkerIs(t *testing.T) {
	tex, r := set(t)

	if !strings.Contains(tex, `\footnote{The width was measured`) {
		t.Errorf("the note did not set at the marker:\n%s", tex)
	}
	// The corpus has "difficulties. [^4]" with a space in front of the
	// marker, which sets as ". \footnote{" and puts a space before the
	// superscript.
	if strings.Contains(tex, ` \footnote{`) {
		t.Error("a space was left in front of the note")
	}
	if r.Notes != 1 || len(r.Orphans) != 0 {
		t.Errorf("the renderer set %d notes and orphaned %v", r.Notes, r.Orphans)
	}
}

func TestAMarkerWithNoDefinitionIsShownAndReported(t *testing.T) {
	r := &Renderer{Book: testBook(t)}

	s := r.Inline("as shown[^99]")
	if !strings.Contains(s, "99") {
		t.Errorf("the orphan marker was dropped: %s", s)
	}
	if len(r.Orphans) != 1 {
		t.Errorf("the renderer orphaned %v", r.Orphans)
	}
}

// A book in a language with no CJK font would set every character as a blank
// space, which is the one failure that looks like success.
func TestAChineseBookWithNoFontIsRefused(t *testing.T) {
	b := testBook(t)
	b.Lang = corpus.ZH

	if _, _, err := Document(b, Options{}); err == nil {
		t.Error("Document set a Chinese book with no font")
	}
}

func TestTheDocumentIsOneWholeFile(t *testing.T) {
	b := testBook(t)
	tex, _, err := Document(b, Options{Contents: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`\documentclass`,
		`\begin{document}`,
		`\end{document}`,
		`\begin{abstract}`,
		`\section*{1\quad The Margin}`,
		`\bibentry{1}`,
		`\bibentry{2}`,
		"A Remark in the Margin",
	} {
		if !strings.Contains(tex, want) {
			t.Errorf("the document has no %q", want)
		}
	}
	// A paper of one section is shorter than the shortest book worth a table
	// of contents.
	if strings.Contains(tex, `\tableofcontents`) {
		t.Error("a paper of one section was given a table of contents")
	}
}

func TestTheColophonSaysHowTheBookWasMade(t *testing.T) {
	b := testBook(t)
	lines := strings.Join(b.Colophon(), " ")

	if !strings.Contains(lines, "arxiv:1637.0001") {
		t.Errorf("the colophon does not say where the paper came from: %s", lines)
	}
	for _, secret := range []string{"/Users/", "/home/", "server1", "server2", "server3"} {
		if strings.Contains(lines, secret) {
			t.Errorf("the colophon names %q, and a book is published", secret)
		}
	}
}
