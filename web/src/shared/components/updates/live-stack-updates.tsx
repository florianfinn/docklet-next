// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { formatDateTime } from "@/domain/format";

import { useUpdateAction } from "./use-update-action";

import { AUTO_UPDATE_ANCHOR } from "@/domain/job-links";
import { useEffect, useState } from "react";
import { identityLifetime } from "@/data/identity-lifetime";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { PolicyMode, StackUpdatePolicy, UpdatePoint } from "@/domain/stack-updates";
import { useImageError } from "@/features/images/use-image-error";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { StackUpdateDialog } from "./stack-update-dialog";
import { RestorePointState } from "./restore-point-state";
import { DeletePointDialog } from "./delete-point-dialog";
import { UpdateUnavailable } from "./live-state";

function PolicyDialog({ stackId, policy, close, saved }: { stackId: string; policy: StackUpdatePolicy; close: () => void; saved: () => void }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), action = useUpdateAction(`${stackId}:${policy.targetIdentity}:${policy.revision}`);
  const [mode, setMode] = useState<PolicyMode>(policy.mode);
  useEffect(() => identityLifetime(adapter).listen(close), [adapter, close]);
  const read = useCapability("getAutoUpdatePolicy", { kind: "stack", id: stackId });
  const write = useCapability(mode === "apply" ? "setAutoUpdateApply" : "setAutoUpdate", { kind: "stack", id: stackId });
  return <Dialog open onOpenChange={(v) => { if (!v) { action.reset(); close(); } }}><DialogContent><DialogHeader><DialogTitle>{t("policy")}</DialogTitle><DialogDescription>{t("policyHelp")}</DialogDescription></DialogHeader>
    <Select value={mode} onValueChange={(v) => setMode(v as PolicyMode)}><SelectTrigger aria-label={t("policy")}><SelectValue /></SelectTrigger><SelectContent>{(["off", "notify", "apply"] as const).map((m) => <SelectItem value={m} key={m}>{t(m)}</SelectItem>)}</SelectContent></Select>
    {(!read.available || !write.available) && <UpdateUnavailable reason={read.reason ?? write.reason} />}{action.error && <p role="alert">{action.error}</p>}
    <Button disabled={!read.available || !write.available || action.busy} onClick={() => void action.run(() => adapter.saveAutoUpdatePolicy!(stackId, mode, policy.revision), () => { saved(); close(); })}>{t("save")}</Button>
  </DialogContent></Dialog>;
}
export function LiveStackUpdates({ stackId }: { stackId: string }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const read = useCapability("getAutoUpdatePolicy", { kind: "stack", id: stackId }), check = useCapability("checkUpdates", { kind: "stack", id: stackId });
  const policy = useQuery({ queryKey: ["stack", stackId, "update-policy"], queryFn: () => adapter.getAutoUpdatePolicy!(stackId), enabled: read.available && !!adapter.getAutoUpdatePolicy });
  const action = useUpdateAction(), [editing, setEditing] = useState<StackUpdatePolicy>();
  const p = policy.data;
  return <section id={AUTO_UPDATE_ANCHOR} className="flex flex-col gap-3 rounded-xl border p-4"><h3>{t("policy")}</h3>
    {!read.available ? <UpdateUnavailable reason={read.reason} /> : policy.isError ? <p role="alert">{errorText(policy.error)}</p> : p ? <>
      <p>{t(p.mode)}{p.stale && ` · ${t("stale")}`}</p>
      <p>{t("checked")}: {p.checkedAt ? formatDateTime(p.checkedAt) : t("unknown")} {p.code && errorText(p.code)}</p>
      <p>{t("applied")}: {p.applyCheckedAt ? formatDateTime(p.applyCheckedAt) : t("unknown")} {p.applyCode && errorText(p.applyCode)}</p>
      {p.bookkeepingCode && <p role="status">{errorText(p.bookkeepingCode)}</p>}
      {p.candidates.map((c) => <p key={`${c.service}:${c.digest}`}>{c.service}: {c.version || c.imageRef} · {t(`notification.${c.notificationState}`)}</p>)}
      <Button variant="outline" onClick={() => setEditing(p)}>{t("editPolicy")}</Button>
    </> : <p>{t("loading")}</p>}
    <Button disabled={!check.available || action.busy} title={check.reason} onClick={() => void action.run(() => adapter.checkStackUpdates!(stackId), () => { void policy.refetch(); })}>{t("check")}</Button>
    {action.error && <p role="alert">{action.error}</p>}
    {editing && p && editing.targetIdentity === p.targetIdentity && editing.registrationId === p.registrationId && <PolicyDialog key={`${stackId}:${editing.targetIdentity}:${editing.revision}`} stackId={stackId} policy={editing} close={() => setEditing(undefined)} saved={() => { void policy.refetch(); }} />}
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
