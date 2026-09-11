package sources

import (
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// licenceRule maps a licence, as the services spell it, to what the corpus
// may publish.
type licenceRule struct {
	// match is looked for in the lowercased licence string or URL.
	match  string
	access corpus.Access
	// name is what gets written into sources.yaml, so that the manifest says
	// "CC BY 4.0" rather than repeating a URL with a trailing slash.
	name string
}

// licenceRules is the whole table, most specific first.
//
// Order matters and the non-commercial and no-derivatives variants have to
// come before plain CC BY, because "cc-by-nc" contains "cc-by" and a table
// that checked the short one first would classify every restricted licence as
// open.
var licenceRules = []licenceRule{
	{"cc0", corpus.AccessPublicDomain, "CC0 1.0"},
	{"publicdomain/zero", corpus.AccessPublicDomain, "CC0 1.0"},
	{"publicdomain/mark", corpus.AccessPublicDomain, "public domain"},
	{"public-domain", corpus.AccessPublicDomain, "public domain"},

	{"cc-by-nc-nd", corpus.AccessPermissive, "CC BY-NC-ND 4.0"},
	{"cc-by-nc-sa", corpus.AccessPermissive, "CC BY-NC-SA 4.0"},
	{"cc-by-nd", corpus.AccessPermissive, "CC BY-ND 4.0"},
	{"cc-by-nc", corpus.AccessPermissive, "CC BY-NC 4.0"},
	{"by-nc-nd", corpus.AccessPermissive, "CC BY-NC-ND 4.0"},
	{"by-nc-sa", corpus.AccessPermissive, "CC BY-NC-SA 4.0"},
	{"by-nd", corpus.AccessPermissive, "CC BY-ND 4.0"},
	{"by-nc", corpus.AccessPermissive, "CC BY-NC 4.0"},

	{"cc-by-sa", corpus.AccessOpen, "CC BY-SA 4.0"},
	{"by-sa", corpus.AccessOpen, "CC BY-SA 4.0"},
	{"cc-by", corpus.AccessOpen, "CC BY 4.0"},
	{"creativecommons.org/licenses/by/", corpus.AccessOpen, "CC BY 4.0"},

	// arXiv's own licence lets anyone read and redistribute the paper as
	// submitted, and does not let anyone make a derivative of it. A
	// translation is a derivative, which is why this is permissive and not
	// open, and why the corpus says on every page that the translation is
	// ours and the paper is not.
	{"arxiv.org/licenses/nonexclusive-distrib", corpus.AccessPermissive, "arXiv non-exclusive licence to distribute"},

	{"open-government-licence", corpus.AccessOpen, "Open Government Licence"},
	{"opengovernmentlicence", corpus.AccessOpen, "Open Government Licence"},

	// A publisher that has made a paper free to read has not made it free to
	// redistribute, and those are different permissions. Everything in this
	// group reads and does not republish.
	{"acm-open", corpus.AccessOpen, "ACM Open"},
	{"free-to-read", corpus.AccessRestricted, "free to read, all rights reserved"},
	{"all rights reserved", corpus.AccessRestricted, "all rights reserved"},
	{"elsevier-user", corpus.AccessRestricted, "publisher terms, all rights reserved"},
	{"springer-tdm", corpus.AccessRestricted, "publisher terms, all rights reserved"},
	{"ieee", corpus.AccessRestricted, "IEEE terms, all rights reserved"},
}

// Licence reads a licence string or URL and says what the corpus may publish
// under it.
//
// Anything the table does not recognise is unknown, and unknown publishes
// nothing. That default is the whole point: a licence nobody has read is not
// a licence to republish, and the cost of guessing wrong here is a public
// repository full of somebody else's copyrighted text.
func Licence(s string) (name string, access corpus.Access) {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return "", corpus.AccessUnknown
	}
	for _, r := range licenceRules {
		if strings.Contains(low, r.match) {
			return r.name, r.access
		}
	}
	// Keep what the service said, so a person reading reports/resolve.md can
	// see the string that was not recognised and add a rule for it.
	return strings.TrimSpace(s), corpus.AccessUnknown
}

// The access classes in order, most permissive first. A service that lists
// several licences for one work is listing what applied at different times,
// and the question being asked is what may be done today, so the best one
// wins.
var accessRank = map[corpus.Access]int{
	corpus.AccessPublicDomain: 0,
	corpus.AccessOpen:         1,
	corpus.AccessPermissive:   2,
	corpus.AccessRestricted:   3,
	corpus.AccessUnknown:      4,
}

// corpusUnknownRank is the rank of knowing nothing, which is where a search
// for the best licence starts.
const corpusUnknownRank = 4

func rank(a corpus.Access) int {
	if r, ok := accessRank[a]; ok {
		return r
	}
	return corpusUnknownRank
}

// Better reports whether access a permits more than access b.
func Better(a, b corpus.Access) bool { return rank(a) < rank(b) }
