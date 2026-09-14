// The same table as TestTokensCutLatinOnPunctuationAndCJKIntoPairs and
// TestTokensDropsTheSingleLettersAndFoldsTheCase in emit/search_test.go.
//
// Deliberately the same table. The query side and the index side are two
// implementations of one function in two languages, and the only thing that
// can hold them together is running both against the same cases. A case
// added on one side belongs on the other.
import assert from "node:assert/strict";
import { test } from "node:test";

import { tokens } from "./tokens.ts";

test("Latin cuts on punctuation and CJK cuts into pairs", () => {
  const cases: [string, string][] = [
    ["The back-propagation rule.", "the|back|propagation|rule"],
    ["GPT-3 has 175B parameters", "gpt|has|175b|parameters"],
    ["Mạng đối kháng sinh mẫu", "mạng|đối|kháng|sinh|mẫu"],
    ["生成对抗网络", "生成|成对|对抗|抗网|网络"],
    ["敵対的生成ネットワーク", "敵対|対的|的生|生成|成ネ|ネッ|ット|トワ|ワー|ーク"],
    // A script boundary is a token boundary, so a Chinese term quoted in
    // an English sentence is findable in the English index.
    ["the term 网络 means network", "the|term|网络|means|network"],
  ];
  for (const [input, want] of cases) {
    assert.equal(tokens(input).join("|"), want, input);
  }
});

test("a single letter is dropped and the case is folded", () => {
  assert.equal(
    tokens("A Network Of Units, x and y").join("|"),
    "network|of|units|and",
  );
});

test("a lone CJK character is a term of its own", () => {
  assert.equal(tokens("网").join("|"), "网");
});
