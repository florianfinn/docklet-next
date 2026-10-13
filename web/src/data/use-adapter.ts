import { createContext, useContext } from "react";

import type { DockletAdapter } from "./adapter";

export const AdapterContext = createContext<DockletAdapter | null>(null);

export function useAdapter(): DockletAdapter {
  const adapter = useContext(AdapterContext);
  if (!adapter) throw new Error("useAdapter outside AdapterProvider");
  return adapter;
}
