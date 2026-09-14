package translate

import "strings"

// rebrace puts braces round every argument of a TeX command that was written
// without them, so that two spellings of one formula compare as one formula.
//
// TeX lets a one token argument go bare. "\frac12" and "\frac{1}{2}" are the
// same fraction, "x^2" and "x^{2}" are the same power, and which one a paper
// prints is a matter of who typed it. A model copying a formula out of one
// language and into another has no reason to keep the choice, and the third
// re-translation of the GAN paper died on exactly that: three answers refused
// and the file given up on because the source said "$\frac{1}{2}$" and every
// answer said "$\frac12$".
//
// That is the third time this comparison has thrown away correct work for a
// difference that is not one. The first was the order of two spans in a
// paragraph, the second was a space beside a delimiter, and both are written
// up above. The lesson each time is the same: the check is for a formula that
// changed, and a formula that is set differently has not changed.
//
// Braces are added and never taken away, which is what makes this safe. Two
// formulas that come out equal here render the same, because the only thing
// the pass does is write out what TeX would have inferred. A renamed
// variable, a dropped factor and a moved exponent all still differ.
func rebrace(s string) string {
	t := &tex{r: []rune(s)}
	t.run()
	return t.b.String()
}

// arity is how many brace arguments a command takes, for the commands a paper
// writes with the braces left off.
//
// A list and not a rule, because TeX has no rule: the arity of a command is
// whatever its definition says, and a document can define its own. What is
// here is the accent and fraction commands, which are the ones that take a
// single symbol and so are the ones anybody writes bare. A command that is
// not here keeps whatever it was written with, which is the same on both
// sides of the comparison and so costs nothing.
//
// The upright and font commands are here even though nobody writes
// "\mathbbR", because a command left out of the list is a command whose
// braced argument is not looked inside, and the argument of a \mathbf can
// hold a \frac.
var arity = func() map[string]int {
	m := map[string]int{}
	for _, s := range strings.Fields(`
		frac dfrac tfrac cfrac binom dbinom tbinom
		overset underset stackrel
	`) {
		m[`\`+s] = 2
	}
	for _, s := range strings.Fields(`
		sqrt hat bar tilde vec dot ddot check breve acute grave
		overline underline widehat widetilde overbrace underbrace
		boldsymbol bm mathbb mathbf mathcal mathrm mathit mathsf mathtt
		mathfrak operatorname
		text textit textbf textrm textnormal mbox
	`) {
		m[`\`+s] = 1
	}
	return m
}()

// tex walks a formula once, copying what it reads and bracing what it must.
type tex struct {
	r []rune
	i int
	b strings.Builder
}

func (t *tex) run() {
	for t.i < len(t.r) {
		c := t.r[t.i]
		switch {
		case c == '\\' && t.i+1 < len(t.r) && !letter(t.r[t.i+1]):
			// An escaped character: \{ and \} and \\ and the rest. Two runes
			// and no argument, and reading the second one as a control word
			// would turn "\\{" into a group.
			t.b.WriteRune(c)
			t.b.WriteRune(t.r[t.i+1])
			t.i += 2
		case c == '\\':
			word := t.word()
			t.b.WriteString(word)
			// A starred command is its own command. "\operatorname*" takes
			// its argument after the star, and reading the star as the
			// argument would write "\operatorname{*}".
			if t.i < len(t.r) && t.r[t.i] == '*' {
				t.b.WriteRune('*')
				t.i++
			}
			if word == `\sqrt` {
				t.optional()
			}
			t.args(arity[word])
		case c == '^' || c == '_':
			t.b.WriteRune(c)
			t.i++
			t.args(1)
		default:
			t.b.WriteRune(c)
			t.i++
		}
	}
}

// word reads a control word: a backslash and the letters after it.
func (t *tex) word() string {
	at := t.i
	t.i++
	for t.i < len(t.r) && letter(t.r[t.i]) {
		t.i++
	}
	return string(t.r[at:t.i])
}

// args reads n arguments and writes each one braced.
func (t *tex) args(n int) {
	for k := 0; k < n; k++ {
		t.spaces()
		arg, ok := t.arg()
		if !ok {
			// A command with nothing after it is a formula that is already
			// wrong, and it is left as it was written rather than completed
			// here. The comparison sees the same wrong thing on both sides.
			return
		}
		t.b.WriteString("{" + rebrace(arg) + "}")
	}
}

// arg reads one argument and says whether there was one. A group is its
// contents without the braces, and anything else is the single token TeX
// would have taken.
func (t *tex) arg() (string, bool) {
	if t.i >= len(t.r) {
		return "", false
	}
	switch c := t.r[t.i]; {
	case c == '{':
		return t.group()
	case c == '}' || c == '$':
		// The group or the formula this is inside has ended, so there is no
		// argument to take. Taking the character would unbalance the one and
		// close the other, and "$\frac$" would come out "$\frac{$}".
		return "", false
	case c == '\\':
		if t.i+1 < len(t.r) && !letter(t.r[t.i+1]) {
			at := t.i
			t.i += 2
			return string(t.r[at:t.i]), true
		}
		return t.word(), true
	default:
		t.i++
		return string(c), true
	}
}

// group reads a braced group and returns what is inside it. A group that
// never closes is not a group, and its opening brace is read as a character.
func (t *tex) group() (string, bool) {
	depth := 1
	for i := t.i + 1; i < len(t.r); i++ {
		switch t.r[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				out := string(t.r[t.i+1 : i])
				t.i = i + 1
				return out, true
			}
		}
	}
	t.b.WriteRune('{')
	t.i++
	return "", false
}

// optional copies a bracketed optional argument, which is how a paper writes
// the index of a root: "\sqrt[3]{x}". It is copied and not braced, because
// the brackets are the argument's delimiters and braces would be a different
// formula.
func (t *tex) optional() {
	at := t.i
	t.spaces()
	if t.i >= len(t.r) || t.r[t.i] != '[' {
		t.i = at
		return
	}
	for i := t.i; i < len(t.r); i++ {
		if t.r[i] == ']' {
			t.b.WriteString(string(t.r[t.i : i+1]))
			t.i = i + 1
			return
		}
	}
	t.i = at
}

// spaces steps over the whitespace between a command and its argument, which
// TeX ignores.
func (t *tex) spaces() {
	for t.i < len(t.r) && (t.r[t.i] == ' ' || t.r[t.i] == '\t' || t.r[t.i] == '\n') {
		t.i++
	}
}

func letter(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
}
