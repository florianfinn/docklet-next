import { useEffect, useSyncExternalStore } from "react";

/**
 * Border glow (edge light) that follows the mouse. One document listener, one
 * animation frame per burst of moves and one fixed overlay: the overlay is laid
 * over the element whose border is nearest to the pointer and paints a small
 * radial gradient masked to the border ring only. The light appears only while
 * the pointer is within the configured reach of a border (inside or outside)
 * and never fills the surface. Pointer moves only write CSS variables; React never
 * re-renders for it.
 */
export const GLOW_SELECTOR = [
  '[data-slot="button"]',
  '[data-slot="card"]',
  '[data-slot="popover-content"]',
  '[data-slot="hover-card-content"]',
  // Dialogs and sheets light their own perimeter; the overlay layer sits above them (see glowLayer).
  '[role="dialog"]',
  '[role="alertdialog"]',
  '[data-slot="toggle"]',
  '[data-slot="sidebar-menu-button"]',
  // Notifications: the edge light is their only lit edge; status stays in icon, text and progress.
  ".toast-card",
  '[data-sonner-toast][data-styled="true"]',
  ".nav-row",
  "[data-glow]",
].join(",");

/**
 * Rows sit a few px apart (navigation, wizard steps, service list); a
 * neighbouring row must never take the light of the hovered one.
 */
const ROW_SELECTOR = '[data-slot="sidebar-menu-button"], .nav-row, [data-glow="row"]';

/** Dense graphs and explicit opt-outs never glow. */
const EXCLUDE = '.react-flow, [data-glow="off"]';

export interface GlowReach {
  /** Distance in px from the border at which the light starts inside the element. */
  inside: number;
  /** Distance in px from the border at which the light starts outside the element. */
  outside: number;
}

/** Default reach; the design lab tunes both sides separately (visual-style-store). */
export const GLOW_REACH: GlowReach = { inside: 32, outside: 22 };
/** Light outside the border is a softer shimmer than the light inside it. */
const OUTSIDE_STRENGTH = 0.6;
/** The inside reach never exceeds this share of the element's shorter side, so a block's centre stays dark. */
const INSIDE_SHARE = 0.4;
const MIN_INSIDE = 12;

let reach: GlowReach = GLOW_REACH;

/** Updates the reach of a running glow without restarting it; no React render involved. */
export function setGlowReach(next: GlowReach): void {
  reach = { inside: Math.max(1, next.inside), outside: Math.max(1, next.outside) };
}

/** Rows, buttons and inputs up to this height; the share cap above is for blocks only. */
const ROW_MAX = 48;
/** On a row the reach covers at least this share of its height, so the centre line is lit like its neighbours. */
const ROW_CENTRE_SHARE = 0.75;

/**
 * Inside reach for an element of this size. Blocks cap it at 40 % of their
 * shorter side so their centre stays dark. A row is lit across its whole
 * height: its reach is the chosen one up to the row height, but never less
 * than 75 % of it, so the exact middle (equally far from top and bottom)
 * gets a continuous light instead of a dark stripe.
 */
export function insideReach(width: number, height: number, inside: number = reach.inside): number {
  const short = Math.min(width, height);
  if (short <= ROW_MAX) return Math.max(MIN_INSIDE, Math.min(inside, short), ROW_CENTRE_SHARE * short);
  return Math.min(inside, Math.max(MIN_INSIDE, INSIDE_SHARE * short));
}

/**
 * Element the glow search starts from. A sidebar badge or action overlays its
 * row without being inside it; it counts as part of that row.
 */
export function glowStart(hovered: Element | null): Element | null {
  const item = hovered?.closest('[data-slot="sidebar-menu-item"]');
  if (!item || item.contains(hovered?.closest(GLOW_SELECTOR) ?? null)) return hovered;
  return item.querySelector(':scope > [data-slot="sidebar-menu-button"]') ?? hovered;
}

export function glowTarget(start: Element | null): HTMLElement | null {
  const target = start?.closest<HTMLElement>(GLOW_SELECTOR) ?? null;
  if (!target || target.closest(EXCLUDE)) return null;
  return target;
}

/**
 * Elements that may carry the light: the pointer's own targets (innermost
 * first) and targets found just beside the pointer. On a navigation row only
 * the row and its own ancestors count, so the light never jumps to the row
 * above or below while the pointer rests in the middle of a row; between rows
 * the nearest row wins. Elsewhere the probes keep the soft light outside real
 * card and button borders.
 */
export function glowCandidates(own: readonly HTMLElement[], probed: readonly (HTMLElement | null)[], layerOf: (el: Element) => number = () => 0): HTMLElement[] {
  const out = new Set(own);
  if (own[0]?.matches(ROW_SELECTOR)) return [...out];
  // A neighbour below the hovered layer (page card under a toast or sheet) never takes the light.
  const floor = own[0] ? layerOf(own[0]) : 0;
  for (const el of probed) if (el && layerOf(el) >= floor) out.add(el);
  return [...out];
}

/**
 * Stacking layer of an element in the page: the highest numeric z-index of
 * its positioned ancestors (0 for plain page content). The overlay is placed
 * one above its target's layer, so whatever covers the target (sticky header,
 * sheet, dialog, toast) also covers its light, and a toast's light is above
 * the toast.
 */
export function glowLayer(el: Element, win: Pick<Window, "getComputedStyle"> = window): number {
  let layer = 0;
  for (let node: Element | null = el; node && node !== node.ownerDocument.body; node = node.parentElement) {
    const style = win.getComputedStyle(node);
    if (style.position === "static") continue;
    const z = Number.parseInt(style.zIndex, 10);
    if (Number.isFinite(z)) layer = Math.max(layer, z);
  }
  return layer;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

/**
 * Point on the border of a rounded rectangle (uniform corner radius) nearest
 * to (x, y), relative to its top-left corner, with the signed distance to that
 * border (negative inside). In a corner region the point lies on the corner
 * arc, so it moves continuously around the corner.
 */
export function roundedEdgePoint(x: number, y: number, width: number, height: number, radius: number): { x: number; y: number; distance: number } {
  const r = clamp(radius, 0, Math.min(width, height) / 2);
  const inX = x >= r && x <= width - r;
  const inY = y >= r && y <= height - r;
  if (!inX && !inY) {
    const cx = clamp(x, r, width - r);
    const cy = clamp(y, r, height - r);
    const len = Math.hypot(x - cx, y - cy);
    if (len === 0) return { x: cx, y: cy, distance: -r };
    return { x: cx + ((x - cx) / len) * r, y: cy + ((y - cy) / len) * r, distance: len - r };
  }
  if (!inX) return x < r ? { x: 0, y, distance: -x } : { x: width, y, distance: x - width };
  if (!inY) return y < r ? { x, y: 0, distance: -y } : { x, y: height, distance: y - height };
  const sides = [x, width - x, y, height - y];
  const side = sides.indexOf(Math.min(...sides));
  const distance = -sides[side]!;
  if (side === 0) return { x: 0, y, distance };
  if (side === 1) return { x: width, y, distance };
  if (side === 2) return { x, y: 0, distance };
  return { x, y: height, distance };
}

export interface EdgeGlow {
  /** Centre of the light on the border, relative to the element's top-left corner. */
  x: number;
  y: number;
  /** Signed distance of the pointer to the real border; negative inside. */
  distance: number;
  /** 0 … 1; 0 means no light. */
  strength: number;
}

/**
 * Edge light for a pointer at (x, y) relative to an element. The light centre
 * is projected with a corner radius of at least the inside reach, so it never
 * jumps across the diagonal of a sharp or slightly rounded corner while the
 * light is visible; the strength uses the real border distance. Inside, the
 * light falls off quadratically, so it is already strong well before the border.
 */
export function edgeGlow(x: number, y: number, width: number, height: number, radius: number, glowReach: GlowReach = reach): EdgeGlow {
  const inside = insideReach(width, height, glowReach.inside);
  const { distance } = roundedEdgePoint(x, y, width, height, radius);
  const centre = roundedEdgePoint(x, y, width, height, Math.max(radius, inside));
  const t = -distance / inside;
  const raw = distance <= 0 ? 1 - t * t : (1 - distance / glowReach.outside) * OUTSIDE_STRENGTH;
  const s = clamp(raw, 0, 1);
  // Smoothstep, so the light fades in and out softly at the edge of its reach.
  return { x: centre.x, y: centre.y, distance, strength: s * s * (3 - 2 * s) };
}

/** Starts the glow; returns the cleanup that removes listeners, frame and overlay. */
export function startPointerGlow(doc: Document = document): () => void {
  const overlay = doc.createElement("div");
  overlay.dataset.slot = "pointer-glow";
  overlay.setAttribute("aria-hidden", "true");
  overlay.innerHTML = '<span data-part="ring"></span>';
  doc.body.append(overlay);

  const win = doc.defaultView ?? window;
  const radii = new WeakMap<Element, { css: string; px: number }>();
  let hovered: Element | null = null;
  let current: HTMLElement | null = null;
  let x = 0;
  let y = 0;
  let active = false;
  let frame = 0;

  const radiusOf = (el: HTMLElement) => {
    let r = radii.get(el);
    if (!r) {
      const css = win.getComputedStyle(el).borderRadius;
      r = { css, px: Number.parseFloat(css) || 0 };
      radii.set(el, r);
    }
    return r;
  };

  // Layers are read once per paint; the toaster drops below a modal dialog, so they are not cached longer.
  let layers = new Map<Element, number>();
  const layerOf = (el: Element) => {
    let l = layers.get(el);
    if (l === undefined) {
      l = glowLayer(el, win);
      layers.set(el, l);
    }
    return l;
  };

  const candidates = (): HTMLElement[] => {
    const own: HTMLElement[] = [];
    for (let el = glowTarget(glowStart(hovered)); el && own.length < 4; el = glowTarget(el.parentElement)) own.push(el);
    if (own[0]?.matches(ROW_SELECTOR)) return own; // saves the probes; glowCandidates drops them anyway
    const probe = reach.outside + 2;
    const probed = (
      [
        [x + probe, y],
        [x - probe, y],
        [x, y + probe],
        [x, y - probe],
      ] as const
    ).map(([px, py]) => glowTarget(doc.elementFromPoint?.(px, py) ?? null));
    return glowCandidates(own, probed, layerOf);
  };

  const paint = () => {
    frame = 0;
    layers = new Map();
    let best: { el: HTMLElement; rect: DOMRect; glow: EdgeGlow } | null = null;
    if (active) {
      for (const el of candidates()) {
        if (!el.isConnected) continue;
        const rect = el.getBoundingClientRect();
        const glow = edgeGlow(x - rect.left, y - rect.top, rect.width, rect.height, radiusOf(el).px);
        if (glow.strength > 0 && (!best || glow.strength > best.glow.strength)) best = { el, rect, glow };
      }
    }
    const s = overlay.style;
    if (!best) {
      s.opacity = "0";
      current = null;
      return;
    }
    if (best.el !== current) {
      current = best.el;
      s.borderRadius = radiusOf(best.el).css;
    }
    s.zIndex = String(layerOf(best.el) + 1);
    s.transform = `translate(${best.rect.left}px, ${best.rect.top}px)`;
    s.width = `${best.rect.width}px`;
    s.height = `${best.rect.height}px`;
    s.setProperty("--gx", `${best.glow.x}px`);
    s.setProperty("--gy", `${best.glow.y}px`);
    s.opacity = best.glow.strength.toFixed(3);
  };
  const schedule = () => {
    if (!frame) frame = win.requestAnimationFrame(paint);
  };
  const onMove = (e: PointerEvent) => {
    if (e.pointerType !== "mouse") return;
    x = e.clientX;
    y = e.clientY;
    hovered = e.target as Element | null;
    active = true;
    schedule();
  };
  const onLeave = () => {
    active = false;
    schedule();
  };
  const onScroll = () => {
    if (!active) return;
    hovered = doc.elementFromPoint?.(x, y) ?? hovered;
    schedule();
  };

  doc.addEventListener("pointermove", onMove, { passive: true });
  doc.documentElement.addEventListener("pointerleave", onLeave);
  win.addEventListener("scroll", onScroll, { passive: true, capture: true });
  return () => {
    doc.removeEventListener("pointermove", onMove);
    doc.documentElement.removeEventListener("pointerleave", onLeave);
    win.removeEventListener("scroll", onScroll, { capture: true });
    if (frame) win.cancelAnimationFrame(frame);
    overlay.remove();
  };
}

const finePointer = typeof window !== "undefined" ? window.matchMedia?.("(hover: hover) and (pointer: fine)") : undefined;

function useFinePointer(): boolean {
  return useSyncExternalStore(
    (notify) => {
      finePointer?.addEventListener("change", notify);
      return () => finePointer?.removeEventListener("change", notify);
    },
    () => finePointer?.matches ?? false,
    () => false,
  );
}

/** Runs the glow only when enabled, with a fine pointer and without reduced motion. */
export function usePointerGlow(enabled: boolean, intensity: number, reducedMotion: boolean, inside: number = GLOW_REACH.inside, outside: number = GLOW_REACH.outside): void {
  const fine = useFinePointer();
  const active = enabled && fine && !reducedMotion;
  useEffect(() => setGlowReach({ inside, outside }), [inside, outside]);
  useEffect(() => {
    document.documentElement.style.setProperty("--pointer-glow", String(intensity));
  }, [intensity]);
  useEffect(() => {
    if (!active) return;
    document.documentElement.dataset.pointerGlow = "on";
    const stop = startPointerGlow();
    return () => {
      stop();
      delete document.documentElement.dataset.pointerGlow;
    };
  }, [active]);
}
