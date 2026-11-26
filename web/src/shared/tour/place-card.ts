// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour card placement.

/** Space around the highlighted element, gap to the card and minimum viewport margin (px). */
export const PAD = 8;
const GAP = 12;
const MARGIN = 16;

/** Card position: below, right, above or left of the highlight; otherwise bottom right over it. */
export function placeCard(rect: DOMRect | null, width: number, height: number, vw: number, vh: number): { left: number; top: number } {
  const clampX = (x: number) => Math.max(MARGIN, Math.min(x, vw - width - MARGIN));
  const clampY = (y: number) => Math.max(MARGIN, Math.min(y, vh - height - MARGIN));
  if (!rect) return { left: clampX((vw - width) / 2), top: clampY((vh - height) / 2) };
  const top = rect.top - PAD, left = rect.left - PAD, right = rect.right + PAD, bottom = rect.bottom + PAD;
  // Clamped on both axes, so a highlight scrolled partly out of view never pushes the card off screen.
  const at = (x: number, y: number) => ({ left: clampX(x), top: clampY(y) });
  if (vh - bottom >= height + GAP + MARGIN) return at(left, bottom + GAP);
  if (vw - right >= width + GAP + MARGIN) return at(right + GAP, top);
  if (top >= height + GAP + MARGIN) return at(left, top - GAP - height);
  if (left >= width + GAP + MARGIN) return at(left - GAP - width, top);
  return at(vw, vh);
}
