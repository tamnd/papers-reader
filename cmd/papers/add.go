package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/sources"
)

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	arxiv := fs.String("arxiv", "", "the arXiv id of the paper, in any form")
	doi := fs.String("doi", "", "the DOI of the paper, in any form")
	id := fs.String("id", "", "the id to file it under, rather than the suggested one")
	field := fs.String("field", "", "the group of the manifest it belongs in")
	aka := fs.String("aka", "", "other names the paper goes by, comma separated")
	idea := fs.String("core-idea", "", "one line on what the paper is for")
	difficulty := fs.Int("difficulty", 0, "how hard it is to read, 1 to 5")
	noCache := fs.Bool("no-cache", false, "ignore the response cache and ask the service again")
	dry := fs.Bool("dry-run", false, "print the entry and write nothing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers add -arxiv <id> [flags]
       papers add -doi <doi> [flags]

Looks a paper up by its identifier and writes one entry into
manifests/papers.yaml, at the end of the group its field belongs to.

The identifier is a pin. Everything else in the entry is what the service
said, and the entry is printed before it is written so that the two can be
told apart. Nothing is fetched and nothing is resolved: the next steps are
papers resolve and papers fetch, and this prints them.

The id it suggests is the first author's surname, the year, and the first
word of the title that means anything. That is right about as often as not,
because the keyword half the corpus uses is a name the field gave the paper
rather than a word out of its title, so read it and pass -id if it is wrong.
An id can never be changed afterwards.

The group comes from the arXiv primary category where there is only one
sensible reading of it and from -field otherwise, and a DOI carries no
category at all so -field is required with -doi. The groups are:

    %s

A paper added here has no number. The hundred of the seed list keep theirs
for ever so that a reader who arrives from the numbered list can find entry
63, and that only works if nothing is ever numbered 101.

`, fieldNames())
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch {
	case *arxiv == "" && *doi == "":
		return fmt.Errorf("say which paper to add: -arxiv or -doi")
	case *arxiv != "" && *doi != "":
		return fmt.Errorf("a paper is added by one identifier or the other, not both")
	}

	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	cache := c.Work("resolve")
	if *noCache {
		cache = ""
	}
	client := sources.NewClient(papers.Version, cache)

	ctx := context.Background()
	var cand sources.Candidate
	if *arxiv != "" {
		cand, err = sources.ArXivByID(ctx, client, *arxiv)
	} else {
		cand, err = sources.CrossrefByDOI(ctx, client, *doi)
	}
	if err != nil {
		if sources.NotFound(err) {
			return fmt.Errorf("no such paper: the service has no record of %s", firstOf(*arxiv, *doi))
		}
		return err
	}

	entry, err := newEntry(cand, *id, *field, *aka, *idea, *difficulty)
	if err != nil {
		return err
	}
	text, err := corpus.EntryText(entry)
	if err != nil {
		return err
	}
	fmt.Printf("%s says:\n\n%s\n", cand.Source, text)

	if *dry {
		fmt.Println("dry run, nothing written")
		return nil
	}
	if err := corpus.AppendPaper(c.PapersManifest(), entry); err != nil {
		return err
	}
	fmt.Println("wrote", c.PapersManifest())
	fmt.Printf("next: papers resolve -id %s, then papers fetch -id %[1]s\n", entry.ID)
	return nil
}

// newEntry turns what a service said into a manifest entry, with the flags
// winning wherever a person has said something.
//
// Status is listed and nothing else can set it. The statuses are a pipeline
// and a paper that has just been named has not been through any of it, so an
// entry arriving as fetched or extracted would be a claim about files that
// are not there.
func newEntry(cand sources.Candidate, id, field, aka, idea string, difficulty int) (corpus.Paper, error) {
	p := corpus.Paper{
		ID:         id,
		Title:      cand.Title,
		Authors:    cand.Authors,
		Year:       cand.Year,
		Venue:      cand.Venue,
		ArXiv:      cand.ArXiv,
		DOI:        cand.DOI,
		Difficulty: difficulty,
		CoreIdea:   idea,
		Status:     corpus.Listed,
	}
	if aka != "" {
		for _, s := range strings.Split(aka, ",") {
			if s = strings.TrimSpace(s); s != "" {
				p.AKA = append(p.AKA, s)
			}
		}
	}
	if p.ID == "" {
		var err error
		if p.ID, err = corpus.SuggestID(p.Authors, p.Year, p.Title); err != nil {
			return p, fmt.Errorf("%w, so pass -id", err)
		}
	}
	var err error
	if p.Field, err = groupOf(field, cand); err != nil {
		return p, err
	}
	if difficulty < 0 || difficulty > 5 {
		return p, fmt.Errorf("difficulty is 1 to 5, not %d", difficulty)
	}
	// The access class is left for papers resolve, which is the command that
	// reads the licence and writes the fact down. Guessing it here would be
	// writing a guess into the field the file's own header says is a guess,
	// which is one guess too many.
	return p, nil
}

// groupOf is the manifest group the paper belongs in: what -field said, or
// what the arXiv primary category makes obvious, or an error naming both
// ways out.
func groupOf(field string, cand sources.Candidate) (corpus.Field, error) {
	if field != "" {
		f := corpus.Field(strings.TrimSpace(field))
		for _, known := range corpus.Fields {
			if f == known {
				return f, nil
			}
		}
		return "", fmt.Errorf("%q is not one of the groups: %s", field, fieldNames())
	}
	if f, ok := sources.FieldOf(cand.Category); ok {
		return f, nil
	}
	if cand.Category != "" {
		return "", fmt.Errorf("arXiv files this under %s, which the corpus reads more than one way, so say -field", cand.Category)
	}
	return "", fmt.Errorf("nothing here says which group the paper belongs in, so say -field")
}

// firstOf is the first of the arguments that is not empty, for an error
// message that has to name whichever identifier was given.
func firstOf(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
