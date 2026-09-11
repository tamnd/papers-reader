package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/papers-reader/polite"
	"time"
)

// testClient points a client at a test server, with the rate limit off and
// the cache in a temporary directory.
func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient("test", t.TempDir())
	c.HTTP = srv.Client()
	c.Gate.Interval = 0
	return c
}

// serve answers every request from a map of path to body, and counts the
// requests so a test can tell a cache hit from a fetch.
func serve(t *testing.T, routes map[string]string) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		body, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, "no", http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestGetCachesAndReturnsTheSameBytes(t *testing.T) {
	srv, hits := serve(t, map[string]string{"/thing": "hello"})
	c := testClient(t, srv)

	for i := 0; i < 3; i++ {
		body, err := c.Get(context.Background(), srv.URL+"/thing")
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "hello" {
			t.Fatalf("got %q", body)
		}
	}
	if *hits != 1 {
		t.Errorf("the server was asked %d times, want 1", *hits)
	}
}

// A cache entry past its date is not a cache entry. The client's clock is a
// field so this does not involve waiting thirty days.
func TestTheCacheExpires(t *testing.T) {
	srv, hits := serve(t, map[string]string{"/thing": "hello"})
	c := testClient(t, srv)
	now := time.Now()
	c.Now = func() time.Time { return now }

	if _, err := c.Get(context.Background(), srv.URL+"/thing"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(CacheFor + time.Hour)
	if _, err := c.Get(context.Background(), srv.URL+"/thing"); err != nil {
		t.Fatal(err)
	}
	if *hits != 2 {
		t.Errorf("the server was asked %d times, want 2", *hits)
	}
}

// A 404 is an answer and gets cached. A 500 is not and does not, because
// storing an outage for thirty days turns a bad afternoon into a bad month.
func TestOnlyAnswersAreCached(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/broken" {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()
	c := testClient(t, srv)

	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), srv.URL+"/missing"); !NotFound(err) {
			t.Fatalf("a 404 came back as %v", err)
		}
		if _, err := c.Get(context.Background(), srv.URL+"/broken"); err == nil || NotFound(err) {
			t.Fatalf("a 500 came back as %v", err)
		}
	}
	if hits["/missing"] != 1 {
		t.Errorf("the 404 was fetched %d times, want 1", hits["/missing"])
	}
	if hits["/broken"] != 2 {
		t.Errorf("the 500 was fetched %d times, want 2", hits["/broken"])
	}
}

func TestTheClientSaysWhoItIs(t *testing.T) {
	var agent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := testClient(t, srv)
	if _, err := c.Get(context.Background(), srv.URL+"/x"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"papers-reader/test", "github.com/tamnd/papers", polite.Mailto} {
		if !strings.Contains(agent, want) {
			t.Errorf("the user agent %q does not contain %q", agent, want)
		}
	}
}

func TestTheRateLimitHolds(t *testing.T) {
	srv, _ := serve(t, map[string]string{"/a": "x", "/b": "x"})
	c := testClient(t, srv)
	c.Gate.Interval = 40 * time.Millisecond

	start := time.Now()
	for _, p := range []string{"/a", "/b"} {
		if _, err := c.Get(context.Background(), srv.URL+p); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < c.Gate.Interval {
		t.Errorf("two requests to one host took %v, which is under the %v floor", d, c.Gate.Interval)
	}
}

func TestWithMailto(t *testing.T) {
	c := NewClient("test", "")
	for raw, want := range map[string]string{
		"https://api.crossref.org/works/10.1/x":       "https://api.crossref.org/works/10.1/x?mailto=",
		"https://api.openalex.org/works?filter=a%3Ab": "https://api.openalex.org/works?filter=a%3Ab&mailto=",
	} {
		if got := c.withMailto(raw); !strings.HasPrefix(got, want) {
			t.Errorf("withMailto(%q) is %q, want it to start %q", raw, got, want)
		}
	}
}

// An empty cache directory turns the cache off, which is what --no-cache does
// and what most of these tests want.
func TestNoCacheDirMeansNoCache(t *testing.T) {
	srv, hits := serve(t, map[string]string{"/thing": "hello"})
	c := testClient(t, srv)
	c.CacheDir = ""
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), srv.URL+"/thing"); err != nil {
			t.Fatal(err)
		}
	}
	if *hits != 2 {
		t.Errorf("the server was asked %d times, want 2", *hits)
	}
}
