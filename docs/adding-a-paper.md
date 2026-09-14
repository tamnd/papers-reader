# Adding a paper

What it takes to go from a paper somebody mentioned to a paper the corpus holds, in commands, in time and in money.

The short answer is that adding the entry is free and instant, and everything after it costs what reading the pages costs. The numbers below are measured off the ledger of the run that built the current corpus rather than estimated, so they are what this fleet did on these papers and not a general claim about models.

## The commands

```
papers add -arxiv 1706.03762           # or -doi 10.1145/359340.359342 -field security
papers resolve -id vaswani-2017-attention
papers fetch -id vaswani-2017-attention
papers extract -id vaswani-2017-attention
papers assemble -id vaswani-2017-attention
papers split -corpus . -id vaswani-2017-attention -prune
papers tags assign -corpus . -id vaswani-2017-attention
papers refs build -id vaswani-2017-attention
papers translate -id vaswani-2017-attention
papers report coverage -write
papers audit -hard
papers publish
```

`papers split` and `papers tags assign` are one step in two commands and always run as a pair. Splitting strips the attribute blocks that carry the permanent tags and assigning puts them back, so a split without an assign after it leaves the paper untagged and the audit says so.

`papers add` writes one entry into `manifests/papers.yaml` and stops there. It does not resolve, it does not fetch and it does not read anything, because those are decisions about a licence and about machine time that should be taken one at a time and not as a side effect of naming a paper.

`papers suggest` is the other way in. It lists works that several papers already in the corpus cite and that the corpus does not hold, with the `papers add` line to run for each one that carries an identifier. That is the only part of growing the corpus a program can do on its own: which papers the ones already here keep pointing at is a fact sitting in the bibliographies, and everything else about whether to add a paper is a judgement.

## What each step costs

Nothing before `extract` asks a model anything.

| step | what it does | cost |
| --- | --- | --- |
| add | one lookup at arXiv or Crossref, cached | under a second, no model |
| resolve | walks the ladder until a URL has a licence, cached | a few seconds, no model |
| fetch | one download, hashed | seconds, no model |
| extract | one ask per page | see below |
| assemble, split, tags | text handling | seconds, no model |
| refs | parses the bibliography and links the citations | seconds, no model |
| translate | one ask per section per language | 4,461 tokens in and 483 out, 1m 41s |
| roundtrip | one ask per sampled page | 12,159 tokens in and 240 out, 22s |
| glossary | one ask per term per language, for the whole corpus | 1,127 tokens in and 145 out, 1m 48s |
| audit, report, publish | reading what is already written | seconds, no model |

Glossary is the odd row: a term is agreed once for the whole corpus and every paper after that reuses it, so it is a cost of the language and not a cost of the paper.

Extraction is the one that matters. Over the 760 answered asks of the current corpus it came to 12,134 tokens in and 1,193 tokens out per page, and 46 seconds of waiting per page. The waiting is the fleet and not the work: a page goes to whichever route is free, a route that does not answer sends the page to the next one, and the slowest papers in `reports/usage.md` are the ones that spent their time being handed around rather than being read.

## Two shapes of paper

A restricted paper is capped at three pages by `restrictedPages`, because three pages is enough to read a title, an author list and an abstract and the corpus may not carry more of it than that. It is about 36 thousand tokens in, 3,500 out and two and a half minutes, and it produces one section. Eighty eight of the hundred and one papers in the manifest are this shape.

A full text paper is between 8 and 18 pages and between 8 and 18 sections, 11 at the median. End to end in all three languages that is roughly:

- extract: 13 pages, 158 thousand tokens in, 15 thousand out, ten minutes
- translate: 11 sections across three languages, 33 asks, 147 thousand tokens in, 16 thousand out, 55 minutes

So about 305 thousand tokens in and 31 thousand out, and something over an hour of waiting, most of which is unattended. Roundtrip is on top of that and is priced per page judged, because it runs on a sample rather than on everything.

## Money

Nothing in the run that built this corpus was billed by the token. The ChatGPT and Codex routes are subscriptions paid by the month whether or not a paper is read, and the olmOCR route is a GPU in the house that was paid for once, so the marginal cost of a page is electricity. `reports/usage.md` prints `$0.00` for that reason and prints a sentence under each model saying which kind of nothing it is, because a zero with nothing behind it reads like a rate nobody looked up.

To price a run on a metered API, put the rate in `manifests/prices.json` as dollars per million tokens by model name and `papers report usage` does the multiplication. Cached input is charged at the full input price there, which overstates the bill, and a report that overstates is the one to prefer.

## What to check before running any of it

The id `papers add` suggests is the first author's surname, the year and the first word of the title that means anything, and that is right about as often as not. The keyword half of the ids in this corpus is the name the field gave the paper rather than a word from its title, which is why it is `dijkstra-1968-the` and `brooks-1987-nosilverbullet`. Read the suggestion and pass `-id` when it is wrong, because an id is permanent: it is the directory name, the tag prefix and the URL, and changing one breaks every link into the paper.

The group is the other thing worth a second look. It comes from the arXiv primary category where there is only one sensible reading of it and from `-field` otherwise, and a DOI carries no category at all so `-field` is required with `-doi`.

A paper added here has no number. The hundred of the seed list keep theirs for ever so that a reader arriving from the numbered list can find entry 63, and that only works if nothing is ever numbered 101.
