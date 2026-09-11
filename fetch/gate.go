package fetch

import (
	"fmt"

	"github.com/tamnd/papers-reader/corpus"
)

// May reports whether the PDF of a paper may be downloaded at all, and says
// why not when the answer is no.
//
// This is the licence gate, and it is deliberately a separate function from
// the downloader so that it can be read on its own and tested on its own. The
// rule is short. A paper may be fetched when its access class allows a body
// to be published from it, which is public-domain, open and permissive.
//
// Everything else is no, including restricted. A restricted paper publishes
// front matter and a short abstract, and neither of those comes out of the
// PDF: the abstract comes from the metadata the resolver already has. So
// there is no reason to hold the file, and holding a file there is no reason
// to hold is how a corpus quietly becomes a mirror.
func May(rec *corpus.Source) (bool, string) {
	switch {
	case rec == nil || rec.ID == "":
		return false, "no record, so nothing is known about it and nothing may be done with it"
	case rec.URL == "":
		return false, "resolved to no location, so there is nothing to fetch"
	case !rec.Access.Valid():
		return false, fmt.Sprintf("has an access class of %q, which is not one of the five", rec.Access)
	case rec.Access == corpus.AccessUnknown:
		return false, "is unknown, and unknown publishes nothing"
	case rec.Access == corpus.AccessRestricted:
		return false, "is restricted, so it gets front matter and an abstract from the metadata and the PDF is not needed"
	case rec.Licence == "":
		// Rule S06 in the audit: being able to download it is not a licence.
		// If this ever fires the resolver has a bug, because it is supposed to
		// be impossible to be open with no licence recorded.
		return false, "has no licence recorded, and being able to download something is not permission to republish it"
	}
	return rec.Access.Body(), ""
}
