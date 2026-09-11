package tags

import (
	"strings"
	"testing"
)

func TestParseTag(t *testing.T) {
	for _, s := range []string{"0A3F", "0a3f", " 0A3F ", "FFFF", "0000"} {
		if _, err := ParseTag(s); err != nil {
			t.Errorf("ParseTag(%q): %v", s, err)
		}
	}
	got, err := ParseTag("0a3f")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0A3F" {
		t.Errorf("lowercase normalised to %q, want 0A3F", got)
	}
	for _, s := range []string{"", "0A3", "0A3FF", "0G3F", "tag", "0A-F"} {
		if _, err := ParseTag(s); err == nil {
			t.Errorf("ParseTag(%q) was accepted", s)
		}
	}
}

func TestTagValue(t *testing.T) {
	if got := Tag("0000").Value(); got != 0 {
		t.Errorf("0000 is %d", got)
	}
	if got := Tag("0A3F").Value(); got != 0x0A3F {
		t.Errorf("0A3F is %d", got)
	}
	if got := Tag("FFFF").Value(); got != 0xFFFF {
		t.Errorf("FFFF is %d", got)
	}
}

// A tag means one thing for ever. Pointing an existing tag at a different
// anchor is the one edit the register must refuse, because every link ever
// written to that tag would silently start meaning something else.
func TestRegisterRefusesReuse(t *testing.T) {
	r := NewRegister()
	if err := r.Add("0001", "vaswani-2017-attention-s1"); err != nil {
		t.Fatal(err)
	}
	err := r.Add("0001", "vaswani-2017-attention-s2")
	if err == nil {
		t.Fatal("a tag was pointed at a second anchor")
	}
	if !strings.Contains(err.Error(), "never reused") {
		t.Errorf("the error does not say why: %v", err)
	}
	// Adding the same pair again is what a re-run does, and it is fine.
	if err := r.Add("0001", "vaswani-2017-attention-s1"); err != nil {
		t.Errorf("re-adding the same pair failed: %v", err)
	}
}

func TestRegisterRefusesTwoTagsOnOneAnchor(t *testing.T) {
	r := NewRegister()
	if err := r.Add("0001", "vaswani-2017-attention-s1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("0002", "vaswani-2017-attention-s1"); err == nil {
		t.Fatal("one anchor was given two tags")
	}
}

func TestRegisterRefusesAnEmptyAnchor(t *testing.T) {
	if err := NewRegister().Add("0001", ""); err == nil {
		t.Fatal("a tag was registered against nothing")
	}
}

// Next climbs from the highest tag issued rather than filling the lowest gap.
// A gap is a retired tag, and handing it out again is exactly the reuse the
// register exists to prevent.
func TestNextClimbsPastAGap(t *testing.T) {
	r := NewRegister()
	for _, e := range []Entry{{"0001", "a"}, {"0002", "b"}, {"00FF", "c"}} {
		if err := r.Add(e.Tag, e.Anchor); err != nil {
			t.Fatal(err)
		}
	}
	next, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if next != "0100" {
		t.Errorf("Next is %s, want 0100", next)
	}
}

func TestNextOnAnEmptyRegister(t *testing.T) {
	next, err := NewRegister().Next()
	if err != nil {
		t.Fatal(err)
	}
	if next != "0000" {
		t.Errorf("the first tag is %s, want 0000", next)
	}
}

func TestReadAndWriteRoundTrip(t *testing.T) {
	in := "0002,vaswani-2017-attention-s2\n\n0001,vaswani-2017-attention-s1\n"
	r, err := Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if r.Len() != 2 {
		t.Fatalf("read %d entries, want 2", r.Len())
	}
	anchor, ok := r.Anchor("0002")
	if !ok || anchor != "vaswani-2017-attention-s2" {
		t.Errorf("0002 names %q", anchor)
	}
	tag, ok := r.Tag("vaswani-2017-attention-s1")
	if !ok || tag != "0001" {
		t.Errorf("s1 carries %q", tag)
	}

	// The file is written sorted, so two machines that issued the same tags
	// in a different order produce the same bytes.
	var out strings.Builder
	if err := r.Write(&out); err != nil {
		t.Fatal(err)
	}
	want := "0001,vaswani-2017-attention-s1\n0002,vaswani-2017-attention-s2\n"
	if out.String() != want {
		t.Errorf("Write gave\n%q\nwant\n%q", out.String(), want)
	}
}

func TestReadRejectsABadLine(t *testing.T) {
	cases := []string{
		"0001 vaswani-2017-attention-s1\n",
		"001,vaswani-2017-attention-s1\n",
		"0001,\n",
		"0001,a\n0001,b\n",
	}
	for _, in := range cases {
		if _, err := Read(strings.NewReader(in)); err == nil {
			t.Errorf("Read accepted %q", in)
		} else if !strings.Contains(err.Error(), "line ") {
			t.Errorf("the error does not name the line: %v", err)
		}
	}
}

func TestLoadAMissingFileIsEmpty(t *testing.T) {
	r, err := Load(t.TempDir() + "/tags")
	if err != nil {
		t.Fatalf("a corpus before its first tag run failed to load: %v", err)
	}
	if r.Len() != 0 {
		t.Errorf("an absent register has %d entries", r.Len())
	}
}

func TestParseAttrs(t *testing.T) {
	body := `### 3.2 Multi-Head Attention {#vaswani-2017-attention-s3-2 .section tag=0A3F}

Some text, and an equation.

$$E = mc^2$$ {#vaswani-2017-attention-eq-1 .equation tag=0a40}

A heading with no tag {#untagged .section}
`
	got := ParseAttrs(body)
	if len(got) != 2 {
		t.Fatalf("found %d attribute blocks, want 2", len(got))
	}
	if got[0].Anchor != "vaswani-2017-attention-s3-2" || got[0].Tag != "0A3F" {
		t.Errorf("first block is %+v", got[0])
	}
	if len(got[0].Classes) != 1 || got[0].Classes[0] != "section" {
		t.Errorf("classes are %v", got[0].Classes)
	}
	if got[1].Tag != "0A40" {
		t.Errorf("a lowercase tag in the text parsed as %q", got[1].Tag)
	}
}

func TestFormat(t *testing.T) {
	a := Attr{Anchor: "vaswani-2017-attention-s3-2", Classes: []string{"section"}, Tag: "0A3F"}
	want := "{#vaswani-2017-attention-s3-2 .section tag=0A3F}"
	if got := Format(a); got != want {
		t.Errorf("Format is %q, want %q", got, want)
	}
	// What Format writes is what ParseAttrs reads back.
	back := ParseAttrs(Format(a))
	if len(back) != 1 || back[0].Anchor != a.Anchor || back[0].Tag != a.Tag {
		t.Errorf("the round trip gave %+v", back)
	}
}
