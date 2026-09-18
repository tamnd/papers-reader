package code

import (
	"regexp"
	"strings"
)

// statement matches a line that English prose does not write and a program
// does. Every one of these was picked for being unambiguous rather than for
// being common: a rule that guessed from indentation would report every
// quotation in the corpus.
var statement = regexp.MustCompile(`^\s*(?:` +
	// A brace on a line of its own, which is C, Java, Go and every
	// descendant of them, and is not a sentence.
	`[{}]\s*;?` +
	// A preprocessor directive. The hash is not a heading because a heading
	// has a space after it.
	`|#(?:include|define|ifdef|ifndef|endif|pragma)\b` +
	// A comment. Two slashes at the head of a line is a comment in every
	// language the corpus prints and is nothing at all in English, and the
	// P4 paper needs it: its table declarations are a name, a brace, three
	// lines of comment and a brace, and nothing else in them says program.
	`|//` +
	// A declaration or a keyword at the head of a line, in the languages the
	// papers in this corpus print. These are ALGOL, which is most of what the
	// older papers print and reads as ordinary prose in lower case, so they
	// are matched in capitals only. A listing printed with one keyword to a
	// line has very little else on it: the second half of McCabe's SEARCH is
	// twelve lines, of which four are a bare THEN or ELSE, and without those
	// four it was under the half the assembler wants before it will fence a
	// paragraph. IF, FOR and WHILE are deliberately not here, because in
	// capitals at the head of a line they are also how a paper heads a
	// section about them.
	`|(?:BEGIN|END|THEN|ELSE|DO|REPEAT|UNTIL|GOTO|PROCEDURE)\b` +
	// The same keywords in lower case, but only when the word is the whole
	// line. In lower case and in a sentence these are English, so the rest
	// of the line has to be empty before they count for anything: prose
	// never sets a line that is the single word begin. Aho and Corasick
	// print their four algorithms in a block structured notation that nests
	// three deep, so a third of the lines of each listing are a bare begin
	// or end, and with those lines counting for nothing none of the four
	// reached the half the assembler wants and none of them was fenced.
	`|(?:begin|end)\s*;?\s*$` +
	`|(?:int|double|float|char|void|struct|union|typedef|static|const|unsigned|long|short|bool)\s+\w+\s*[;(=]` +
	`|(?:if|while|for|switch)\s*\(` +
	`|(?:def|func|function|procedure|class)\s+\w+\s*\(` +
	`)`)

// assign matches an assignment, which the papers in this corpus write three
// ways: with a colon and an equals sign, which is ALGOL, Pascal and the
// notation most of the older algorithms are printed in, with a left arrow,
// which is what the journals set when they had the sort available and what
// Smalltalk wrote for its whole life, and with the arrow spelled out in TeX,
// which is what a reader writes when it meets one and has no arrow to hand.
// It is separate from the pattern above because that one is anchored at the
// head of a line and an assignment sits in the middle of one.
//
// The spelled out arrow is the whole of the Aho and Corasick paper: every
// assignment in all four of its algorithms came back as `\leftarrow`, some
// inside a math span and some bare, so the listings carried no arrow for
// this to find and none of them was fenced.
//
// English does not write either of them. Floyd's Algorithm 97 is ten lines of
// ALGOL 60, and the only keyword in it the pattern above knows is a lower
// case begin, which prose writes too, so without this the listing carried no
// evidence at all and went out of the corpus unfenced. McCabe's SEARCH is the
// arrow: the procedure is printed in two halves, the first half assigns with
// a colon and was fenced, the second half assigns with an arrow and was not,
// so thirteen lines of the same procedure went out as prose and rule C08
// reported them.
var assign = regexp.MustCompile(`\S\s*(?::=|←|⟵|\\(?:leftarrow|gets)\b)`)

// semicolon matches a line that ends in a semicolon, which in prose happens
// where a sentence is joined to the next and in a program happens on almost
// every line.
var semicolon = regexp.MustCompile(`[^\s;];\s*$`)

// Mark reports whether one line carries a mark of program text rather than of
// prose.
//
// It lives here rather than in the audit because two things read it and they
// have to agree. Audit rule C08 reports a run of lines that reads as a program
// and is not in a fence, and the assembler refuses to join a paragraph that
// reads that way onto the prose above it. A reader who saw the audit call a
// passage a listing while the assembler had already run its first line into
// the end of a sentence would be right to ask which of them was wrong.
//
// A mark is evidence and not proof, which is why neither caller acts on one
// of them. Each says how many it wants and over how many lines, because they
// are asking different questions: C08 is looking for a listing hidden inside
// a body and the assembler is deciding whether one paragraph is the rest of
// another.
func Mark(line string) bool {
	return Statement(line) || semicolon.MatchString(line)
}

// Statement reports whether a line carries the stronger half of a mark: a
// brace, a directive, a comment, a declaration or an assignment, and not
// merely a semicolon at the end of it.
//
// The two halves are not equal evidence and one caller needs to know which
// it has. A semicolon ending a line is how a program is written and also how
// verse is punctuated, and the GPT-3 paper prints nine lines of a generated
// poem with a semicolon at the end of two of them. Nothing else about those
// lines says program. The other marks have no reading in English at all.
func Statement(line string) bool {
	return statement.MatchString(line) || assign.MatchString(line)
}

// caption matches the caption a paper prints over a numbered listing, which
// is the same shape as a figure caption and is read by the same pattern in
// the tags package.
var caption = regexp.MustCompile(`^\s*\*?\*?(?:Algorithm|Listing)\s+\d+(?:\.\d+)*\*?\*?\s*[.:]`)

// Caption reports whether a line is the caption over a numbered listing.
//
// Two things read it and they have to agree, the same way they do about
// Mark. Audit rule C06 wants the caption to carry an attribute block, and
// the assembler has to cut the caption off the front of a listing before it
// fences one, because a caption inside a fence is a caption the tagger never
// sees and C06 then reports a listing that has no anchor. If the two had
// different ideas of what a caption is, the assembler would fence one the
// rule was still waiting for.
func Caption(line string) bool { return caption.MatchString(line) }

// Marks is how many lines of a stretch of text carry one.
func Marks(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if Mark(line) {
			n++
		}
	}
	return n
}
