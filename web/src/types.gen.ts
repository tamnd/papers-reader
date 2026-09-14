// Code generated from schema/site.schema.json by make schema. DO NOT EDIT.

/**
 * The shape of the emitted JSON. Every file of a build carries the same one, and a reader that finds a version it does not know refuses to render rather than guessing.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "version".
 */
export type Version = number;
/**
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "lang".
 */
export type Lang = "en" | "vi" | "zh" | "ja";
/**
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "field".
 */
export type Field =
  | "theory"
  | "algorithms"
  | "languages"
  | "systems"
  | "networks"
  | "databases"
  | "architecture"
  | "security"
  | "ai-ml"
  | "graphics"
  | "hci"
  | "software";
/**
 * A paper identifier: an author, a year and one keyword, lowercase, joined by hyphens.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "id".
 */
export type Id = string;
/**
 * What may be published from the paper. A restricted paper is a stub of front matter and a short abstract, and an unknown one is nothing at all.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "access".
 */
export type Access = "public-domain" | "open" | "permissive" | "restricted" | "unknown";
/**
 * The publisher's page for the paper. A landing page and never a PDF: the corpus links a reader to where the paper is published and does not republish the file.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "landing".
 */
export type Landing = string;
/**
 * How much of a paper exists in one language. Three states and not a percentage, because a paper is not sixty per cent done.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "state".
 */
export type State = "full" | "stub" | "none";
/**
 * Headings and lists are kinds of their own rather than prose, because a heading is what rule L03 compares across languages and a list set as a paragraph would align against a real paragraph somewhere else.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "blockKind".
 */
export type BlockKind = "p" | "heading" | "list" | "math" | "figure" | "code" | "table";
/**
 * A fragment identifier a link elsewhere in the corpus can point at. It carries the paper identifier because a side by side view has two papers' worth of markup in one document.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "anchor".
 */
export type Anchor = string;
/**
 * The stable identifier papers tags assigns a numbered thing, which is what a cross reference survives a re-extraction by.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "tag".
 */
export type Tag = string;

/**
 * The shape of the JSON that papers emit writes and the reading app consumes. The Go emitter and the TypeScript reader both validate against this file, and audit rule P05 runs it over a build of the corpus, so the two sides cannot drift apart without something saying so. A change to the shape bumps the version integer and updates both sides in one commit, which is the reason this file lives in the same repository as both. There is one file rather than one per document so that the shared definitions are shared and not copied: a paper identifier is the same string in index.json and in graph.json, and it should be impossible to tighten one and forget the other. A document is validated against the definition named after it, index.json against #/$defs/index, graph.json against #/$defs/graph, every p/<id>/<lang>.json against #/$defs/page and every search-<lang>.json against #/$defs/search.
 */
export interface Site {
  [k: string]: unknown;
}
/**
 * index.json is the catalogue: every paper, every field, every reading list. It is read on the first request of every visit, which is why it holds what a catalogue page needs and nothing a paper page would need.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "index".
 */
export interface Index {
  version: Version;
  /**
   * When the build ran, in UTC to the second. A site built twice from one commit differs in this field and in nothing else.
   */
  generated: string;
  /**
   * Every language the corpus can be read in, English first.
   *
   * @minItems 1
   */
  langs: [Lang, ...Lang[]];
  /**
   * The languages whose glossary is under the coverage floor. A draft language is still offered, with the page saying what it is, because hiding it would be the same corpus with less of it visible and no more of it true. English is never in this list.
   */
  draft_langs: Lang[];
  fields: IndexField[];
  collections: IndexList[];
  papers: IndexPaper[];
}
/**
 * One subject field and how many papers are in it.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "indexField".
 */
export interface IndexField {
  id: Field;
  title: string;
  count: number;
}
/**
 * One reading list, its members resolved to identifiers in the order the list says to read them. Resolved by the emitter rather than by the app, because the words a collection may hold instead of a list and the order that sorts by prerequisites are corpus rules, and an app that reimplemented them would be a second place for them to be wrong.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "indexList".
 */
export interface IndexList {
  id: string;
  title: string;
  description?: string;
  members: Id[];
}
/**
 * One paper as the catalogue knows it.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "indexPaper".
 */
export interface IndexPaper {
  id: Id;
  /**
   * The position in the seed list of a hundred, absent on every paper added afterwards. That is how the hundred stay a hundred while the corpus grows.
   */
  number?: number;
  /**
   * The title as the paper printed it, in English.
   */
  title: string;
  /**
   * The title as each translation printed it, keyed by language. A language whose translator left the title in English is absent rather than present and equal to the English. English is never a key here.
   */
  titles?: {
    [k: string]: string;
  };
  authors: string[];
  year: number;
  venue?: string;
  field: Field;
  /**
   * The one thing the paper is remembered for, which is the line a catalogue card is read for.
   */
  core_idea?: string;
  difficulty?: number;
  access: Access;
  landing?: Landing;
  /**
   * How much of the paper exists in each language. A language the paper has nothing in is absent rather than present and none, so the app can offer what is there without filtering.
   */
  status: {
    [k: string]: State;
  };
  sections: number;
  figures: number;
  equations: number;
  refs: number;
  /**
   * How many papers of this corpus cite it. Carried here so the catalogue can sort by influence without loading the graph.
   */
  cited_by: number;
  cites_in_corpus: number;
  prerequisites?: Id[];
}
/**
 * graph.json is the citation graph as the app draws it: the same edges as reports/graph.md and built from the same place, in the shape a layout wants rather than the shape a table wants.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "graph".
 */
export interface Graph {
  version: Version;
  nodes: GraphNode[];
  edges: GraphEdge[];
}
/**
 * One paper at a point on the chart. The year and the field are where the layout puts it, and the two degrees are what a hover needs.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "graphNode".
 */
export interface GraphNode {
  id: Id;
  title: string;
  year: number;
  field: Field;
  /**
   * How many papers here cite it.
   */
  in: number;
  /**
   * How many papers here it cites.
   */
  out: number;
  /**
   * Whether anybody has read this paper's bibliography. A node with no out edges and this false cites nothing here because nobody has looked, which the chart draws differently from a paper that was read and cites nothing.
   */
  parsed: boolean;
}
/**
 * One paper citing another, both of them in the corpus. Self loops are not edges and a paper cited twice by one bibliography is one edge.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "graphEdge".
 */
export interface GraphEdge {
  from: Id;
  to: Id;
}
/**
 * p/<id>/<lang>.json is one paper in one language, whole. One file per paper per language rather than one per section, because a reader opens a paper and reads it and a request per section would be twenty requests to read one paper. Every language of a paper has the same sections in the same order with the same block indices, which is what makes the side by side view a matter of putting block i against block i, so a change here that could move an index in one language and not another is a change that breaks reading two languages together.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "page".
 */
export interface Page {
  version: Version;
  id: Id;
  lang: Lang;
  /**
   * What a translation was made from. Absent on the English.
   */
  source_lang?: "en" | "vi" | "zh" | "ja";
  /**
   * This language is under the glossary coverage floor. The page is still built and still offered, with the app saying what it is.
   */
  draft?: boolean;
  provenance: Provenance;
  front: PageFront;
  sections: PageSection[];
  refs: Ref[];
  notes?: Note[];
}
/**
 * How the page came to exist, for the colophon at the foot of it. A field the sections disagree about is absent rather than being the first or the commonest of them. The two honesty flags go the other way: one section written by a small model makes the whole page's answer true, because a reader deciding how far to trust a page wants to know that some of it is provisional.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "provenance".
 */
export interface Provenance {
  /**
   * How the text was got off the page.
   */
  extraction?: "native" | "layout" | "vision";
  /**
   * The tool or the model that did it, named as it names itself.
   */
  extraction_tool?: string;
  /**
   * The model that wrote this language. Absent on the English, which was not translated.
   */
  translation_model?: string;
  /**
   * The oldest glossary any section of this page was translated against, because that is the one a reader would be caught out by.
   */
  glossary_version?: number;
  small_model: boolean;
  gateway: boolean;
}
/**
 * The paper's own front matter and its first page as printed. Blocks and not an abstract, because finding the abstract on a front page is a heuristic that runs per language and can land on a different paragraph in the Vietnamese than it did in the English. Cutting the front page at a different place in each language is exactly the drift block indices exist to rule out, so the front page is carried whole and the app decides how much of it to show.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "pageFront".
 */
export interface PageFront {
  title: string;
  authors: string[];
  venue?: string;
  year?: number;
  blocks: Block[];
}
/**
 * One paragraph, heading, list, formula, figure, listing or table. The index i is the alignment key: every language of a section has the same blocks in the same order with the same indices, which is what audit rules L03, L04, L16 and L18 enforce, so laying the English beside the Vietnamese is a matter of putting block i against block i with no diffing and no guessing.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "block".
 */
export interface Block {
  kind: BlockKind;
  i: number;
  /**
   * The rendered block, sanitised against the allowlist the emitter holds. For a heading it is the heading text and not an h element, because the level is a field of its own and the app chooses the element.
   */
  html?: string;
  /**
   * The depth of a heading, counted in hashes.
   */
  level?: number;
  /**
   * The mathematics as the corpus holds it, carried beside the rendered form so that a reader can copy the formula out and so that the search can index it as text.
   */
  tex?: string;
  /**
   * A figure, as a path relative to the root of the site, which is deliberately the same path it has in the corpus.
   */
  src?: string;
  w?: number;
  h?: number;
  /**
   * What the paper printed: the equation's tag, the figure's number. Never a number this toolchain invented, because every cross reference in the prose is to the paper's own numbering.
   */
  number?: string;
  caption_html?: string;
  /**
   * The word on the opening fence of a listing, which is the word a highlighter expects. Mostly absent: what this corpus holds is pseudocode.
   */
  lang?: string;
  /**
   * A listing as the corpus holds it, unhighlighted and unescaped.
   */
  text?: string;
  anchor?: Anchor;
  tag?: Tag;
}
/**
 * One content file of the paper. The front matter is in front and the bibliography is in refs, so neither of those is a section here.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "pageSection".
 */
export interface PageSection {
  anchor: Anchor;
  tag?: Tag;
  /**
   * The number the paper printed, which is what every cross reference in the prose is to. A section the paper did not number has none.
   */
  number?: string;
  title: string;
  /**
   * How deep the section sits, counted off its number: 3 is level two and 3.2 is level three. The paper's title is level one, which is why a top level section is two.
   */
  level: number;
  kind: "section" | "appendix";
  /**
   * Written by a model small enough that the result is provisional. The app prints this on the section and not only in the colophon, because a reader who has just read a paragraph is owed the warning where the paragraph is.
   */
  small_model?: boolean;
  /**
   * Written through a gateway rather than the intended model, for the same reason and with the same consequence.
   */
  gateway?: boolean;
  blocks: Block[];
}
/**
 * One entry of the paper's bibliography, from the refs manifest and not from the references content file, because the manifest is the parsed form and it is what carries the resolution. A bibliography is not translated, so this is the same list in every language of the page.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "ref".
 */
export interface Ref {
  key: string;
  raw: string;
  /**
   * A paper identifier: an author, a year and one keyword, lowercase, joined by hyphens.
   */
  resolves_to?: string;
}
/**
 * One footnote, gathered from wherever its definition fell. Gathered because the page break decides where a definition lands and it is regularly sections away from the marker that refers to it.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "note".
 */
export interface Note {
  key: string;
  html: string;
}
/**
 * search-<lang>.json: an inverted index over one language of the corpus, built at emit time and searched in the browser. One file per language, loaded on the first search, so that a reader reading the Vietnamese does not pay for the Japanese. There is no search service because there is no server anywhere else on this site either, and sending every query a reader types somewhere else would mean the reading list of everybody who uses this site existing somewhere else.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "search".
 */
export interface Search {
  version: Version;
  lang: Lang;
  /**
   * Every block worth finding, in a fixed order. A posting in either index is a position in this list.
   */
  posts: Post[];
  terms: Postings;
  symbols: Postings;
}
/**
 * One block a search can return. It carries enough to draw a result line without fetching the page, because a page of ten results would otherwise be ten page fetches to show any of them.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "post".
 */
export interface Post {
  p: Id;
  /**
   * A fragment identifier a link elsewhere in the corpus can point at. It carries the paper identifier because a side by side view has two papers' worth of markup in one document.
   */
  s?: string;
  i: number;
  t?: string;
  k: BlockKind;
  /**
   * The opening of the block as plain text, cut at a word boundary.
   */
  x: string;
}
/**
 * A term to the positions in posts it occurs in, ascending and without repeats. There are two of these in an index. terms is the prose, an ordinary word index over the text of the corpus, with any term in more than a third of the blocks dropped, which is a stopword list that does not have to be written four times in four languages. symbols is the mathematics and the program text, indexed apart from the prose so that the app can rank a hit in a formula against a hit in a paragraph rather than having to guess which the reader wanted.
 *
 * This interface was referenced by `Site`'s JSON-Schema
 * via the `definition` "postings".
 */
export interface Postings {
  /**
   * @minItems 1
   */
  [k: string]: [number, ...number[]];
}
