package extract

import "testing"

// Reference 10 of the MapReduce paper. The page prints barc and the reading
// came back with bench, which is three edits and a plausible address, so a
// second reading writes it again.
func TestReaddressPutsBackWhatThePagePrints(t *testing.T) {
	const layer = "[10] Jim Gray.\nSort benchmark home page.\nhttp://research.microsoft.com/barc/SortBenchmark/.\n"
	const got = "[10] Jim Gray. Sort benchmark home page. http://research.microsoft.com/bench/SortBenchmark/.\n"
	const want = "[10] Jim Gray. Sort benchmark home page. http://research.microsoft.com/barc/SortBenchmark/.\n"
	if out := Readdress(got, layer); out != want {
		t.Errorf("Readdress wrote\n%q\nwant\n%q", out, want)
	}
}

func TestReaddressLeavesAloneWhatItCannotVouchFor(t *testing.T) {
	for _, c := range []struct {
		name  string
		layer string
		text  string
	}{
		{
			"the layer has no address at all",
			"Sort benchmark home page.\n",
			"Sort benchmark home page. http://research.microsoft.com/bench/SortBenchmark/.\n",
		},
		{
			"the layer has nothing like this address",
			"See http://www.example.org/a/quite/different/place.\n",
			"See http://research.microsoft.com/bench/SortBenchmark/.\n",
		},
		{
			"the address is already the one on the page",
			"See http://research.microsoft.com/barc/SortBenchmark/.\n",
			"See http://research.microsoft.com/barc/SortBenchmark/.\n",
		},
		{
			"two layer addresses are equally close",
			"See http://example.org/a1 and http://example.org/a3.\n",
			"See http://example.org/a2.\n",
		},
		{
			"an address inside a listing is the program",
			"See http://example.org/barc.\n",
			"A paragraph.\n\n```sh\ncurl http://example.org/bench\n```\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := Readdress(c.text, c.layer); out != c.text {
				t.Errorf("Readdress rewrote\n%q\ninto\n%q", c.text, out)
			}
		})
	}
}

// Four and not five. A paper that lists numbered addresses one after
// another has several within five of each other, and picking the wrong one
// is worse than leaving a wrong one alone.
func TestNearestDoesNotReachAcrossANumberedRun(t *testing.T) {
	layer := []string{"http://example.org/data/set01", "http://example.org/data/set02", "http://example.org/data/set03"}
	if _, ok := nearest("http://example.org/data/set04", layer); ok {
		t.Error("a fourth address in a numbered run was taken for one of the first three")
	}
}
