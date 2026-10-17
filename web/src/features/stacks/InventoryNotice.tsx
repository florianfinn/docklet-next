// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original localized inventory observation notices.

import { useFormatter, useTranslations } from "use-intl";
import type { Host, InventoryObservation } from "@/domain/model";

export function InventoryNotice({ hosts, items }: { hosts: Host[]; items: { hostId: string; inventory?: InventoryObservation }[] }) {
  const t = useTranslations("stacks.inventory");
  const format = useFormatter();
  return <div role="status" className="space-y-1 text-sm text-muted-foreground">
    {hosts.map((host) => {
      const retained = items.find((item) => item.hostId === host.id && item.inventory?.stale)?.inventory;
      const state = retained ?? host.inventory;
      if (!state?.stale && host.status === "online") return null;
      const reason = state?.reason ?? (host.status === "update-required" ? "agent.update-required" : "agent.unavailable");
      return <p key={host.id}>{t("host", { name: host.name, reason: t(reason === "agent.update-required" ? "updateRequired" : reason === "agent.invalid-token" ? "credentialRejected" : reason === "inventory.mapping-error" ? "mappingError" : reason === "inventory.read-error" ? "readError" : "unavailable") })} {state?.observedAt ? t("asOf", { time: format.dateTime(new Date(state.observedAt), { dateStyle: "medium", timeStyle: "medium" }) }) : t("noSnapshot")}</p>;
    })}
  </div>;
}
