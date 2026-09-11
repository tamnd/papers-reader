package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	papers "github.com/tamnd/papers-reader"
	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/fetch"
	"github.com/tamnd/papers-reader/relay"
)

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	root := fs.String("corpus", "", "path to a checkout of tamnd/papers")
	ids := fs.String("id", "", "fetch these papers only, comma separated")
	field := fs.String("field", "", "fetch one field only")
	all := fs.Bool("all", false, "fetch every paper that resolved and is not here yet")
	again := fs.Bool("again", false, "download papers that are already on disk too")
	limit := fs.Int("limit", 0, "stop after this many downloads")
	dry := fs.Bool("dry-run", false, "print what would be fetched and download nothing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: papers fetch [flags]

Downloads every paper that resolved to a location, into pdf/, which is
gitignored and never committed under any licence and never served anywhere.

Downloading and publishing are different questions. A PDF on the open web
may be read, and that is what the extraction stages do with it. What may be
published out of it is decided later by the licence: a restricted paper is
read and measured like any other and still publishes nothing but its front
matter and a short abstract. Only unknown papers are skipped, and only
because unknown means nothing was found, so there is nowhere to fetch from.

What comes back has to be a real PDF. It has to begin %s, be over %d bytes
and under %d, and arrive with a 200. Anything else leaves no file behind,
because a file in pdf/ is how every later stage knows a paper was fetched.

A paper already on disk whose hash matches its record is left alone, so
running this twice costs nothing. A paper whose hash does not match is
reported and not overwritten, because a file that changed under us is
something a person should look at.

Some hosts refuse a network rather than a program, and a few of them hold
the only copy of a paper. If %s or ~/.config/papers/relay.json names
ssh destinations, a download that failed from here is tried again from one
of them, with the same User-Agent and the same checks on what comes back.

`, fetch.Magic, fetch.Floor, fetch.Ceiling, relay.Env)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *ids == "" && *field == "" && !*all {
		return fmt.Errorf("say which papers to fetch: --id, --field or --all")
	}

	c, err := openCorpus(*root)
	if err != nil {
		return err
	}
	if err := ignored(c); err != nil {
		return err
	}
	manifest, err := c.LoadPapers()
	if err != nil {
		return err
	}
	recorded, err := c.LoadSources()
	if err != nil {
		return err
	}

	todo, err := selectPapers(manifest, recorded, *ids, *field, true)
	if err != nil {
		return err
	}

	f := fetch.New(papers.Version)
	hops, err := relay.Load()
	if err != nil {
		return err
	}
	f.Relay = hops
	if !hops.Empty() {
		fmt.Printf("a host that refuses this network will be tried again through %s\n", hops)
	}
	byID := index(recorded)
	ctx := context.Background()

	var got, held, refused, already, caught int
	for _, p := range todo {
		rec, _ := recorded.ByID(p.ID)
		if ok, why := fetch.May(rec); !ok {
			fmt.Printf("  %-34s not fetched, it %s\n", p.ID, why)
			held++
			continue
		}
		dest := c.PDF(p.ID)
		if !*again {
			if sum, _, err := fetch.Sum(dest); err == nil && (rec.SHA256 == "" || sum == rec.SHA256) {
				if rec.SHA256 == "" {
					// The file is here and the record does not know it.
					// That is what a run killed between the download and
					// the write leaves behind, and it matters, because the
					// hash is the only thing that later tells the corpus
					// whether the file it has is the file it fetched.
					// Writing it down costs a read of a file that is
					// already on this disk, so there is no reason to make
					// anybody download the paper a second time for it.
					out := byID[p.ID]
					out.SHA256 = sum
					if out.Fetched == "" {
						if info, err := os.Stat(dest); err == nil {
							out.Fetched = info.ModTime().UTC().Format(time.DateOnly)
						}
					}
					byID[p.ID] = out
					caught++
					continue
				}
				already++
				continue
			}
		}
		if *dry {
			fmt.Printf("  %-34s would fetch %s\n", p.ID, rec.URL)
			continue
		}

		file, err := download(ctx, f, c, p.ID, rec, dest)
		if err != nil {
			fmt.Printf("  %-34s %v\n", p.ID, err)
			if errors.Is(err, fetch.ErrNotPDF) {
				refused++
			}
			continue
		}
		fmt.Printf("  %-34s %s %d KB\n", p.ID, short(file.SHA256), file.Bytes>>10)

		out := byID[p.ID]
		out.Fetched = file.Fetched.Format(time.DateOnly)
		out.SHA256 = file.SHA256
		byID[p.ID] = out
		got++

		if *limit > 0 && got >= *limit {
			fmt.Printf("stopping at the limit of %d\n", *limit)
			break
		}
	}

	if *dry {
		fmt.Println("dry run, nothing downloaded")
		return nil
	}
	if got > 0 || caught > 0 {
		if err := writeSources(c, byID); err != nil {
			return err
		}
		fmt.Println("wrote", c.SourcesManifest())
	}
	fmt.Printf("%d papers, %d fetched, %d already here, %d with nowhere to fetch from, %d refused as not a paper\n",
		len(todo), got, already, held, refused)
	if caught > 0 {
		fmt.Printf("%d were on disk with no hash in the record, and now have one\n", caught)
	}
	return nil
}

// download gets one paper, and protects a file that is already here.
//
// A record with a hash has been fetched before, and the corpus has a text
// layer measurement, page numbers and possibly figures that all refer to that
// exact file. So the new copy goes to the work directory first and only moves
// into place if it is the same file. A publisher that silently reissues a
// paper is a thing that happens, and finding out by having the page numbers
// drift is a bad way to find out.
func download(ctx context.Context, f *fetch.Fetcher, c *corpus.Corpus, id string, rec *corpus.Source, dest string) (fetch.File, error) {
	if rec.SHA256 == "" {
		return f.Get(ctx, rec.URL, dest)
	}
	staging := filepath.Join(c.Work("fetch"), id+".pdf")
	file, err := f.Get(ctx, rec.URL, staging)
	if err != nil {
		return file, err
	}
	if file.SHA256 != rec.SHA256 {
		os.Remove(staging)
		return fetch.File{}, fmt.Errorf("the file at %s is not the one recorded: it hashes to %s and the record says %s, so it was left where it was", rec.URL, short(file.SHA256), short(rec.SHA256))
	}
	if err := os.Rename(staging, dest); err != nil {
		return fetch.File{}, err
	}
	file.Path = dest
	return file, nil
}

// ignored refuses to download anything into a corpus that would commit it.
//
// Audit rule S03 catches a PDF that got into the index, which is the check
// that matters and it runs too late to help here. This is the cheap one: if
// pdf/ is not ignored then the very next `git add .` puts a hundred PDFs in a
// public repository, so the fetcher stops before writing the first one.
func ignored(c *corpus.Corpus) error {
	b, err := os.ReadFile(filepath.Join(c.Root, ".gitignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s has no .gitignore, so pdf/ would be committed: refusing to download anything", c.Root)
		}
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "pdf/" {
			return nil
		}
	}
	return fmt.Errorf("%s/.gitignore does not ignore pdf/, so a download would end up committed: refusing to download anything", c.Root)
}

// short is a hash, shortened for a terminal. The full one goes in the record.
func short(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12]
}
