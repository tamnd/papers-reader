package corpus

import "testing"

func TestSuggestIDIsSurnameYearAndFirstRealWord(t *testing.T) {
	for _, c := range []struct {
		authors []string
		year    int
		title   string
		want    string
	}{
		{[]string{"Ashish Vaswani", "Noam Shazeer"}, 2017, "Attention Is All You Need", "vaswani-2017-attention"},
		{[]string{"Satoshi Nakamoto"}, 2008, "Bitcoin: A Peer-to-Peer Electronic Cash System", "nakamoto-2008-bitcoin"},
		// The article goes and the word after it is the keyword.
		{[]string{"Claude E. Shannon"}, 1948, "A Mathematical Theory of Communication", "shannon-1948-mathematical"},
		// A hyphenated first word is cut at the hyphen, because an id has
		// hyphens of its own and three parts.
		{[]string{"Leslie Lamport"}, 1978, "Time-Clocks and the Ordering of Events", "lamport-1978-time"},
		// A suffix is not a surname.
		{[]string{"Frederick P. Brooks Jr."}, 1987, "No Silver Bullet", "brooks-1987-silver"},
		// A particle belongs to the surname after it.
		{[]string{"George van den Driessche"}, 2016, "Mastering the Game of Go", "vandendriessche-2016-mastering"},
		// An accent comes off the letter rather than taking the letter with
		// it, so the id can be typed on any keyboard.
		{[]string{"Bui Tuong Phong"}, 1975, "Illumination for Computer Generated Pictures", "phong-1975-illumination"},
		{[]string{"Lukasz Kaiser"}, 2017, "Depthwise Separable Convolutions", "kaiser-2017-depthwise"},
		{[]string{"Ole-Johan Dahl"}, 1966, "Simula, an Algol Based Simulation Language", "dahl-1966-simula"},
	} {
		got, err := SuggestID(c.authors, c.year, c.title)
		if err != nil {
			t.Errorf("%q: %v", c.title, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q by %s is %s, want %s", c.title, c.authors[0], got, c.want)
		}
	}
}

func TestSuggestIDSaysWhatIsMissing(t *testing.T) {
	for _, c := range []struct {
		name    string
		authors []string
		year    int
		title   string
	}{
		{"no authors", nil, 2017, "Attention Is All You Need"},
		{"an empty author", []string{"  "}, 2017, "Attention Is All You Need"},
		{"no year", []string{"Ashish Vaswani"}, 0, "Attention Is All You Need"},
		{"a title of nothing but stopwords", []string{"Ashish Vaswani"}, 2017, "On the Of and The"},
		{"a title of nothing at all", []string{"Ashish Vaswani"}, 2017, ""},
	} {
		if got, err := SuggestID(c.authors, c.year, c.title); err == nil {
			t.Errorf("%s produced %s", c.name, got)
		}
	}
}

// Whatever comes out has to be an id, because the thing that consumes it
// refuses anything else and by then nobody knows where the string came from.
func TestSuggestIDOnlyEverProducesAnID(t *testing.T) {
	for _, name := range []string{"Ashish Vaswani", "Ole-Johan Dahl", "Bui Tuong Phong", "J. J. Furman"} {
		got, err := SuggestID([]string{name}, 1999, "A Mathematical Theory of Communication")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !ValidID(got) {
			t.Errorf("%s produced %q, which is not an id", name, got)
		}
	}
}
