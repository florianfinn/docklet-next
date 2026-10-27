// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";
import { useQueryClient } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useActionRuns } from "@/data/live";
import { Button } from "@/shared/ui/button";
import { Progress } from "@/shared/ui/progress";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { isHostVisible } from "@/domain/hosts";

function ActiveScan({ hostId, id, progress, label }: { hostId: string; id: string; progress?: number; label: string }) {
  const adapter = useAdapter(), t = useTranslations("security"), client = useQueryClient();
  const hosts = useHostScope();
  const cancel = useCapability("cancelScan", { kind: "host", id: hostId });
  const [error, setError] = useState(false), [cancelling, setCancelling] = useState(false);
  useEffect(() => adapter.observeActionDetail?.(hostId, id, (state) => {
    if (state.status === "ready" && !["running", "queued"].includes(state.run.status)) {
      void client.invalidateQueries({ queryKey: ["security"] });
      void client.invalidateQueries({ queryKey: ["images"] });
    }
  }), [adapter, hostId, id, client]);
  return <div className="mb-3 flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3 text-sm" data-scan-activity={id}>
    <span className="min-w-0 break-all">{hosts.byId.get(hostId)?.name ?? hostId} · {label}</span><Progress value={progress} className="h-1.5 min-w-24 flex-1" /><span>{progress === undefined ? t("scanning") : `${progress}%`}</span>
    {adapter.cancelScan && <Button size="sm" variant="outline" disabled={!cancel.available || cancelling} onClick={() => { setCancelling(true); void adapter.cancelScan!(hostId, id).catch(() => setError(true)).finally(() => setCancelling(false)); }}>{t("cancel")}</Button>}
    {error && <p role="alert">{t("scanUnavailable")}</p>}
  </div>;
}
export function ScanProgress({ selection }: { selection: readonly string[] }) {
  const runs = useActionRuns(), t = useTranslations("security"), adapter = useAdapter();
  return runs.filter((r) => r.kind === "scan" && ["running", "queued"].includes(r.status) && isHostVisible(selection, r.hostId ?? "0")).map((r) => <ActiveScan key={`${r.hostId}:${r.id}`} hostId={r.hostId ?? "0"} id={r.id} progress={adapter.kind === "http" ? r.scanProgress : r.progress} label={r.steps.find((s) => s.state === "running")?.label ?? t("scanning")} />);
}
