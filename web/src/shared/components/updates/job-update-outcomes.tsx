// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useImageError } from "@/features/images/use-image-error";
import { ActionDetail } from "@/features/activity/ActionDetail";
import { UpdateRecovery } from "./update-recovery";
import { openActionDialog } from "@/features/activity/action-dialog-store";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import type { ActionRun } from "@/domain/model";

export function JobUpdateOutcomes({ run }: { run: ActionRun }) {
  const adapter = useAdapter(), errors = useImageError(), t = useTranslations("common.stackUpdates"), activity = useTranslations("activity");
  const ref = run.jobRef, host = run.hostId ?? "0";
  const query = useQuery({ queryKey: ["activity", host, "job-targets", ref?.jobId, ref?.runId, run.status], queryFn: () => adapter.getUpdateJobTargets!(host, ref!.jobId, ref!.runId), enabled: !!ref && !!adapter.getUpdateJobTargets, refetchInterval: run.status === "running" || run.status === "queued" ? 2000 : false });
  if (!ref) return null;
  return <>{query.isError && <QueryError compact error={query.error} />}{query.data?.map((target) => <div key={target.id} className="rounded-lg border p-3 text-sm" data-job-target={target.id}>
    <p>{target.id} · {activity(`jobStep.${target.status}`)}</p>
    {target.code && <p>{errors(target.code)}</p>}
    {(target.status === "failed" || target.status === "canceled") && <p>{t("volumeNotRestored")}</p>}
    {target.activityId && target.environmentId && target.status !== "skipped" && <ActionDetail run={{ ...run, id: target.activityId, hostId: target.environmentId, jobRef: undefined, updateRecovery: undefined }}>{(child) => <UpdateRecovery run={child} volumeWarning={false} />}</ActionDetail>}
    {target.activityId && <Button type="button" variant="link" onClick={() => openActionDialog(target.activityId!)}>{t("openRun", { id: target.activityId })}</Button>}
  </div>)}</>;
}
