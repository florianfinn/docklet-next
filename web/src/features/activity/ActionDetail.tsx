// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { useLocale, useTranslations } from "use-intl";
import type { ActionDetailState } from "@/data/adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { renderRun } from "@/data/message-code";
import { useAdapter } from "@/data/use-adapter";
import type { ActionRun } from "@/domain/model";
import { QueryError } from "@/shared/components/page";

/** Only a mounted detail body owns history. Summary updates cannot erase it. */
export function ActionDetail({ run, children }: { run: ActionRun; children: (run: ActionRun) => ReactNode }) {
  const adapter = useAdapter();
  const lifetime = identityLifetime(adapter);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const locale = useLocale();
  const t = useTranslations("activity");
  const host = run.hostId ?? "0";
  const key = JSON.stringify([host, run.id, generation]);
  const [view, setView] = useState<{ key: string; state: ActionDetailState }>();
  useEffect(() => {
    if (!adapter.observeActionDetail || !lifetime.active()) return;
    let active = true;
    const off = adapter.observeActionDetail(host, run.id, (state) => { if (active) setView({ key, state }); });
    return () => { active = false; off(); };
  }, [adapter, host, run.id, key, lifetime]);
  if (!adapter.observeActionDetail) return children(run);
  const state = view?.key === key ? view.state : undefined;
  const full = state?.status === "ready" || state?.status === "error" ? state.run : undefined;
  if (full) return <>{children(renderRun(full, locale))}{state?.status === "error" ? <QueryError compact error={state.error} /> : state?.status === "ready" && state.refreshing ? <p role="status" className="text-sm text-muted-foreground">{t("detailLoading")}</p> : null}</>;
  if (state?.status === "error") return <QueryError compact error={state.error} />;
  return <p role="status" className="p-3 text-sm text-muted-foreground">{t("detailLoading")}</p>;
}
