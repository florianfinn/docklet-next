// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Scoped volume capabilities with the existing demo port for #181.

import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";

export function useVolumeCapability(operation: string, id: string) {
  const adapter = useAdapter();
  return useCapability(operation, adapter.kind === "demo" ? undefined : { kind: "volume", id });
}
