package code

import "testing"

// Every tag Sniff can return is a tag rule C02 allows, which is the whole
// reason the list and the sniffer live in one package.
func TestEverySignatureNamesATagTheCorpusUses(t *testing.T) {
	for _, s := range signatures {
		if !Langs[s.lang] {
			t.Errorf("Sniff can return %q and it is not a tag this corpus uses", s.lang)
		}
	}
	if !Langs[Sniff("nothing in here looks like a program")] {
		t.Error("the fallback is not a tag this corpus uses")
	}
}

// The five bare fences of the MapReduce paper, which is what this was
// written for. One of them has an answer and four of them do not.
func TestSniffNamesWhatItIsSureOfAndTextOtherwise(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want string
	}{
		{
			"the appendix program",
			"#include \"mapreduce/mapreduce.h\"\n\nclass WordCounter : public Mapper {\n  public:\n    virtual void Map(const MapInput& input) {\n    }\n};",
			"cpp",
		},
		{
			"the map and reduce pseudocode",
			"map(String key, String value):\n    // key: document name\n    for each word w in value:\n        EmitIntermediate(w, \"1\");",
			"text",
		},
		{
			"a pair of type signatures",
			"map     (k1,v1)          -> list(k2,v2)\nreduce  (k2,list(v2))    -> list(v2)",
			"text",
		},
		{
			"a table of job statistics",
			"Number of jobs                29,423\nAverage job completion time   634 secs",
			"text",
		},
		{
			"a Python function",
			"def bound(n):\n    return n\n",
			"python",
		},
		{
			"a C function",
			"static int bound(int n) {\n    return n;\n}\n",
			"c",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Sniff(c.text); got != c.want {
				t.Errorf("Sniff said %q, want %q", got, c.want)
			}
		})
	}
}

func TestLabelTagsABareFenceAndTouchesNothingElse(t *testing.T) {
	const body = "A paragraph.\n\n```\ndef bound(n):\n    return n\n```\n\n```text\n  a table  \n```\n"
	const want = "A paragraph.\n\n```python\ndef bound(n):\n    return n\n```\n\n```text\n  a table  \n```\n"
	if got := Label(body); got != want {
		t.Errorf("Label wrote:\n%q\nwant:\n%q", got, want)
	}
}

// C01 is the rule for a fence that never closed, and it is a worse problem
// than a missing tag. Tagging it would make a broken listing look tidy.
func TestLabelLeavesAnUnclosedFenceAlone(t *testing.T) {
	const body = "A paragraph.\n\n```\ndef bound(n):\n    return n\n"
	if got := Label(body); got != body {
		t.Errorf("Label tagged an unclosed fence:\n%q", got)
	}
}
