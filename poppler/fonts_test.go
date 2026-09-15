package poppler

import "testing"

// pdffonts prints eight columns and the last of them, the object reference,
// is two fields. Counting the type from the left got the encoding as well, so
// every font came back typed "Type 1 Builtin" and nothing that asked what a
// font was could get an answer.
func TestParseFonts(t *testing.T) {
	out := []byte(
		"name                                 type              encoding         emb sub uni object ID\n" +
			"------------------------------------ ----------------- ---------------- --- --- --- ---------\n" +
			"[none]                               Type 3            Custom           yes no  yes     34  0\n" +
			"ABCDEF+CMR10                         Type 1            Builtin          yes yes no       5  0\n" +
			"GHIJKL+NimbusRomNo9L                 CID TrueType      Identity-H       yes yes yes    112  0\n" +
			"Times-Roman                          Type 1            WinAnsi          no  no  yes      7  0\n")

	want := []Font{
		{"[none]", "Type 3", true, true},
		{"ABCDEF+CMR10", "Type 1", true, false},
		{"GHIJKL+NimbusRomNo9L", "CID TrueType", true, true},
		{"Times-Roman", "Type 1", false, true},
	}
	got := ParseFonts(out)
	if len(got) != len(want) {
		t.Fatalf("read %d fonts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("font %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A nameless bitmap is the dvips output that turns a paper's mathematics into
// the font positions of its glyphs. A named Type 3 is somebody's logo and an
// ordinary font is an ordinary font.
func TestFontBitmap(t *testing.T) {
	for _, c := range []struct {
		f    Font
		want bool
	}{
		{Font{Name: "[none]", Type: "Type 3"}, true},
		{Font{Name: "ABCDEF+Logo", Type: "Type 3"}, false},
		{Font{Name: "[none]", Type: "Type 1"}, false},
		{Font{Name: "ABCDEF+CMMI10", Type: "Type 1"}, false},
	} {
		if got := c.f.Bitmap(); got != c.want {
			t.Errorf("%+v is a bitmap %v, want %v", c.f, got, c.want)
		}
	}
}
