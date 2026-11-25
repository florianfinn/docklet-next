// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { CheckCircle2, Clock3, Loader2, MinusCircle, XCircle } from "lucide-react";
import { Link } from "react-router";
import { inventoryId } from "@/data/http/inventory-mapping";
import { findRouteContainer } from "@/features/containers/container-route";
import { jobHref } from "@/domain/job-links";
import type { UpdateJobTarget } from "@/domain/stack-updates";
import type { ContainerInfo, Stack } from "@/domain/model";
import { ToneBadge } from "@/shared/components/tone";
import { useContainers, useStacks, useHosts } from "@/data/queries";
import { HostChip } from "@/shared/components/host-chip";
import { useImageError } from "@/features/images/use-image-error";
import { ActionDetail } from "@/features/activity/ActionDetail";
import { UpdateRecovery } from "./update-recovery";
import { openActionDialog } from "@/features/activity/action-dialog-store";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import type { ActionRun } from "@/domain/model";

export function JobUpdateOutcomes({ run }: { run: ActionRun }) {
  const adapter = useAdapter(), errors = useImageError(), t = useTranslations("common.stackUpdates"), activity = useTranslations("activity");
  const ref = run.jobRef, host = run.hostId ?? "0", hosts = useHosts();
  const stacks = useStacks(!!ref), containers = useContainers(!!ref);
  const query = useQuery({ queryKey: ["activity", host, "job-targets", ref?.jobId, ref?.runId, run.status, run.bulk?.completed], queryFn: () => adapter.getUpdateJobTargets!(host, ref!.jobId, ref!.runId), enabled: !!ref && !!adapter.getUpdateJobTargets, gcTime: 0, placeholderData: keepPreviousData, refetchInterval: !run.bulk && (run.status === "running" || run.status === "queued") ? 2000 : false });
  if (!ref) return null;
  return <>{query.isError && <QueryError compact error={query.error} />}{query.data?.map((target) => {
    const href = targetHref(target, stacks.isError ? undefined : stacks.data, containers.isError ? undefined : containers.data);
    const name = target.name || target.targetId || target.id;
    const failed = target.status === "failed" || target.status === "partial" || target.status === "needs_attention";
    const tone = failed ? "err" : target.status === "skipped" ? "warn" : target.status === "succeeded" ? "ok" : "gray";
    const Icon = failed ? XCircle : target.status === "skipped" ? MinusCircle : target.status === "succeeded" ? CheckCircle2 : target.status === "running" ? Loader2 : Clock3;
    return <div key={target.id} className="rounded-lg border p-3 text-sm" data-job-target={target.id}>
    <p className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">{target.targetKind && <span className="text-muted-foreground">{t(`targetKind.${target.targetKind}`)}</span>}{href ? <Link to={href} className={target.name ? "min-w-0 break-all text-primary-text hover:underline" : "min-w-0 break-all font-mono text-primary-text hover:underline"}>{name}</Link> : <span className={target.name ? "min-w-0 break-all" : "min-w-0 break-all font-mono"}>{name}</span>}{target.environmentId && <HostChip host={hosts.data?.find((h) => h.id === target.environmentId)} />}<ToneBadge tone={tone}><Icon className="size-3.5" aria-hidden />{activity(`jobStep.${target.status}`)}</ToneBadge>{target.inCurrentPlan === false && <ToneBadge tone="gray">{activity("bulk.outsidePlan")}</ToneBadge>}{target.updateAvailable && <ToneBadge tone="info">{activity("bulk.updateAvailable")}</ToneBadge>}</p>
    {target.code && <p>{errors(target.code)}</p>}
    {/* The child's recovery evidence decides the volume warning; a target code alone cannot prove an effect. */}
    {target.activityId && target.environmentId && target.status !== "skipped" && <ActionDetail run={{ ...run, id: target.activityId, hostId: target.environmentId, jobRef: undefined, updateRecovery: undefined }}>{(child) => <UpdateRecovery run={child} />}</ActionDetail>}
    {target.activityId && <Button type="button" variant="link" onClick={() => openActionDialog(target.activityId!)}>{t("openRun", { id: target.activityId })}</Button>}
  </div>; })}</>;
}

/** Snapshots stay readable; links resolve only against a fresh current inventory. */
function targetHref(target: UpdateJobTarget, stacks: Stack[] | undefined, containers: ContainerInfo[] | undefined): string | undefined {
  // A malformed snapshot renders an unlinked row instead of failing the whole list.
  if (!target.environmentId || !target.targetId || target.environmentId.includes("~") || target.targetId.includes("~")) return undefined;
  const ref = inventoryId(target.environmentId, target.targetId);
  if (target.targetKind === "stack") {
    const stack = stacks?.find((row) => row.id === ref && row.hostId === target.environmentId);
    return stack && !stack.inventory?.stale && !stack.inventory?.scopeStale ? jobHref({ kind: "auto-update", stackId: ref }) : undefined;
  }
  if (target.targetKind === "container") {
    const container = findRouteContainer(containers, ref);
    return container && container.hostId === target.environmentId && container.name === target.targetId && !container.inventory?.stale && !container.inventory?.scopeStale ? `/containers/${encodeURIComponent(ref)}` : undefined;
  }
  return undefined;
}
