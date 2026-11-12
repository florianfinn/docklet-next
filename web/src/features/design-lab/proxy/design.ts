// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { createStore, useStore } from "@/shared/lib/store";
export type ProxyDesign = "classic" | "manager" | "workspace";
export const designStore = createStore<ProxyDesign>("manager", {
  key: "docklet.proxy-design", version: 2,
  merge: (stored) => stored === "classic" || stored === "workspace" ? stored : "manager",
});
export const useProxyDesign = () => useStore(designStore, (value) => value);
