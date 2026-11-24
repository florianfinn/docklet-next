// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original localized inventory observation notices. M14 host maintenance and availability. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { isHostAvailable } from "@/domain/hosts";
import { useFormatter, useTranslations } from "use-intl";
import type { Host, InventoryObservation, UnreadableKind } from "@/domain/model";
import { unreadableRows } from "@/domain/unreadable";

export function InventoryNotice({ hosts, items, kinds = [] }: { hosts: Host[]; items: { hostId: string; inventory?: InventoryObservation }[]; kinds?: readonly UnreadableKind[] }) {
  const t = useTranslations("stacks.inventory");
  const format = useFormatter();
  return <div role="status" className="space-y-1 text-sm text-muted-foreground">
    {hosts.flatMap((host) => {
      const out = [];
      const unreadable = unreadableRows(host.inventory, kinds);
      if (unreadable.count > 0) out.push(<p key={`${host.id}:rows`} className="text-warning">{[t("unreadable", { name: host.name, count: unreadable.count }), ...(unreadable.names.length ? [t("unreadableNames", { names: unreadable.names.join(", ") })] : [])].join(" ")}</p>);
      const retained = items.find((item) => item.hostId === host.id && item.inventory?.stale)?.inventory;
      const state = retained ?? host.inventory;
      if (!state?.stale && isHostAvailable(host)) return out;
      const reason = state?.reason ?? (host.status === "update-required" ? "agent.update-required" : "agent.unavailable");
      // A stale catalogue of an available host is the only fact; it names no host reason.
      const scopeOnly = !!state?.scopeStale && !state.reason && isHostAvailable(host);
      const label = scopeOnly ? t("scopeStale") : t(reason === "agent.update-required" ? "updateRequired" : reason === "agent.invalid-token" ? "credentialRejected" : reason === "inventory.mapping-error" ? "mappingError" : reason === "inventory.read-error" ? "readError" : "unavailable");
      const sentences = [t("host", { name: host.name, reason: label }), ...(state?.scopeStale && !scopeOnly ? [t("scopeStaleNote")] : []),
        state?.observedAt ? t("asOf", { time: format.dateTime(new Date(state.observedAt), { dateStyle: "medium", timeStyle: "medium" }) }) : t("noSnapshot")];
      out.push(<p key={host.id}>{sentences.join(" ")}</p>);
      return out;
    })}
  </div>;
}
