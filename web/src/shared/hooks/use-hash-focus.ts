import { useEffect, type RefObject } from "react";
import { useLocation } from "react-router";

/**
 * Brings the element into view and focuses it once per link when the URL hash
 * names `anchor`. The element needs tabIndex={-1} and the anchor as id.
 */
export function useHashFocus(anchor: string, ref: RefObject<HTMLElement | null>, ready = true): void {
  const { hash } = useLocation();
  useEffect(() => {
    if (!ready || hash !== `#${anchor}`) return;
    const el = ref.current;
    el?.scrollIntoView({ block: "center" });
    el?.focus({ preventScroll: true });
  }, [hash, anchor, ref, ready]);
}
