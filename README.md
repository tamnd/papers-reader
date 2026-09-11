# papers-reader

The toolchain that builds [tamnd/papers](https://github.com/tamnd/papers), and the reading app that serves it.

One Go binary takes a paper from a line in a manifest to tagged Markdown in four languages, and an Astro site turns that corpus into something you can read on a phone.

The model plumbing underneath is [tamnd/llm](https://github.com/tamnd/llm).

## What it does

```
resolve    find where a paper can legally be fetched from, and under what licence
fetch      download it, hash it, record it
classify   measure what each PDF's text layer is worth, and pick the path
extract    turn pages into Markdown, with the mathematics as LaTeX
figures    crop the diagrams out of the pages
refs       parse the bibliography and link the citations
split      cut the paper into one file per section
tags       hand out permanent identifiers
translate  produce Vietnamese, Chinese and Japanese
audit      check the result against numbered rules
emit       build the JSON the reading app consumes
```

Each of those is a subcommand of `papers`, and each one is idempotent.
Run it twice on a finished paper and it does nothing the second time.

## Install

```sh
go install github.com/tamnd/papers-reader/cmd/papers@latest
```

Point it at a checkout of the corpus, either with `PAPERS_CORPUS` or by running it from inside one.

```sh
export PAPERS_CORPUS=~/github/tamnd/papers
papers list --field ai-ml
papers audit --hard
```

## Design

**The corpus is data and this is the only thing that writes it.**
Every path, every manifest schema and every access rule lives in `corpus/`, so a change to the shape of the corpus is a change to one package.

**Nothing is published on a guess.**
`papers resolve` accepts a candidate only when the title similarity clears 0.92, the year is within one, and at least one surname matches.
A paper that fails any of the three stays unresolved, and an unresolved paper publishes nothing at all.

**Being able to download something is not permission to republish it.**
A paper with no licence anybody can name is restricted, which publishes its title, its authors, its year, its links and an abstract under 250 words, and no body text and no figures.
That is the default for most of the corpus, and the number of papers whose text may be published is written at the top of `reports/resolve.md` every time the resolver runs.

**The text layer is measured, not guessed at.**
`papers classify` counts characters, mathematical glyphs, embedded fonts and full page images over a band of body pages, and decides from the numbers whether `pdftotext` alone can read the file.
The year is a hint and nothing more: there are 1980 papers with a clean text layer and 2005 papers that are photographs of a printout.

**Extraction says how it was done.**
Pages read by `pdftotext` on a born digital file are marked `native` and were never guessed by a model.
Pages read by a layout model or a vision model say so, and the audit treats them differently.

**Tags are permanent.**
A section keeps its tag across re-extraction, re-splitting and renumbering, which is what lets a link written today survive the paper being read again by a better model next year.

**The audit is a contract, not a lint.**
Eighty-four numbered rules in nine groups, each one a sentence you can argue with.
A rule reports pass, fail, or not run, and those are three different states.
Hard rules fail the build.

## Layout

```
cmd/papers/      the command line
corpus/          paper ids, fields, access classes, manifests, front matter
sources/         resolving a paper to a URL and a licence
fetch/           downloading and hashing
polite/          one request at a time per host, with a floor on the gap
poppler/         the PDF tools, found and version checked
relay/           asking another machine to fetch what this network cannot
classify/        what a PDF's text layer is worth
work/            the model queue, the routing table and the waiting
extract/         PDF to Markdown
mathtex/         LaTeX repair and validation
figures/         cropping diagrams out of pages
refs/            bibliography parsing and citation linking
split/           one paper into one file per section
tags/            the permanent identifier register
translate/       the four language pipeline
glossary/        the controlled vocabulary
audit/           the numbered rules
report/          the coverage, resolve and usage reports
emit/            JSON for the reading app
web/             the Astro reading app
```

## Requirements

Go 1.27 or later.
`pdftotext` and `pdftoppm` from Poppler for the extraction commands.
Node 22 or later for the web app.
Nothing else. The Go side has two dependencies: `gopkg.in/yaml.v3`, and `github.com/tamnd/llm`, which is ours and has none of its own.

## Licence

MIT. See [LICENSE](LICENSE).

The corpus it builds is licensed separately, per paper, and that is explained in the corpus repository.
