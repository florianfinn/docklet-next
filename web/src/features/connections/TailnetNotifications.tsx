// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers. M41 #625: the disconnect notice turns into a reconnect notice.

import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "@/data/query-keys";
import { useTailnet } from "@/data/use-tailnet";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { tailnetChain, tailnetHostDown, type ChainLink } from "@/domain/tailnet";

/** Notify observed interruptions, with the same first failing link as the host dialog. */
export function TailnetNotifications() {
  const t = useTranslations("connections");
  const adapter = useAdapter();
  const { status } = useTailnet();
  const hosts = useQuery({ queryKey: queryKeys.hosts, queryFn: () => adapter.listHosts(), enabled: false });
  const previous = useRef(new Map<string, string>());
  const notices = useRef(new Set<string | number>());
  // One notice per observed interruption: its toast id and failing link, until the host is reachable again.
  const open = useRef(new Map<string, { id: string | number; link: ChainLink }>());
  useEffect(() => {
    const clear = () => { previous.current.clear(); open.current.clear(); for (const id of notices.current) toast.dismiss(id); notices.current.clear(); };
    const stop = identityLifetime(adapter).listen(clear);
    return () => { stop(); clear(); };
  }, [adapter]);
  useEffect(() => {
    if (!hosts.data || hosts.isError) return;
    const next = new Map<string, string>();
    for (const host of hosts.data) {
      if (host.networkRoute !== "tailnet") continue;
      // One interruption belongs to one registration: identity starts with the registration revision, while the
      // session part changes on every reconnect. A replacement registration starts without the old notice.
      const key = `${host.id}|${host.identity?.split("|")[0] ?? ""}`;
      const failed = tailnetChain(host, status).find((row) => row.state === "failed")?.link;
      const first = tailnetHostDown(host) ? failed : undefined;
      const value = first ?? "none";
      const before = previous.current.get(key);
      const notice = open.current.get(key);
      if (first && before !== undefined && value !== before) {
        const message = t("offlineNotice", { name: host.name, link: t(`links.${first}`) });
        const id = notice ? toast.warning(message, { id: notice.id }) : toast.warning(message);
        notices.current.add(id);
        open.current.set(key, { id, link: first });
      } else if (!first && notice && host.status === "online" && !failed) {
        // Pending, unknown or a still-failed hub or peer link is no proof of a reconnect.
        toast.success(t("reconnectedNotice", { name: host.name, link: t(`links.${notice.link}`) }), { id: notice.id });
        open.current.delete(key);
      }
      next.set(key, value);
    }
    // A removed host, a replaced registration or a host that left the tailnet route gets no success notice.
    for (const key of open.current.keys()) if (!next.has(key)) open.current.delete(key);
    previous.current = next;
  }, [hosts.data, hosts.isError, status, t]);
  return null;
}
