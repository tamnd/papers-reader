package extract

import "testing"

func TestDelinkWritesTheLinkBackAsTheTextThePagePrinted(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want string
	}{
		{
			"reference 10 of the MapReduce paper",
			"[10] Jim Gray. Sort benchmark home page. [http://research.microsoft.com/barc/SortBenchmark/ ](http://research.microsoft.com/barc/SortBenchmark/).",
			"[10] Jim Gray. Sort benchmark home page. http://research.microsoft.com/barc/SortBenchmark/.",
		},
		{
			"a link with words for its text",
			"See [the sort benchmark page](http://example.org/x) for the rules.",
			"See the sort benchmark page for the rules.",
		},
		{
			"a link with nothing for its text keeps the target",
			"Available at [](http://example.org/x).",
			"Available at http://example.org/x.",
		},
		{
			"an address written as prose is already right",
			"Available at http://example.org/x.",
			"Available at http://example.org/x.",
		},
		{
			"a numbered citation is not a link",
			"The method of [3] reaches it.",
			"The method of [3] reaches it.",
		},
		{
			"a citation into the corpus is not a link",
			"The method of [[dean-2004-mapreduce]] reaches it.",
			"The method of [[dean-2004-mapreduce]] reaches it.",
		},
		{
			"an image is Unlink's business",
			"![a chain of transactions](../images/t.png)",
			"![a chain of transactions](../images/t.png)",
		},
		{
			"a paper writing about Markdown is writing about it",
			"The form `[a](b)` is how Markdown writes a link.",
			"The form `[a](b)` is how Markdown writes a link.",
		},
		{
			"two links on one line",
			"See [a](http://x) and [b](http://y).",
			"See a and b.",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Delink(c.in); got != c.want {
				t.Errorf("Delink wrote\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

// A listing is program text and a bracket followed by a parenthesis in one
// is the program.
func TestDelinkLeavesAFencedBlockAlone(t *testing.T) {
	const body = "A paragraph.\n\n```c\n    x = a[i](n);\n```\n"
	if got := Delink(body); got != body {
		t.Errorf("Delink rewrote a listing:\n%q", got)
	}
}
