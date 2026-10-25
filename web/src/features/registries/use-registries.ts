// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
export function useRegistries(enabled = true) {
  const adapter = useAdapter(), capability = useCapability("listContainerRegistries");
  const query = useQuery({ queryKey: ["registries"], queryFn: () => adapter.listContainerRegistries(), enabled: enabled && capability.available });
  return { ...query, capability };
}
