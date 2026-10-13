/**
 * Scroll cues for browsers without scroll-driven animations (Firefox, older
 * Safari). Sets the same edge fade as styles/scroll.css, but only on regions
 * that really overflow, as `data-cue` plus `--cue-start`/`--cue-end`. Work
 * is batched per frame and skipped while the tab is hidden.
 */
export const CUE_REGIONS = ".overflow-y-auto, .overflow-auto, .overflow-x-auto, .cm-scroller, [data-scroll-cue]";
/** Surfaces keep their shadow and animations; closing elements keep their exit animation. */
export const CUE_EXCLUDED = '[data-scroll-cue="off"], [data-slot$="-content"], [role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"], [data-state="closed"]';

/** Fade length at an edge with more content beyond it (1.5rem). */
const CUE_PX = 24;

export interface CueMetrics {
  scrollTop: number;
  scrollLeft: number;
  scrollHeight: number;
  scrollWidth: number;
  clientHeight: number;
  clientWidth: number;
}

/** Axis and fade lengths of one region; none while it fits. */
export function cueOf(m: CueMetrics, horizontal: boolean): { start: number; end: number } | undefined {
  const pos = horizontal ? m.scrollLeft : m.scrollTop;
  const max = horizontal ? m.scrollWidth - m.clientWidth : m.scrollHeight - m.clientHeight;
  if (max <= 1) return undefined;
  return { start: Math.round(Math.min(CUE_PX, Math.max(0, pos))), end: Math.round(Math.min(CUE_PX, Math.max(0, max - pos))) };
}

function isHorizontal(el: Element): boolean {
  return el.classList.contains("overflow-x-auto") || el.getAttribute("data-scroll-cue") === "x";
}

export function updateCue(el: HTMLElement): void {
  const cue = el.matches(CUE_EXCLUDED) ? undefined : cueOf(el, isHorizontal(el));
  if (!cue) {
    if (el.hasAttribute("data-cue")) {
      el.removeAttribute("data-cue");
      el.style.removeProperty("--cue-start");
      el.style.removeProperty("--cue-end");
    }
    return;
  }
  const axis = isHorizontal(el) ? "x" : "y";
  if (el.getAttribute("data-cue") !== axis) el.setAttribute("data-cue", axis);
  const start = `${cue.start}px`;
  const end = `${cue.end}px`;
  if (el.style.getPropertyValue("--cue-start") !== start) el.style.setProperty("--cue-start", start);
  if (el.style.getPropertyValue("--cue-end") !== end) el.style.setProperty("--cue-end", end);
}

/** Page fade above the bottom edge while more page follows. */
function updatePage(root: HTMLElement): void {
  const more = root.scrollHeight - root.clientHeight - root.scrollTop > 1;
  root.toggleAttribute("data-page-cue", more);
}

export function supportsScrollTimeline(): boolean {
  return typeof CSS !== "undefined" && typeof CSS.supports === "function" && CSS.supports("animation-timeline: scroll()");
}

/** Starts the fallback when needed; returns its cleanup. `force` runs it regardless (tests). */
export function installScrollCueFallback(doc: Document = document, force = false): () => void {
  if (!force && supportsScrollTimeline()) return () => undefined;
  const root = doc.documentElement;
  root.setAttribute("data-scroll-cue-js", "");
  let frame = 0;
  const all = () => {
    frame = 0;
    if (doc.hidden) return;
    doc.querySelectorAll<HTMLElement>(CUE_REGIONS).forEach(updateCue);
    doc.querySelectorAll<HTMLElement>("[data-cue]").forEach((el) => !el.matches(CUE_REGIONS) && updateCue(el));
    updatePage(root);
  };
  const schedule = () => {
    if (!frame) frame = requestAnimationFrame(all);
  };
  const onScroll = (e: Event) => {
    const el = e.target;
    if (el === doc || el === root) updatePage(root);
    else if (el instanceof HTMLElement && el.matches(CUE_REGIONS)) updateCue(el);
  };
  // Content growth (logs, lists) changes overflow without resizing the region; mutations cover it.
  const mutations = new MutationObserver(schedule);
  mutations.observe(doc.body, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ["class", "data-state"] });
  const win = doc.defaultView ?? window;
  win.addEventListener("resize", schedule);
  doc.addEventListener("scroll", onScroll, { capture: true, passive: true });
  doc.addEventListener("visibilitychange", schedule);
  schedule();
  return () => {
    if (frame) cancelAnimationFrame(frame);
    mutations.disconnect();
    win.removeEventListener("resize", schedule);
    doc.removeEventListener("scroll", onScroll, { capture: true });
    doc.removeEventListener("visibilitychange", schedule);
    root.removeAttribute("data-scroll-cue-js");
    root.removeAttribute("data-page-cue");
  };
}
