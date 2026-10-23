// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original network resource capability aggregation for #204.

import { useQueries } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";

/** Toolbar availability is aggregated from the displayed networks, using each network's descriptor. */
export function useNetworkCapabilities(operationId: string, networkIds: readonly string[]) {
  const adapter = useAdapter();
  const t = useTranslations("activity.capability");
  const queries = useQueries({ queries: networkIds.map((id) => ({
    queryKey: ["capability", operationId, "network", id],
    queryFn: () => adapter.getCapability(operationId, { kind: "network", id }),
  })) });
  const entries = networkIds.map((id, i) => {
    const descriptor = queries[i]?.data;
    const available = adapter.kind === "demo" ? descriptor?.available !== false : descriptor?.available === true;
    const reason = available ? undefined : descriptor?.disabledReasonCode === "capability.mapping-unavailable" ? t("mappingUnavailable") : t("unavailable");
    return [id, { available, reason }] as const;
  });
  return new Map(entries);
}
