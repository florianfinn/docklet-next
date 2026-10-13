import { useEffect, useLayoutEffect, useRef, useState } from "react";

import { INITIAL_FIT, nextFit, type FitState, type FitView } from "@/shared/lib/fit-view";

/**
 * Picks the widest overview view that fits without horizontal overflow. The
 * ref goes on the element whose laid-out width counts (a table, or a host
 * block inside the block grid); its parent's content box is the available
 * width and is observed with ResizeObserver. Runs in a layout effect, so an overflowing
 * view never reaches the screen. A new `resetKey` (mode, rows, …) starts
 * again from the widest view.
 */
export function useFitView<E extends HTMLElement>(resetKey: string, views?: readonly FitView[]) {
  const ref = useRef<E>(null);
  const [state, setState] = useState<FitState>(INITIAL_FIT);
  const [key, setKey] = useState(resetKey);
  if (key !== resetKey) {
    setKey(resetKey);
    setState(INITIAL_FIT);
  }

  // Web fonts change text widths without resizing the container: measure again from the widest view.
  useEffect(() => {
    const fonts = typeof document !== "undefined" ? document.fonts : undefined;
    if (!fonts?.addEventListener) return;
    const remeasure = () => setState(INITIAL_FIT);
    fonts.addEventListener("loadingdone", remeasure);
    return () => fonts.removeEventListener("loadingdone", remeasure);
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
  }, [state.view, resetKey, views]);

  return { view: state.view, ref };
}
