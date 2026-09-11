package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tamnd/papers-reader/polite"
	"github.com/tamnd/papers-reader/relay"
)

// Magic is the first five bytes of every PDF ever written. A file that does
// not start with it is not a PDF whatever the server called it, and servers
// call all sorts of things PDFs: a login page, a cookie wall, an interstitial
// asking whether you are a robot.
const Magic = "%PDF-"

// Floor is the smallest thing worth believing is a paper.
//
// What lands under the floor is an error page that happens to be a valid PDF,
// which publishers do produce, and a truncated download.
//
// This was twenty kilobytes, on the reasoning that the shortest real paper in
// the corpus is a couple of hundred. That reasoning was drawn from scans, and
// it is wrong for anything born digital with no images in it. RFC 896, which
// is Nagle's congestion control paper, is a complete nine page PDF in 16,949
// bytes, and the old floor threw it away. Eight kilobytes still catches every
// publisher error page we have seen, which run from a few hundred bytes to
// about five thousand.
const Floor = 8 << 10

// Ceiling is the largest download that will be accepted. A paper that is
// bigger than this is a book, a dataset or a mistake, and in all three cases
// stopping is better than filling a disk.
const Ceiling = 256 << 20

// ErrNotPDF is what the guard returns. It is one error for every way a
// download can fail to be a paper, because the caller does the same thing
// with all of them: leave the record alone and say so.
var ErrNotPDF = errors.New("not a PDF")

// ErrTooBig is the one way of failing that is worth telling apart, because it
// is the one that a second attempt cannot fix. Everything else about a
// download can come out differently from another network. A file's size
// cannot.
var ErrTooBig = errors.New("bigger than a paper")

// Fetcher downloads PDFs and refuses everything else.
//
// It writes nothing outside the directory it is given, and it writes by
// rename, so an interrupted run leaves either the previous file or no file
// and never a half of one.
type Fetcher struct {
	HTTP      *http.Client
	UserAgent string
	Gate      *polite.Gate
	// Floor and Ceiling are the size bounds. Zero means the default.
	Floor, Ceiling int64
	// Stall is how long a download may go without a byte arriving before it
	// is abandoned, and Whole is the outer bound on the whole request. Zero
	// means the default for both.
	Stall, Whole time.Duration
	// Relay is the machines to try when this network cannot reach a host.
	// Empty, which is the usual state, means every download goes out from
	// here and a host that refuses us has refused us.
	Relay *relay.Set
}

// The timeouts. A run over the whole corpus is a hundred requests and some
// of the hosts will not answer at all, so the question is not how patient to
// be with one paper but how long the other ninety nine should wait for it.
//
// Headers is the first one. A publisher that is going to serve the file
// starts doing so quickly; a publisher that is stalling a program it does
// not want stalls from the first byte.
//
// Stall is the one that matters for the download itself, and it took two
// tries to work out why. A whole-request deadline was two minutes, then five,
// and both lost papers, because a departmental web server from 2003 serving a
// 40 megabyte scan at thirty kilobytes a second is slow and is working. The
// deadline was measuring the wrong thing. What distinguishes a host that is
// never going to finish from one that is merely slow is not how long it has
// taken, it is whether bytes are still arriving. So the clock resets on every
// read and only a download that has gone silent is given up on.
//
// Whole is left as an outer bound on the pathological case, a server that
// dribbles a byte at a time forever. Nothing real should ever reach it.
const (
	Headers = 20 * time.Second
	Stall   = 45 * time.Second
	Whole   = 30 * time.Minute
)

// New returns a fetcher with the manners on.
func New(version string) *Fetcher {
	return &Fetcher{
		// No Timeout on the client. An absolute deadline is what the stall
		// clock replaces, and having both would put the old bug back.
		HTTP: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: Headers,
			},
		},
		UserAgent: polite.UserAgent(version),
		Gate:      &polite.Gate{Interval: polite.MinInterval},
		Floor:     Floor,
		Ceiling:   Ceiling,
		Stall:     Stall,
		Whole:     Whole,
	}
}

// File is what one successful download turned out to be.
type File struct {
	Path    string
	SHA256  string
	Bytes   int64
	Fetched time.Time
}

// Get downloads one URL to one path.
//
// Everything about it is suspicious by default. The response has to be a 200,
// it has to begin with the PDF magic, it has to be over the floor and under
// the ceiling, and only then does anything reach the destination path. A
// download that fails any of those leaves no file behind, because a file on
// disk is what the rest of the pipeline reads as "this paper was fetched".
func (f *Fetcher) Get(ctx context.Context, raw, dest string) (File, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return File{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return File{}, fmt.Errorf("%s: %w: only http and https are fetched", raw, ErrNotPDF)
	}

	file, err := f.direct(ctx, u, raw, dest)
	switch {
	case err == nil:
		return file, nil
	case f.Relay.Empty():
		return File{}, err
	case errors.Is(err, ErrTooBig):
		// A book is a book from every network, and downloading it twice to
		// find that out again would be the only thing worse than once.
		return File{}, err
	}
	file, second := f.relayed(ctx, u, raw, dest)
	if second != nil {
		return File{}, fmt.Errorf("%v; and through %s: %v", err, f.Relay, second)
	}
	return file, nil
}

// direct downloads from here, which is what happens almost every time.
func (f *Fetcher) direct(ctx context.Context, u *url.URL, raw, dest string) (File, error) {
	f.gate().Wait(u.Host)

	ctx, cancel := context.WithTimeout(ctx, f.whole())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return File{}, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/pdf,*/*;q=0.5")

	resp, err := f.client().Do(req)
	if err != nil {
		return File{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return File{}, fmt.Errorf("%s: %s", raw, resp.Status)
	}

	return f.into(raw, dest, resp.Header.Get("Content-Type"), func(w io.Writer) (int64, error) {
		body := watch(resp.Body, f.stall(), cancel)
		defer body.stop()
		n, err := io.Copy(w, io.LimitReader(body, f.ceiling()+1))
		if err != nil && body.stalled() {
			return n, fmt.Errorf("%s: nothing arrived for %s after %d bytes, so the host has stopped sending", raw, f.stall(), n)
		}
		return n, err
	})
}

// relayed asks another machine to make the same request.
//
// The checks afterwards are the ones every download gets. A relay moves where
// the request leaves from and changes nothing about what is accepted, which
// matters, because a machine somewhere else is exactly the place a bot wall
// or a captcha page would come back from unnoticed.
func (f *Fetcher) relayed(ctx context.Context, u *url.URL, raw, dest string) (File, error) {
	var last error
	for _, hop := range f.Relay.Hops {
		f.gate().Wait(u.Host)

		var res relay.Response
		file, err := f.into(raw, dest, "", func(w io.Writer) (int64, error) {
			counted := &counter{w: w, max: f.ceiling(), over: f.tooBig(raw)}
			r, err := hop.Get(ctx, raw, relay.Options{
				UserAgent: f.UserAgent,
				Stall:     f.stall(),
				Whole:     f.whole(),
			}, counted)
			res = r
			return counted.n, err
		})
		if err == nil && res.Code != http.StatusOK {
			err = fmt.Errorf("%s: %s said %d", raw, hop.Host, res.Code)
		}
		if err == nil {
			return file, nil
		}
		last = err
	}
	return File{}, last
}

// into writes a download to a temporary file beside its destination, checks
// that what arrived is a paper, and only then moves it into place.
//
// Nothing reaches dest until every check has passed, so an interrupted run
// leaves either the previous file or no file. The two callers differ only in
// where the bytes come from.
func (f *Fetcher) into(raw, dest, ctype string, fill func(io.Writer) (int64, error)) (File, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return File{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".fetch-*")
	if err != nil {
		return File{}, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	sum := sha256.New()
	n, err := fill(io.MultiWriter(tmp, sum))
	if err != nil {
		return File{}, err
	}
	if err := tmp.Close(); err != nil {
		return File{}, err
	}

	if err := f.check(tmp.Name(), raw, ctype, n); err != nil {
		return File{}, err
	}
	// CreateTemp makes a file only its owner can read, which is right for a
	// temporary file and wrong for a paper.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return File{}, err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return File{}, err
	}
	return File{
		Path:    dest,
		SHA256:  hex.EncodeToString(sum.Sum(nil)),
		Bytes:   n,
		Fetched: time.Now().UTC(),
	}, nil
}

// counter counts what goes through it and stops at the ceiling.
//
// A relayed download is pushed into a writer rather than pulled from a
// reader, so there is nothing for io.LimitReader to wrap and the limit has to
// live on the writing side. Erroring is what stops it: the error travels back
// up through the copy, the command is killed, and the half a book that had
// arrived is thrown away with the temporary file.
type counter struct {
	w    io.Writer
	max  int64
	n    int64
	over error
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if err == nil && c.n > c.max {
		return n, c.over
	}
	return n, err
}

// watcher is a reader that gives up when the bytes stop coming.
//
// It holds a timer that cancels the request, and every read that returns
// something resets it. A slow download resets it a thousand times and
// finishes; a download that has gone silent does not reset it at all and is
// cancelled, which unblocks the read the goroutine is sitting in.
type watcher struct {
	r     io.Reader
	after time.Duration
	timer *time.Timer
	fired atomic.Bool
}

func watch(r io.Reader, after time.Duration, cancel context.CancelFunc) *watcher {
	w := &watcher{r: r, after: after}
	w.timer = time.AfterFunc(after, func() {
		w.fired.Store(true)
		cancel()
	})
	return w
}

func (w *watcher) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.timer.Reset(w.after)
	}
	return n, err
}

func (w *watcher) stop() { w.timer.Stop() }

// stalled reports whether it was this watcher that ended the download, as
// opposed to the caller's own context or a network error. Without it every
// abandoned download reports "context canceled", which says nothing about
// which of the three happened.
func (w *watcher) stalled() bool { return w.fired.Load() }

// check decides whether what arrived is a paper.
//
// The content type is looked at last and trusted least. Plenty of servers
// send a PDF as application/octet-stream, and plenty of others send an HTML
// error page as application/pdf, so the bytes decide and the header is only
// worth quoting back in the message.
func (f *Fetcher) check(path, raw, ctype string, n int64) error {
	switch {
	case n > f.ceiling():
		return f.tooBig(raw)
	case n < f.floor():
		return fmt.Errorf("%s: %w: %d bytes, under the %d byte floor, so this is an error page or a truncated download", raw, ErrNotPDF, n, f.floor())
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	head := make([]byte, len(Magic))
	if _, err := io.ReadFull(file, head); err != nil {
		return fmt.Errorf("%s: %w: %v", raw, ErrNotPDF, err)
	}
	if string(head) != Magic {
		what := ctype
		if what == "" {
			what = "nothing in particular"
		}
		return fmt.Errorf("%s: %w: it begins %q and the server called it %s", raw, ErrNotPDF, printable(head), what)
	}
	return nil
}

func (f *Fetcher) tooBig(raw string) error {
	return fmt.Errorf("%s: %w: %w: over %d bytes, which is a book and not a paper", raw, ErrNotPDF, ErrTooBig, f.ceiling())
}

// printable keeps an error message readable when the download was HTML, a
// gzip stream or something else with no business being in a terminal.
func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			sb.WriteByte('.')
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// Adopt takes in a file that is already on disk and reports what it is.
//
// Two papers in the corpus are open access at their publisher and that
// publisher answers 403 to every program, from every network tried, while
// answering perfectly well to a person with a browser. There is no client to
// fix and nothing to work around that would not amount to lying about who we
// are, so the remaining honest route is for somebody to fetch the file
// themselves and say where they got it.
//
// What that file gets is the same three checks every download gets, because
// the one thing worse than a missing paper is a corpus that believes a saved
// error page is one.
func (f *Fetcher) Adopt(path string) (File, error) {
	sum, n, err := Sum(path)
	if err != nil {
		return File{}, err
	}
	if err := f.check(path, path, "", n); err != nil {
		return File{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	return File{
		Path:    path,
		SHA256:  sum,
		Bytes:   n,
		Fetched: info.ModTime().UTC(),
	}, nil
}

// Sum is the SHA-256 of a file already on disk, so that a second run can tell
// whether it still has what the record says it has.
func Sum(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func (f *Fetcher) client() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	return http.DefaultClient
}

func (f *Fetcher) gate() *polite.Gate { return f.Gate }

func (f *Fetcher) floor() int64 {
	if f.Floor > 0 {
		return f.Floor
	}
	return Floor
}

func (f *Fetcher) ceiling() int64 {
	if f.Ceiling > 0 {
		return f.Ceiling
	}
	return Ceiling
}

func (f *Fetcher) stall() time.Duration {
	if f.Stall > 0 {
		return f.Stall
	}
	return Stall
}

func (f *Fetcher) whole() time.Duration {
	if f.Whole > 0 {
		return f.Whole
	}
	return Whole
}
