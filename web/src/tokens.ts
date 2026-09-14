// The query side of the search index.
//
// This is a port of Tokens in emit/search.go and it has to stay one. The
// index was tokenised by that function at build time, so a query tokenised
// any other way looks up terms that are not in it: one character of drift
// here and a search for a Japanese word silently returns nothing. The test
// beside this file is the same table as the Go test, which is what makes
// the pair of them one thing rather than two.
//
// Tokenisation is by script and not by language. A Chinese term quoted in
// an English paper is indexed and found the same way in either file, and
// one function serves all four.

// A character of a script written without spaces between its words.
//
// The two single characters are there because Unicode files them under the
// Common script rather than under Katakana or Han, and they are ordinary
// letters of Japanese: the prolonged sound mark is the second character of
// ネットワーク and the iteration mark is the second character of 人々.
// Without them a word is cut in half at the point a reader is most likely
// to type.
const wide =
  /[ー々\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u;

// A letter or a decimal digit, matching Go's unicode.IsLetter and
// unicode.IsDigit rather than the looser \p{N}, which would take Roman
// numerals and fractions in as digits.
const letter = /[\p{L}\p{Nd}]/u;

/**
 * The terms of a string, lowercased.
 *
 * Latin and Vietnamese cut on anything that is not a letter or a digit, and
 * a token of one character is dropped: it matches most of the corpus and it
 * is never what anybody typed. Chinese, Japanese and Korean cut into
 * overlapping pairs, which is crude and is what a hundred paper corpus can
 * afford in a browser without shipping a segmenter with it. A single
 * character of one of those scripts is a token of its own, because there is
 * no pair to make and it is a word often enough.
 */
export function tokens(s: string): string[] {
  const out: string[] = [];
  let run: string[] = [];
  let cjk = false;
  const flush = () => {
    if (run.length === 0) return;
    if (!cjk) {
      if (run.length > 1) out.push(run.join(""));
    } else if (run.length === 1) {
      out.push(run[0] as string);
    } else {
      for (let i = 0; i + 1 < run.length; i++) {
        out.push(`${run[i]}${run[i + 1]}`);
      }
    }
    run = [];
  };
  for (const r of s.toLowerCase()) {
    if (!letter.test(r)) {
      flush();
    } else if (wide.test(r) !== cjk) {
      flush();
      cjk = wide.test(r);
      run.push(r);
    } else {
      run.push(r);
    }
  }
  flush();
  return out;
}
