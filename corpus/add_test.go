package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// manifest is a small papers.yaml with the shape of the real one: a header
// comment, a comment naming each group, a blank line between entries and one
// author list wrapped by hand.
const manifest = `# The corpus manifest: one entry per paper.
#
# Entries 1 to 100 are the seed canon and keep their number for ever.

papers:
  # theory
  - id: turing-1936-computable
    title: On Computable Numbers
    authors: [A. M. Turing]
    year: 1936
    venue: Proceedings of the London Mathematical Society
    field: theory
    number: 1
    status: listed

  # networks
  - id: cerf-1974-tcpip
    title: A Protocol for Packet Network Intercommunication
    authors: [Vinton G. Cerf, Robert E. Kahn]
    year: 1974
    venue: IEEE Transactions on Communications
    field: networks
    number: 2
    status: listed

  - id: mckeown-2008-openflow
    title: OpenFlow, Enabling Innovation in Campus Networks
    authors: [Nick McKeown, Tom Anderson, Hari Balakrishnan, Guru Parulkar,
              Larry Peterson, Jennifer Rexford]
    year: 2008
    venue: ACM SIGCOMM Computer Communication Review
    field: networks
    number: 3
    status: listed
`

func written(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "papers.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func added(t *testing.T, p Paper) (string, *Papers) {
	t.Helper()
	path := written(t, manifest)
	if err := AppendPaper(path, p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadPapers(path)
	if err != nil {
		t.Fatalf("the manifest no longer parses: %v\n%s", err, b)
	}
	return string(b), m
}

func TestAPaperGoesAtTheEndOfItsOwnGroup(t *testing.T) {
	body, m := added(t, Paper{
		ID:      "jacobson-1988-congestion",
		Title:   "Congestion Avoidance and Control",
		Authors: []string{"Van Jacobson"},
		Year:    1988,
		Venue:   "ACM SIGCOMM",
		Field:   Networks,
		Status:  Listed,
	})
	want := []string{"turing-1936-computable", "cerf-1974-tcpip", "mckeown-2008-openflow", "jacobson-1988-congestion"}
	if got := m.IDs(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the manifest reads %v, want %v", got, want)
	}
	if !strings.HasSuffix(body, "  - id: jacobson-1988-congestion\n    title: Congestion Avoidance and Control\n    authors: [Van Jacobson]\n    year: 1988\n    venue: ACM SIGCOMM\n    field: networks\n    status: listed\n") {
		t.Errorf("the entry is not written the way the file writes one:\n%s", body)
	}
}

// A field the manifest has never held has no group to join, so the entry goes
// at the bottom rather than somewhere a program guessed.
func TestAPaperOfANewFieldGoesAtTheEnd(t *testing.T) {
	_, m := added(t, Paper{
		ID:      "phong-1975-illumination",
		Title:   "Illumination for Computer Generated Pictures",
		Authors: []string{"Bui Tuong Phong"},
		Year:    1975,
		Field:   Graphics,
		Status:  Listed,
	})
	if got := m.IDs(); got[len(got)-1] != "phong-1975-illumination" {
		t.Errorf("the manifest reads %v, want the new paper last", got)
	}
}

// The first group ends in the middle of the file, so this is the case where
// the splice has to leave everything under it alone.
func TestAPaperInTheFirstGroupLeavesTheRestOfTheFileAlone(t *testing.T) {
	body, m := added(t, Paper{
		ID:     "cook-1971-np",
		Title:  "The Complexity of Theorem Proving Procedures",
		Year:   1971,
		Field:  Theory,
		Status: Listed,
	})
	want := []string{"turing-1936-computable", "cook-1971-np", "cerf-1974-tcpip", "mckeown-2008-openflow"}
	if got := m.IDs(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the manifest reads %v, want %v", got, want)
	}
	if !strings.Contains(body, "    status: listed\n\n  # networks\n") {
		t.Errorf("the comment naming the next group has moved:\n%s", body)
	}
}

// The reason the whole file is not re-encoded. Everything the new entry is
// not goes back to disk byte for byte, comments, blank lines, hand wrapping
// and all.
func TestAddingAPaperChangesNothingElseInTheFile(t *testing.T) {
	body, _ := added(t, Paper{ID: "cook-1971-np", Year: 1971, Field: Theory, Status: Listed})
	for _, line := range strings.Split(manifest, "\n") {
		if !strings.Contains(body, line+"\n") && line != "" {
			t.Errorf("%q is no longer in the file", line)
		}
	}
	if n := strings.Count(body, "\n\n"); n != strings.Count(manifest, "\n\n")+1 {
		t.Errorf("the file has %d blank lines in it, want one more than it had", n)
	}
}

// The hundred stay a hundred. Number is omitempty and nothing sets it, so an
// entry written by this function has no number at all rather than a
// hundred and first one.
func TestAnAddedPaperHasNoNumber(t *testing.T) {
	body, m := added(t, Paper{ID: "cook-1971-np", Year: 1971, Field: Theory, Status: Listed})
	p, ok := m.ByID("cook-1971-np")
	if !ok {
		t.Fatal("the paper is not in the manifest")
	}
	if p.Number != 0 {
		t.Errorf("the new paper is number %d", p.Number)
	}
	entry, _, _ := strings.Cut(body[strings.Index(body, "  - id: cook-1971-np"):], "\n\n")
	if strings.Contains(entry, "number:") {
		t.Errorf("the entry carries a number:\n%s", entry)
	}
	for _, id := range []string{"turing-1936-computable", "cerf-1974-tcpip", "mckeown-2008-openflow"} {
		if p, _ := m.ByID(id); p.Number == 0 {
			t.Errorf("%s has lost its number", id)
		}
	}
}

func TestAPaperThatIsAlreadyThereIsRefused(t *testing.T) {
	path := written(t, manifest)
	err := AppendPaper(path, Paper{ID: "cerf-1974-tcpip", Field: Networks, Status: Listed})
	if err == nil {
		t.Fatal("a duplicate id was accepted")
	}
	if !strings.Contains(err.Error(), "already in") {
		t.Errorf("the error does not say the paper is already there: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != manifest {
		t.Error("the refused entry was written anyway")
	}
}

func TestAnIdThatIsNotAnIdIsRefusedBeforeTheFileIsRead(t *testing.T) {
	path := written(t, manifest)
	for _, id := range []string{"", "Cook-1971-NP", "cook"} {
		if err := AppendPaper(path, Paper{ID: id, Field: Theory}); err == nil {
			t.Errorf("%q was accepted as an id", id)
		}
	}
	b, _ := os.ReadFile(path)
	if string(b) != manifest {
		t.Error("the file was written anyway")
	}
}
