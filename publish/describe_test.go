package publish

import (
	"strings"
	"testing"
)

func batch(paths ...string) []Change {
	out := make([]Change, len(paths))
	for i, p := range paths {
		out[i] = Change{Path: p, New: true}
	}
	return out
}

func TestTheSubjectNamesThePaperWhenThereIsOnlyOne(t *testing.T) {
	title, _ := Describe(batch(
		"content/vi/dean-2004-mapreduce/01_intro.md",
		"content/zh/dean-2004-mapreduce/01_intro.md",
		"content/ja/dean-2004-mapreduce/01_intro.md",
	), "")
	want := "content: dean-2004-mapreduce in Vietnamese, Chinese and Japanese"
	if title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
}

func TestTheSubjectCountsThePapersWhenThereAreSeveral(t *testing.T) {
	title, _ := Describe(batch(
		"content/vi/a-1900-x/01.md",
		"content/vi/b-1901-y/01.md",
		"content/zh/c-1902-z/01.md",
	), "")
	want := "content: three papers in Vietnamese and Chinese"
	if title != want {
		t.Errorf("title = %q, want %q", title, want)
	}
}

func TestASubjectIsOneLine(t *testing.T) {
	title, _ := Describe(batch(
		"content/vi/a-1900-x/01.md",
		"manifests/glossary.yaml",
		"reports/usage.md",
	), "")
	if strings.Contains(title, "\n") {
		t.Errorf("title = %q and a wrapped subject is truncated by the squash merge", title)
	}
}

func TestABatchOfManifestsAloneStillHasASubject(t *testing.T) {
	title, body := Describe(batch("manifests/glossary.yaml", "reports/usage.md"), "")
	if !strings.HasPrefix(title, "manifests:") {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"manifests/glossary.yaml", "reports/usage.md"} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not name %s:\n%s", want, body)
		}
	}
}

func TestTheBodyGroupsByLanguageAndThenByPaper(t *testing.T) {
	_, body := Describe(batch(
		"content/zh/b-1901-y/01.md",
		"content/vi/a-1900-x/01.md",
		"content/vi/a-1900-x/02.md",
		"content/vi/b-1901-y/01.md",
		"manifests/glossary.yaml",
	), "This is the run pushing what it has.")
	want := `This is the run pushing what it has.

Vietnamese, three files in two papers:

- a-1900-x, two files
- b-1901-y, one file

Chinese, one file in one paper:

- b-1901-y, one file

Also one file:

- manifests/glossary.yaml
`
	if body != want {
		t.Errorf("body is\n%s\nwant\n%s", body, want)
	}
}

// The languages come out in the corpus order whatever order the run
// finished them in, because a reader comparing two batches should not have
// to work out why the Chinese moved above the Vietnamese.
func TestTheLanguagesAreAlwaysInTheSameOrder(t *testing.T) {
	one := batch("content/ja/a/1.md", "content/zh/a/1.md", "content/vi/a/1.md")
	two := batch("content/vi/a/1.md", "content/ja/a/1.md", "content/zh/a/1.md")
	_, a := Describe(one, "")
	_, b := Describe(two, "")
	if a != b {
		t.Errorf("the same batch in two orders reads two ways:\n%s\n---\n%s", a, b)
	}
	if i, j := strings.Index(a, "Vietnamese"), strings.Index(a, "Japanese"); i > j {
		t.Errorf("Japanese comes before Vietnamese:\n%s", a)
	}
}

// The house style for everything in these repositories that a person reads.
// This is generated prose going into a public history and nobody proofreads
// it, so the rules are a test.
func TestTheProseIsWrittenTheWayTheRepositoryWritesProse(t *testing.T) {
	title, body := Describe(batch(
		"content/vi/dean-2004-mapreduce/01_intro.md",
		"content/zh/goodfellow-2014-gan/01_intro.md",
		"manifests/glossary.yaml",
	), "This is translation run 20260914T101500Z pushing out what it has finished so far.\nThe run is still going and the next batch will look like this one.\n")
	for _, s := range []string{title, body} {
		if strings.ContainsAny(s, "\u2014\u2013") {
			t.Errorf("there is a dash in %q that a developer would not have typed", s)
		}
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.HasSuffix(line, ":") {
			continue
		}
		if !strings.HasSuffix(line, ".") {
			t.Errorf("%q is a sentence broken across two lines", line)
		}
	}
}

// A path that is not a content file is listed rather than guessed at. The
// alternative is a batch that quietly drops the glossary out of its own
// description, and the glossary is the file a reviewer most wants to see
// named.
func TestNothingInTheBatchIsLeftOutOfTheBody(t *testing.T) {
	changes := batch(
		"content/vi/a-1900-x/01.md",
		"content/en/a-1900-x/01.md",
		"manifests/refs/a-1900-x.yaml",
		"figures/a-1900-x/fig1.png",
		"tags/register",
	)
	_, body := Describe(changes, "")
	for _, c := range changes {
		name := c.Path
		if lang, id, ok := content(name); ok {
			name = id
			_ = lang
		}
		if !strings.Contains(body, name) {
			t.Errorf("body does not mention %s:\n%s", c.Path, body)
		}
	}
}

func TestTheCountsReadTheWayAPersonWritesThem(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "one file"}, {2, "two files"}, {12, "twelve files"}, {13, "13 files"},
	}
	for _, c := range cases {
		if got := files(c.n); got != c.want {
			t.Errorf("files(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	if got := list([]string{"Vietnamese", "Chinese", "Japanese"}); got != "Vietnamese, Chinese and Japanese" {
		t.Errorf("list = %q", got)
	}
	if got := list([]string{"Vietnamese", "Chinese"}); got != "Vietnamese and Chinese" {
		t.Errorf("list = %q", got)
	}
}
