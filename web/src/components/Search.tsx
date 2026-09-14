// Search, in the browser, over the index papers emit wrote.
//
// The index is fetched on the first query and not before, because it is
// most of a megabyte and most visits never search. One file per language,
// so a reader reading the Vietnamese never downloads the Japanese.
//
// There is no search service. There is no server between the reader and the
// corpus anywhere else on this site, and sending every query somebody types
// to one would mean the reading list of everybody who uses this site
// existing somewhere else.
//
// The ranking is in rank.ts, where it can be tested. What is left here is
// the box, the fetch and the drawing.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { Lang, Post, Search } from "../types.gen";
import { capped, keysOf, perPaper, rank } from "../rank";

interface Props {
  /** The site base, from import.meta.env.BASE_URL on the page, because an
   * island has no idea where it was mounted. */
  base: string;
  langs: Lang[];
  names: Record<string, string>;
  /** Paper to title, so a result line can name its paper without the index
   * carrying a hundred titles a few thousand times over. */
  titles: Record<string, string>;
}

/** How many results are drawn. Past this the ranking is guesswork and the
 * honest answer is that the query was too broad. */
const shown = 40;

/** The site base with exactly one slash on the end.
 *
 * import.meta.env.BASE_URL is "/papers-reader" here and "/" when the site
 * is served from a root, so neither assuming a trailing slash nor assuming
 * there is none produces a link that works in both. */
function root(base: string): string {
  return base.endsWith("/") ? base : `${base}/`;
}

// The parts of a snippet that matched, so a reader can see why a line came
// back. Marked on the words of the query and not on the terms of the index,
// which are pairs of characters and would light up half a Japanese sentence
// to show a match on one word of it.
function mark(text: string, query: string) {
  const words = query
    .toLowerCase()
    .split(/[^\p{L}\p{Nd}]+/u)
    .filter((w) => w.length > 1);
  if (words.length === 0) return text;
  const pattern = new RegExp(
    `(${words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|")})`,
    "giu",
  );
  return text
    .split(pattern)
    .map((part, i) =>
      words.includes(part.toLowerCase()) ? <mark key={i}>{part}</mark> : part,
    );
}

const kinds: Record<string, string> = {
  heading: "a heading",
  list: "a list",
  math: "a formula",
  figure: "a figure",
  code: "a listing",
  table: "a table",
  quote: "a quotation",
};

/** Where a hit is, in words, for the line under the title. */
function where(post: Post): string {
  const at = post.t !== undefined && post.t !== "" ? post.t : "the first page";
  const kind = kinds[post.k];
  return kind ? `${at}, ${kind}` : at;
}

export default function SearchBox({ base, langs, names, titles }: Props) {
  const fallback = (langs[0] ?? "en") as Lang;
  const [lang, setLang] = useState<Lang>(fallback);
  const [query, setQuery] = useState("");
  const [indexes, setIndexes] = useState<Partial<Record<Lang, Search>>>({});
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState("");
  const box = useRef<HTMLInputElement>(null);

  // The query and the language are in the address, so a search can be sent
  // to somebody else and a back button comes back to the results rather
  // than to an empty box.
  useEffect(() => {
    const at = new URLSearchParams(window.location.search);
    const q = at.get("q");
    const l = at.get("lang");
    if (q) setQuery(q);
    if (l && (langs as string[]).includes(l)) setLang(l as Lang);
    box.current?.focus();
  }, [langs]);

  useEffect(() => {
    const at = new URLSearchParams();
    if (query !== "") at.set("q", query);
    if (lang !== fallback) at.set("lang", lang);
    const search = at.toString();
    const to = search === "" ? window.location.pathname : `?${search}`;
    window.history.replaceState(null, "", to);
  }, [query, lang, fallback]);

  const load = useCallback(
    async (want: Lang) => {
      setLoading(true);
      setFailed("");
      try {
        const res = await fetch(`${root(base)}search-${want}.json`);
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
        const doc = (await res.json()) as Search;
        setIndexes((was) => ({ ...was, [want]: doc }));
      } catch (err) {
        setFailed(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    },
    [base],
  );

  const index = indexes[lang];
  const asked = query.trim() !== "";
  useEffect(() => {
    if (asked && indexes[lang] === undefined) void load(lang);
  }, [asked, lang, indexes, load]);

  const keys = useMemo(
    () => (index ? keysOf(index) : { terms: [], symbols: [] }),
    [index],
  );
  const hits = useMemo(
    () => (index && asked ? rank(index, keys, query) : []),
    [index, keys, asked, query],
  );
  const list = useMemo(() => capped(hits).slice(0, shown), [hits]);

  return (
    <div className="finder">
      <form role="search" onSubmit={(e) => e.preventDefault()}>
        <input
          ref={box}
          type="search"
          value={query}
          placeholder="a word, a name, a piece of a formula"
          aria-label="Search the corpus"
          onChange={(e) => setQuery(e.target.value)}
        />
        <span className="langs">
          {langs.map((l) => (
            <button
              key={l}
              type="button"
              aria-pressed={l === lang}
              onClick={() => setLang(l)}
            >
              {names[l] ?? l}
            </button>
          ))}
        </span>
      </form>

      {failed !== "" && (
        <p className="missing">
          The index for this language did not load: {failed}
        </p>
      )}
      {asked && index === undefined && loading && failed === "" && (
        <p className="lede">Fetching the index for this language.</p>
      )}
      {asked && index !== undefined && (
        <p className="lede">
          {hits.length === 0
            ? "Nothing in this language matches."
            : list.length < hits.length
              ? `${hits.length} blocks match. Here are ${list.length} of them, at most ${perPaper} from any one paper:`
              : `${hits.length} ${hits.length === 1 ? "block matches" : "blocks match"}:`}
        </p>
      )}

      <ol className="results">
        {list.map((h) => (
          <li key={`${h.post.p}/${h.post.s ?? ""}/${h.post.i}`}>
            <a
              href={`${root(base)}p/${h.post.p}/${lang}${h.post.s ? `#${h.post.s}` : ""}`}
            >
              {titles[h.post.p] ?? h.post.p}
            </a>
            <p className="where">{where(h.post)}</p>
            <p className="snippet">{mark(h.post.x, query)}</p>
          </li>
        ))}
      </ol>
    </div>
  );
}
