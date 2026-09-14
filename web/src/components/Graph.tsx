// The citation graph, drawn.
//
// Time across, field down. Not a force directed blob: the usual rendering
// of a citation graph is a cloud of dots that settles somewhere different
// on every load and tells a reader nothing they can carry away. Here the
// horizontal position is the year the paper was published and the vertical
// band is the field it belongs to, so the picture says something true
// before anybody touches it. It is the same picture on every load, and two
// people can talk about the left hand side of it and mean the same thing.
//
// Every edge points from the paper that cites to the paper it cites, which
// is from later to earlier, so the arcs mostly lean left. The ones that do
// not are the papers published in the same year as the work they cite.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { GraphEdge, GraphNode } from "../types.gen";

interface Props {
  base: string;
  nodes: GraphNode[];
  edges: GraphEdge[];
  /** Field to its title, in the order the catalogue lists them, so the
   * bands are in the same order as the front page. */
  fields: { id: string; title: string }[];
  /** Paper to how many references its bibliography has, which is the
   * denominator the chart is a numerator of: the edges here are the few
   * references that resolved to another paper in this corpus, and without
   * the total beside them a paper that cites three looks like a paper that
   * cites three. */
  refs: Record<string, number>;
}

interface Placed extends GraphNode {
  x: number;
  y: number;
  r: number;
}

/** The width the chart is laid out at before it knows better. A server
 * rendering has no element to measure, and a chart that appears at a
 * plausible size and then settles is better than one that appears empty. */
const assumed = 920;
const leftPad = 10;
const rightPad = 14;
/** Room at the top for the years. */
const topPad = 26;
const bottomPad = 12;
/** Room at the top of a band for its field name. The names go above their
 * band rather than in a gutter beside it, because half of them are four
 * words long and a gutter wide enough for those would be a quarter of the
 * chart given over to labels. */
const bandLabel = 22;
/** A row of a band, and the space two papers of one field need between
 * them before they can share a row. Two papers eleven months apart are a
 * few pixels apart on a fifty year axis, so without this the dense years
 * are one smudge. */
const rowHeight = 19;
const apart = 17;

/** The site base with exactly one slash on the end.
 *
 * import.meta.env.BASE_URL is "/papers-reader" here and "/" when the site
 * is served from a root, so neither assuming a trailing slash nor assuming
 * there is none produces a link that works in both. */
function root(base: string): string {
  return base.endsWith("/") ? base : `${base}/`;
}

function radius(n: GraphNode): number {
  return 3.4 + Math.min(n.in, 9) * 0.85;
}

interface Layout {
  placed: Placed[];
  bands: { id: string; title: string; top: number; height: number }[];
  ticks: { year: number; x: number }[];
  height: number;
}

function layout(
  nodes: GraphNode[],
  fields: { id: string; title: string }[],
  width: number,
): Layout {
  const years = nodes.map((n) => n.year);
  const first = Math.floor(Math.min(...years) / 10) * 10;
  const last = Math.ceil((Math.max(...years) + 1) / 10) * 10;
  const span = Math.max(last - first, 1);
  const inner = Math.max(width - leftPad - rightPad, 120);
  const at = (year: number) => leftPad + ((year - first) / span) * inner;

  const placed: Placed[] = [];
  const bands: Layout["bands"] = [];
  let top = topPad;
  for (const f of fields) {
    const mine = nodes
      .filter((n) => n.field === f.id)
      .sort((a, b) => a.year - b.year || a.id.localeCompare(b.id));
    if (mine.length === 0) continue;
    // Greedy rows. A paper goes on the first row whose last paper is far
    // enough to its left, which keeps the common case to one row and only
    // spends height on the years that are actually crowded.
    const lastOn: number[] = [];
    for (const n of mine) {
      const x = at(n.year);
      let row = lastOn.findIndex((had) => x - had >= apart);
      if (row === -1) {
        row = lastOn.length;
        lastOn.push(x);
      } else {
        lastOn[row] = x;
      }
      placed.push({
        ...n,
        x,
        y: top + bandLabel + 8 + row * rowHeight,
        r: radius(n),
      });
    }
    const height = bandLabel + 14 + lastOn.length * rowHeight;
    bands.push({ id: f.id, title: f.title, top, height });
    top += height;
  }

  const ticks: { year: number; x: number }[] = [];
  for (let y = first; y <= last; y += 10) ticks.push({ year: y, x: at(y) });
  return { placed, bands, ticks, height: top + bottomPad };
}

/** Everything reachable from one paper along the edges, in one direction.
 * Following from to to is what a paper cites and what those cite, which is
 * where its ideas came from. The other way is what came after it. */
function reach(from: string, next: Map<string, string[]>): Set<string> {
  const seen = new Set<string>();
  const todo = [from];
  while (todo.length > 0) {
    const at = todo.pop() as string;
    for (const to of next.get(at) ?? []) {
      if (seen.has(to)) continue;
      seen.add(to);
      todo.push(to);
    }
  }
  return seen;
}

export default function GraphChart({
  base,
  nodes,
  edges,
  fields,
  refs,
}: Props) {
  const box = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(assumed);
  const [hot, setHot] = useState<string | undefined>(undefined);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const watch = new ResizeObserver(([entry]) => {
      if (entry) setWidth(entry.contentRect.width);
    });
    watch.observe(el);
    setWidth(el.clientWidth);
    return () => watch.disconnect();
  }, []);

  const { placed, bands, ticks, height } = useMemo(
    () => layout(nodes, fields, width),
    [nodes, fields, width],
  );
  const where = useMemo(() => new Map(placed.map((p) => [p.id, p])), [placed]);

  const [cites, citedBy] = useMemo(() => {
    const a = new Map<string, string[]>();
    const b = new Map<string, string[]>();
    for (const e of edges) {
      a.set(e.from, [...(a.get(e.from) ?? []), e.to]);
      b.set(e.to, [...(b.get(e.to) ?? []), e.from]);
    }
    return [a, b];
  }, [edges]);

  const lit = useMemo(() => {
    if (hot === undefined) return undefined;
    return { up: reach(hot, cites), down: reach(hot, citedBy) };
  }, [hot, cites, citedBy]);

  const side = useCallback(
    (id: string): "hot" | "up" | "down" | "cold" => {
      if (lit === undefined) return "cold";
      if (id === hot) return "hot";
      if (lit.up.has(id)) return "up";
      if (lit.down.has(id)) return "down";
      return "cold";
    },
    [lit, hot],
  );

  const shown = hot === undefined ? undefined : where.get(hot);

  return (
    <div className="chart" ref={box}>
      <svg
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        className={lit ? "graph lit" : "graph"}
        role="img"
        aria-label={`${nodes.length} papers by year and field, with the ${edges.length} citations between them. The same information is in the list below this chart.`}
      >
        {ticks.map((t) => (
          <text
            key={t.year}
            className="tick"
            x={t.x}
            y={topPad - 12}
            textAnchor="middle"
          >
            {t.year}
          </text>
        ))}

        {bands.map((b) => (
          <g key={b.id}>
            <text className="band" x={leftPad} y={b.top + 13}>
              {b.title}
            </text>
            {/* The decade lines are drawn per band rather than down the
                whole chart, so they stop short of the field names instead
                of running through them. */}
            {ticks.map((t) => (
              <line
                key={t.year}
                className="grid"
                x1={t.x}
                y1={b.top + bandLabel - 4}
                x2={t.x}
                y2={b.top + b.height - 4}
              />
            ))}
          </g>
        ))}

        <g className="edges">
          {edges.map((e) => {
            const a = where.get(e.from);
            const b = where.get(e.to);
            if (!a || !b) return null;
            // An edge takes the colour of its far end from the paper
            // being hovered, so a citation the hovered paper makes is the
            // colour of the work it drew on and not a third colour of its
            // own. The first hop and the fifth are then the same colour,
            // which is the point: the whole line back is one thing.
            const state =
              hot === undefined
                ? "cold"
                : e.from === hot
                  ? "up"
                  : e.to === hot
                    ? "down"
                    : side(e.from) === "up" && side(e.to) === "up"
                      ? "up"
                      : side(e.from) === "down" && side(e.to) === "down"
                        ? "down"
                        : "off";
            // The arc bows away from the line between the two papers by a
            // fraction of how far apart they are, so a citation across
            // forty years is a long shallow curve and two papers a year
            // apart are a small hop rather than a line through everything
            // between them.
            const mx = (a.x + b.x) / 2;
            const my = (a.y + b.y) / 2 - Math.abs(a.x - b.x) * 0.16 - 8;
            return (
              <path
                key={`${e.from}-${e.to}`}
                className={`edge ${state}`}
                d={`M ${a.x} ${a.y} Q ${mx} ${my} ${b.x} ${b.y}`}
              />
            );
          })}
        </g>

        <g className="nodes">
          {placed.map((n) => (
            <a key={n.id} href={`${root(base)}p/${n.id}`}>
              <circle
                className={`node ${side(n.id)}${n.parsed ? "" : " unread"}`}
                cx={n.x}
                cy={n.y}
                r={n.r}
                onMouseEnter={() => setHot(n.id)}
                onFocus={() => setHot(n.id)}
                onMouseLeave={() => setHot(undefined)}
                onBlur={() => setHot(undefined)}
              >
                <title>{`${n.title} (${n.year})`}</title>
              </circle>
            </a>
          ))}
        </g>
      </svg>

      {shown && (
        <div
          className="tip"
          style={{
            left: `${Math.min(Math.max(shown.x - 110, 0), Math.max(width - 240, 0))}px`,
            top: `${shown.y + 14}px`,
          }}
        >
          <strong>{shown.title}</strong>
          <span>
            {`${shown.year}. Cited by ${shown.in} ${shown.in === 1 ? "paper" : "papers"} here.`}
            {shown.parsed
              ? ` Of its ${refs[shown.id] ?? 0} references, ${shown.out} ${shown.out === 1 ? "is a paper" : "are papers"} here.`
              : " Its bibliography has not been read yet, so it cites nothing here for want of looking."}
          </span>
        </div>
      )}
    </div>
  );
}
