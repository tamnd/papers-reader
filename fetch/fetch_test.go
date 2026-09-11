package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/papers-reader/corpus"
)

// pdf is a PDF only in the sense that matters here: it starts with the magic
// and it is over the floor. No real paper is in this file and none should
// ever be.
func pdf(n int) []byte {
	b := make([]byte, 0, n)
	b = append(b, []byte(Magic+"1.4\n")...)
	for len(b) < n {
		b = append(b, 'x')
	}
	return b
}

func fetcher(t *testing.T) *Fetcher {
	t.Helper()
	f := New("test")
	f.Gate = nil // no waiting in tests
	return f
}

func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetWritesTheFileAndItsHash(t *testing.T) {
	body := pdf(Floor + 100)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(body)
	})
	dest := filepath.Join(t.TempDir(), "deep", "codd-1970-relational.pdf")

	got, err := fetcher(t).Get(context.Background(), srv.URL+"/paper.pdf", dest)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bytes != int64(len(body)) {
		t.Errorf("wrote %d bytes, want %d", got.Bytes, len(body))
	}
	sum, n, err := Sum(dest)
	if err != nil {
		t.Fatal(err)
	}
	if sum != got.SHA256 || n != got.Bytes {
		t.Errorf("the file on disk is %s at %d bytes, the record says %s at %d", sum, n, got.SHA256, got.Bytes)
	}
}

// Every one of these is a real thing a publisher has served in place of a
// paper, and every one of them has to leave no file behind. A file on disk is
// how the rest of the pipeline knows a paper was fetched, so a bad download
// that leaves one is worse than a bad download that fails.
func TestGetRefusesWhatIsNotAPaper(t *testing.T) {
	cases := []struct {
		name   string
		ctype  string
		body   []byte
		status int
		want   string
	}{
		{"a login page", "text/html", []byte("<!DOCTYPE html><title>Sign in</title>" + strings.Repeat(" ", Floor)), 200, "it begins"},
		{"a PDF that is really an error page", "application/pdf", pdf(400), 200, "under the"},
		{"a truncated download", "application/pdf", []byte(Magic), 200, "under the"},
		{"a paywall redirecting to a 403", "text/html", []byte("no"), 403, "403"},
		{"an empty body", "application/pdf", nil, 200, "under the"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ctype)
				w.WriteHeader(tc.status)
				w.Write(tc.body)
			})
			dir := t.TempDir()
			dest := filepath.Join(dir, "paper.pdf")

			_, err := fetcher(t).Get(context.Background(), srv.URL, dest)
			if err == nil {
				t.Fatal("this was accepted as a paper")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error is %q, want it to mention %q", err, tc.want)
			}
			if _, err := os.Stat(dest); !os.IsNotExist(err) {
				t.Error("a refused download left a file behind")
			}
			left, _ := os.ReadDir(dir)
			if len(left) != 0 {
				t.Errorf("a refused download left %d temporary files behind", len(left))
			}
		})
	}
}

// Content-Type is the least trustworthy thing in the response, in both
// directions. A PDF served as octet-stream is still a PDF.
func TestTheBytesDecideAndNotTheHeader(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(pdf(Floor + 1))
	})
	dest := filepath.Join(t.TempDir(), "paper.pdf")
	if _, err := fetcher(t).Get(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("a PDF served as octet-stream was refused: %v", err)
	}
}

func TestGetStopsAtTheCeiling(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(pdf(Floor * 4))
	})
	f := fetcher(t)
	f.Ceiling = Floor * 2
	_, err := f.Get(context.Background(), srv.URL, filepath.Join(t.TempDir(), "paper.pdf"))
	if err == nil || !errors.Is(err, ErrNotPDF) {
		t.Fatalf("a download over the ceiling gave %v", err)
	}
}

func TestGetSaysWhoItIs(t *testing.T) {
	var agent string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		agent = r.Header.Get("User-Agent")
		w.Write(pdf(Floor + 1))
	})
	if _, err := fetcher(t).Get(context.Background(), srv.URL, filepath.Join(t.TempDir(), "p.pdf")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"papers-reader/test", "github.com/tamnd/papers"} {
		if !strings.Contains(agent, want) {
			t.Errorf("the user agent is %q, want it to contain %q", agent, want)
		}
	}
}

// A file URL would read a paper off the local disk and record it as fetched,
// which is a fine way to put something in the corpus that nobody can check.
func TestGetOnlyTalksHTTP(t *testing.T) {
	_, err := fetcher(t).Get(context.Background(), "file:///etc/passwd", filepath.Join(t.TempDir(), "p.pdf"))
	if err == nil || !errors.Is(err, ErrNotPDF) {
		t.Fatalf("a file URL gave %v", err)
	}
}

func TestTheLicenceGate(t *testing.T) {
	ok := corpus.Source{ID: "a", Access: corpus.AccessOpen, Licence: "CC BY 4.0", URL: "https://example.test/a.pdf"}
	cases := []struct {
		name string
		rec  *corpus.Source
		want bool
	}{
		{"open with a licence", &ok, true},
		{"public domain", &corpus.Source{ID: "a", Access: corpus.AccessPublicDomain, Licence: "public domain", URL: "u"}, true},
		{"permissive", &corpus.Source{ID: "a", Access: corpus.AccessPermissive, Licence: "arXiv non-exclusive", URL: "u"}, true},
		{"restricted", &corpus.Source{ID: "a", Access: corpus.AccessRestricted, Licence: "all rights reserved", URL: "u"}, false},
		{"unknown", &corpus.Source{ID: "a", Access: corpus.AccessUnknown, URL: "u"}, false},
		{"open with no licence", &corpus.Source{ID: "a", Access: corpus.AccessOpen, URL: "u"}, false},
		{"open with no location", &corpus.Source{ID: "a", Access: corpus.AccessOpen, Licence: "CC BY 4.0"}, false},
		{"no record at all", nil, false},
		{"a class nobody has heard of", &corpus.Source{ID: "a", Access: corpus.Access("free-ish"), URL: "u"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := May(tc.rec)
			if got != tc.want {
				t.Errorf("May is %v, want %v, because %q", got, tc.want, why)
			}
			if !got && why == "" {
				t.Error("the gate said no and did not say why")
			}
		})
	}
}
