// What the result order is supposed to be.
//
// The index here is typed out rather than emitted, because the point of
// these cases is the ordering and a fixture small enough to read is one you
// can check the expected answer of by eye.
import assert from "node:assert/strict";
import { test } from "node:test";

import type { Post, Search } from "./types.gen.ts";
import type { Hit } from "./rank.ts";
import { capped, keysOf, perPaper, rank } from "./rank.ts";

function post(p: string, i: number, k: string, x: string, t?: string): Post {
  return { p, i, k: k as Post["k"], x, ...(t === undefined ? {} : { t }) };
}

/** An index over five blocks, with the postings written out by hand. */
function fixture(): Search {
  return {
    version: 1,
    lang: "en",
    posts: [
      post("a-1970-x", 0, "p", "a paragraph about networks", "One"),
      post("a-1970-x", 1, "math", "network gamma", "One"),
      post("b-1971-y", 0, "heading", "networks", "Two"),
      post("b-1971-y", 1, "p", "a paragraph about routing", "Two"),
      post("c-1972-z", 0, "p", "networks and routing together", "Three"),
    ],
    terms: {
      networks: [0, 2, 4],
      routing: [3, 4],
      paragraph: [0, 3],
      netmask: [3],
    },
    symbols: {
      networks: [1],
      gamma: [1],
    },
  };
}

const index = fixture();
const keys = keysOf(index);
const papers = (query: string) =>
  rank(index, keys, query).map((h) => `${h.post.p}:${h.post.i}`);
const found = (query: string) => papers(query).sort();

test("nothing matches nothing", () => {
  assert.deepEqual(rank(index, keys, ""), []);
  assert.deepEqual(rank(index, keys, ", . ;"), []);
  assert.deepEqual(rank(index, keys, "thermodynamics"), []);
});

test("a block with both words is above a block with one", () => {
  // c:0 has both. b:1 and b:0 have one each, and so do a:0 and a:1.
  assert.equal(papers("networks routing")[0], "c-1972-z:0");
  assert.equal(rank(index, keys, "networks routing")[0]?.terms, 2);
});

test("prose outranks the same word in a formula", () => {
  const order = papers("networks");
  assert.equal(order.indexOf("a-1970-x:1"), order.length - 1);
});

test("a heading outranks a paragraph on the same word", () => {
  assert.equal(papers("networks")[0], "b-1971-y:0");
});

test("a word in both the prose and the symbols of one block is one word", () => {
  // a:1 holds networks twice over, in the prose index and in the symbol
  // index. That is one term matched, so it stays below c:0, which matched
  // two different terms.
  const hits = rank(index, keys, "networks gamma");
  assert.equal(hits[0]?.post.p, "a-1970-x");
  assert.equal(hits[0]?.terms, 2);
});

test("only the last word matches on its beginning", () => {
  // net is the last word, so it reaches networks and netmask, which is
  // every block in the fixture.
  assert.deepEqual(found("net"), [
    "a-1970-x:0",
    "a-1970-x:1",
    "b-1971-y:0",
    "b-1971-y:1",
    "c-1972-z:0",
  ]);
  // With a word after it, net is no longer the one being typed and matches
  // nothing on its own, so only routing answers.
  assert.deepEqual(found("net routing"), ["b-1971-y:1", "c-1972-z:0"]);
});

test("two letters are too few to match on a beginning", () => {
  assert.deepEqual(papers("ne"), []);
});

test("the same query twice is the same order", () => {
  assert.deepEqual(papers("networks routing"), papers("networks routing"));
});

test("one paper cannot take the whole list", () => {
  const many: Hit[] = [0, 1, 2, 3, 4].map((i) => ({
    post: post("a-1970-x", i, "p", ""),
    terms: 1,
    score: 1,
  }));
  const mixed = [...many.slice(0, 2), ...([
    { post: post("b-1971-y", 0, "p", ""), terms: 1, score: 1 },
  ] as Hit[]), ...many.slice(2)];
  assert.equal(capped(many).length, perPaper);
  // The order the ranking put them in is kept, and the other paper is
  // still there after the cap took the tail of the first one.
  assert.deepEqual(
    capped(mixed).map((h) => `${h.post.p}:${h.post.i}`),
    ["a-1970-x:0", "a-1970-x:1", "b-1971-y:0", "a-1970-x:2"],
  );
});
