package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// The fixtures below are the shape of each service's response with invented
// content. No copyrighted text is in this file and none should ever be: the
// thing under test is the parsing and the ladder, not any paper.

const arxivAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/1706.03762v7</id>
    <published>2017-06-12T00:00:00Z</published>
    <title>Attention Is All You
      Need</title>
    <summary>A short abstract that stands in for the real one.</summary>
    <author><name>Ashish Vaswani</name></author>
    <author><name>Noam Shazeer</name></author>
    <link href="http://arxiv.org/abs/1706.03762v7" rel="alternate" type="text/html"/>
    <link title="pdf" href="http://arxiv.org/pdf/1706.03762v7" rel="related" type="application/pdf"/>
  </entry>
</feed>`

const arxivOAIRecordXML = `<?xml version="1.0" encoding="UTF-8"?>
<OAI-PMH xmlns="http://www.openarchives.org/OAI/2.0/">
  <GetRecord><record><metadata>
    <arXiv xmlns="http://arxiv.org/OAI/arXiv/">
      <id>1706.03762</id>
      <license>http://creativecommons.org/licenses/by/4.0/</license>
    </arXiv>
  </metadata></record></GetRecord>
</OAI-PMH>`

const crossrefWorkJSON = `{"message":{
  "DOI":"10.1145/3065386",
  "title":["A Title From Crossref"],
  "abstract":"<jats:p>Abstract Some abstract text.</jats:p>",
  "author":[{"given":"E. F.","family":"Codd"}],
  "published-print":{"date-parts":[[1970,6]]},
  "issued":{"date-parts":[[1969]]},
  "link":[{"URL":"https://example.test/tdm.pdf","content-type":"application/pdf","intended-application":"text-mining"},
          {"URL":"https://example.test/paper.pdf","content-type":"application/pdf","intended-application":"similarity-checking"}],
  "license":[{"URL":"https://example.test/all-rights-reserved","content-version":"tdm"},
             {"URL":"http://creativecommons.org/licenses/by/4.0/","content-version":"vor"}],
  "URL":"https://doi.org/10.1145/3065386"}}`

const unpaywallJSON = `{
  "doi":"10.1145/3065386",
  "title":"A Title From Crossref",
  "year":1970,
  "is_oa":true,
  "z_authors":[{"given":"E. F.","family":"Codd"}],
  "best_oa_location":{"url_for_pdf":null,"url_for_landing_page":"https://example.test/landing","license":"cc-by","version":"publishedVersion"},
  "oa_locations":[
    {"url_for_pdf":"https://example.test/accepted.pdf","url_for_landing_page":"https://example.test/a","license":"cc-by","version":"acceptedVersion"},
    {"url_for_pdf":"https://example.test/published.pdf","url_for_landing_page":"https://example.test/p","license":"cc-by","version":"publishedVersion"}]}`

const openalexJSON = `{
  "doi":"https://doi.org/10.1145/3065386",
  "display_name":"A Title From OpenAlex",
  "publication_year":1970,
  "cited_by_count":4211,
  "authorships":[{"author":{"display_name":"E. F. Codd"}}],
  "best_oa_location":{"pdf_url":"https://example.test/oa.pdf","landing_page_url":"https://example.test/oa","license":"cc-by-nc-nd"},
  "abstract_inverted_index":{"a":[0,3],"model":[1],"of":[2],"bank":[4]}}`

func TestParseArXiv(t *testing.T) {
	cands, err := parseArXiv([]byte(arxivAtom))
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("got %d entries", len(cands))
	}
	c := cands[0]
	// The title spans two lines in the feed and has to come back as one line,
	// because a title with a newline in it fails every comparison.
	if c.Title != "Attention Is All You Need" {
		t.Errorf("title is %q", c.Title)
	}
	if c.Year != 2017 {
		t.Errorf("year is %d", c.Year)
	}
	if c.ArXiv != "1706.03762" {
		t.Errorf("id is %q, and the version should be gone", c.ArXiv)
	}
	if c.Landing != "https://arxiv.org/abs/1706.03762" {
		t.Errorf("landing is %q", c.Landing)
	}
	if !strings.HasPrefix(c.URL, "https://") {
		t.Errorf("the pdf url is %q and should have been upgraded to https", c.URL)
	}
	if len(c.Authors) != 2 {
		t.Errorf("authors are %v", c.Authors)
	}
}

func TestCleanArXivID(t *testing.T) {
	for in, want := range map[string]string{
		"1706.03762":                            "1706.03762",
		"1706.03762v7":                          "1706.03762",
		"arXiv:1706.03762v12":                   "1706.03762",
		"http://arxiv.org/abs/1706.03762v7":     "1706.03762",
		"https://arxiv.org/pdf/1312.1719":       "1312.1719",
		"https://arxiv.org/pdf/1312.1719v2.pdf": "1312.1719",
		"cs/9901001":                            "cs/9901001",
		"":                                      "",
	} {
		if got := CleanArXivID(in); got != want {
			t.Errorf("CleanArXivID(%q) is %q, want %q", in, got, want)
		}
	}
}

// A stated licence and the default one are different claims and the caller
// gets told which it has, because "the author chose CC BY" and "the author
// chose nothing and arXiv may redistribute" are not the same permission.
func TestArXivLicence(t *testing.T) {
	licence, stated, err := parseOAILicence([]byte(arxivOAIRecordXML))
	if err != nil {
		t.Fatal(err)
	}
	if !stated || !strings.Contains(licence, "creativecommons") {
		t.Errorf("a stated licence came back as %q, stated=%v", licence, stated)
	}

	const noLicence = `<OAI-PMH><GetRecord><record><metadata><arXiv><id>1</id></arXiv></metadata></record></GetRecord></OAI-PMH>`
	licence, stated, err = parseOAILicence([]byte(noLicence))
	if err != nil {
		t.Fatal(err)
	}
	if stated {
		t.Error("a missing licence element was reported as a stated licence")
	}
	if !strings.Contains(licence, "nonexclusive-distrib") {
		t.Errorf("the default licence is %q", licence)
	}
	// Whichever way it arrived, arXiv's own licence is permissive and not
	// open, because a translation is a derivative and it does not allow one.
	if _, access := Licence(licence); access != corpus.AccessPermissive {
		t.Errorf("the arXiv default came out as %s", access)
	}
}

func TestCrossref(t *testing.T) {
	cand, err := parseCrossrefWork([]byte(crossrefWorkJSON))
	if err != nil {
		t.Fatal(err)
	}
	if cand.Year != 1970 {
		t.Errorf("year is %d, and published-print says 1970 while issued says 1969", cand.Year)
	}
	// A text-mining link is not the paper, it is the licence to mine it.
	if cand.URL != "https://example.test/paper.pdf" {
		t.Errorf("url is %q", cand.URL)
	}
	// Crossref lists every licence a work has ever had. The most permissive
	// wins, because the question is what may be done today.
	if cand.Licence != "CC BY 4.0" {
		t.Errorf("licence is %q", cand.Licence)
	}
	if cand.Abstract != "Some abstract text." {
		t.Errorf("abstract is %q, and should have lost its tags and its heading", cand.Abstract)
	}
}

func TestCleanDOI(t *testing.T) {
	for in, want := range map[string]string{
		"10.1145/3065386":                   "10.1145/3065386",
		"https://doi.org/10.1145/3065386":   "10.1145/3065386",
		"http://dx.doi.org/10.1145/3065386": "10.1145/3065386",
		"doi:10.1145/3065386":               "10.1145/3065386",
		"  10.1145/3065386  ":               "10.1145/3065386",
		"10.1145/ABC":                       "10.1145/abc",
		"not-a-doi":                         "",
		"https://example.test/paper.pdf":    "",
	} {
		if got := CleanDOI(in); got != want {
			t.Errorf("CleanDOI(%q) is %q, want %q", in, got, want)
		}
	}
}

// Unpaywall's own best location sometimes has no PDF, only a landing page,
// and a landing page cannot be extracted.
func TestUnpaywallPrefersALocationWithAPDF(t *testing.T) {
	cand, err := parseUnpaywall([]byte(unpaywallJSON))
	if err != nil {
		t.Fatal(err)
	}
	if cand.URL != "https://example.test/published.pdf" {
		t.Errorf("url is %q, and the published version should have won", cand.URL)
	}
	if cand.Licence != "cc-by" {
		t.Errorf("licence is %q", cand.Licence)
	}
}

func TestOpenAlex(t *testing.T) {
	cand, err := parseOpenAlexWork([]byte(openalexJSON))
	if err != nil {
		t.Fatal(err)
	}
	if cand.DOI != "10.1145/3065386" {
		t.Errorf("doi is %q", cand.DOI)
	}
	if cand.Abstract != "a model of a bank" {
		t.Errorf("the inverted index rebuilt as %q", cand.Abstract)
	}
	// CC BY-NC-ND is permissive, not open, and the difference decides whether
	// a translation may be published.
	name, access := Licence(cand.Licence)
	if access != corpus.AccessPermissive {
		t.Errorf("cc-by-nc-nd came out as %s (%s)", access, name)
	}
}

func TestDeinvertIgnoresNonsense(t *testing.T) {
	if got := deinvert(nil); got != "" {
		t.Errorf("an empty index rebuilt as %q", got)
	}
	// A position far beyond anything a real abstract has is a malformed
	// response, and allocating a slice for it is how a service's bad day
	// becomes this process being killed.
	if got := deinvert(map[string][]int{"x": {1 << 30}}); got != "" {
		t.Errorf("an absurd position rebuilt as %q", got)
	}
}

func TestPublisherRules(t *testing.T) {
	cases := []struct{ landing, pdf, name string }{
		{"https://www.usenix.org/conference/osdi14/technical-sessions/presentation/ongaro",
			"https://www.usenix.org/conference/osdi14/technical-sessions/presentation/ongaro/download", "publisher:usenix"},
		{"https://www.rfc-editor.org/info/rfc896", "https://www.rfc-editor.org/rfc/rfc896.pdf", "publisher:rfc-editor"},
		{"https://datatracker.ietf.org/doc/html/rfc896", "https://www.rfc-editor.org/rfc/rfc896.pdf", "publisher:ietf-datatracker"},
		{"https://dl.acm.org/doi/10.1145/362384.362685", "https://dl.acm.org/doi/pdf/10.1145/362384.362685", "publisher:acm"},
		{"https://dl.acm.org/doi/abs/10.1145/362384.362685", "https://dl.acm.org/doi/pdf/10.1145/362384.362685", "publisher:acm"},
		{"https://link.springer.com/article/10.1007/BF01386390", "https://link.springer.com/content/pdf/10.1007/BF01386390.pdf", "publisher:springer"},
		{"https://bitcoin.org/en/bitcoin-paper", "https://bitcoin.org/bitcoin.pdf", "publisher:bitcoin.org"},
		{"https://www.dougengelbart.org/pubs/augment-3906.html", "https://www.dougengelbart.org/pubs/augment-3906.pdf", "publisher:engelbart-institute"},
	}
	for _, tc := range cases {
		cand, ok := Publisher(tc.landing)
		if !ok {
			t.Errorf("no rule claimed %s", tc.landing)
			continue
		}
		if cand.URL != tc.pdf {
			t.Errorf("%s gave %q, want %q", tc.landing, cand.URL, tc.pdf)
		}
		if cand.Source != tc.name {
			t.Errorf("%s was handled by %q, want %q", tc.landing, cand.Source, tc.name)
		}
	}
}

// Returning nothing is the right answer far more often than guessing, so a
// host a rule claims but a path it does not recognise has to come back empty.
func TestPublisherRulesDeclineWhatTheyDoNotKnow(t *testing.T) {
	for _, landing := range []string{
		"https://www.usenix.org/publications/login",
		"https://dl.acm.org/profile/81100123456",
		"https://link.springer.com/journal/236",
		"https://example.test/whatever",
		"not a url at all",
		"",
	} {
		if cand, ok := Publisher(landing); ok {
			t.Errorf("a rule claimed %q and returned %q", landing, cand.URL)
		}
	}
}

// A publisher rule may say what a site's rights position is, and only a site
// that really does publish everything under one licence may name a licence.
// The difference matters: ACM's position is that it holds the rights, which
// is worth recording, and it is not a licence and must never be written into
// the record as one.
func TestWhatAPublisherRuleMaySay(t *testing.T) {
	for _, name := range []string{"usenix", "rfc-editor", "ietf-datatracker", "bitcoin.org"} {
		access, ok := PublisherAccess(name)
		if !ok || access != corpus.AccessOpen {
			t.Errorf("%s is an open site and came out %s, ok=%v", name, access, ok)
		}
		if publisherLicence(name) == "" {
			t.Errorf("%s publishes everything under one licence and does not name it", name)
		}
	}
	for _, name := range []string{"acm", "springer", "elsevier", "ieee", "wiley"} {
		access, ok := PublisherAccess(name)
		if !ok || access != corpus.AccessRestricted {
			t.Errorf("%s holds its own rights and came out %s, ok=%v", name, access, ok)
		}
		if l := publisherLicence(name); l != "" {
			t.Errorf("%s handed out the licence %q, and all rights reserved is not a licence", name, l)
		}
	}
	// A site whose rights position nobody knows says nothing at all, and
	// nothing at all is the right answer.
	for _, name := range []string{"llvm.org", "engelbart-institute"} {
		if access, ok := PublisherAccess(name); ok {
			t.Errorf("%s handed out %s from a URL shape", name, access)
		}
	}
}

// Rights follow the host and not the rung. A copy sitting on a publisher's
// own site is under that publisher's terms whoever linked to it, and a copy
// on a university web server says nothing about rights however respectable
// the university.
func TestHostRule(t *testing.T) {
	cases := map[string]corpus.Access{
		"https://dl.acm.org/doi/pdf/10.1145/359545.359563":                           corpus.AccessRestricted,
		"https://www.usenix.org/legacy/events/osdi04/tech/full_papers/dean/dean.pdf": corpus.AccessOpen,
		"https://academic.oup.com/comjnl/article-pdf/5/1/10/x.pdf":                   corpus.AccessRestricted,
		"https://www.cs.virginia.edu/~robins/Turing_Paper_1936.pdf":                  corpus.AccessUnknown,
		"https://people.eecs.berkeley.edu/~brewer/cs262b/x.pdf":                      corpus.AccessUnknown,
		"not a url at all": corpus.AccessUnknown,
	}
	for raw, want := range cases {
		got := corpus.AccessUnknown
		if r, ok := HostRule(raw); ok {
			got = r.Access
		}
		if got != want {
			t.Errorf("%s came out %s, want %s", raw, got, want)
		}
	}
}

// A paper that was found and has no licence on it is restricted, not
// unknown. All rights reserved is what the law says about a work that does
// not say otherwise, so restricted is not a guess, and it publishes a great
// deal more than unknown does, which publishes nothing at all.
func TestAPaperWithNoLicenceIsRestricted(t *testing.T) {
	res := &Result{}
	rec := record(corpus.Paper{ID: "x-1970-y"}, Candidate{
		URL:    "https://example.test/x.pdf",
		DOI:    "10.1145/3065386",
		Source: "crossref",
	}, res)
	if rec.Access != corpus.AccessRestricted {
		t.Errorf("a published paper with no licence came out %s", rec.Access)
	}
	if rec.Licence != "" {
		t.Errorf("it was given the licence %q, and nobody stated one", rec.Licence)
	}

	// A tech report on a university server has no DOI and no licence either,
	// and its rights are no different for that.
	res = &Result{}
	rec = record(corpus.Paper{ID: "x-1970-y"}, Candidate{URL: "https://example.test/x.pdf", Source: "seed"}, res)
	if rec.Access != corpus.AccessRestricted {
		t.Errorf("a file found with no DOI and no licence came out %s", rec.Access)
	}

	// Nothing was found at all, so there is nothing to say and nothing to
	// publish. This is what unknown is for.
	res = &Result{}
	rec = record(corpus.Paper{ID: "x-1970-y"}, Candidate{Source: "seed"}, res)
	if rec.Access != corpus.AccessUnknown {
		t.Errorf("a paper that resolved to nothing came out %s", rec.Access)
	}
}

// The manifest has to say which papers a person found. Half this corpus is
// pre-web work that no API has a copy of, and somebody sat and looked for
// each one, so a record that does not say so reads as if a service found it.
func TestTheRecordSaysWhoFoundIt(t *testing.T) {
	cases := map[string]string{
		"pin":       corpus.ByPin,
		"seed":      corpus.BySeed,
		"arxiv":     "",
		"crossref":  "",
		"unpaywall": "",
		"openalex":  "",
		"publisher": "",
	}
	for rung, want := range cases {
		res := &Result{Rung: rung}
		rec := record(corpus.Paper{ID: "x-1970-y"}, Candidate{URL: "https://example.test/x.pdf", Source: rung}, res)
		if rec.By != want {
			t.Errorf("a paper accepted at the %s rung has by %q, want %q", rung, rec.By, want)
		}
		if rec.Chosen() != (want != "") {
			t.Errorf("Chosen is %v for the %s rung", rec.Chosen(), rung)
		}
		// A pin is a location, not a licence, so it must not claim to be one
		// a person decided. Only `by: hand` freezes a record.
		if rec.Hand() {
			t.Errorf("the %s rung wrote a record that no re-run will ever correct", rung)
		}
	}
}

func TestLicenceTable(t *testing.T) {
	cases := map[string]corpus.Access{
		"http://creativecommons.org/publicdomain/zero/1.0/": corpus.AccessPublicDomain,
		"cc0": corpus.AccessPublicDomain,
		"http://creativecommons.org/licenses/by/4.0/": corpus.AccessOpen,
		"cc-by":       corpus.AccessOpen,
		"cc-by-sa":    corpus.AccessOpen,
		"cc-by-nc":    corpus.AccessPermissive,
		"cc-by-nc-nd": corpus.AccessPermissive,
		"http://arxiv.org/licenses/nonexclusive-distrib/1.0/": corpus.AccessPermissive,
		"all rights reserved": corpus.AccessRestricted,
		"":                    corpus.AccessUnknown,
		"some licence nobody has written a rule for": corpus.AccessUnknown,
	}
	for in, want := range cases {
		if _, got := Licence(in); got != want {
			t.Errorf("Licence(%q) is %s, want %s", in, got, want)
		}
	}
}

// The ordering of the table is load bearing: "cc-by-nc" contains "cc-by", and
// a table checked in the wrong order would call every restricted licence open
// and publish translations of papers that forbid derivatives.
func TestTheLicenceTableIsOrderedSpecificFirst(t *testing.T) {
	for _, s := range []string{"cc-by-nc", "cc-by-nc-nd", "cc-by-nc-sa", "cc-by-nd", "by-nc-nd"} {
		if _, got := Licence(s); got == corpus.AccessOpen {
			t.Errorf("%q was classified open", s)
		}
	}
}

// An unrecognised licence is echoed back so a person reading the report can
// see the string and write a rule for it.
func TestAnUnknownLicenceKeepsItsText(t *testing.T) {
	name, access := Licence("Publisher Special Terms 2019")
	if access != corpus.AccessUnknown {
		t.Errorf("access is %s", access)
	}
	if name != "Publisher Special Terms 2019" {
		t.Errorf("the string was lost, got %q", name)
	}
}

// ladder wires a resolver to a test server and points every service's base
// URL at it, so the whole ladder runs and `go test ./...` touches no network.
// A test that reaches the real arXiv is a test that fails on a train.
func ladder(t *testing.T, h http.HandlerFunc) *Resolver {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := testClient(t, srv)
	c.Base = Endpoints{
		ArXiv:     srv.URL + "/arxiv/query",
		ArXivOAI:  srv.URL + "/arxiv/oai2",
		Crossref:  srv.URL + "/crossref/works",
		Unpaywall: srv.URL + "/unpaywall/v2",
		OpenAlex:  srv.URL + "/openalex/works",
	}
	return &Resolver{Client: c}
}

func TestResolveTakesThePinFirst(t *testing.T) {
	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("a pinned paper still asked the network for %s", req.URL)
		http.Error(w, "no", http.StatusNotFound)
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:    "codd-1970-relational",
		Title: "A Relational Model of Data for Large Shared Data Banks",
		Year:  1970,
		URL:   "https://example.test/codd.pdf",
	})
	if !res.OK() {
		t.Fatalf("a pinned url did not resolve: %+v", res)
	}
	if res.Rung != "pin" {
		t.Errorf("resolved off %q, want pin", res.Rung)
	}
	// A pin says where the file is. It says nothing about the licence, so the
	// paper is restricted and restricted publishes no body text.
	if res.Record.Access != corpus.AccessRestricted {
		t.Errorf("a bare pinned url produced access %s", res.Record.Access)
	}
	if res.Record.Licence != "" {
		t.Errorf("a bare pinned url produced the licence %q", res.Record.Licence)
	}
}

// An arXiv pin says where the file is, and the Atom feed the pin is read from
// does not carry a licence. The OAI interface does, so the resolver goes and
// asks it rather than leaving the paper unknown.
func TestAnArXivPinPicksUpItsLicence(t *testing.T) {
	var asked int
	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/arxiv/query":
			w.Write([]byte(arxivAtom))
		case "/arxiv/oai2":
			asked++
			w.Write([]byte(arxivOAIRecordXML))
		default:
			t.Errorf("a pinned paper asked %s as well", req.URL)
			http.Error(w, "no", http.StatusNotFound)
		}
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:    "vaswani-2017-attention",
		Title: "Attention Is All You Need",
		Year:  2017,
		ArXiv: "1706.03762",
	})
	if !res.OK() {
		t.Fatalf("a pinned arXiv id did not resolve: %+v", res)
	}
	if res.Rung != "pin" {
		t.Errorf("resolved off %q, want pin", res.Rung)
	}
	if asked != 1 {
		t.Errorf("the OAI interface was asked %d times, want once", asked)
	}
	if res.Record.Access != corpus.AccessOpen {
		t.Errorf("a CC-BY submission came out as %s", res.Record.Access)
	}
}

// A licence arXiv only assumes is not a licence the author gave, and the
// record has to say so rather than quietly claiming the permission.
func TestTheArXivDefaultLicenceIsCalledOut(t *testing.T) {
	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/arxiv/query":
			w.Write([]byte(arxivAtom))
		default:
			w.Write([]byte(`<OAI-PMH><GetRecord><record><metadata><arXiv><id>1706.03762</id></arXiv></metadata></record></GetRecord></OAI-PMH>`))
		}
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:    "vaswani-2017-attention",
		Title: "Attention Is All You Need",
		Year:  2017,
		ArXiv: "1706.03762",
	})
	if res.Record.Access != corpus.AccessPermissive {
		t.Errorf("the arXiv default came out as %s", res.Record.Access)
	}
	var said bool
	for _, n := range res.Notes {
		if strings.Contains(n, "chose no licence") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing said the licence was arXiv's and not the author's: %q", res.Notes)
	}
}

// The seed list is the last rung, not the first. It is a reading list, and a
// link on a reading list is not a licence.
func TestResolveFallsBackToTheSeed(t *testing.T) {
	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	})
	r.Skip = map[string]bool{"arxiv": true, "crossref": true, "unpaywall": true, "openalex": true}
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:    "hoare-1962-quicksort",
		Title: "Quicksort",
		Year:  1962,
		Seed:  "https://example.test/quicksort.pdf",
	})
	if !res.OK() {
		t.Fatalf("the seed was not used: %+v", res)
	}
	if res.Rung != "seed" {
		t.Errorf("resolved off %q, want seed", res.Rung)
	}
	if res.Record.Licence != "" {
		t.Errorf("a seed url produced the licence %q, and a reading list is not a licence", res.Record.Licence)
	}
	if res.Record.Access == corpus.AccessOpen || res.Record.Access == corpus.AccessPublicDomain {
		t.Errorf("a seed url produced access %s, and a reading list cannot say that", res.Record.Access)
	}
}

// The whole ladder, end to end, on a paper with nothing pinned. arXiv has
// never heard of it, Crossref supplies the DOI, and Unpaywall supplies the
// copy and the licence.
func TestResolveWalksTheLadder(t *testing.T) {
	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasPrefix(req.URL.Path, "/arxiv/"):
			w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"></feed>`))
		case strings.HasPrefix(req.URL.Path, "/crossref/"):
			// A search result, and a work with no link, which is the usual
			// case: Crossref knows the DOI and not where a copy is.
			w.Write([]byte(`{"message":{"items":[{"DOI":"10.1145/3065386",
			  "title":["A Title From Crossref"],
			  "author":[{"given":"E. F.","family":"Codd"}],
			  "published-print":{"date-parts":[[1970,6]]}}]}}`))
		case strings.HasPrefix(req.URL.Path, "/unpaywall/"):
			w.Write([]byte(unpaywallJSON))
		default:
			http.Error(w, "no", http.StatusNotFound)
		}
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:      "codd-1970-relational",
		Title:   "A Title From Crossref",
		Authors: []string{"E. F. Codd"},
		Year:    1970,
		Expect:  corpus.AccessOpen,
	})
	if !res.OK() {
		t.Fatalf("nothing resolved: notes=%v misses=%v", res.Notes, res.Misses)
	}
	if res.Rung != "unpaywall" {
		t.Errorf("resolved off %q, want unpaywall, because Crossref gave a DOI and no fetchable copy", res.Rung)
	}
	if res.Record.URL != "https://example.test/published.pdf" {
		t.Errorf("url is %q", res.Record.URL)
	}
	if res.Record.Access != corpus.AccessOpen || res.Record.Licence != "CC BY 4.0" {
		t.Errorf("record is %s / %q", res.Record.Access, res.Record.Licence)
	}
}

// A refused candidate is kept with the score that refused it, because the
// difference between "nothing found" and "OpenAlex offered this and the year
// was 55 out" is the difference between twenty minutes and two.
func TestResolveRecordsNearMisses(t *testing.T) {
	const junk = `{"results":[{"doi":"https://doi.org/10.9999/fabricated",
	  "display_name":"A Paper With A Similar Sounding Name",
	  "publication_year":2025,
	  "authorships":[{"author":{"display_name":"Nobody At All"}}],
	  "best_oa_location":{"pdf_url":"https://example.test/junk.pdf","license":"cc-by"}}]}`

	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/openalex/") {
			w.Write([]byte(junk))
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:      "vaswani-2017-attention",
		Title:   "Attention Is All You Need",
		Authors: []string{"Ashish Vaswani"},
		Year:    2017,
	})
	if res.OK() {
		t.Fatalf("the fabricated record was accepted: %+v", res.Accepted)
	}
	if len(res.Misses) == 0 {
		t.Fatal("the refusal was not recorded")
	}
	m := res.Misses[0]
	if m.Rung != "openalex" {
		t.Errorf("the miss is attributed to %q", m.Rung)
	}
	if m.Verdict.Why == "" || m.Verdict.TitleScore >= TitleFloor {
		t.Errorf("verdict is %+v", m.Verdict)
	}
}

// A paper resolved to a copy nobody has a licence for publishes nothing, and
// says so, rather than quietly recording a URL that reads like permission.
func TestResolveSaysWhenTheLicenceIsUnknown(t *testing.T) {
	const noLicence = `{"doi":"10.1145/3065386","title":"A Title From Crossref","year":1970,
	  "z_authors":[{"given":"E. F.","family":"Codd"}],
	  "best_oa_location":{"url_for_pdf":"https://example.test/x.pdf","license":"publisher specific terms"}}`

	r := ladder(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasPrefix(req.URL.Path, "/crossref/"):
			w.Write([]byte(`{"message":{"items":[{"DOI":"10.1145/3065386","title":["A Title From Crossref"],
			  "author":[{"given":"E. F.","family":"Codd"}],"published-print":{"date-parts":[[1970]]}}]}}`))
		case strings.HasPrefix(req.URL.Path, "/unpaywall/"):
			w.Write([]byte(noLicence))
		default:
			http.Error(w, "no", http.StatusNotFound)
		}
	})
	res := r.Resolve(context.Background(), corpus.Paper{
		ID:      "codd-1970-relational",
		Title:   "A Title From Crossref",
		Authors: []string{"E. F. Codd"},
		Year:    1970,
		Expect:  corpus.AccessOpen,
	})
	if !res.OK() {
		t.Fatalf("nothing resolved: %v %v", res.Notes, res.Misses)
	}
	// Nobody has a rule for that licence, so it gets the cautious class and a
	// note. What it must not get is open, which is the only thing a missing
	// rule could turn into a mistake nobody notices.
	if res.Record.Access != corpus.AccessRestricted {
		t.Errorf("access is %s, and nobody has a rule for that licence", res.Record.Access)
	}
	var sawLicence, sawExpect bool
	for _, n := range res.Notes {
		if strings.Contains(n, "no rule for the licence") {
			sawLicence = true
		}
		if strings.Contains(n, "expected open") {
			sawExpect = true
		}
	}
	if !sawLicence {
		t.Errorf("the unrecognised licence was not reported: %v", res.Notes)
	}
	// The manifest guessed open and the licence says otherwise. That
	// disagreement is the whole reason expect is a separate field.
	if !sawExpect {
		t.Errorf("the disagreement with expect was not reported: %v", res.Notes)
	}
}

// The one that matters. A candidate whose title negates the paper's is not
// the paper, however close the characters are.
func TestResolveRefusesANegation(t *testing.T) {
	r := &Resolver{}
	want := Want{Title: "Attention Is All You Need", Year: 2017, Authors: []string{"Ashish Vaswani"}}
	got := Candidate{Title: "Attention Is Not All You Need", Year: 2017, Authors: []string{"Ashish Vaswani"}}
	if v := Verify(want, got); v.OK {
		t.Fatalf("a negated title was accepted with score %.3f", v.TitleScore)
	}
	_ = r
}

func TestAbsorbDoesNotOverwrite(t *testing.T) {
	known := Candidate{DOI: "10.1/first", Title: "The First Title"}
	absorb(&known, Candidate{DOI: "10.1/second", Title: "The Second Title", URL: "https://example.test/x.pdf", Year: 1970})
	if known.DOI != "10.1/first" || known.Title != "The First Title" {
		t.Errorf("a later rung overwrote an earlier one: %+v", known)
	}
	if known.URL != "https://example.test/x.pdf" || known.Year != 1970 {
		t.Errorf("a later rung did not fill in the gaps: %+v", known)
	}
}

// The licence is the exception to absorb's rule. A paper on arXiv under one
// licence and in a repository under another means both are true, so the most
// permissive wins rather than the first seen.
func TestAbsorbTakesTheBestLicence(t *testing.T) {
	known := Candidate{Licence: "all rights reserved"}
	absorb(&known, Candidate{Licence: "cc-by"})
	if known.Licence != "cc-by" {
		t.Errorf("licence is %q, want cc-by", known.Licence)
	}
	absorb(&known, Candidate{Licence: "cc-by-nc-nd"})
	if known.Licence != "cc-by" {
		t.Errorf("a worse licence replaced a better one, got %q", known.Licence)
	}
}

func TestMarkdownPutsTheWorkFirst(t *testing.T) {
	ok := &Result{ID: "a-1970-one", Rung: "arxiv", Accepted: &Candidate{}, Record: corpus.Source{Access: corpus.AccessOpen, Licence: "CC BY 4.0"}}
	bad := &Result{ID: "b-1971-two", Misses: []Miss{{
		Rung:      "openalex",
		Candidate: Candidate{Title: "Something Else Entirely", Year: 2025},
		Verdict:   Verdict{TitleScore: 0.41, YearOff: 54, Why: "the titles are too different"},
	}}}
	md := Markdown([]*Result{ok, bad})

	if strings.Index(md, "## Not resolved") > strings.Index(md, "## Resolved\n") {
		t.Error("the resolved list comes before the work")
	}
	for _, want := range []string{"b-1971-two", "Something Else Entirely", "0.410", "+54", "the titles are too different", "a-1970-one", "CC BY 4.0"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not mention %q", want)
		}
	}
}

// A run over four papers used to leave behind a report that said four
// papers, four resolved, which reads as the state of the corpus and is not.
// Prior is how the papers a run did not touch get into the report anyway.
func TestAPriorRecordStillCounts(t *testing.T) {
	fresh := &Result{ID: "a-1970-one", Rung: "pin", Accepted: &Candidate{}, Record: corpus.Source{Access: corpus.AccessRestricted}}
	old := Prior(corpus.Source{
		ID:      "b-1971-two",
		Access:  corpus.AccessOpen,
		Licence: "CC BY 4.0",
		URL:     "https://example.test/b.pdf",
	})
	if !old.OK() {
		t.Fatal("a record with a url did not count as resolved")
	}
	never := Prior(corpus.Source{ID: "c-1972-three"})
	if never.OK() {
		t.Error("a record with no url counted as resolved")
	}

	md := Markdown([]*Result{fresh, old, never})
	if !strings.Contains(md, "3 papers, 2 resolved, 1 not") {
		t.Error("the report counted the run and not the corpus")
	}
	for _, want := range []string{"b-1971-two", "CC BY 4.0", "recorded earlier", "c-1972-three"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report does not mention %q", want)
		}
	}
}
