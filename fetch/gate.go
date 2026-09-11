package fetch

import (
	"fmt"

	"github.com/tamnd/papers-reader/corpus"
)

// May reports whether the PDF of a paper may be downloaded at all, and says
// why not when the answer is no.
//
// Downloading and publishing are two different questions and this file
// answers the first one. A PDF that somebody has put on the open web may be
// read, and reading it is what the extraction stages do. The copy lives in
// `pdf/`, which is in .gitignore, is never committed under any licence, and
// is never served anywhere. Audit rule S03 checks git's index rather than
// trusting that.
//
// What may be published from the file once it is read is the other question
// and it is answered by Publish, off the access class, exactly as before. A
// restricted paper can be measured, classified and read and still publish
// nothing but its front matter and a short abstract, and that is the whole
// point of keeping the two apart: the licence decides what goes into a public
// repository, not what a program on this machine is allowed to look at.
func May(rec *corpus.Source) (bool, string) {
	switch {
	case rec == nil || rec.ID == "":
		return false, "no record, so nothing is known about it and there is nowhere to fetch from"
	case rec.URL == "":
		return false, "resolved to no location, so there is nothing to fetch"
	case !rec.Access.Valid():
		return false, fmt.Sprintf("has an access class of %q, which is not one of the five", rec.Access)
	case rec.Access == corpus.AccessUnknown:
		// Unknown does not mean restricted, it means nothing was found. There
		// is no location to fetch from and no paper anybody has identified.
		return false, "is unknown, which means nothing was found for it, so there is nothing to fetch"
	}
	return true, ""
}

// Publish reports whether the text of a paper may go into the corpus, and
// says what it gets instead when the answer is no.
//
// This is the licence gate and it has not moved. Public domain, open and
// permissive publish the full text, the mathematics and the figures.
// Restricted publishes the title, the authors, the year, the links and an
// abstract under 250 words, and no body text and no figures. Unknown
// publishes nothing at all.
func Publish(rec *corpus.Source) (bool, string) {
	switch {
	case rec == nil || rec.ID == "":
		return false, "no record, so nothing may be published about it"
	case !rec.Access.Valid():
		return false, fmt.Sprintf("has an access class of %q, which is not one of the five", rec.Access)
	case rec.Access == corpus.AccessUnknown:
		return false, "is unknown, and unknown publishes nothing"
	case rec.Access == corpus.AccessRestricted:
		return false, "is restricted, so it publishes front matter and a short abstract and no body text"
	case rec.Licence == "":
		// Rule S06 in the audit: being able to download it is not a licence.
		// If this ever fires the resolver has a bug, because it is supposed to
		// be impossible to be open with no licence recorded.
		return false, "has no licence recorded, and being able to download something is not permission to republish it"
	}
	return rec.Access.Body(), ""
}
