// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { PolicyControl } from "./policy-control";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { formatDateTime } from "@/domain/format";

import { useUpdateAction } from "./use-update-action";

import { AUTO_UPDATE_ANCHOR } from "@/domain/job-links";
import { useState } from "react";

import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { UpdatePoint } from "@/domain/stack-updates";
import { useImageError } from "@/features/images/use-image-error";
import { Button } from "@/shared/ui/button";

import { StackUpdateDialog } from "./stack-update-dialog";
import { RestorePointState } from "./restore-point-state";
import { DeletePointDialog } from "./delete-point-dialog";
import { UpdateUnavailable } from "./live-state";

export function LiveStackUpdates({ stackId }: { stackId: string }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const read = useCapability("getAutoUpdatePolicy", { kind: "stack", id: stackId }), check = useCapability("checkUpdates", { kind: "stack", id: stackId });
  const policy = useQuery({ queryKey: ["stack", stackId, "update-policy"], queryFn: () => adapter.getAutoUpdatePolicy!(stackId), enabled: read.available && !!adapter.getAutoUpdatePolicy });
  const action = useUpdateAction(), timeZone = useHubTimeZone();
  const baselineRead = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  const baseline = useQuery({ queryKey: ["stack", stackId, "update-baseline"], queryFn: () => adapter.getUpdateBaseline!(stackId), enabled: baselineRead.available && !!adapter.getUpdateBaseline });
  const p = policy.data;
  return <section id={AUTO_UPDATE_ANCHOR} className="flex flex-col gap-3 rounded-xl border p-4"><h3>{t("policy")}</h3>
    {!baselineRead.available ? <UpdateUnavailable reason={baselineRead.reason} /> : baseline.isError ? <p role="alert">{errorText(baseline.error)}</p> : baseline.data && !baseline.data.available && <p role="status">{errorText(baseline.data.code ?? "update.baseline-unavailable")}</p>}
    {!read.available ? <UpdateUnavailable reason={read.reason} /> : policy.isError ? <p role="alert">{errorText(policy.error)}</p> : p ? <>
      <p>{t(p.mode === "check" ? "checkOnly" : "scheduled")}{p.stale && ` · ${t("stale")}`}</p>
      <p>{t("checked")}: {p.checkedAt ? formatDateTime(p.checkedAt) : t("unknown")} {p.code && errorText(p.code)}</p>
      <p>{t("applied")}: {p.applyCheckedAt ? formatDateTime(p.applyCheckedAt) : t("unknown")} {p.applyCode && errorText(p.applyCode)}</p>
      {p.bookkeepingCode && <p role="status">{errorText(p.bookkeepingCode)}</p>}
      {p.candidates.map((c) => <p key={`${c.service}:${c.digest}`}>{c.service}: {c.version || c.imageRef} · {t(`notification.${c.notificationState}`)}</p>)}
      <PolicyControl policy={p} timeZone={timeZone} saved={() => { void policy.refetch(); }} />
    </> : <p>{t("loading")}</p>}
    <Button disabled={!check.available || action.busy} title={check.reason} onClick={() => void action.run(() => adapter.checkStackUpdates!(stackId), () => { void policy.refetch(); })}>{t("check")}</Button>
    {action.error && <p role="alert">{action.error}</p>}
  </section>;
}
export function LiveRestorePoints({ stackId }: { stackId: string }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const read = useCapability("listRestorePoints", { kind: "stack", id: stackId });
  const query = useQuery({ queryKey: ["stack", stackId, "update-points"], queryFn: () => adapter.listRestorePoints!(stackId), enabled: read.available && !!adapter.listRestorePoints });
  const [selected, setSelected] = useState<{ stackId: string; point: UpdatePoint }>(), [deleting, setDeleting] = useState<{ stackId: string; point: UpdatePoint }>();
  const remove = useCapability("deleteRestorePoint", { kind: "stack", id: stackId });
  const refresh = async () => { const result = await query.refetch(); if (result.isError) throw result.error; return result.data!; };
  const restore = useCapability("restoreRestorePoint", { kind: "stack", id: stackId }), baselineRead = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  return <section className="flex flex-col gap-3"><p>{t("noData")}</p>{!read.available ? <UpdateUnavailable reason={read.reason} /> : query.isError ? <p role="alert">{errorText(query.error)}</p> : !query.data ? <p>{t("loading")}</p> : !query.data.length ? <p>{t("noPoints")}</p> : query.data.map((point) => <article className="rounded-lg border p-3" key={point.id}>
    <time dateTime={point.createdAt}>{formatDateTime(point.createdAt)}</time><RestorePointState point={point} /><p>{t(point.sourceState === "partial-source" ? "partial" : "snapshot")}</p>{point.images.map((i) => <p key={i.service}>{i.service}: {i.reference} · {t(i.present ? i.running ? "running" : "stopped" : "absent")}</p>)}
    {point.code && <p>{errorText(point.code)}</p>}<Button variant="outline" disabled={!read.available || !restore.available || !baselineRead.available || !point.available || point.orphaned || point.holdState !== "none"} title={restore.reason ?? baselineRead.reason} onClick={() => setSelected({ stackId, point })}>{t("restore")}</Button>
    <Button variant="outline" disabled={!read.available || !remove.available || point.holdState !== "none"} title={remove.reason ?? read.reason} onClick={() => setDeleting({ stackId, point })}>{t("deletePoint")}</Button>
  </article>)}{selected?.stackId === stackId && <StackUpdateDialog key={`${stackId}:${selected.point.id}`} stackId={stackId} point={selected.point} onClose={() => setSelected(undefined)} />}
    {deleting?.stackId === stackId && <DeletePointDialog key={`${stackId}:${deleting.point.id}:${deleting.point.targetIdentity}`} scope={{ kind: "stack", id: stackId }} point={deleting.point} readError={query.isError ? query.error : undefined} latest={query.data?.find((p) => p.id === deleting.point.id)} refresh={refresh} close={() => setDeleting(undefined)} />}</section>;
}
