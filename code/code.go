// Package code is where the program text of a body starts and stops.
//
// It is the code side of what mathtex is for the mathematics, and it is a leaf
// package for the same reason: extract writes the fences, translate has to
// carry them across a language byte for byte, and the C rules of the audit
// read them back and say whether the listings survived. Three ideas of where a
// fence begins would be three answers about the same file.
//
// What it is not is a Markdown parser. Markdown has two ways to write a code
// block and this package knows one of them, the fence, because the fence is
// the only one the prompt asks for and the only one that can carry a language.
// The other way, four spaces of indentation, is what a listing degrades into
// when a reader forgets the fence, and telling the two apart is what rule C08
// is for rather than something this package should quietly paper over.
package code

import "strings"

// A Block is one fenced region of a body.
type Block struct {
	// Lang is what the opening fence is tagged with, lower cased, and empty
	// for a fence with no tag. It is the tag as written and not a language
	// this package has recognised: rule C02 decides which tags the corpus
	// allows and this package does not have opinions.
	Lang string
	// Text is what is between the fences, without either of them and without
	// the newline that ends the last line. Byte for byte otherwise, including
	// the trailing spaces, which is the whole point of C07.
	Text string
	// Line is the body line the opening fence sits on, counting from one, and
	// End is the line the closing fence sits on. End is zero for a block that
	// was never closed.
	Line, End int
	// Mark is the run of characters the fence is made of, three or more
	// backticks or three or more tildes. A block closes on its own mark and
	// not on the other one, which is how a listing that contains three
	// backticks can be written at all.
	Mark string
	// Indent is the white space before the opening fence. Markdown allows up
	// to three spaces there and a fence indented further is not a fence, so
	// this is never more than three.
	Indent string
}

// Closed reports whether the block has a closing fence.
func (b Block) Closed() bool { return b.End > 0 }

// Lines is how many lines of program text the block holds.
func (b Block) Lines() int {
	if b.Text == "" {
		return 0
	}
	return strings.Count(b.Text, "\n") + 1
}

// Blocks cuts a body into its fenced regions.
//
// unclosed is the block left open at the end of the body, and nil when there
// is none. It is returned separately and also included in the slice, because
// the rules want both questions answered: C01 wants to know that a fence never
// closed and C05 wants to count the lines of every block including that one.
//
// A fence closes on its own mark and on a run at least as long as the one that
// opened it, which is CommonMark's rule and is what lets a listing containing
// three backticks be written inside four. Anything on the line after a closing
// run means it is not a closing run, so ```` ```go ```` opens and ```` ``` ````
// closes and ```` ```end ```` is a line of the listing.
func Blocks(body string) (blocks []Block, unclosed *Block) {
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		indent, mark, lang, ok := opens(lines[i])
		if !ok {
			continue
		}
		b := Block{Lang: lang, Line: i + 1, Mark: mark, Indent: indent}
		var text []string
		for j := i + 1; j < len(lines); j++ {
			if closes(lines[j], mark) {
				b.End = j + 1
				break
			}
			text = append(text, lines[j])
		}
		b.Text = strings.Join(text, "\n")
		if b.End == 0 {
			// An unclosed fence runs to the end of the body, which is the
			// cautious reading and also what every Markdown renderer does.
			blocks = append(blocks, b)
			last := &blocks[len(blocks)-1]
			return blocks, last
		}
		blocks = append(blocks, b)
		i = b.End - 1
	}
	return blocks, nil
}

// Inside reports, for each line of the body counting from one at index one,
// whether that line is inside a fence or is one of the fences itself.
//
// Index zero is unused and always false, so that a caller holding a line
// number from a Finding can index it without doing arithmetic that is easy to
// get wrong by one.
func Inside(body string) []bool {
	lines := strings.Split(body, "\n")
	out := make([]bool, len(lines)+1)
	blocks, _ := Blocks(body)
	for _, b := range blocks {
		end := b.End
		if end == 0 {
			end = len(lines)
		}
		for i := b.Line; i <= end && i < len(out); i++ {
			out[i] = true
		}
	}
	return out
}

// opens reads an opening fence: up to three spaces, a run of three or more
// backticks or tildes, and a tag.
//
// The tag is taken as the first word of what follows, lower cased. A fence
// written ```` ```c linenos ```` is a C listing with an attribute nobody here
// reads, and reading the whole of the rest of the line as the language would
// make it a listing in a language called "c linenos".
//
// A backtick fence may not have a backtick in its tag, because a run of
// backticks with more backticks later on the line is inline code and not a
// fence. A tilde fence may have anything.
func opens(line string) (indent, mark, lang string, ok bool) {
	indent = line[:len(line)-len(strings.TrimLeft(line, " "))]
	if len(indent) > 3 {
		return "", "", "", false
	}
	rest := line[len(indent):]
	for _, c := range []string{"`", "~"} {
		run := run(rest, c)
		if run < 3 {
			continue
		}
		tag := strings.TrimSpace(rest[run:])
		if c == "`" && strings.Contains(tag, "`") {
			return "", "", "", false
		}
		lang, _, _ = strings.Cut(tag, " ")
		return indent, strings.Repeat(c, run), strings.ToLower(lang), true
	}
	return "", "", "", false
}

// closes reads a closing fence: up to three spaces, a run of the same
// character at least as long as the opening one, and nothing else.
func closes(line, mark string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	c := mark[:1]
	if run(trimmed, c) < len(mark) {
		return false
	}
	return strings.TrimSpace(strings.TrimLeft(trimmed, c)) == ""
}

// run is how many of c the string opens with.
func run(s, c string) int {
	n := 0
	for strings.HasPrefix(s[n:], c) {
		n++
	}
	return n
}
