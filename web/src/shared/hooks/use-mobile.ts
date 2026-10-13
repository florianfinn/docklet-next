import * as React from "react"

// Adapted from shadcn/ui use-mobile: useSyncExternalStore instead of setState in an effect.
const MOBILE_BREAKPOINT = 768
const query = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

export function useIsMobile() {
  return React.useSyncExternalStore(
    (notify) => {
      const mql = window.matchMedia(query)
      mql.addEventListener("change", notify)
      return () => mql.removeEventListener("change", notify)
    },
    () => window.matchMedia(query).matches,
    () => false,
  )
}
