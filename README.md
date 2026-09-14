# papers-reader

The toolchain that builds [tamnd/papers](https://github.com/tamnd/papers), and the reading app that serves it.

One Go binary takes a paper from a line in a manifest to tagged Markdown in four languages, and an Astro site turns that corpus into something you can read on a phone.

The model plumbing underneath is [tamnd/llm](https://github.com/tamnd/llm).

## What it does

```
resolve    find where a paper can legally be fetched from, and under what licence
fetch      download it, hash it, record it
classify   measure what each PDF's text layer is worth, and pick the path
render     rasterise the pages of the papers that need a model to read them
extract    turn pages into Markdown, with the mathematics as LaTeX
figures    crop the diagrams out of the pages
pagemap    record which page of the file is which page of the paper
refs       parse the bibliography and link the citations
split      cut the paper into one file per section
tags       hand out permanent identifiers
translate  produce Vietnamese, Chinese and Japanese
roundtrip  put a sample of the translations back into English and judge them
audit      check the result against numbered rules
report     write what the corpus knows about itself: coverage, the citation graph, and what it cost
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

**A model that drops a paragraph is caught by the file it was reading.**
Nine rules decide whether a page is accepted, and eight of them ask whether the answer is well formed, which a page missing its last paragraph still is.
The ninth compares the reading against the page's own text layer and refuses a page that has no answer for twenty consecutive words the file was typeset from.
It was written for page 5 of the Bitcoin paper, where the reader stopped at the transaction diagram and left the paragraph under it out, and that page had passed everything else.
`papers extract --recheck` runs the rules over pages already on disk and asks no model, which is how a rule added this late gets applied to everything read before it.

**Tags are permanent.**
A section keeps its tag across re-extraction, re-splitting and renumbering, which is what lets a link written today survive the paper being read again by a better model next year.

**A stub is not a shortfall.**
`papers report coverage` writes `reports/coverage.md`, which counts every paper as full, stub or none, per field and per language.
A restricted paper gets its front matter and a short abstract and that is the whole of what its licence allows, so it counts as done rather than as a paper somebody forgot.
The last table in the report is the one to act on: it names what each unfinished paper is waiting on, which is a fetch, a licence check, a layout tool or a vision model, and the count behind each of those is what decides whether to go and get it.

**The corpus cites itself, and the graph says how much.**
`papers report graph` writes `reports/graph.md`: which paper cites which, which are cited most, and which are connected to nothing yet.
An edge is a bibliography entry of one paper here that was resolved to another paper here, so almost every reference points somewhere else and the edge count is small next to the reference count.
The last table is the reading list: the papers outside the corpus that two or more papers inside it cite, which is what decides what to add next.

**What it cost is written down.**
Every ask put to a model is one line in a ledger, and `papers report usage` rolls that up into `reports/usage.md` by stage, by model and by paper.
The ledger lives beside the routing table rather than in the corpus, because it names the hosts that were asked, and the report names none of them.
A model that is not in a price table gets a dash in the money column rather than a zero, because a subscription costs a turn and not a sum of money, and printing zero dollars would be claiming a measurement nobody made.

`papers report all` writes all four of these and the audit in one pass, which is the step before publishing.
Running the commands one at a time is four chances to forget one, and a corpus whose coverage says ninety seven per cent while its audit was written a week ago is worse than one with no reports at all.

**The reading app reads a build, not the corpus.**
`papers emit` turns the corpus into static JSON: `index.json`, the catalogue with every paper, field and reading list, `graph.json`, the citation graph in the shape a chart wants rather than the shape a table wants, `p/<id>/<lang>.json`, one paper in one language, whole, and `search-<lang>.json`, an inverted index over that language.
A page is a list of blocks, and every language of a paper has the same blocks in the same order with the same indices, which is what makes reading two languages side by side a matter of putting block i against block i with no diffing and no guessing.
The formulas are rendered by KaTeX at build time rather than in the browser, so a page costs no JavaScript to read and does not reflow under the reader.
The HTML on a page is held to an allowlist of seventeen elements with nothing on it that can execute, which matters here more than it would in most places: most of this text was written by a model reading a photograph of a page, and a model that returned a script tag instead of a sentence has to produce a broken paragraph and not a broken site.
Rules P01, P02 and P03 are the three that fail a build whose formulas will not render, whose links point at nothing, or whose figures are not there.
Search runs in the browser against the emitted index rather than against a search service, because there is no server between the reader and the corpus anywhere else on this site and sending every query somebody types somewhere else would mean their reading list existing somewhere else.
The whole English index is nine hundred kilobytes, under three hundred gzipped, and each language is a separate file loaded on the first search, so a reader reading the Vietnamese does not pay for the Japanese.
The shape of both is pinned by `schema/site.schema.json`, which the Go emitter and the TypeScript reader both validate against, so the two sides cannot drift apart without something saying so.
Audit rule P05 runs that validation over a build of the corpus on every audit, which is what makes the file a contract rather than documentation: a change to the shape has to move the schema, the emitter and the app in one commit or the build fails.
Nothing it writes is committed, and a site directory can be deleted and built again from a checkout at any time.

**The app is Astro over that build, and mostly no JavaScript.**
`web/` is the reading app: the catalogue as a grid by field, a page per field and per reading list, a page per paper saying what is in it and what cites it, the paper itself in each language, and the same paper in two languages side by side.
It reads the emitted JSON off `web/public` at build time and generates static pages, so there is no server and nothing is fetched to read a paper.
The types it reads the build with are generated from the same schema the emitter validates against, committed so the app builds without a generator, and checked in CI so the committed copy cannot be an old one.
Side by side is block i against block i, drawn as one grid so the two columns cannot drift, and a block that is in one language and not the other is drawn as a gap and said out loud rather than closed up.
There is no CSS framework: the corpus is text and the typography is the design.
`make site CORPUS=<papers>` builds the emit and then the app, and `npm run dev` in `web/` serves it against whatever was last emitted.

**A draft says it is a draft.**
A language whose glossary covers less than ninety per cent of the terms is emitted with `draft` against it, and the page says so.
It is still offered, because hiding it would be the same corpus with less of it visible and no more of it true.
Rule P04 is the one that checks the emitter has not quietly stopped saying it.

**The audit is a contract, not a lint.**
Ninety-two numbered rules in nine groups, each one a sentence you can argue with.
A rule reports pass, fail, or not run, and those are three different states.
Hard rules fail the build.
Three of them exist because the other three states can hide the worst outcome.
M14 is for a paper whose formulas were flattened into the prose: there is no mathematics left to check, so every rule about mathematics reports that it had nothing to look at and the audit comes back green over a paper that was destroyed.
C08 is the same failure in the C group, where a listing that never got a fence passes every rule about fences by not having one.
T11 is the third of them: a table the reader answered in raw HTML has no mathematics the M rules can see and no fences the C rules can count, so the file reads as clean prose and the table is unreadable.

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
render/          pages to pictures, for the readers that need one
work/            the model queue, the routing table and the waiting
prompt/          what the models are asked, pinned and hashed
extract/         PDF to Markdown
mathtex/         LaTeX repair, equation numbers and cross references
katex/           the real KaTeX, run at build time to check and render
code/            where the program text of a page starts and stops
figures/         cropping diagrams out of pages
pagemap/         the page of the file against the page of the paper
refs/            bibliography parsing and citation linking
split/           one paper into one file per section
tags/            the permanent identifier register
translate/       the four language pipeline
glossary/        the controlled vocabulary
roundtrip/       the back translation check on a sample of the translations
audit/           the numbered rules
report/          the coverage, graph, resolve and usage reports
emit/            JSON for the reading app
schema/          site.schema.json, the contract between the emitter and the app
web/             the Astro reading app
```

## Requirements

Go 1.27 or later.
`pdftotext` and `pdftoppm` from Poppler for the extraction commands.
Node 22 or later for the web app.
Nothing else. The Go side has four dependencies: `gopkg.in/yaml.v3`, `github.com/tamnd/llm`, which is ours and has none of its own, `github.com/dop251/goja`, a JavaScript engine that exists so the toolchain can run the real KaTeX to check every formula it writes without putting Node in the build, and `github.com/santhosh-tekuri/jsonschema`, which runs the site schema so that the contract with the reading app is checked by a validator rather than by hand.

## Licence

MIT. See [LICENSE](LICENSE).

The corpus it builds is licensed separately, per paper, and that is explained in the corpus repository.
