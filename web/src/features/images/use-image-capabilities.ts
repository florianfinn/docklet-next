// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useQueries } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
export function useImageHostCapabilities(operation: "pullImage" | "pruneImages", ids: string[]) {
  const adapter = useAdapter();
  const queries = useQueries({ queries: ids.map((id) => ({ queryKey: ["capability", operation, "host", id], queryFn: () => adapter.getCapability(operation, { kind: "host", id }) })) });
  return new Map(ids.map((id, i) => [id, adapter.kind === "demo" ? queries[i]?.data?.available !== false : queries[i]?.data?.available === true]));
}
