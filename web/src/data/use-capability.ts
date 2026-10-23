// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original UI capability hook for issue 38.

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { GENERIC_REASON, HUB_REF, type ResourceRef } from "./capability";
import { useAdapter } from "./use-adapter";

/** A control remains disabled until both its adapter mapping and server descriptor are readable. */
export function useCapability(operationId: string, resourceRef: ResourceRef = HUB_REF) {
  const adapter = useAdapter();
  const t = useTranslations("activity.capability");
  const query = useQuery({
    queryKey: ["capability", operationId, resourceRef.kind, resourceRef.id],
    queryFn: () => adapter.getCapability(operationId, resourceRef),
  });
  const available = adapter.kind === "demo" ? query.data?.available !== false : query.data?.available === true;
  const code = query.data?.disabledReasonCode ?? GENERIC_REASON;
  const reason = code === "capability.mapping-unavailable" ? t("mappingUnavailable") : t("unavailable");
  return { available, pending: query.isPending, reason: available ? undefined : reason };
}
