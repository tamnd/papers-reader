package corpus

import "testing"

const testSources = `sources:
  - id: vaswani-2017-attention
    access: open
    licence: arXiv non-exclusive licence to distribute
    url: https://arxiv.org/pdf/1706.03762v7
    landing: https://arxiv.org/abs/1706.03762
    fetched: 2026-09-11
    sha256: 0000000000000000000000000000000000000000000000000000000000000000
    pages: 15
    text_layer: native
  - id: turing-1936-computable
    access: public-domain
    licence: public domain
    url: https://example.org/turing.pdf
    by: hand
    note: the 1936 volume is out of copyright, checked against the publisher's own page
`

func TestSourcesAccess(t *testing.T) {
	c := testCorpus(t, threePapers, "", testSources)
	s, err := c.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Access("vaswani-2017-attention"); got != AccessOpen {
		t.Errorf("access is %q, want open", got)
	}
	if got := s.Access("turing-1936-computable"); got != AccessPublicDomain {
		t.Errorf("access is %q, want public-domain", got)
	}
}

// A paper nobody has resolved is unknown, not permitted. This is the single
// most important default in the corpus: everything that writes a file asks
// this question first, and the answer for a paper with no record has to be no.
func TestSourcesAccessDefaultsToUnknown(t *testing.T) {
	c := testCorpus(t, threePapers, "", testSources)
	s, err := c.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	got := s.Access("sutskever-2014-seq2seq")
	if got != AccessUnknown {
		t.Fatalf("a paper with no source record is %q, want unknown", got)
	}
	if got.Body() || got.Figures() || got.Abstract() {
		t.Error("a paper with no source record may not be published in any form")
	}
}

func TestSourcesResolved(t *testing.T) {
	c := testCorpus(t, threePapers, "", testSources)
	s, err := c.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Resolved(); got != 2 {
		t.Errorf("Resolved is %d, want 2", got)
	}
	if _, ok := s.ByID("sutskever-2014-seq2seq"); ok {
		t.Error("ByID invented a source record")
	}
}

// An empty sources.yaml is the state a fresh corpus is in, and it has to load
// rather than fail.
func TestSourcesEmpty(t *testing.T) {
	c := testCorpus(t, threePapers, "", "sources: []\n")
	s, err := c.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	if s.Resolved() != 0 {
		t.Error("an empty manifest resolved something")
	}
	if s.Access("vaswani-2017-attention") != AccessUnknown {
		t.Error("an empty manifest granted access")
	}
}

// A record a person decided says so, and that is the only thing standing
// between somebody's afternoon of reading a publisher's terms and a re-run
// that throws it away.
func TestSourcesByHand(t *testing.T) {
	c := testCorpus(t, threePapers, "", testSources)
	s, err := c.LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	hand, ok := s.ByID("turing-1936-computable")
	if !ok {
		t.Fatal("the hand written record is not there")
	}
	if !hand.Hand() {
		t.Errorf("by is %q, so nothing will protect this record", hand.By)
	}
	tool, ok := s.ByID("vaswani-2017-attention")
	if !ok {
		t.Fatal("the resolved record is not there")
	}
	if tool.Hand() {
		t.Error("a record the resolver wrote claims a person wrote it")
	}
	if tool.TextLayer != "native" {
		t.Errorf("the text layer is %q, and it is a layer name and not a flag", tool.TextLayer)
	}
}
