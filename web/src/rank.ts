// Ranking a query against a search index.
//
// Apart from the component that draws the results, because this is the part
// that can be wrong without looking wrong. A result list is plausible
// whatever order it is in, so the ordering rules are written here where a
// test can state what they are and hold them to it.

// The imports below spell the extension, because this module and the
// tokeniser it uses are run by node directly for the tests beside them, and
// node resolves what is on the disk rather than what a bundler would have
// guessed.
import type { Post, Postings, Search } from "./types.gen.ts";
import { tokens } from "./tokens.ts";

export interface Hit {
  post: Post;
  /** How many of the query's terms this block matched. */
  terms: number;
  score: number;
}

// A hit in the mathematics or the program text counts for less than a hit in
// the prose, so a reader who typed a word gets the paragraph that uses it
// above the formula that happens to contain the same letters. It counts for
// something, because a reader who typed softmax may well want the formula.
const proseWeight = 1;
const symbolWeight = 0.6;
// The last term of a query is the one still being typed, so it matches on a
// prefix too. A prefix is weaker evidence than a whole word.
const prefixWeight = 0.5;
// A section heading says what a stretch of a paper is about, so a hit in one
// is worth more than a hit in a paragraph of it.
const headingBonus = 1.4;

/**
 * How much of a term has to be typed before it matches on a prefix.
 *
 * Below this a prefix matches so much of the corpus that the list changes
 * under the reader's hands as they type, which reads as the search being
 * broken rather than as it being eager.
 */
export const enough = 3;

/**
 * The keys of both indexes of one language.
 *
 * Kept by the caller rather than taken from the index here, because prefix
 * matching walks them on every keystroke and Object.keys over twenty
 * thousand terms is not something to do four times a second.
 */
export interface Keys {
  terms: string[];
  symbols: string[];
}

/** The keys of an index, for passing to rank. */
export function keysOf(index: Search): Keys {
  return { terms: Object.keys(index.terms), symbols: Object.keys(index.symbols) };
}

/**
 * The blocks matching a query, best first.
 *
 * Ordered by how many of the query's terms a block matched before anything
 * else. A two word query that no single block holds both words of still
 * answers rather than returning nothing, but every block holding both is
 * above every block holding one, which is the behaviour of an AND search
 * without the empty page an AND search gives you when you are one word too
 * specific.
 *
 * Ties break on the identifier and then on the block, so the same query
 * against the same index is the same list every time. A result list that
 * reshuffles between two keystrokes that produced the same matches is a
 * list nobody can click on.
 */
export function rank(index: Search, keys: Keys, query: string): Hit[] {
  const terms = tokens(query);
  if (terms.length === 0) return [];

  // Position in posts to what it has matched so far. The terms matched are
  // a bitmask rather than a count because one term can be found in both the
  // prose and the symbols of one block, and that is one term matched with
  // two pieces of evidence, not two terms matched.
  const total = new Map<number, { score: number; matched: number }>();
  const add = (at: number, by: number, term: number) => {
    const was = total.get(at);
    if (was === undefined) {
      total.set(at, { score: by, matched: 1 << term });
      return;
    }
    was.score += by;
    was.matched |= 1 << term;
  };

  terms.forEach((term, i) => {
    const last = i === terms.length - 1;
    const sides: [Postings, string[], number][] = [
      [index.terms, keys.terms, proseWeight],
      [index.symbols, keys.symbols, symbolWeight],
    ];
    for (const [postings, all, weight] of sides) {
      const exact = postings[term];
      if (exact) {
        for (const at of exact) add(at, weight, i);
        continue;
      }
      if (!last || term.length < enough) continue;
      for (const key of all) {
        if (!key.startsWith(term)) continue;
        for (const at of postings[key] ?? []) add(at, weight * prefixWeight, i);
      }
    }
  });

  const hits: Hit[] = [];
  for (const [at, got] of total) {
    const post = index.posts[at];
    if (post === undefined) continue;
    hits.push({
      post,
      terms: bits(got.matched),
      score: got.score * (post.k === "heading" ? headingBonus : 1),
    });
  }
  hits.sort(
    (a, b) =>
      b.terms - a.terms ||
      b.score - a.score ||
      a.post.p.localeCompare(b.post.p) ||
      a.post.i - b.post.i,
  );
  return hits;
}

/**
 * At most this many blocks of any one paper.
 *
 * Without it a paper that uses a word in every other paragraph takes the
 * whole page and the reader never learns that four other papers use it too.
 * Three is enough to show that a paper keeps coming back to the thing and
 * few enough to leave room for the rest of the corpus.
 */
export const perPaper = 3;

/** The hits with no more than per of any one paper, order preserved. */
export function capped(hits: Hit[], per: number = perPaper): Hit[] {
  const seen = new Map<string, number>();
  const out: Hit[] = [];
  for (const h of hits) {
    const had = seen.get(h.post.p) ?? 0;
    if (had >= per) continue;
    seen.set(h.post.p, had + 1);
    out.push(h);
  }
  return out;
}

function bits(n: number): number {
  let out = 0;
  for (let b = n; b !== 0; b >>>= 1) out += b & 1;
  return out;
}
