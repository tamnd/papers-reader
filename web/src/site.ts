// Reading the build.
//
// The app reads the JSON papers emit wrote into web/public, at build time,
// off the disk. Not over HTTP, and not from a checkout of the corpus.
//
// Off the disk because Astro is generating static pages and the alternative
// is a server. From web/public because that is the one directory Astro
// copies into the output verbatim, so the same files the pages were built
// from are the files the browser can fetch afterwards, which is what the
// language switcher and the search need. Building the pages from one copy
// and serving another would be two sources of truth for the same bytes.
//
// From the build and not from the corpus because the corpus layout is the
// toolchain's business. Everything in here is held to
// schema/site.schema.json, which audit rule P05 runs on every audit, and
// the types below are generated from that same file by make schema.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import type {
  Block,
  Graph,
  Index,
  IndexPaper,
  Lang,
  Page,
  Search,
} from "./types.gen";

export type { Block, Graph, Index, IndexPaper, Lang, Page, Search };

/** The shape of the JSON this app knows how to read. */
export const VERSION = 1;

/** Every language the corpus is written in, English first. */
export const LANGS: Lang[] = ["en", "vi", "zh", "ja"];

/** What each language calls itself, which is what a reader recognises. */
export const LANG_NAMES: Record<Lang, string> = {
  en: "English",
  vi: "Tiếng Việt",
  zh: "中文",
  ja: "日本語",
};

// From the working directory and not from import.meta.url, which is the
// obvious way to write this and is wrong: the build bundles this module into
// dist and import.meta.url then points at the bundle, so ../public resolves
// somewhere that does not exist. Astro runs from the project root in both
// the dev server and the build, so the working directory is the one thing
// that means the same in both.
const root = join(process.cwd(), "public");

function read<T>(path: string): T {
  const full = join(root, path);
  if (!existsSync(full)) {
    throw new Error(
      `${path} is not in web/public. Run papers emit -corpus <papers> -out web/public first.`,
    );
  }
  return JSON.parse(readFileSync(full, "utf8")) as T;
}

// A build of a shape this app does not know is refused rather than rendered
// on a guess. The two sides move in one commit, so a mismatch here means
// somebody deployed a build from a different revision of the toolchain, and
// a site that half renders is harder to diagnose than one that does not.
function checked<T extends { version: number }>(path: string, doc: T): T {
  if (doc.version !== VERSION) {
    throw new Error(
      `${path} is version ${doc.version} and this app reads version ${VERSION}`,
    );
  }
  return doc;
}

let cachedIndex: Index | undefined;

/** The catalogue. */
export function index(): Index {
  cachedIndex ??= checked("index.json", read<Index>("index.json"));
  return cachedIndex;
}

let cachedGraph: Graph | undefined;

/** The citation graph. */
export function graph(): Graph {
  cachedGraph ??= checked("graph.json", read<Graph>("graph.json"));
  return cachedGraph;
}

/** One paper in one language. */
export function page(id: string, lang: Lang): Page {
  const path = `p/${id}/${lang}.json`;
  return checked(path, read<Page>(path));
}

/** The languages a paper was emitted in, in LANGS order. */
export function langsOf(id: string): Lang[] {
  const dir = join(root, "p", id);
  if (!existsSync(dir)) return [];
  const there = new Set(
    readdirSync(dir)
      .filter((name) => name.endsWith(".json"))
      .map((name) => name.slice(0, -".json".length)),
  );
  return LANGS.filter((l) => there.has(l));
}

/** Every paper page in the build, which is what getStaticPaths walks. */
export function everyPage(): { id: string; lang: Lang }[] {
  return index().papers.flatMap((p) =>
    langsOf(p.id).map((lang) => ({ id: p.id, lang })),
  );
}

/** One paper as the catalogue knows it. */
export function paper(id: string): IndexPaper {
  const found = index().papers.find((p) => p.id === id);
  if (!found) throw new Error(`${id} is not in the catalogue`);
  return found;
}

/** A search index, for the pages that ship one to the browser. */
export function search(lang: Lang): Search | undefined {
  const path = `search-${lang}.json`;
  if (!existsSync(join(root, path))) return undefined;
  return checked(path, read<Search>(path));
}

/**
 * A link, with the base the site is deployed under on the front of it.
 *
 * Every href in this app goes through here. Astro exposes the base as
 * import.meta.env.BASE_URL and forgetting it produces links that work in
 * the dev server and are broken on the deployed site, which is the kind of
 * thing nobody notices until it is live.
 */
export function href(...parts: string[]): string {
  const base = import.meta.env.BASE_URL.replace(/\/$/, "");
  return [base, ...parts.filter((p) => p !== "")].join("/");
}

/**
 * The front page with the two lines the masthead already printed taken off
 * the top of it.
 *
 * The emitter carries the front page whole and leaves this to the app on
 * purpose: finding the abstract on a front page is a heuristic, and a
 * heuristic that runs per language can cut the Vietnamese at a different
 * place than the English, which is the one kind of drift the block indices
 * exist to rule out. Doing it here instead means the cut is a matter of
 * what is drawn and never of what the two columns line up on.
 *
 * The rule is narrow: only the first two blocks are candidates, only a
 * paragraph is, and it goes only if it is the title or if it opens with the
 * first author's name. Anything else, including an affiliation and the word
 * Abstract, stays, because the front page of a paper carries things nothing
 * else on the page does.
 */
export function frontBody(p: Page): Block[] {
  const bare = (s: string) =>
    s
      .replace(/<[^>]*>/g, " ")
      .toLowerCase()
      .replace(/[^\p{L}\p{N}]+/gu, "");
  const title = bare(p.front.title);
  const first = bare(p.front.authors[0] ?? "");
  const blocks = [...p.front.blocks];
  while (blocks.length > 0) {
    const b = blocks[0] as Block;
    if (b.kind !== "p" || p.front.blocks.indexOf(b) > 1) break;
    const text = bare(b.html ?? "");
    if (text === title || (first !== "" && text.startsWith(first))) {
      blocks.shift();
      continue;
    }
    break;
  }
  return blocks;
}

/** Every block of a page in reading order, the front matter first. */
export function blocks(p: Page): Block[] {
  return [...p.front.blocks, ...p.sections.flatMap((s) => s.blocks)];
}

/**
 * Whether a page is provisional: a small model or a gateway wrote some of
 * it. A corpus that is mostly machine translated and does not say so on the
 * page is dishonest, so this decides whether the marker is shown and it is
 * read straight off the provenance the emitter wrote.
 */
export function provisional(p: Page): boolean {
  return p.draft === true || p.provenance.small_model || p.provenance.gateway;
}

/** A year, or a range, as the catalogue lines want it. */
export function years(papers: IndexPaper[]): [number, number] {
  const ys = papers.map((p) => p.year);
  return [Math.min(...ys), Math.max(...ys)];
}
