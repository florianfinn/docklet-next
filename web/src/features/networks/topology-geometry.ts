import type { TopologyNetwork } from "@/domain/model";

/*
 * Content-driven node geometry. Each line has a fixed height and the column
 * is as wide as its longest line, so layout positions are exact before React
 * Flow measures anything. Widths are estimates per character (Montserrat and
 * Geist Mono at the node font sizes, calibrated in Chromium and rounded up).
 * A network name wider than NODE_MAX_W wraps onto whole extra title lines;
 * other text beyond it truncates with a title.
 * The node components use px spacing so the root font size cannot shift them.
 */

export const NODE_MIN_W = 196;
export const NODE_MAX_W = 420;
/** Space between columns for the membership bus (left) and dependency arrows (right). */
export const GUTTER = 60;

/** Line heights in px; the node components use exactly these. */
export const LINE = { title: 18, meta: 15, address: 16 } as const;
const NET_CHROME = 2 + 16 + 2 + 6; // border, vertical padding, gaps above line 2 and 3
const NET_FOOT = 18;
const CONTAINER_CHROME = 2 + 16 + 2; // border, vertical padding, gap above the meta line
/** Space below each block inside its column. */
export const BLOCK_GAP = 12;

const CHAR = { title: 8.1, sans: 6, mono: 6.05 } as const;
const BADGE = 46;
const NET_PAD_X = 2 + 24;
const CONTAINER_PAD_X = 2 + 24;

export const textWidth = (text: string, kind: keyof typeof CHAR) => Math.ceil(text.length * CHAR[kind]);

/** Montserrat SemiBold 12.5 px advance widths measured in Chromium, rounded up; other glyphs count 9 px. */
const TITLE_GLYPHS: Record<string, number> = Object.fromEntries(
  "a7.6 b8.6 c7.3 d8.6 e7.8 f4.7 g8.7 h8.6 i3.7 j3.7 k8 l3.7 m13.2 n8.6 o8.1 p8.6 q8.6 r5.3 s6.5 t5.3 u8.6 v7.3 w11.5 x7.2 y7.3 z6.7 A9.4 B9.6 C9 D10.4 E8.4 F8 G9.7 H10.2 I4 J6.6 K9.2 L7.5 M12 N10.2 O10.6 P9.1 Q10.6 R9.2 S7.9 T7.6 U9.9 V9.2 W14.3 X8.7 Y8.3 Z8.3 08.5 14.8 27.3 37.3 48.5 57.3 67.9 77.6 88.2 97.9 _6.3 -4.9 .3.1"
    .split(" ")
    .map((g) => [g[0]!, Number(g.slice(1))]),
);
/** Rendered title width with 4 % headroom; unlike the average it never underestimates W or M. */
export const titleWidth = (text: string) => Math.ceil([...text].reduce((w, ch) => w + (TITLE_GLYPHS[ch] ?? 9), 0) * 1.04);
/** Most lines a network name wraps onto; beyond, it clamps with an ellipsis and keeps its title. */
export const NAME_MAX_LINES = 3;
/** Widest glyph: a line broken inside a name can leave up to this much unused. */
const BREAK_SLACK = 15;
const clampWidth = (w: number) => Math.min(NODE_MAX_W, Math.max(NODE_MIN_W, Math.ceil(w)));

/** Subnet lines of a network block: one per known family, "no subnet" otherwise. */
export function subnetLines(n: Pick<TopologyNetwork, "subnet" | "subnet6">): number {
  return n.subnet && n.subnet6 ? 2 : 1;
}

const badgeCount = (n: Pick<TopologyNetwork, "internal" | "builtIn" | "ipv6">) => [n.internal, n.builtIn && !n.internal, n.ipv6].filter(Boolean).length;
/** Icon and badges beside the name in the title row. */
const titleChrome = (n: Pick<TopologyNetwork, "internal" | "builtIn" | "ipv6">) => 14 + 6 + badgeCount(n) * (BADGE + 6);

/** Lines the network name takes in a block of this width; 1 unless the name is wider than the room beside icon and badges. */
export function networkNameLines(n: Pick<TopologyNetwork, "name" | "internal" | "builtIn" | "ipv6">, width: number): number {
  const room = width - NET_PAD_X - titleChrome(n);
  const name = titleWidth(n.name);
  if (name <= room) return 1;
  return Math.min(NAME_MAX_LINES, Math.ceil(name / Math.max(1, room - BREAK_SLACK)));
}

export function networkSize(n: TopologyNetwork): { width: number; height: number } {
  // The average keeps ordinary blocks at their calibrated width; glyph widths widen W/M-heavy names.
  const title = titleChrome(n) + Math.max(textWidth(n.name, "title"), titleWidth(n.name));
  const driver = textWidth(n.parent ? `${n.driver} · ${n.parent}` : n.driver, "mono");
  const first = driver + 8 + textWidth(n.subnet ?? n.subnet6 ?? "", "mono");
  const second = n.subnet && n.subnet6 ? textWidth(n.subnet6, "mono") : 0;
  const width = clampWidth(NET_PAD_X + Math.max(title, first, second));
  return { width, height: NET_CHROME + networkNameLines(n, width) * LINE.title + subnetLines(n) * LINE.meta + NET_FOOT };
}

/** One membership as the focus shows it on a container block. */
export interface AddressRow {
  network: string;
  /** Known addresses, one line each; host mode and unknown addresses take one line. */
  values: string[];
  /** Host network mode: the address carries a short "host" badge. */
  host?: boolean;
}

export const addressLines = (rows: readonly AddressRow[]) => rows.reduce((n, r) => n + Math.max(1, r.values.length), 0);

/** Rendered height of a container block showing `lines` address lines. */
export const containerHeight = (lines: number) => CONTAINER_CHROME + LINE.title + LINE.meta + (lines ? 2 + lines * LINE.address : 0);

/**
 * Container block for its widest focus state: every membership with all
 * known addresses (a focused network shows a subset), so selection never
 * moves or overlaps blocks.
 */
export function containerSize(c: { title: string; meta: string; rows: readonly AddressRow[] }): { width: number; height: number; reserved: number } {
  const title = 8 + 6 + textWidth(c.title, "title");
  const meta = textWidth(c.meta, "sans");
  const address = Math.max(0, ...c.rows.map((r) => textWidth(r.network, "sans") + 6 + Math.max(0, ...r.values.map((v) => textWidth(v, "mono"))) + (r.host ? 34 : 0)));
  return { width: clampWidth(CONTAINER_PAD_X + Math.max(title, meta, address)), height: containerHeight(0), reserved: containerHeight(addressLines(c.rows)) + BLOCK_GAP };
}

export interface Bounds {
  x: number;
  y: number;
  width: number;
  height: number;
}
export interface Size {
  width: number;
  height: number;
}

/** Canvas padding in px: room for the zone label on top and the controls and legend below. */
export const FIT_PAD = { x: 40, top: 44, bottom: 36 } as const;
/** Fit never enlarges past this or shrinks text below legibility; beyond, the map pans. */
export const FIT_ZOOM = { min: 0.6, max: 1.2 } as const;
export const CANVAS_MIN_H = 320;

/** Target zoom when packing lane rows, so a fitted graph stays close to full size. */
const PACK_ZOOM = 0.9;

/**
 * Widest lane row (layout px) the canvas shows at about PACK_ZOOM, in 40 px
 * steps so small resizes keep the layout; unknown width (0) gives Infinity.
 */
export function layoutBudget(canvasWidth: number): number {
  if (canvasWidth <= 0) return Infinity;
  return Math.max(NODE_MIN_W, Math.floor((canvasWidth - 2 * FIT_PAD.x) / PACK_ZOOM / 40) * 40);
}

/** Canvas height for the content at this width: as tall as the fitted content, at least CANVAS_MIN_H; CSS caps it to the viewport. */
export function canvasHeight(bounds: Bounds, width: number): number {
  if (width <= 0 || bounds.width <= 0) return CANVAS_MIN_H;
  const zoom = Math.min(FIT_ZOOM.max, Math.max(FIT_ZOOM.min, (width - 2 * FIT_PAD.x) / bounds.width));
  return Math.max(CANVAS_MIN_H, Math.ceil(bounds.height * zoom + FIT_PAD.top + FIT_PAD.bottom));
}

/**
 * Viewport that shows the content: centred when it fits, otherwise anchored
 * at the top left at the smallest legible zoom so the user pans from the start.
 */
export function fitViewport(bounds: Bounds, size: Size): { x: number; y: number; zoom: number } {
  const room = { w: size.width - 2 * FIT_PAD.x, h: size.height - FIT_PAD.top - FIT_PAD.bottom };
  const fit = Math.min(room.w / Math.max(1, bounds.width), room.h / Math.max(1, bounds.height));
  const zoom = Math.min(FIT_ZOOM.max, Math.max(FIT_ZOOM.min, fit));
  const w = bounds.width * zoom;
  const h = bounds.height * zoom;
  const x = w <= room.w ? (size.width - w) / 2 : FIT_PAD.x;
  const y = h <= room.h ? FIT_PAD.top + (room.h - h) / 2 : FIT_PAD.top;
  return { x: Math.round(x - bounds.x * zoom), y: Math.round(y - bounds.y * zoom), zoom };
}

/** Horizontal distance of a column's membership bus from its blocks. */
export const BUS = 14;
/** x of the rail left of all lanes that carries memberships between lane rows. */
export const RAIL_X = -40;

export interface MembershipRoute {
  sx: number;
  sy: number;
  tx: number;
  ty: number;
  /** Free band between the network blocks and the members of the source and of the target row. */
  from: number;
  to: number;
}

/**
 * Orthogonal path of a membership that never crosses a block: down the
 * network's bus, along the free band of the row to the member's column and
 * down its bus; between rows it takes the rail left of all lanes.
 */
export function membershipPoints({ sx, sy, tx, ty, from, to }: MembershipRoute): [number, number][] {
  const g1 = sx - BUS;
  const g2 = tx - BUS;
  if (from === to && Math.abs(g1 - g2) < 1) return [[sx, sy], [g1, sy], [g1, ty], [tx, ty]];
  if (from === to) return [[sx, sy], [g1, sy], [g1, from], [g2, from], [g2, ty], [tx, ty]];
  return [[sx, sy], [g1, sy], [g1, from], [RAIL_X, from], [RAIL_X, to], [g2, to], [g2, ty], [tx, ty]];
}

/** SVG path through the points with rounded corners. */
export function roundedPath(points: [number, number][], radius = 6): string {
  const pts = points.filter((p, i) => i === 0 || p[0] !== points[i - 1]![0] || p[1] !== points[i - 1]![1]);
  let d = `M${pts[0]![0]},${pts[0]![1]}`;
  for (let i = 1; i < pts.length - 1; i++) {
    const [px, py] = pts[i - 1]!;
    const [x, y] = pts[i]!;
    const [nx, ny] = pts[i + 1]!;
    const r = Math.min(radius, Math.hypot(x - px, y - py) / 2, Math.hypot(nx - x, ny - y) / 2);
    const a = [x - Math.sign(x - px) * r, y - Math.sign(y - py) * r];
    const b = [x + Math.sign(nx - x) * r, y + Math.sign(ny - y) * r];
    d += ` L${a[0]},${a[1]} Q${x},${y} ${b[0]},${b[1]}`;
  }
  const last = pts.at(-1)!;
  return `${d} L${last[0]},${last[1]}`;
}
