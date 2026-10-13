import { useCallback, useSyncExternalStore } from "react";

/** Live match of a CSS media query; false where matchMedia is missing. */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (notify: () => void) => {
      const mql = typeof window !== "undefined" ? window.matchMedia?.(query) : undefined;
      mql?.addEventListener("change", notify);
      return () => mql?.removeEventListener("change", notify);
    },
    [query],
  );
  return useSyncExternalStore(
    subscribe,
    () => (typeof window !== "undefined" ? (window.matchMedia?.(query).matches ?? false) : false),
    () => false,
  );
}
