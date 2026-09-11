package corpus

import (
	"strings"
	"testing"
)

const testCollections = `collections:
  - id: canon-100
    title: The hundred
    order: number
    members: all-with-number
  - id: everything
    title: Everything in the corpus
    order: number
    members: all
  - id: llm-path
    title: How we got to the transformer
    order: manual
    members:
      - vaswani-2017-attention
      - sutskever-2014-seq2seq
  - id: prereq
    title: In prerequisite order
    order: topological
    members:
      - vaswani-2017-attention
      - sutskever-2014-seq2seq
`

func TestCollectionMembersWord(t *testing.T) {
	c := testCorpus(t, threePapers, testCollections, "")
	cols, err := c.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	canon, ok := cols.ByID("canon-100")
	if !ok {
		t.Fatal("canon-100 is missing")
	}
	if !canon.Members.All || !canon.Members.WithNumber {
		t.Errorf("all-with-number parsed as %+v", canon.Members)
	}
	if _, ok := cols.ByID("no-such-list"); ok {
		t.Error("ByID invented a collection")
	}
}

func TestCollectionResolve(t *testing.T) {
	c := testCorpus(t, threePapers, testCollections, "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	cols, err := c.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ id, want string }{
		{"canon-100", "turing-1936-computable sutskever-2014-seq2seq vaswani-2017-attention"},
		{"everything", "turing-1936-computable sutskever-2014-seq2seq vaswani-2017-attention"},
		{"llm-path", "vaswani-2017-attention sutskever-2014-seq2seq"},
		{"prereq", "sutskever-2014-seq2seq vaswani-2017-attention"},
	}
	for _, tc := range cases {
		col, ok := cols.ByID(tc.id)
		if !ok {
			t.Fatalf("%s is missing", tc.id)
		}
		got, err := col.Resolve(m)
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if strings.Join(got, " ") != tc.want {
			t.Errorf("%s resolves to %v, want %s", tc.id, got, tc.want)
		}
	}
}

// A collection that names a paper the corpus does not have is a mistake in
// the manifest, and saying so beats quietly presenting a shorter reading list.
func TestCollectionRejectsAStranger(t *testing.T) {
	bad := `collections:
  - id: broken
    title: Names a paper we do not have
    members: [nobody-1999-nothing]
`
	c := testCorpus(t, threePapers, bad, "")
	m, err := c.LoadPapers()
	if err != nil {
		t.Fatal(err)
	}
	cols, err := c.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	col, _ := cols.ByID("broken")
	if _, err := col.Resolve(m); err == nil {
		t.Fatal("a collection naming a paper outside the corpus resolved")
	}
}

func TestCollectionRejectsAnUnknownWord(t *testing.T) {
	bad := `collections:
  - id: broken
    title: Members is a word nobody defined
    members: most
`
	c := testCorpus(t, threePapers, bad, "")
	if _, err := c.LoadCollections(); err == nil {
		t.Fatal("members: most was accepted")
	}
}
