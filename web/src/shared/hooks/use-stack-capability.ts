// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original stack action host-version presentation alongside admission descriptors.

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";

/** Host metadata can disable an action before admission checks the current version. */
export function useStackCapability(id: string, owningHostId?: string) {
  const capability = useCapability("runStackCommand", { kind: "stack", id });
  const adapter = useAdapter();
  const hosts = useQuery({ queryKey: queryKeys.hosts, queryFn: () => adapter.listHosts() });
  const t = useTranslations("containers.operationReasons");
  const hostId = id.includes("~") ? id.split("~")[0] : owningHostId;
  const host = hosts.data?.find((item) => item.id === hostId);
  if (host?.status === "update-required") return { ...capability, available: false, reason: t("updateRequired") };
  if (host?.status === "token-rejected") return { ...capability, available: false, reason: t("credentialRejected") };
  return capability;
}
