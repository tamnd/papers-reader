package main

import (
	"testing"
	"time"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/extract"
)

// recorded is a corpus with one paper's extraction record written down.
func recorded(t *testing.T, id string, first, last int) *corpus.Corpus {
	t.Helper()
	c := &corpus.Corpus{Root: t.TempDir()}
	r := extract.Record{Path: "vision", First: first, Last: last, When: time.Now()}
	if err := r.Write(c.Work(id)); err != nil {
		t.Fatal(err)
	}
	return c
}

// The map has to describe the pages the paper was read on. The UNC tech
// report of No Silver Bullet is read from page 4, and its map described
// pages 1 to 3, which are a cover sheet, a notice and a second title page.
func TestARestrictedPaperIsMappedOverTheWindowItWasReadOn(t *testing.T) {
	c := recorded(t, "brooks-1987-nosilverbullet", 4, 6)
	first, last := restrictedRange(c, "brooks-1987-nosilverbullet", 21)
	if first != 4 || last != 6 {
		t.Errorf("mapped pages %d to %d, want 4 to 6", first, last)
	}
}

// A paper nothing has read yet has no window to follow, and the front of
// the file is the only guess there is.
func TestAPaperWithNoRecordIsMappedFromTheFront(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir()}
	first, last := restrictedRange(c, "codd-1970-relational", 11)
	if first != 1 || last != restrictedPages {
		t.Errorf("mapped pages %d to %d, want 1 to %d", first, last, restrictedPages)
	}
}

// A window that runs past the end of a short paper is cut back to it,
// because reading a page that is not there is a tool error a long way from
// here.
func TestTheMappedWindowStopsAtTheEndOfTheFile(t *testing.T) {
	c := recorded(t, "lovelace-1843-notes", 2, 4)
	first, last := restrictedRange(c, "lovelace-1843-notes", 3)
	if first != 2 || last != 3 {
		t.Errorf("mapped pages %d to %d of a 3 page file, want 2 to 3", first, last)
	}
}
