package sources

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/tamnd/papers-reader/polite"
)

// CacheFor is how long a cached response is trusted. Thirty days, because
// where a paper lives does not change often and re-running the resolver over
// the hundred while editing one rule should cost nothing.
const CacheFor = 30 * 24 * time.Hour

// Client is the polite HTTP client every lookup goes through.
//
// One request at a time per host, a floor between requests, an honest user
// agent, and every response cached on disk. The zero value is not usable;
// call NewClient.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	Mailto    string
	// Gate is the per host rate limit. Tests turn it off.
	Gate *polite.Gate
	// CacheDir is where responses are kept. An empty CacheDir turns the cache
	// off, which is what a test wants and what a person debugging a rule asks
	// for with --no-cache.
	CacheDir string
	CacheFor time.Duration
	Now      func() time.Time
	// Base is where each service lives. It is a field rather than a constant
	// so that the tests can point the whole ladder at one httptest server and
	// `go test ./...` needs no network. Nothing else should ever set it.
	Base Endpoints
}

// Endpoints is the base URL of each service on the ladder.
type Endpoints struct {
	ArXiv     string
	ArXivOAI  string
	Crossref  string
	Unpaywall string
	OpenAlex  string
}

// DefaultEndpoints is where the services actually are.
var DefaultEndpoints = Endpoints{
	ArXiv:     "https://export.arxiv.org/api/query",
	ArXivOAI:  "https://export.arxiv.org/oai2",
	Crossref:  "https://api.crossref.org/works",
	Unpaywall: "https://api.unpaywall.org/v2",
	OpenAlex:  "https://api.openalex.org/works",
}

// NewClient returns a client that behaves itself.
func NewClient(version, cacheDir string) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: polite.UserAgent(version),
		Mailto:    polite.Mailto,
		Gate:      &polite.Gate{Interval: polite.MinInterval},
		CacheDir:  cacheDir,
		CacheFor:  CacheFor,
		Now:       time.Now,
		Base:      DefaultEndpoints,
	}
}

// cached is one stored response.
type cached struct {
	URL     string    `json:"url"`
	Fetched time.Time `json:"fetched"`
	Status  int       `json:"status"`
	Body    []byte    `json:"body"`
}

// Get fetches a URL, through the cache, politely.
//
// A 404 is returned as a response rather than as an error, because "this
// service does not have this paper" is an answer and the next rung of the
// ladder wants to hear it. A 5xx is an error, because it is not an answer.
func (c *Client) Get(ctx context.Context, raw string) ([]byte, error) {
	if hit, ok := c.readCache(raw); ok {
		if hit.Status == http.StatusNotFound {
			return nil, errNotFound
		}
		return hit.Body, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	c.Gate.Wait(u.Host)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json, application/atom+xml;q=0.9, */*;q=0.1")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		c.writeCache(cached{URL: raw, Fetched: c.now(), Status: resp.StatusCode})
		return nil, errNotFound
	case resp.StatusCode >= 400:
		// Not cached. A rate limit or an outage is a fact about today, and
		// storing it for thirty days would turn a bad afternoon into a bad
		// month.
		return nil, fmt.Errorf("%s: %s", raw, resp.Status)
	}
	c.writeCache(cached{URL: raw, Fetched: c.now(), Status: resp.StatusCode, Body: body})
	return body, nil
}

// errNotFound is "the service answered, and it does not have this".
var errNotFound = fmt.Errorf("not found")

// NotFound reports whether an error means the service answered and had
// nothing, as opposed to the service not answering.
func NotFound(err error) bool { return err == errNotFound }

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// cachePath is where a URL's response lives. The name is a hash because a URL
// is not a filename, and the URL itself is stored inside the file so that a
// directory of hashes can still be read by a person.
func (c *Client) cachePath(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readCache(raw string) (cached, bool) {
	if c.CacheDir == "" {
		return cached{}, false
	}
	b, err := os.ReadFile(c.cachePath(raw))
	if err != nil {
		return cached{}, false
	}
	var hit cached
	if err := json.Unmarshal(b, &hit); err != nil {
		return cached{}, false
	}
	if c.CacheFor > 0 && c.now().Sub(hit.Fetched) > c.CacheFor {
		return cached{}, false
	}
	return hit, true
}

func (c *Client) writeCache(hit cached) {
	if c.CacheDir == "" {
		return
	}
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(hit)
	if err != nil {
		return
	}
	// A failed cache write is not a failed resolution. The worst it costs is
	// the request again next time.
	_ = os.WriteFile(c.cachePath(hit.URL), b, 0o644)
}

// withMailto adds the politeness parameter.
func (c *Client) withMailto(raw string) string {
	if c.Mailto == "" {
		return raw
	}
	sep := "?"
	if u, err := url.Parse(raw); err == nil && u.RawQuery != "" {
		sep = "&"
	}
	return raw + sep + "mailto=" + url.QueryEscape(c.Mailto)
}
