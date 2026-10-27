// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useLocation } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import type { ScanSettings } from "@/domain/security";
import { SCAN_SCHEDULE_ANCHOR } from "@/domain/scan-schedule";
import { WEEKDAYS } from "@/domain/schedule";
import { QueryError } from "@/shared/components/page";
import { TimePicker } from "@/shared/components/date-time/time-picker";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Field, FieldContent, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Label } from "@/shared/ui/label";
import { Checkbox } from "@/shared/ui/checkbox";
import { Switch } from "@/shared/ui/switch";
import { SettingsForm } from "../SettingsForm";
import { useSectionDraft } from "../use-section-draft";

function ScanForm({ value, refresh }: { value: ScanSettings; refresh: () => Promise<unknown> }) {
  const adapter = useAdapter(), t = useTranslations("settings.liveScans"), tc = useTranslations("common.schedule");
  const read = useCapability("getScanSettings"), write = useCapability("saveScanSettings");
  const locale = useLocale();
  const section = useSectionDraft(value), d = section.draft!;
  const ref = useRef<HTMLFieldSetElement>(null), { hash } = useLocation();
  useEffect(() => { if (hash === `#${SCAN_SCHEDULE_ANCHOR}`) { ref.current?.scrollIntoView({ block: "center" }); ref.current?.focus(); } }, [hash]);
  return <SettingsForm title={t("title")} dirty={section.dirty} valid={read.available && write.available} onDiscard={section.reset} onSave={async () => {
    const owner = identityLifetime(adapter).version();
    await adapter.saveScanSettings!(d);
    if (owner !== identityLifetime(adapter).version()) return;
    section.commit(d); await refresh();
  }}>
    {(!read.available || !write.available) && <p role="status">{read.reason ?? write.reason}</p>}
    <FieldGroup>
      <Field orientation="horizontal"><Switch id="scan-feature" checked={d.enabled} disabled={!write.available} onCheckedChange={(enabled) => section.patch({ enabled })} /><FieldContent><FieldLabel htmlFor="scan-feature">{t("enabled")}</FieldLabel></FieldContent></Field>
      <Field orientation="horizontal"><Switch id="scan-automatic" checked={d.automatic} disabled={!write.available} onCheckedChange={(automatic) => section.patch({ automatic })} /><FieldContent><FieldLabel htmlFor="scan-automatic">{t("automatic")}</FieldLabel></FieldContent></Field>
      <FieldSet ref={ref} id={SCAN_SCHEDULE_ANCHOR} tabIndex={-1} className="min-w-0 scroll-mt-24 rounded-xl border p-4" disabled={!write.available}>
        <FieldLegend>{t("schedule")}</FieldLegend>
        <SegmentedToggle value={d.schedule.mode} label={tc("recurrence.label")} options={(["daily", "weekly"] as const).map((mode) => ({ value: mode, label: tc(`recurrence.${mode}`) }))} onChange={(mode) => section.patch({ schedule: mode === "weekly" ? { mode, time: d.schedule.mode === "manual" ? "03:00" : d.schedule.time, day: "sun" } : { mode: "daily", time: d.schedule.mode === "manual" ? "03:00" : d.schedule.time } })} fit="content" wrap />
        {d.schedule.mode === "weekly" && <SegmentedToggle value={d.schedule.day} label={tc("weekday")} options={WEEKDAYS.map((day) => ({ value: day, label: tc(`days.${day}`), ariaLabel: tc(`daysLong.${day}`) }))} onChange={(day) => { if (d.schedule.mode === "weekly") section.patch({ schedule: { ...d.schedule, day } }); }} fit="content" wrap />}
        {d.schedule.mode !== "manual" && <Field><FieldLabel htmlFor="scan-time">{t("time")}</FieldLabel><TimePicker prompt={t("time")} id="scan-time" label={t("time")} value={d.schedule.time} onChange={(time) => { if (d.schedule.mode !== "manual") section.patch({ schedule: { ...d.schedule, time } }); }} /></Field>}
        {d.schedule.mode === "manual" && <p>{t("unscheduled")}</p>}
        <p className="text-xs text-muted-foreground">{t("zone", { zone: value.timeZone })}</p>
        <p className="text-sm">{value.projectionUnavailable ? t("nextUnavailable") : value.nextRun ? t("next", { at: new Intl.DateTimeFormat(locale, { timeZone: value.timeZone, dateStyle: "medium", timeStyle: "short" }).format(new Date(value.nextRun)) }) : t("noNext")}</p>
        {section.dirty && <p className="text-xs text-muted-foreground">{t("savedProjection")}</p>}
      </FieldSet>
      <Field orientation="horizontal"><Checkbox id="scan-unfixed" checked={d.ignoreUnfixed} disabled={!write.available} onCheckedChange={(v) => section.patch({ ignoreUnfixed: v === true })} /><Label htmlFor="scan-unfixed">{t("ignoreUnfixed")}</Label></Field>
      <FieldSet disabled={!write.available}><FieldLegend>{t("notifications")}</FieldLegend>
        {(["notifyCritical", "notifyHigh", "notifyFailed"] as const).map((key) => <Field key={key} orientation="horizontal"><Checkbox id={`scan-${key}`} checked={d[key]} onCheckedChange={(v) => section.patch({ [key]: v === true })} /><Label htmlFor={`scan-${key}`}>{t(key)}</Label></Field>)}
        <p className="text-xs text-muted-foreground">{t("notificationHint")}</p>
      </FieldSet>
    </FieldGroup>
  </SettingsForm>;
}
export function SecurityLiveSection() {
  const adapter = useAdapter(), read = useCapability("getScanSettings"), t = useTranslations("settings.liveScans");
  const lifetime = identityLifetime(adapter);
  const query = useQuery({ queryKey: ["settings", "scans"], queryFn: () => adapter.getScanSettings!(), enabled: read.available && !!adapter.getScanSettings });
  if (!read.available) return <p role="status">{read.reason}</p>;
  if (query.isError) return <QueryError error={query.error} onRetry={() => void query.refetch()} />;
  if (!query.data) return <p aria-busy="true">{t("loading")}</p>;
  return <ScanForm key={lifetime.version()} value={query.data} refresh={query.refetch} />;
}
