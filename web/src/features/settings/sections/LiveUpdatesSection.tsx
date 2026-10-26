// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { formatDateTime } from "@/domain/format";

import { useUpdateAction } from "@/shared/components/updates/use-update-action";

import { UPDATE_CHECK_ANCHOR, UPDATE_POLICIES_ANCHOR } from "@/domain/job-links";
import { useEffect, useState } from "react";
import { identityLifetime } from "@/data/identity-lifetime";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { Link } from "react-router";
import { useHosts, useStacks } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { DeletePointDialog } from "@/shared/components/updates/delete-point-dialog";
import { RestorePointState } from "@/shared/components/updates/restore-point-state";
import type { UpdateSchedules, UpdatePoint } from "@/domain/stack-updates";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { UpdateUnavailable } from "@/shared/components/updates/live-state";
import { useImageError } from "@/features/images/use-image-error";

function ScheduleDialog({ hostId, value, close, saved }: { hostId: string; value: UpdateSchedules; close: () => void; saved: () => void }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), action = useUpdateAction(`${hostId}:${value.registrationId}:${value.revision}`);
  const [pollingInterval, setPolling] = useState(value.pollingInterval), [autoUpdateInterval, setApply] = useState(value.autoUpdateInterval), [pollingEnabled, setPollingEnabled] = useState(value.pollingEnabled), [autoUpdateEnabled, setApplyEnabled] = useState(value.autoUpdateEnabled);
  useEffect(() => identityLifetime(adapter).listen(close), [adapter, close]);
  const read = useCapability("listSchedule", { kind: "host", id: hostId }), write = useCapability("saveUpdateSchedule", { kind: "host", id: hostId });
  return <Dialog open onOpenChange={(v) => { if (!v) { action.reset(); close(); } }}><DialogContent><DialogHeader><DialogTitle>{t("schedules")}</DialogTitle><DialogDescription>{t("cron", { zone: value.timeZone })}</DialogDescription></DialogHeader>
    <label>{t("polling")}<Input value={pollingInterval} onChange={(e) => setPolling(e.target.value)} /></label><label className="flex gap-2"><Checkbox checked={pollingEnabled} onCheckedChange={(v) => setPollingEnabled(v === true)} />{t("enabled")}</label>
    <label>{t("applySchedule")}<Input value={autoUpdateInterval} onChange={(e) => setApply(e.target.value)} /></label><label className="flex gap-2"><Checkbox checked={autoUpdateEnabled} onCheckedChange={(v) => setApplyEnabled(v === true)} />{t("enabled")}</label>
    <p>{t("cron", { zone: value.timeZone })}</p>{action.error && <p role="alert">{action.error}</p>}
    {(!read.available || !write.available) && <UpdateUnavailable reason={read.reason ?? write.reason} />}
    <Button disabled={!read.available || !write.available || action.busy} onClick={() => void action.run(() => adapter.saveUpdateSchedule!(hostId, { expectedRevision: value.revision, pollingInterval, autoUpdateInterval, pollingEnabled, autoUpdateEnabled }), () => { saved(); close(); })}>{t("save")}</Button>
  </DialogContent></Dialog>;
}
export function HostUpdateSettings({ hostId, name }: { hostId: string; name: string }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError(), [editing, setEditing] = useState<UpdateSchedules>();
  const read = useCapability("listSchedule", { kind: "host", id: hostId }), hintRead = useCapability("listSystemComponents", { kind: "host", id: hostId });
  const query = useQuery({ queryKey: ["schedule", hostId], queryFn: () => adapter.getUpdateSchedules!(hostId), enabled: read.available && !!adapter.getUpdateSchedules });
  const hints = useQuery({ queryKey: ["schedule", hostId, "hints"], queryFn: () => adapter.getSystemImageHints!(hostId), enabled: hintRead.available && !!adapter.getSystemImageHints });
  const [deleting, setDeleting] = useState<{ hostId: string; point: UpdatePoint }>();
  const orphanDelete = useCapability("deleteHostRestorePoint", { kind: "host", id: hostId });
  const orphanRead = useCapability("listHostRestorePoints", { kind: "host", id: hostId });
  const orphans = useQuery({ queryKey: ["schedule", hostId, "orphan-points"], queryFn: () => adapter.listHostRestorePoints!(hostId), enabled: orphanRead.available && !!adapter.listHostRestorePoints });
  const refreshPoints = async () => { const result = await orphans.refetch(); if (result.isError) throw result.error; return result.data!; };
  const data = query.data;
  return <section className="flex flex-col gap-3 rounded-xl border p-4"><h3 className="font-semibold">{name}</h3>
    {!read.available ? <UpdateUnavailable reason={read.reason} /> : query.isError ? <p role="alert">{errorText(query.error)}</p> : data ? <>
      <p>{t("cron", { zone: data.timeZone })}</p>{data.jobs.map((job) => <div key={job.id}><p>{t(job.kind === "auto-update" ? "applySchedule" : "polling")}: {job.schedule} · {t(job.enabled ? "enabled" : "disabled")}</p><p>{t("next")}: {job.nextRun ? formatDateTime(job.nextRun) : t("unknown")}</p>{job.currentRun && <p>{t("currentRun", { id: job.currentRun.id, status: t(`jobStatus.${job.currentRun.status}`) })}</p>}{job.code && <p>{errorText(job.code)}</p>}</div>)}
      {(!data.masterPollingEnabled || !data.masterAutoUpdateEnabled) && <p>{t("masterOff")}</p>}
      <Button variant="outline" onClick={() => setEditing(data)}>{t("editSchedules")}</Button>
    </> : <p>{t("loading")}</p>}
    {!hintRead.available ? <UpdateUnavailable reason={hintRead.reason} /> : hints.isError ? <p role="alert">{errorText(hints.error)}</p> : hints.data && <div><h4>{t("hints")}</h4><p>{t("hubVersion", { version: hints.data.hubVersion })}</p>{hints.data.images.map((h) => <p key={h.kind}>{h.kind}: <code>{h.reference}</code> {h.compatibleReference && <code>{h.compatibleReference}</code>} {h.currentVersion ?? t("unknown")} · {t(`hint.${h.state}`)} {h.candidateVersion} {h.requiresHubUpdate && t("hubFirst")} {h.code && errorText(h.code)}</p>)}</div>}
    <h4>{t("orphans")}</h4>{!orphanRead.available ? <UpdateUnavailable reason={orphanRead.reason} /> : orphans.isError ? <p role="alert">{errorText(orphans.error)}</p> : orphans.data?.filter((p) => p.orphaned).map((p) => <article key={p.id} className="rounded-lg border p-3"><p className="break-all">{p.targetId} · {p.targetIdentity} · {formatDateTime(p.createdAt)}</p><RestorePointState point={p} />{p.code && <p>{errorText(p.code)}</p>}<Button variant="outline" disabled={!orphanRead.available || !orphanDelete.available || p.holdState !== "none"} onClick={() => setDeleting({ hostId, point: p })}>{t("deletePoint")}</Button></article>)}
    {deleting?.hostId === hostId && <DeletePointDialog key={`${hostId}:${deleting.point.id}:${deleting.point.targetIdentity}`} scope={{ kind: "host", id: hostId }} point={deleting.point} readError={orphans.isError ? orphans.error : undefined} latest={orphans.data?.find((p) => p.id === deleting.point.id)} refresh={refreshPoints} close={() => setDeleting(undefined)} />}
    {editing && data && editing.registrationId === data.registrationId && <ScheduleDialog key={`${hostId}:${editing.registrationId}:${editing.revision}`} hostId={hostId} value={editing} close={() => setEditing(undefined)} saved={() => { void query.refetch(); }} />}
  </section>;
}
export function LiveUpdatesSection() {
  const hosts = useHosts(), stacks = useStacks(), t = useTranslations("common.stackUpdates");
  return <div className="flex flex-col gap-4"><h2 id={UPDATE_CHECK_ANCHOR}>{t("schedules")}</h2>{hosts.data?.map((h) => <HostUpdateSettings key={h.id} hostId={h.id} name={h.name} />)}<h2 id={UPDATE_POLICIES_ANCHOR}>{t("policy")}</h2>{stacks.data?.map((s) => <Link key={s.id} to={`/stacks/${encodeURIComponent(s.id)}`}>{s.name}</Link>)}</div>;
}
