// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live host schedule and count retention editor.

import { useState } from "react";
import { Link } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { backupErrorMessage } from "@/data/backup-error";
import { WEEKDAYS, isTime, type Weekday } from "@/domain/schedule";
import type { BackupPolicy, UsableBackupTarget, Volume } from "@/domain/volumes";
import { TimePicker } from "@/shared/components/date-time/time-picker";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";
import { useVolumeCapability } from "../use-volume-capability";

export function BackupPolicyCard({ volume, targets, defaultRetentionCount = 7, hostOnline }: { volume: Volume; targets: UsableBackupTarget[]; defaultRetentionCount?: number; hostOnline: boolean }) {
  const locale = useLocale();
  const t = useTranslations("volumes.backups"), ts = useTranslations("common.schedule"), all = useTranslations();
  const adapter = useAdapter();
  const capability = useVolumeCapability("setBackupPolicy", volume.id);
  const initial: BackupPolicy = volume.backup ?? { enabled: true, targetId: "local", schedule: { mode: "daily", time: "03:00" }, retentionCount: defaultRetentionCount, stopContainers: false };
  const [draft, setDraft] = useState(initial), [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  const patch = (value: Partial<BackupPolicy>) => setDraft((old) => ({ ...old, ...value }));
  const invalid = !Number.isInteger(draft.retentionCount) || draft.retentionCount < 0 || draft.retentionCount > 3650 || (draft.schedule.mode === "daily" || draft.schedule.mode === "weekly") && !isTime(draft.schedule.time);
  async function save(policy: BackupPolicy | null) {
    setBusy(true); setError(undefined);
    try { await adapter.setBackupPolicy(volume.id, policy); } catch (cause) { setError(backupErrorMessage(cause, all)); } finally { setBusy(false); }
  }
  const schedule = draft.schedule;
  const timeZone = volume.timeZone || "UTC";
  return <Card className="gap-3 py-4">
    <CardHeader><CardTitle>{t("policy")}</CardTitle></CardHeader>
    <CardContent className="flex flex-col gap-4">
      <p className="text-sm" data-testid="backup-next">{t("next")}: {volume.backup?.nextRunAt ? new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone }).format(new Date(volume.backup.nextRunAt)) : t("nextManual")} · {timeZone}</p>
      <Field orientation="horizontal"><Switch id="bk-enabled" checked={draft.enabled} onCheckedChange={(enabled) => patch({ enabled })} /><FieldLabel htmlFor="bk-enabled">{t("enabled")}</FieldLabel></Field>
      <Field><FieldLabel htmlFor="bk-target">{t("target")}</FieldLabel><Select value={draft.targetId} onValueChange={(targetId) => patch({ targetId })}><SelectTrigger id="bk-target" className="w-full"><SelectValue /></SelectTrigger><SelectContent>{targets.map((target) => <SelectItem key={target.id} value={target.id}>{target.name} · {t(`targetKinds.${target.kind}`)}</SelectItem>)}</SelectContent></Select><FieldDescription><Link to="/settings/backups" className="text-primary-text hover:underline">{t("manageTargets")}</Link></FieldDescription></Field>
      {schedule.mode === "custom" ? <div className="space-y-2"><p>{t("customSchedule")}: <code>{schedule.cron}</code></p><Button variant="outline" onClick={() => patch({ schedule: { mode: "daily", time: "03:00" } })}>{t("replaceSchedule")}</Button></div> : <>
        <SegmentedToggle label={ts("recurrence.label")} value={schedule.mode} onChange={(mode: "manual" | "daily" | "weekly") => patch({ schedule: mode === "manual" ? { mode } : mode === "daily" ? { mode, time: schedule.mode === "manual" ? "03:00" : schedule.time } : { mode, day: schedule.mode === "weekly" ? schedule.day : "sun", time: schedule.mode === "manual" ? "03:00" : schedule.time } })} options={(["manual", "daily", "weekly"] as const).map((mode) => ({ value: mode, label: mode === "manual" ? ts("manual") : ts(`recurrence.${mode}`) }))} wrap />
        {schedule.mode === "weekly" && <SegmentedToggle<Weekday> label={ts("weekday")} value={schedule.day} onChange={(day) => patch({ schedule: { ...schedule, day } })} options={WEEKDAYS.map((day) => ({ value: day, label: ts(`days.${day}`), ariaLabel: ts(`daysLong.${day}`) }))} wrap />}
        {schedule.mode !== "manual" && <Field><FieldLabel htmlFor="bk-time">{ts("time")}</FieldLabel><TimePicker id="bk-time" value={schedule.time} onChange={(time) => patch({ schedule: { ...schedule, time } })} label={ts("time")} prompt={ts("timePrompt")} /></Field>}
      </>}
      <Field><FieldLabel htmlFor="bk-retention">{t("retention")}</FieldLabel><Input id="bk-retention" type="number" min={0} max={3650} value={Number.isNaN(draft.retentionCount) ? "" : draft.retentionCount} onChange={(event) => patch({ retentionCount: event.target.value === "" ? NaN : Number(event.target.value) })} /><FieldDescription>{t("retentionHint")}</FieldDescription></Field>
      <Field orientation="horizontal"><Switch id="bk-stop" checked={draft.stopContainers} onCheckedChange={(stopContainers) => patch({ stopContainers })} /><FieldLabel htmlFor="bk-stop">{t("stop")}</FieldLabel></Field>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap gap-2"><Button disabled={busy || invalid || !hostOnline || !capability.available || !targets.some((target) => target.id === draft.targetId)} title={capability.reason} onClick={() => void save(draft)}>{t("save")}</Button>{volume.backup && <Button variant="outline" disabled={busy || !hostOnline || !capability.available} onClick={() => void save(null)}>{t("remove")}</Button>}</div>
    </CardContent>
  </Card>;
}
