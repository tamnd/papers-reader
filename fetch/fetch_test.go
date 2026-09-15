package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/relay"
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

// The floor is there to catch error pages, not short papers. RFC 896, which
// is Nagle's congestion control paper, is a complete nine page PDF in 16,949
// bytes, because it is text with no images in it. The old twenty kilobyte
// floor threw it away, so this is the case that pins the new one down.
func TestAShortPaperIsStillAPaper(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdf(16949))
	})
	dest := filepath.Join(t.TempDir(), "nagle-1984-congestion.pdf")
	if _, err := fetcher(t).Get(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("a complete sixteen kilobyte paper was refused: %v", err)
	}
}

// The pair of tests that say what the stall clock is for. A download is
// abandoned for going silent and never for taking its time, because a
// departmental server from 2003 serving a large scan slowly is working and a
// whole-request deadline cannot tell the two apart.
func TestASlowDownloadIsNotAStalledOne(t *testing.T) {
	body := pdf(Floor * 2)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		for i := 0; i < len(body); i += 512 {
			w.Write(body[i:min(i+512, len(body))])
			w.(http.Flusher).Flush()
			time.Sleep(5 * time.Millisecond)
		}
	})
	f := fetcher(t)
	// Every chunk arrives well inside the stall window, and the whole
	// download takes many times longer than it.
	f.Stall = 40 * time.Millisecond
	dest := filepath.Join(t.TempDir(), "slow.pdf")
	got, err := f.Get(context.Background(), srv.URL, dest)
	if err != nil {
		t.Fatalf("a slow but healthy download was abandoned: %v", err)
	}
	if got.Bytes != int64(len(body)) {
		t.Errorf("wrote %d bytes of %d", got.Bytes, len(body))
	}
}

func TestAStalledDownloadIsAbandoned(t *testing.T) {
	done := make(chan struct{})
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write(pdf(Floor * 2))
		w.(http.Flusher).Flush()
		// Then nothing, ever, which is what a host that has given up on us
		// without saying so looks like from here.
		<-done
	})
	t.Cleanup(func() { close(done) })

	f := fetcher(t)
	f.Stall = 100 * time.Millisecond
	dir := t.TempDir()
	_, err := f.Get(context.Background(), srv.URL, filepath.Join(dir, "stalled.pdf"))
	if err == nil {
		t.Fatal("a download that stopped sending was accepted")
	}
	if !strings.Contains(err.Error(), "stopped sending") {
		t.Errorf("the error is %q, and it should say the host went silent", err)
	}
	// Enough bytes arrived to pass every other check, so this is also the
	// case where a stall could leave a plausible looking half a paper behind.
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("a stalled download left %d files behind", len(left))
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

// The relay. Three papers in the corpus are held by hosts that answer 403 to
// this network and 200 to another one, so the fetcher gets a second go from
// somewhere else. What it will not do is relax any of the checks, because a
// machine on another network is exactly where an unnoticed captcha page would
// come back from.
//
// These tests run curl through /bin/sh rather than ssh, which is as close to
// a second machine as a test can get without needing one.
func hops(script string) *relay.Set {
	return &relay.Set{Hops: []relay.Hop{{
		Host: script,
		Run: func(ctx context.Context, host string, argv []string) *exec.Cmd {
			return exec.CommandContext(ctx, "/bin/sh", "-c", host)
		},
	}}}
}

func TestAHostThatRefusesThisNetworkIsTriedFromAnother(t *testing.T) {
	body := pdf(Floor + 64)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "go away", http.StatusForbidden)
	})
	far := filepath.Join(t.TempDir(), "far.pdf")
	if err := os.WriteFile(far, body, 0o644); err != nil {
		t.Fatal(err)
	}
	f := fetcher(t)
	f.Relay = hops(fmt.Sprintf("cat %s; printf '200 application/pdf\\n' >&2", far))

	dest := filepath.Join(t.TempDir(), "paper.pdf")
	got, err := f.Get(context.Background(), srv.URL, dest)
	if err != nil {
		t.Fatalf("the relay did not rescue a 403: %v", err)
	}
	if got.Bytes != int64(len(body)) {
		t.Errorf("wrote %d bytes of %d", got.Bytes, len(body))
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("the relayed download is not on disk: %v", err)
	}
}

func TestARelayedDownloadIsCheckedLikeAnyOther(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "go away", http.StatusForbidden)
	})
	f := fetcher(t)
	// A bot wall, served with a 200 and a straight face.
	f.Relay = hops("printf '<html>are you a robot</html>'; printf '200 text/html\\n' >&2")

	dir := t.TempDir()
	_, err := f.Get(context.Background(), srv.URL, filepath.Join(dir, "paper.pdf"))
	if err == nil {
		t.Fatal("an HTML page came back through the relay and was accepted as a paper")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("a refused relay left %d files behind", len(left))
	}
}

func TestBothFailuresAreReported(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "go away", http.StatusForbidden)
	})
	f := fetcher(t)
	f.Relay = hops("printf ''; printf '429 text/html\\n' >&2")

	_, err := f.Get(context.Background(), srv.URL, filepath.Join(t.TempDir(), "paper.pdf"))
	if err == nil {
		t.Fatal("nothing was downloaded and no error came back")
	}
	for _, want := range []string{"403", "429"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error is %q, and it should say what both ends did", err)
		}
	}
}

// A file over the ceiling is over it from every network, and finding that out
// twice costs a second download of something that was too big the first time.
func TestTheCeilingIsNotWorthASecondOpinion(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(pdf(Floor * 4))
	})
	tried := false
	f := fetcher(t)
	f.Ceiling = Floor * 2
	f.Relay = &relay.Set{Hops: []relay.Hop{{
		Host: "never",
		Run: func(ctx context.Context, host string, argv []string) *exec.Cmd {
			tried = true
			return exec.CommandContext(ctx, "/bin/sh", "-c", "true")
		},
	}}}

	_, err := f.Get(context.Background(), srv.URL, filepath.Join(t.TempDir(), "paper.pdf"))
	if !errors.Is(err, ErrTooBig) {
		t.Fatalf("got %v, want the too big error", err)
	}
	if tried {
		t.Error("a book was downloaded a second time through the relay")
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

// The download gate asks one question: is there somewhere to fetch from.
// The copy lands in pdf/, which is never committed and never served, so a
// restricted paper may be read on this machine like any other.
func TestTheDownloadGate(t *testing.T) {
	ok := corpus.Source{ID: "a", Access: corpus.AccessOpen, Licence: "CC BY 4.0", URL: "https://example.test/a.pdf"}
	cases := []struct {
		name string
		rec  *corpus.Source
		want bool
	}{
		{"open with a licence", &ok, true},
		{"public domain", &corpus.Source{ID: "a", Access: corpus.AccessPublicDomain, Licence: "public domain", URL: "u"}, true},
		{"permissive", &corpus.Source{ID: "a", Access: corpus.AccessPermissive, Licence: "arXiv non-exclusive", URL: "u"}, true},
		{"restricted", &corpus.Source{ID: "a", Access: corpus.AccessRestricted, URL: "u"}, true},
		{"open with no licence recorded", &corpus.Source{ID: "a", Access: corpus.AccessOpen, URL: "u"}, true},
		{"unknown, which means nothing was found", &corpus.Source{ID: "a", Access: corpus.AccessUnknown, URL: "u"}, false},
		{"no location", &corpus.Source{ID: "a", Access: corpus.AccessOpen, Licence: "CC BY 4.0"}, false},
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

// The licence gate has not moved. What may be downloaded and what may be
// published are two different questions, and this is the one that decides
// what goes into a public repository.
func TestTheLicenceGate(t *testing.T) {
	cases := []struct {
		name string
		rec  *corpus.Source
		want bool
	}{
		{"open with a licence", &corpus.Source{ID: "a", Access: corpus.AccessOpen, Licence: "CC BY 4.0", URL: "u"}, true},
		{"public domain", &corpus.Source{ID: "a", Access: corpus.AccessPublicDomain, Licence: "public domain", URL: "u"}, true},
		{"permissive", &corpus.Source{ID: "a", Access: corpus.AccessPermissive, Licence: "arXiv non-exclusive", URL: "u"}, true},
		{"restricted, even with the file on disk", &corpus.Source{ID: "a", Access: corpus.AccessRestricted, URL: "u", SHA256: "abc"}, false},
		{"unknown", &corpus.Source{ID: "a", Access: corpus.AccessUnknown, URL: "u"}, false},
		{"open with no licence", &corpus.Source{ID: "a", Access: corpus.AccessOpen, URL: "u"}, false},
		{"no record at all", nil, false},
		{"a class nobody has heard of", &corpus.Source{ID: "a", Access: corpus.Access("free-ish"), URL: "u"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := Publish(nil, tc.rec)
			if got != tc.want {
				t.Errorf("Publish is %v, want %v, because %q", got, tc.want, why)
			}
			if !got && why == "" {
				t.Error("the gate said no and did not say why")
			}
		})
	}
}

// A corpus that has decided to publish every paper in full opens the gate for
// every class, and still says no to a paper it has no record of, because a
// policy about licences is not a way of publishing something nobody has
// identified.
func TestACorpusPolicyOpensTheLicenceGate(t *testing.T) {
	c := &corpus.Corpus{Root: t.TempDir(), Policy: corpus.Policy{Body: true}}
	for _, a := range corpus.Accesses {
		got, why := Publish(c, &corpus.Source{ID: "a", Access: a, URL: "u"})
		if !got {
			t.Errorf("%s is not published under a body policy, because %q", a, why)
		}
	}
	if got, why := Publish(c, nil); got || why == "" {
		t.Error("a paper with no record was published under a body policy")
	}
}
