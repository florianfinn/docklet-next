import { createStore, useStore } from "@/shared/lib/store";
export type ProxyDesign = "classic" | "manager" | "workspace";
export const designStore = createStore<ProxyDesign>("manager", {
  key: "docklet.proxy-design", version: 2,
  merge: (stored) => stored === "classic" || stored === "workspace" ? stored : "manager",
});
export const useProxyDesign = () => useStore(designStore, (value) => value);
