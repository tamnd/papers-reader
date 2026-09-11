package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/fetch"
)

// runAdopt records a PDF that a person put in pdf/ themselves.
//
// This is the way out of the last corner. A couple of papers in the corpus
// are open access at the publisher and unreachable by any program: the same
// request that a browser makes is refused from here, from three machines on
// two other networks, and with or without a relay. Nothing about the client
// is wrong, so there is nothing about the client to fix, and the fixes that
// would work are all forms of pretending to be something else.
//
// So a person fetches it and tells the corpus where they got it. The record
// is marked `by: hand`, which means the resolver will never touch it again,
// and the file goes through the same checks as a download, so a saved login
// page cannot become a paper by being copied into the right directory.
func runAdopt(args []string) error {
	fs := flag.NewFlagSet("adopt", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	id := fs.String("id", "", "the paper, which must already be in manifests/papers.yaml")
	from := fs.String("from", "", "the URL the file was downloaded from")
	landing := fs.String("landing", "", "the page that URL was found on, if it is a different one")
	access := fs.String("access", "", "the access class: "+classNames())
	licence := fs.String("licence", "", "the licence, named exactly as the publisher names it")
	note := fs.String("note", "", "where the terms were read, so nobody has to read them twice")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers adopt --id <paper> --from <url> [flags]

Records a PDF that is already at pdf/<id>.pdf and that nothing here
downloaded. The file is hashed and checked the same way a download is: it
has to begin %s, be over %d bytes and under %d. Then the record in
manifests/sources.yaml gets the hash, the location and a `+"`by: hand`"+` line.

`+"`by: hand`"+` means the resolver leaves the paper alone from then on, even
under --again, because somebody who went and looked knows more than a ladder
of APIs does and should not have to do it twice. Say in --note where you
looked.

Being able to download something is not permission to republish it. If you
did not read a licence, leave --licence empty and the record stays
restricted, which publishes front matter and a short abstract and no more.

`, fetch.Magic, fetch.Floor, fetch.Ceiling)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("say which paper with --id")
	}

	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return err
	}
	if _, ok := manifest.ByID(*id); !ok {
		return fmt.Errorf("%s is not in %s, so there is nothing to adopt it into", *id, c.PapersManifest())
	}
	recorded, err := c.LoadSources()
	if err != nil {
		return err
	}
	byID := index(recorded)
	rec := byID[*id]
	rec.ID = *id

	path := c.PDF(*id)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s: put the file there first, then run this", path)
	}
	file, err := fetch.New(papers.Version).Adopt(path)
	if err != nil {
		return err
	}

	switch {
	case *from != "":
		rec.URL = *from
	case rec.URL == "":
		return fmt.Errorf("say with --from where the file came from: a paper in the corpus with no location is one nobody else can check")
	}
	if *landing != "" {
		rec.Landing = *landing
	}
	if rec.Landing == "" {
		rec.Landing = rec.URL
	}
	if *access != "" {
		class := corpus.Access(*access)
		if !class.Valid() || class == corpus.AccessUnknown {
			return fmt.Errorf("%q is not an access class to adopt into: use one of %s", *access, classNames())
		}
		rec.Access = class
	}
	if rec.Access == "" || rec.Access == corpus.AccessUnknown {
		// Restricted is what a location with no licence behind it means, and
		// it is the only honest default: it reads the paper and publishes
		// nothing out of it but the front matter.
		rec.Access = corpus.AccessRestricted
	}
	if *licence != "" {
		rec.Licence = *licence
	}
	if *note != "" {
		rec.Note = *note
	}
	rec.By = corpus.ByHand
	rec.SHA256 = file.SHA256
	rec.Fetched = time.Now().UTC().Format(time.DateOnly)
	byID[*id] = rec

	if err := writeSources(c, byID); err != nil {
		return err
	}
	fmt.Printf("  %-34s %s %d KB, %s, by hand from %s\n", rec.ID, short(rec.SHA256), file.Bytes>>10, rec.Access, rec.URL)
	fmt.Println("wrote", c.SourcesManifest())
	return nil
}

// classNames is the access classes a person may adopt into. Unknown is not
// among them: unknown means nothing was found, and something was found here.
func classNames() string {
	return "public-domain, open, permissive, restricted"
}
