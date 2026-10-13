import { visualStyleStore } from "@/shared/preferences/visual-style-store";

import { colourMix, latticeCount, projectPoint, surfaceHeight } from "./dot-wave";

/**
 * Canvas renderer of the dotted backgrounds. One requestAnimationFrame loop
 * serves every mounted field at about 30 frames per second; React renders the
 * canvas once and never per frame. The loop only runs while a field is in the viewport; a hidden
 * page, motion "off", reduced motion and the wave switch off all stop the
 * loop (one static frame stays). Colours come from the theme variables and
 * are bucketed (8 hues × 6 strengths), so a frame sets at most 48 fill styles.
 */

const FRAME_MS = 33;
const HUES = 8;
const STRENGTHS = 6;
const MAX_DPR = 2;

interface Field {
  canvas: HTMLCanvasElement;
  ctx: CanvasRenderingContext2D;
  width: number;
  height: number;
  /** Lattice positions in css px. */
  xs: Float32Array;
  ys: Float32Array;
  buckets: Uint8Array;
  px: Float32Array;
  py: Float32Array;
  pr: Float32Array;
  visible: boolean;
  /** Mean draw time of the last frames in ms, for QA (data-frame-ms). */
  cost: number;
}

const fields = new Set<Field>();
let raf = 0;
let last = 0;
let styles: string[] = [];
let shared: { off: () => void } | null = null;

type Rgb = [number, number, number];
let probe: CanvasRenderingContext2D | null | undefined;

/** Any CSS colour (oklch, hex …) as RGB, through a 1×1 canvas; undefined without canvas support. */
function toRgb(colour: string): Rgb | undefined {
  if (probe === undefined) probe = document.createElement("canvas").getContext("2d", { willReadFrequently: true });
  if (!probe || !colour) return undefined;
  probe.clearRect(0, 0, 1, 1);
  probe.fillStyle = "#000";
  probe.fillStyle = colour;
  probe.fillRect(0, 0, 1, 1);
  const [r = 0, g = 0, b = 0] = probe.getImageData(0, 0, 1, 1).data;
  return [r, g, b];
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** Fill styles for every hue/strength bucket from the current theme and wave strength. */
function buildStyles(): void {
  const grey = toRgb(cssVar("--muted-foreground")) ?? [140, 140, 150];
  const primary = toRgb(cssVar("--primary")) ?? grey;
  const secondary = toRgb(cssVar("--highlight")) ?? primary;
  const mix = (a: Rgb, b: Rgb, k: number): Rgb => [a[0] + (b[0] - a[0]) * k, a[1] + (b[1] - a[1]) * k, a[2] + (b[2] - a[2]) * k];
  const out: string[] = [];
  for (let h = 0; h < HUES; h++) {
    const hue = mix(primary, secondary, h / (HUES - 1));
    for (let s = 0; s < STRENGTHS; s++) {
      const k = s / (STRENGTHS - 1);
      const [r, g, b] = mix(grey, hue, k);
      out.push(`rgba(${Math.round(r)},${Math.round(g)},${Math.round(b)},${(0.26 + 0.64 * k).toFixed(3)})`);
    }
  }
  styles = out;
}

function animating(): boolean {
  const motion = document.documentElement.dataset.motion;
  return visualStyleStore.get().dotWave && motion !== "off" && motion !== "reduced";
}

function layout(f: Field): void {
  const rect = f.canvas.getBoundingClientRect();
  const width = Math.round(rect.width);
  const height = Math.round(rect.height);
  const dpr = Math.min(MAX_DPR, window.devicePixelRatio || 1);
  f.width = width;
  f.height = height;
  f.canvas.width = Math.max(1, Math.round(width * dpr));
  f.canvas.height = Math.max(1, Math.round(height * dpr));
  f.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const { cols, rows, spacing } = latticeCount(width, height);
  const n = cols * rows;
  f.xs = new Float32Array(n);
  f.ys = new Float32Array(n);
  f.buckets = new Uint8Array(n);
  f.px = new Float32Array(n);
  f.py = new Float32Array(n);
  f.pr = new Float32Array(n);
  const ox = (width - (cols - 1) * spacing) / 2;
  const oy = (height - (rows - 1) * spacing) / 2;
  for (let r = 0, i = 0; r < rows; r++) for (let c = 0; c < cols; c++, i++) {
    f.xs[i] = ox + c * spacing;
    f.ys[i] = oy + r * spacing;
  }
  f.canvas.dataset.points = String(n);
}

function draw(f: Field, t: number): void {
  const started = performance.now();
  const style = visualStyleStore.get();
  const wave = style.dotWave;
  const depth = wave ? style.dotDepth : 0;
  const strength = wave ? style.dotWaveStrength : 0;
  const period = style.dotWavePeriod;
  const centre = { x: f.width / 2, y: f.height / 2 };
  const n = f.xs.length;
  for (let i = 0; i < n; i++) {
    const x = f.xs[i] ?? 0;
    const y = f.ys[i] ?? 0;
    // One height per point feeds position and colour, so the colour crest is the height crest.
    const h = depth > 0 || strength > 0 ? surfaceHeight(x, y, t, period) : 0;
    const p = projectPoint(x, y, h, depth, centre);
    f.px[i] = p.x;
    f.py[i] = p.y;
    f.pr[i] = p.r;
    const hue = strength > 0 ? Math.round(colourMix(h) * (HUES - 1)) : 0;
    const k = strength * (0.35 + 0.65 * p.near);
    f.buckets[i] = hue * STRENGTHS + Math.min(STRENGTHS - 1, Math.round(k * (STRENGTHS - 1)));
  }
  const ctx = f.ctx;
  ctx.clearRect(0, 0, f.width, f.height);
  const used = new Set(f.buckets);
  for (const b of used) {
    ctx.fillStyle = styles[b] ?? "rgba(140,140,150,0.26)";
    ctx.beginPath();
    for (let i = 0; i < n; i++) {
      if (f.buckets[i] !== b) continue;
      const x = f.px[i] ?? 0;
      const y = f.py[i] ?? 0;
      const r = f.pr[i] ?? 1;
      ctx.moveTo(x + r, y);
      ctx.arc(x, y, r, 0, Math.PI * 2);
    }
    ctx.fill();
  }
  f.cost = f.cost * 0.9 + (performance.now() - started) * 0.1;
  f.canvas.dataset.frameMs = f.cost.toFixed(2);
}

function frame(now: number): void {
  raf = 0;
  if (document.hidden || !animating()) return;
  if (now - last >= FRAME_MS) {
    last = now;
    for (const f of fields) if (f.visible) draw(f, now / 1000);
  }
  schedule();
}

function anyVisible(): boolean {
  for (const f of fields) if (f.visible) return true;
  return false;
}

function stop(): void {
  cancelAnimationFrame(raf);
  raf = 0;
}

function schedule(): void {
  if (!raf && anyVisible() && animating() && !document.hidden) raf = requestAnimationFrame(frame);
}

/** Redraws every field once (static frame or new settings) and restarts the loop when it should run. */
function refresh(rebuild: boolean): void {
  if (rebuild) buildStyles();
  const t = animating() ? performance.now() / 1000 : 0;
  for (const f of fields) draw(f, t);
  schedule();
}

function startShared(): void {
  buildStyles();
  const offStore = visualStyleStore.subscribe(() => refresh(true));
  // Theme, palette and motion level live on <html>; attribute writes are rare (settings changes only).
  let pending = 0;
  const mo = new MutationObserver(() => {
    if (pending) return;
    pending = requestAnimationFrame(() => {
      pending = 0;
      refresh(true);
    });
  });
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["style", "class", "data-motion"] });
  const onVisibility = () => (document.hidden ? stop() : refresh(false));
  document.addEventListener("visibilitychange", onVisibility);
  shared = {
    off: () => {
      offStore();
      mo.disconnect();
      cancelAnimationFrame(pending);
      document.removeEventListener("visibilitychange", onVisibility);
    },
  };
}

/** Attaches the renderer to a canvas; returns the cleanup. Without 2D canvas support the CSS dots stay. */
export function attachDotField(canvas: HTMLCanvasElement): () => void {
  const ctx = canvas.getContext("2d");
  if (!ctx || typeof ResizeObserver === "undefined") return () => undefined;
  const empty = new Float32Array(0);
  const f: Field = { canvas, ctx, width: 0, height: 0, xs: empty, ys: empty, buckets: new Uint8Array(0), px: empty, py: empty, pr: empty, visible: true, cost: 0 };
  if (!shared) startShared();
  fields.add(f);
  canvas.dataset.ready = "";
  const ro = new ResizeObserver(() => {
    layout(f);
    draw(f, animating() ? performance.now() / 1000 : 0);
  });
  ro.observe(canvas);
  const io =
    typeof IntersectionObserver === "undefined"
      ? null
      : new IntersectionObserver(([entry]) => {
          f.visible = !!entry?.isIntersecting;
          if (f.visible) schedule();
          else if (!anyVisible()) stop();
        });
  io?.observe(canvas);
  layout(f);
  draw(f, 0);
  schedule();
  return () => {
    ro.disconnect();
    io?.disconnect();
    fields.delete(f);
    delete canvas.dataset.ready;
    if (!fields.size) {
      stop();
      shared?.off();
      shared = null;
    }
  };
}
