package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// local runs the curl command here instead of over ssh, through the same
// shell quoting the real thing uses. It is the closest a test can get to a
// second machine without having one.
func local(ctx context.Context, host string, argv []string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", quote(argv))
}

// fake replaces curl with a shell script, so that the test says what the far
// side sends back without needing a network on either end.
func fake(ctx context.Context, host string, argv []string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", host)
}

func TestTheBodyComesBackAndTheStatusDoesNot(t *testing.T) {
	hop := Hop{
		// A PDF on standard output and the status line on standard error,
		// which is the arrangement the -w format string produces.
		Host: `printf '%%PDF-1.4 hello'; printf '200 application/pdf\n' >&2`,
		Run:  fake,
	}
	var body bytes.Buffer
	res, err := hop.Get(context.Background(), "https://example.org/paper.pdf", Options{}, &body)
	if err != nil {
		t.Fatalf("the relay failed: %v", err)
	}
	if res.Code != 200 || res.Type != "application/pdf" {
		t.Errorf("got %d %q", res.Code, res.Type)
	}
	if got := body.String(); got != "%PDF-1.4 hello" {
		t.Errorf("the body is %q, and the status has no business being in it", got)
	}
}

func TestARefusalIsReportedAndNotGuessedAt(t *testing.T) {
	hop := Hop{
		Host: `printf '<html>go away</html>'; printf '403 text/html\n' >&2`,
		Run:  fake,
	}
	var body bytes.Buffer
	res, err := hop.Get(context.Background(), "https://example.org/paper.pdf", Options{}, &body)
	if err != nil {
		t.Fatalf("a 403 is an answer and not an error here: %v", err)
	}
	if res.Code != 403 {
		t.Errorf("the far side said 403 and this reports %d", res.Code)
	}
}

// curl writes its own complaints to standard error before the format string,
// so the status is the last line that looks like one and not the first line
// of anything.
func TestCurlsOwnComplaintsAreNotMistakenForTheStatus(t *testing.T) {
	hop := Hop{
		Host: `printf 'curl: (6) Could not resolve host: example.org\n' >&2; printf '000 \n' >&2`,
		Run:  fake,
	}
	res, err := hop.Get(context.Background(), "https://example.org/paper.pdf", Options{}, &bytes.Buffer{})
	if err == nil && res.Code != 0 {
		t.Fatalf("got %d, and 000 is curl saying it never got a reply", res.Code)
	}
}

func TestAHopThatCannotRunSaysSo(t *testing.T) {
	hop := Hop{Host: `exit 7`, Run: fake}
	_, err := hop.Get(context.Background(), "https://example.org/paper.pdf", Options{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("a command that exited 7 was treated as a download")
	}
}

func TestOnlyTheTwoSchemesAreRelayed(t *testing.T) {
	hop := Hop{Host: `printf ok`, Run: fake}
	for _, u := range []string{"file:///etc/passwd", "ftp://example.org/x.pdf", "ssh://example.org"} {
		if _, err := hop.Get(context.Background(), u, Options{}, &bytes.Buffer{}); err == nil {
			t.Errorf("%s was relayed, and it should not have been", u)
		}
	}
}

// The quoting is the safety story, so it gets a test that actually runs a
// shell. A URL with a semicolon in it has to arrive as one argument and not
// as a second command.
func TestAUrlCannotTurnIntoASecondCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	argv := []string{"printf", "%s", "https://example.org/x.pdf; touch " + marker}
	var out bytes.Buffer
	cmd := local(context.Background(), "", argv)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("the quoted command did not run: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the shell on the far side ran the second half of the URL")
	}
	if !strings.HasSuffix(out.String(), "touch "+marker) {
		t.Errorf("the argument arrived as %q", out.String())
	}
}

func TestTheEnvironmentBeatsTheFile(t *testing.T) {
	t.Setenv(Env, " server1 , ,server2 ")
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "server1, server2" {
		t.Errorf("got %q, and the blank entry should have been dropped", got)
	}
}

func TestNoConfigurationIsNotAProblem(t *testing.T) {
	t.Setenv(Env, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatalf("having no relay is the usual case and it errored: %v", err)
	}
	if !got.Empty() {
		t.Errorf("got %q out of an empty config directory", got)
	}
}

func TestTheFileIsRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(Env, "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	path, err := ConfigPath()
	if err != nil {
		t.Skip("this platform has no user config directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(Config{Hops: []string{"one", "two"}})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "one, two" {
		t.Errorf("got %q from %s", got, path)
	}
}

func TestTheNilSetIsEmpty(t *testing.T) {
	var s *Set
	if !s.Empty() || s.String() != "none" {
		t.Error("a caller who never configured a relay should need no special case")
	}
}
