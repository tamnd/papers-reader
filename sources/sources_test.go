package sources

import "testing"

var attention = Want{
	ID:      "vaswani-2017-attention",
	Title:   "Attention Is All You Need",
	Authors: []string{"Ashish Vaswani", "Noam Shazeer", "Niki Parmar"},
	Year:    2017,
}

func TestVerifyAccepts(t *testing.T) {
	got := Candidate{
		Title:   "Attention is all you need.",
		Authors: []string{"A. Vaswani", "N. Shazeer"},
		Year:    2017,
	}
	v := Verify(attention, got)
	if !v.OK {
		t.Fatalf("the right paper was refused: %s (title %.3f)", v.Why, v.TitleScore)
	}
}

// The preprint year and the proceedings year differ by one on a good number
// of these papers, and that is not a different paper.
func TestVerifyAcceptsAYearOut(t *testing.T) {
	resnet := Want{
		Title:   "Deep Residual Learning for Image Recognition",
		Authors: []string{"Kaiming He", "Xiangyu Zhang"},
		Year:    2016,
	}
	got := Candidate{
		Title:   "Deep Residual Learning for Image Recognition",
		Authors: []string{"Kaiming He", "Jian Sun"},
		Year:    2015,
	}
	if v := Verify(resnet, got); !v.OK {
		t.Errorf("a preprint a year before publication was refused: %s", v.Why)
	}
}

// This is the case the whole resolver is built around. A search index answers
// a title query with a recent preprint that shares some words and nothing
// else, and taking it would mean fetching, extracting, translating and
// publishing the wrong paper under somebody else's name.
func TestVerifyRefusesAPlausibleStranger(t *testing.T) {
	cases := []struct {
		name string
		got  Candidate
		why  string
	}{
		{
			"a different paper with similar words",
			Candidate{
				Title:   "Attention Is Not All You Need: Pure Attention Loses Rank Doubly Exponentially",
				Authors: []string{"Yihe Dong", "Jean-Baptiste Cordonnier"},
				Year:    2021,
			},
			"the titles are too different",
		},
		{
			"the right title, the wrong decade",
			Candidate{
				Title:   "Attention Is All You Need",
				Authors: []string{"Ashish Vaswani"},
				Year:    2025,
			},
			"the years are more than a year apart",
		},
		{
			"the right title and year, nobody in common",
			Candidate{
				Title:   "Attention Is All You Need",
				Authors: []string{"Someone Else", "Another Person"},
				Year:    2017,
			},
			"no author of the candidate is an author of the paper",
		},
		{
			"a candidate with no year at all",
			Candidate{
				Title:   "Attention Is All You Need",
				Authors: []string{"Ashish Vaswani"},
			},
			"the candidate has no year",
		},
	}
	for _, tc := range cases {
		v := Verify(attention, tc.got)
		if v.OK {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if v.Why != tc.why {
			t.Errorf("%s: refused because %q, want %q", tc.name, v.Why, tc.why)
		}
	}
}

func TestTitleSimilarity(t *testing.T) {
	same := []struct{ a, b string }{
		{"Attention Is All You Need", "Attention is all you need."},
		{"MapReduce: Simplified Data Processing on Large Clusters", "MapReduce - Simplified Data Processing on Large Clusters"},
		{"A Relational Model of Data for Large Shared Data Banks", "A relational model of data for large shared data banks"},
	}
	for _, tc := range same {
		if got := TitleSimilarity(tc.a, tc.b); got < TitleFloor {
			t.Errorf("%q vs %q is %.3f, below the floor", tc.a, tc.b, got)
		}
	}
	different := []struct{ a, b string }{
		{"A Relational Model of Data for Large Shared Data Banks", "The Google File System"},
		{"Time, Clocks, and the Ordering of Events in a Distributed System", "The Byzantine Generals Problem"},
	}
	for _, tc := range different {
		if got := TitleSimilarity(tc.a, tc.b); got >= TitleFloor {
			t.Errorf("%q vs %q is %.3f, at or above the floor", tc.a, tc.b, got)
		}
	}
	// Three letters in the middle invert a title, and character similarity
	// alone puts these two at 0.936, which is over the floor. This is the
	// case the negation check exists for.
	if got := TitleSimilarity("Attention Is All You Need", "Attention Is Not All You Need"); got != 0 {
		t.Errorf("a negated title scored %.3f, want 0", got)
	}
	if got := JaroWinkler("attention is all you need", "attention is not all you need"); got < 0.9 {
		t.Errorf("the character similarity is %.3f, so this test is no longer testing anything", got)
	}
	if got := TitleSimilarity("No Silver Bullet: Essence and Accidents", "No silver bullet, essence and accidents"); got < TitleFloor {
		t.Errorf("two titles that both negate scored %.3f, below the floor", got)
	}

	if got := Jaro("", ""); got != 1 {
		t.Errorf("two empty strings are %.3f similar, want 1", got)
	}
	if got := Jaro("abc", ""); got != 0 {
		t.Errorf("something and nothing are %.3f similar, want 0", got)
	}
}

func TestSurname(t *testing.T) {
	cases := map[string]string{
		"Ashish Vaswani":     "vaswani",
		"Vaswani, A.":        "vaswani",
		"Vaswani, Ashish":    "vaswani",
		"A. N. Gomez":        "gomez",
		"Lukasz Kaiser":      "kaiser",
		"Łukasz Kaiser":      "kaiser",
		"Jean-Baptiste Joly": "joly",
		"  ":                 "",
		"":                   "",
	}
	for in, want := range cases {
		if got := Surname(in); got != want {
			t.Errorf("Surname(%q) is %q, want %q", in, got, want)
		}
	}
}

func TestAuthorsOverlap(t *testing.T) {
	want := []string{"Ashish Vaswani", "Noam Shazeer"}
	if !AuthorsOverlap(want, []string{"N. Shazeer", "Somebody Else"}) {
		t.Error("a shared surname was missed")
	}
	if AuthorsOverlap(want, []string{"Jeffrey Dean", "Sanjay Ghemawat"}) {
		t.Error("two disjoint author lists overlapped")
	}
	// An empty list has made no claim, so it matches nothing rather than
	// everything.
	if AuthorsOverlap(nil, []string{"Ashish Vaswani"}) {
		t.Error("an empty want list matched")
	}
	if AuthorsOverlap(want, nil) {
		t.Error("an empty candidate list matched")
	}
}
