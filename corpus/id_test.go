package corpus

import "testing"

func TestParseID(t *testing.T) {
	id, err := ParseID("vaswani-2017-attention")
	if err != nil {
		t.Fatalf("ParseID: %v", err)
	}
	if id.Surname != "vaswani" || id.Year != 2017 || id.Keyword != "attention" {
		t.Errorf("got %+v, want vaswani 2017 attention", id)
	}
	if id.String() != "vaswani-2017-attention" {
		t.Errorf("String is %q", id.String())
	}
}

// The keyword is one word. A paper whose subject needs two words gets the
// shorter one, because an id with a variable number of parts cannot be split
// back into its parts.
func TestParseIDTakesOneKeyword(t *testing.T) {
	for _, s := range []string{"dean-2004-mapreduce", "codd-1970-relational"} {
		if _, err := ParseID(s); err != nil {
			t.Errorf("ParseID(%q): %v", s, err)
		}
	}
	if _, err := ParseID("shannon-1948-information-theory"); err == nil {
		t.Error("a two word keyword was accepted")
	}
}

func TestParseIDRejects(t *testing.T) {
	bad := []string{
		"",
		"Vaswani-2017-attention",
		"vaswani_2017_attention",
		"vaswani-2017",
		"vaswani-17-attention",
		"vaswani-2017-",
		"2017-vaswani-attention",
		"vaswani-2017-attention ",
	}
	for _, s := range bad {
		if _, err := ParseID(s); err == nil {
			t.Errorf("ParseID(%q) was accepted", s)
		}
		if ValidID(s) {
			t.Errorf("ValidID(%q) is true", s)
		}
	}
}

func TestAnchors(t *testing.T) {
	got := SectionAnchor("vaswani-2017-attention", "3.2")
	if want := "vaswani-2017-attention-s3-2"; got != want {
		t.Errorf("SectionAnchor is %q, want %q", got, want)
	}
	got = ItemAnchor("vaswani-2017-attention", "eq", "3.1")
	if want := "vaswani-2017-attention-eq-3-1"; got != want {
		t.Errorf("ItemAnchor is %q, want %q", got, want)
	}
}
