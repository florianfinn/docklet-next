// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "@/data/query-keys";
import { useTailnet } from "@/data/use-tailnet";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { tailnetChain, tailnetHostDown } from "@/domain/tailnet";

/** Notify observed interruptions, with the same first failing link as the host dialog. */
export function TailnetNotifications() {
  const t = useTranslations("connections");
  const adapter = useAdapter();
  const { status } = useTailnet();
  const hosts = useQuery({ queryKey: queryKeys.hosts, queryFn: () => adapter.listHosts(), enabled: false });
  const previous = useRef(new Map<string, string>());
  const notices = useRef(new Set<string | number>());
  useEffect(() => {
    const clear = () => { previous.current.clear(); for (const id of notices.current) toast.dismiss(id); notices.current.clear(); };
    const stop = identityLifetime(adapter).listen(clear);
    return () => { stop(); clear(); };
  }, [adapter]);
  useEffect(() => {
    if (!hosts.data || hosts.isError) return;
    const next = new Map<string, string>();
    for (const host of hosts.data) {
      if (host.networkRoute !== "tailnet") continue;
      const first = tailnetHostDown(host) ? tailnetChain(host, status).find((row) => row.state === "failed")?.link : undefined;
      const value = first ?? "none";
      const before = previous.current.get(host.id);
      if (first && before !== undefined && value !== before) notices.current.add(toast.warning(t("offlineNotice", { name: host.name, link: t(`links.${first}`) })));
      next.set(host.id, value);
    }
    previous.current = next;
  }, [hosts.data, hosts.isError, status, t]);
  return null;
}
