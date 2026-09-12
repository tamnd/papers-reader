package render

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/poppler"
)

// drawn is a stand-in for pdftoppm. It writes a file where poppler would and
// records what it was asked for, which is the whole of what this package is
// responsible for: poppler's own behaviour is poppler's business.
type drawn struct {
	calls []string
	fail  error
}

func (d *drawn) draw(_ context.Context, pdf string, page int, r poppler.Raster, out string) error {
	if d.fail != nil {
		return d.fail
	}
	d.calls = append(d.calls, out)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, fmt.Appendf(nil, "%s page %d at %d", pdf, page, r.DPI), 0o644)
}

func TestProfileName(t *testing.T) {
	for _, c := range []struct {
		p    Profile
		want string
	}{
		{Profile{DPI: 300, Gray: true}, "300-gray"},
		{Profile{DPI: 300}, "300-rgb"},
		{Profile{DPI: 600, Gray: true}, "600-gray"},
	} {
		if got := c.p.Name(); got != c.want {
			t.Errorf("%+v is named %q, want %q", c.p, got, c.want)
		}
	}
}

// A page is never asked again at the resolution it already failed at, so the
// ladder only ever climbs, and it stops rather than wrapping round.
func TestProfileClimbsTheLadderAndStops(t *testing.T) {
	p := Profile{DPI: Base, Gray: true}
	var seen []int
	for {
		seen = append(seen, p.DPI)
		next, ok := p.Next()
		if !ok {
			break
		}
		if next.DPI <= p.DPI {
			t.Fatalf("%s escalated to %s", p, next)
		}
		if next.Gray != p.Gray {
			t.Fatalf("%s escalated to %s and changed colour on the way", p, next)
		}
		p = next
	}
	if len(seen) != len(Ladder) {
		t.Errorf("the ladder walked %v, want %v", seen, Ladder)
	}
}

// A resolution somebody typed is not on the ladder, and the ladder should say
// so rather than quietly starting again at the bottom.
func TestAResolutionOffTheLadderHasNoNext(t *testing.T) {
	if next, ok := (Profile{DPI: 350}).Next(); ok {
		t.Errorf("350 escalated to %s", next)
	}
}

func TestProfileRefusesAbsurdResolutions(t *testing.T) {
	if err := (Profile{DPI: 300}).Valid(); err != nil {
		t.Errorf("300 was refused: %v", err)
	}
	for _, dpi := range []int{0, 50, 4800} {
		if err := (Profile{DPI: dpi}).Valid(); err == nil {
			t.Errorf("%d was accepted", dpi)
		}
	}
}

func TestStoreRendersOnceAndKeepsIt(t *testing.T) {
	d := &drawn{}
	s := Store{Dir: t.TempDir(), Draw: d.draw}
	p := Profile{DPI: 300, Gray: true}

	made, err := s.Render(context.Background(), "paper.pdf", p, 7)
	if err != nil || !made {
		t.Fatalf("the first render came back %v, %v", made, err)
	}
	made, err = s.Render(context.Background(), "paper.pdf", p, 7)
	if err != nil || made {
		t.Fatalf("the second render came back %v, %v", made, err)
	}
	if len(d.calls) != 1 {
		t.Errorf("pdftoppm was run %d times for one page", len(d.calls))
	}
	if !strings.HasSuffix(d.calls[0], filepath.Join("300-gray", "0007.png")) {
		t.Errorf("it rendered to %s", d.calls[0])
	}
}

// The two resolutions are two files, not one file overwritten, because the
// retry logic reads the disk to find out what has been tried.
func TestTwoProfilesOfOnePageSitSideBySide(t *testing.T) {
	d := &drawn{}
	s := Store{Dir: t.TempDir(), Draw: d.draw}
	for _, p := range []Profile{{DPI: 300, Gray: true}, {DPI: 400, Gray: true}} {
		if _, err := s.Render(context.Background(), "paper.pdf", p, 2); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Has(Profile{DPI: 300, Gray: true}, 2) || !s.Has(Profile{DPI: 400, Gray: true}, 2) {
		t.Fatal("one render replaced the other")
	}
	best, ok := s.Best(2, true)
	if !ok || best.DPI != 400 {
		t.Errorf("the best render of page 2 is %s, %v", best, ok)
	}
}

func TestBestOfAPageNobodyRendered(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if best, ok := s.Best(1, true); ok {
		t.Errorf("an empty store offered %s for page 1", best)
	}
}

// A half written PNG left behind by a machine that was rebooted is not a
// rendered page, and a run that treats it as one never looks at that page
// again.
func TestAnEmptyFileIsNotARenderedPage(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	p := Profile{DPI: 300, Gray: true}
	if err := os.MkdirAll(filepath.Dir(s.Path(p, 1)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path(p, 1), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if s.Has(p, 1) {
		t.Error("an empty file counted as a rendered page")
	}
}

func TestPagesAndMissing(t *testing.T) {
	d := &drawn{}
	s := Store{Dir: t.TempDir(), Draw: d.draw}
	p := Profile{DPI: 300, Gray: true}
	for _, page := range []int{1, 2, 5} {
		if _, err := s.Render(context.Background(), "paper.pdf", p, page); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := s.Pages(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 || pages[0] != 1 || pages[2] != 5 {
		t.Errorf("the store holds %v", pages)
	}
	missing := s.Missing(p, 1, 6)
	if len(missing) != 3 || missing[0] != 3 || missing[1] != 4 || missing[2] != 6 {
		t.Errorf("pages %v are missing", missing)
	}
}

// A directory that was never made is no pages rather than an error, because
// the first run over a paper is the common case and it should not have to
// special case itself.
func TestPagesOfAPaperNobodyHasRendered(t *testing.T) {
	pages, err := (Store{Dir: filepath.Join(t.TempDir(), "nothing")}).Pages(Profile{DPI: 300, Gray: true})
	if err != nil || len(pages) != 0 {
		t.Errorf("an unrendered paper came back as %v, %v", pages, err)
	}
}

// Anything poppler does not call grey counts as colour. Being wrong this way
// costs a bigger file on one page; being wrong the other way loses the one
// figure in the paper that was in colour.
func TestColourIsJudgedFromTheImagesOnThePage(t *testing.T) {
	list := []byte(strings.Join([]string{
		"page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio",
		"--------------------------------------------------------------------------------------------",
		"   1     0 image    2550  3300  gray    1   8  jpeg    no        12  0   300   300  1.0M  10%",
		"   4     1 image     600   400   rgb    3   8  jpeg    no        30  0   300   300  200K  20%",
		"   6     2 image     100   100 index    1   8  image   no        41  0   300   300   2K   1%",
	}, "\n"))
	colour := ColourPages(poppler.ParseImages(list))
	if colour[1] {
		t.Error("a grey scan of a page counted as colour")
	}
	if !colour[4] || !colour[6] {
		t.Errorf("the colour pages came out as %v", colour)
	}
}
