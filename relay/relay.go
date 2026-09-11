// Package relay runs a download from somewhere else.
//
// Some hosts refuse this network rather than this program. Three papers in
// the corpus sit behind a publisher that answers 403 to every request from
// here, and the Internet Archive, which has a perfectly good copy of two of
// them, answers 429 to every request from this ISP and has done for hours at
// a stretch. Nothing about the client is wrong, so there is nothing about the
// client to fix. Changing the User-Agent to something a bot wall likes better
// would be lying about who we are, and the whole politeness story in this
// toolchain rests on not doing that.
//
// What does work is asking a machine on a different network to run the same
// honest request. A relay is a list of ssh destinations. The far side runs
// curl with the same User-Agent, the same stall clock and the same contact
// address, and the bytes come back over the connection we already have. The
// request is the same request; only the place it leaves from has moved.
//
// The list of machines is configuration and not code. It lives in
// ~/.config/papers/relay.json or in PAPERS_RELAY, both of which are outside
// every repository, because a host name is the sort of thing that should not
// be committed to a public one.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Hop is one machine that will fetch a URL on this program's behalf.
type Hop struct {
	// Host is an ssh destination: a Host block in ~/.ssh/config, or the
	// user@address form. It is never written to a file in a repository.
	Host string

	// Run builds the command that carries out one download. Nil means ssh,
	// which is what it is everywhere except a test, because a test that needs
	// a second machine on a second network is a test nobody ever runs.
	Run func(ctx context.Context, host string, argv []string) *exec.Cmd
}

// Set is the machines to try, in the order they were given.
type Set struct {
	Hops []Hop
}

// Empty reports whether there is anywhere to relay through. The nil Set is
// empty, so a caller that never configured one needs no special case.
func (s *Set) Empty() bool { return s == nil || len(s.Hops) == 0 }

// String names the machines, for a line on a terminal. It is not written to
// any file the corpus keeps.
func (s *Set) String() string {
	if s.Empty() {
		return "none"
	}
	names := make([]string, len(s.Hops))
	for i, h := range s.Hops {
		names[i] = h.Host
	}
	return strings.Join(names, ", ")
}

// Config is the file format, which is a list of ssh destinations and nothing
// else. There is deliberately no room in it for a password, a key or a port:
// ssh already knows all of that, and a second place to configure ssh is a
// second place to get ssh wrong.
type Config struct {
	Hops []string `json:"hops"`
}

// Env is the variable that overrides the file, holding ssh destinations
// separated by commas.
const Env = "PAPERS_RELAY"

// Load reads the relay from the environment or from the config file.
//
// Missing configuration is not an error. Most of the time there is no relay
// and every download goes out from here, which is the arrangement to prefer:
// a download that needs a second machine is a download somebody should know
// about.
func Load() (*Set, error) {
	if v := strings.TrimSpace(os.Getenv(Env)); v != "" {
		return parse(strings.Split(v, ",")), nil
	}
	path, err := ConfigPath()
	if err != nil {
		return &Set{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Set{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return parse(cfg.Hops), nil
}

// ConfigPath is where Load looks when the environment says nothing.
func ConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "papers", "relay.json"), nil
}

func parse(hosts []string) *Set {
	s := &Set{}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		s.Hops = append(s.Hops, Hop{Host: h})
	}
	return s
}

// Options are the manners the far side is told to keep, so that a relayed
// download behaves the same as one made from here.
type Options struct {
	UserAgent string
	// Stall is how long the transfer may go without a byte before curl gives
	// up on it, and Whole is the outer bound on the request.
	Stall, Whole time.Duration
}

// Response is what the far side saw.
type Response struct {
	Code int
	Type string
}

// Get runs one download on the far machine and writes the body to w.
//
// The status comes back separately from the body, which is the one fiddly
// part. curl writes what -w asks for to standard output by default, and
// standard output is the PDF, so the status would end up inside the file.
// The %{stderr} marker moves the rest of the format string to standard error
// instead, leaving the body alone.
func (h Hop) Get(ctx context.Context, raw string, opt Options, w io.Writer) (Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Response{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Response{}, fmt.Errorf("%s: only http and https are relayed", raw)
	}

	argv := []string{
		"curl", "-sS", "-L",
		// The stall clock, done by curl. Giving up when the transfer has been
		// under a byte a second for this long is the same rule the direct
		// fetcher applies, and for the same reason: a slow host is working
		// and a silent one is not, and elapsed time cannot tell them apart.
		"--speed-limit", "1",
		"--speed-time", seconds(opt.stall()),
		"--max-time", seconds(opt.whole()),
		"-A", opt.UserAgent,
		"-w", "%{stderr}%{http_code} %{content_type}\n",
		"--", raw,
	}

	cmd := h.run(ctx, argv)
	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Response{}, fmt.Errorf("%s: %w: %s", h.Host, err, oneline(stderr.String()))
	}
	return status(h.Host, stderr.String())
}

func (h Hop) run(ctx context.Context, argv []string) *exec.Cmd {
	if h.Run != nil {
		return h.Run(ctx, h.Host, argv)
	}
	return SSH(ctx, h.Host, argv)
}

// SSH is the default way to run a command on the far machine.
//
// BatchMode is on because a relay that stops to ask for a passphrase in the
// middle of a hundred downloads is worse than no relay. Everything else about
// the connection, the address, the user, the port and the key, is left to the
// user's ssh config, which already knows.
func SSH(ctx context.Context, host string, argv []string) *exec.Cmd {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		host, quote(argv),
	}
	return exec.CommandContext(ctx, "ssh", args...)
}

// quote turns a command into one string that survives the shell on the far
// side.
//
// ssh does not take an argument vector. It joins whatever it is given with
// spaces and hands the result to a login shell, so anything with a space, a
// quote or a semicolon in it has to be quoted here or it arrives as something
// else. Single quotes are used because inside them a shell expands nothing at
// all, and the one character that cannot appear inside them, the single quote
// itself, is closed, escaped and reopened.
func quote(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

// status reads the line curl wrote to standard error.
//
// It is the last line rather than the first because curl reports its own
// problems there too, and those come before the format string.
func status(host, stderr string) (Response, error) {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) == 0 {
			continue
		}
		code, err := strconv.Atoi(fields[0])
		if err != nil || code < 100 || code > 599 {
			continue
		}
		res := Response{Code: code}
		if len(fields) > 1 {
			res.Type = fields[1]
		}
		return res, nil
	}
	return Response{}, fmt.Errorf("%s: curl said nothing about the status: %s", host, oneline(stderr))
}

func oneline(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func seconds(d time.Duration) string {
	return strconv.Itoa(int(d.Round(time.Second) / time.Second))
}

// The defaults, which match the ones the direct fetcher uses. They are here
// so that a caller who fills in nothing still gets a request with manners.
const (
	Stall = 45 * time.Second
	Whole = 30 * time.Minute
)

func (o Options) stall() time.Duration {
	if o.Stall > 0 {
		return o.Stall
	}
	return Stall
}

func (o Options) whole() time.Duration {
	if o.Whole > 0 {
		return o.Whole
	}
	return Whole
}
