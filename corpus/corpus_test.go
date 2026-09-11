package corpus

import "testing"

func TestFields(t *testing.T) {
	if len(Fields) != 12 {
		t.Fatalf("there are %d fields, want 12", len(Fields))
	}
	seen := map[Field]bool{}
	for _, f := range Fields {
		if !f.Valid() {
			t.Errorf("%s is in Fields and is not valid", f)
		}
		if f.Title() == "" {
			t.Errorf("%s has no title", f)
		}
		if seen[f] {
			t.Errorf("%s is listed twice", f)
		}
		seen[f] = true
	}
	if Field("ml").Valid() {
		t.Error("ml is not one of the twelve and was accepted")
	}
	if _, err := ParseField("ai/ml"); err == nil {
		t.Error("ParseField accepted a field that does not exist")
	}
}

// The access class decides what gets published, so the table is written out
// here rather than derived, and a change to the rules has to be made in two
// places on purpose.
func TestAccessPermissions(t *testing.T) {
	cases := []struct {
		access   Access
		body     bool
		figures  bool
		abstract bool
	}{
		{AccessPublicDomain, true, true, true},
		{AccessOpen, true, true, true},
		{AccessPermissive, true, true, true},
		{AccessRestricted, false, false, true},
		{AccessUnknown, false, false, false},
		{Access(""), false, false, false},
	}
	for _, c := range cases {
		if got := c.access.Body(); got != c.body {
			t.Errorf("%q Body is %v, want %v", c.access, got, c.body)
		}
		if got := c.access.Figures(); got != c.figures {
			t.Errorf("%q Figures is %v, want %v", c.access, got, c.figures)
		}
		if got := c.access.Abstract(); got != c.abstract {
			t.Errorf("%q Abstract is %v, want %v", c.access, got, c.abstract)
		}
	}
}

func TestAccessValid(t *testing.T) {
	for _, a := range Accesses {
		if !a.Valid() {
			t.Errorf("%s is in Accesses and is not valid", a)
		}
	}
	if Access("").Valid() {
		t.Error("an empty access class was accepted")
	}
	if Access("free").Valid() {
		t.Error("free is not an access class and was accepted")
	}
}

func TestStatusAndLang(t *testing.T) {
	for _, s := range Statuses {
		if !s.Valid() {
			t.Errorf("%s is in Statuses and is not valid", s)
		}
	}
	if Status("pending").Valid() {
		t.Error("pending is not a status and was accepted")
	}
	if !EN.Valid() || EN.Translated() {
		t.Error("English is a language and is not a translation")
	}
	for _, l := range []Lang{VI, ZH, JA} {
		if !l.Translated() {
			t.Errorf("%s should be a translation", l)
		}
	}
	if Lang("fr").Valid() {
		t.Error("fr is not a corpus language and was accepted")
	}
}
