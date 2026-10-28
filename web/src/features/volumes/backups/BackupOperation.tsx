// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original activity following for rediscovered backup operations.

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { backupCodeKey } from "@/data/backup-error";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { ActionDetailState } from "@/data/adapter";
import { openActionDialog } from "@/features/activity/action-dialog-store";
import { Button } from "@/shared/ui/button";

export function BackupOperation({ hostId, activityId }: { hostId: string; activityId: string }) {
  const adapter = useAdapter(), client = useQueryClient(), t = useTranslations("volumes.backups"), all = useTranslations();
  const [state, setState] = useState<ActionDetailState>();
  useEffect(() => adapter.observeActionDetail?.(hostId, activityId, (next) => {
    setState(next);
    if (next.status === "ready" && !["queued", "running"].includes(next.run.status)) {
      for (const key of [["volume"], ["volumes"], ["backup-overview"], ["backup-overviews"], ["backup-targets"], ["backup-repository"]]) void client.invalidateQueries({ queryKey: key });
    }
  }), [adapter, hostId, activityId, client]);
  const run = state?.status === "ready" ? state.run : undefined;
  const code = backupCodeKey(run?.backupOutcome?.errorCode);
  const outcome = run?.backupOutcome?.status;
  const status = outcome && ["succeeded", "failed", "partial", "unavailable", "running"].includes(outcome) ? t(`status.${outcome as "partial"}`) : undefined;
  return <div role="status" className="flex flex-wrap items-center gap-3 rounded-xl border p-3 text-sm"><span>{t("operation")}: {code ? all(code) : status ?? (run ? `${Math.round(run.progress)}%` : t("following"))}</span><Button variant="outline" size="sm" onClick={() => openActionDialog(activityId)}>{t("openActivity")}</Button></div>;
}
