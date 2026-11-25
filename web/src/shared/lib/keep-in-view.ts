// Source: docklet-next/issues/616
// Revision: 69ae087a814afbdde00afcbbb73cce86ed483768
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original anchor target retention while content above it is still loading.

const INPUT = ["wheel", "touchstart", "pointerdown", "keydown"] as const;

/**
 * Keeps a just-focused anchor target in view while reads above it still change the page height. It stops on
 * the first user input, the first time focus leaves the target, or after `ms`.
 */
export function keepInView(target: HTMLElement, ms = 5000): () => void {
  if (typeof ResizeObserver === "undefined") return () => {};
  let top = target.getBoundingClientRect().top, stopped = false;
  const observer = new ResizeObserver(() => {
    if (!target.isConnected || document.activeElement !== target) return stop();
    const next = target.getBoundingClientRect().top;
    if (next === top) return;
    target.scrollIntoView?.({ block: "center" });
    top = target.getBoundingClientRect().top;
  });
  const timer = setTimeout(() => stop(), ms);
  function stop() {
    if (stopped) return;
    stopped = true; observer.disconnect(); clearTimeout(timer);
    for (const type of INPUT) window.removeEventListener(type, stop, true);
    target.removeEventListener("focusout", stop);
  }
  for (const node of new Set([document.documentElement, target.closest("main") ?? document.body])) observer.observe(node);
  for (const type of INPUT) window.addEventListener(type, stop, { capture: true, passive: true });
  target.addEventListener("focusout", stop);
  return stop;
}
