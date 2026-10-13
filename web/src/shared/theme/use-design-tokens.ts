import { useEffect, useSyncExternalStore } from "react";

import { useStore } from "@/shared/lib/store";

import { applyTokens, designStore, motionEnabled, tokenPreviewStore, type DesignTokens } from "@/shared/theme/design-tokens";

const query = typeof window !== "undefined" ? window.matchMedia?.("(prefers-reduced-motion: reduce)") : undefined;

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    (notify) => {
      query?.addEventListener("change", notify);
      return () => query?.removeEventListener("change", notify);
    },
    () => !!query?.matches,
    () => false,
  );
}

/** Keeps the document root in sync with saved tokens, an active settings preview and the OS motion setting. */
export function useApplyDesignTokens(): void {
  const tokens = useStore(designStore, (s) => s.tokens);
  const preview = useStore(tokenPreviewStore, (p) => p);
  const reduced = usePrefersReducedMotion();
  useEffect(() => applyTokens(preview ? { ...tokens, ...preview } : tokens, reduced), [tokens, preview, reduced]);
}

export function useTokens(): DesignTokens {
  return useStore(designStore, (s) => s.tokens);
}

/** True when decorative animation (charts, topology flow) should run. */
export function useMotionEnabled(): boolean {
  const tokens = useTokens();
  const reduced = usePrefersReducedMotion();
  return motionEnabled(tokens, reduced);
}
