package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/papers-reader/polite"
)

// Magic is the first five bytes of every PDF ever written. A file that does
// not start with it is not a PDF whatever the server called it, and servers
// call all sorts of things PDFs: a login page, a cookie wall, an interstitial
// asking whether you are a robot.
const Magic = "%PDF-"

// Floor is the smallest thing worth believing is a paper.
//
// Twenty kilobytes is generous. The shortest real paper in the corpus is a
// couple of hundred. What lands under the floor is an error page that happens
// to be a valid PDF, which publishers do produce, and a truncated download.
const Floor = 20 << 10

// Ceiling is the largest download that will be accepted. A paper that is
// bigger than this is a book, a dataset or a mistake, and in all three cases
// stopping is better than filling a disk.
const Ceiling = 256 << 20

// ErrNotPDF is what the guard returns. It is one error for every way a
// download can fail to be a paper, because the caller does the same thing
// with all of them: leave the record alone and say so.
var ErrNotPDF = errors.New("not a PDF")

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
}

// New returns a fetcher with the manners on.
func New(version string) *Fetcher {
	return &Fetcher{
		// Long, because some publishers take their time, and bounded, because
		// a connection that hangs should not hold up the other ninety nine.
		HTTP:      &http.Client{Timeout: 5 * time.Minute},
		UserAgent: polite.UserAgent(version),
		Gate:      &polite.Gate{Interval: polite.MinInterval},
		Floor:     Floor,
		Ceiling:   Ceiling,
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
	f.gate().Wait(u.Host)

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
	n, err := io.Copy(io.MultiWriter(tmp, sum), io.LimitReader(resp.Body, f.ceiling()+1))
	if err != nil {
		return File{}, err
	}
	if err := tmp.Close(); err != nil {
		return File{}, err
	}

	if err := f.check(tmp.Name(), raw, resp.Header.Get("Content-Type"), n); err != nil {
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

// check decides whether what arrived is a paper.
//
// The content type is looked at last and trusted least. Plenty of servers
// send a PDF as application/octet-stream, and plenty of others send an HTML
// error page as application/pdf, so the bytes decide and the header is only
// worth quoting back in the message.
func (f *Fetcher) check(path, raw, ctype string, n int64) error {
	switch {
	case n > f.ceiling():
		return fmt.Errorf("%s: %w: over %d bytes, which is a book and not a paper", raw, ErrNotPDF, f.ceiling())
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
