import { useEffect, useLayoutEffect, useRef, useState } from "react";

import { INITIAL_FIT, nextFit, type FitState, type FitView } from "@/shared/lib/fit-view";
import { activePopupId, subscribePopups } from "@/shared/lib/popup-coordinator";

/**
 * Picks the widest overview view that fits without horizontal overflow. The
 * ref goes on the element whose laid-out width counts (a table, or a host
 * block inside the block grid); its parent's content box is the available
 * width and is observed with ResizeObserver. Runs in a layout effect, so an overflowing
 * view never reaches the screen. A new `resetKey` (mode, rows, …) starts
 * again from the widest view. A new `remeasureKey` (for example filter values) only
 * measures again from the current view, so controls rendered for that view stay mounted.
 */
export function useFitView<E extends HTMLElement>(resetKey: string, views?: readonly FitView[], remeasureKey?: string) {
  const ref = useRef<E>(null);
  const [state, setState] = useState<FitState>(INITIAL_FIT);
  const [key, setKey] = useState(resetKey);
  if (key !== resetKey) {
    setKey(resetKey);
    setState(INITIAL_FIT);
  }

  // Web fonts change text widths without resizing the container: measure again from the widest view, since widths
  // recorded with the fallback font are stale. That reset remounts the rows, so while a popup is open (a hover card
  // inside a row on phones) only the mounted view is measured and the full reset waits until the popup closes.
  const [fontTick, setFontTick] = useState(0);
  useEffect(() => {
    const fonts = typeof document !== "undefined" ? document.fonts : undefined;
    if (!fonts?.addEventListener) return;
    let unsubscribe: (() => void) | undefined;
    const reset = () => {
      unsubscribe?.();
      unsubscribe = undefined;
      setState(INITIAL_FIT);
    };
    const remeasure = () => {
      if (activePopupId() === null) return reset();
      setFontTick((t) => t + 1);
      unsubscribe ??= subscribePopups(() => { if (activePopupId() === null) reset(); });
    };
    fonts.addEventListener("loadingdone", remeasure);
    return () => {
      fonts.removeEventListener("loadingdone", remeasure);
      unsubscribe?.();
    };
  }, []);

  useLayoutEffect(() => {
    const content = ref.current;
    const container = content?.parentElement;
    if (!content || !container) return;
    // The content box counts: a padded container (host block body) leaves less room than its clientWidth.
    const available = () => {
      const cs = getComputedStyle(container);
      return container.clientWidth - (parseFloat(cs.paddingLeft) || 0) - (parseFloat(cs.paddingRight) || 0);
    };
    const check = () => setState((s) => nextFit(s, available(), content.getBoundingClientRect().width, views));
    check();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(check);
    ro.observe(container);
    return () => ro.disconnect();
  }, [state.view, resetKey, views, remeasureKey, fontTick]);

  return { view: state.view, ref };
}
