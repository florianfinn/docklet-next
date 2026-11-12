// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original approved connected update settings and policy table. M24 S6: observed recovery and update UI follow-ups.

import { useQueries, useQuery } from "@tanstack/react-query";
import { useLocale, useTranslations } from "use-intl";
import { startPresented } from "@/features/activity/run-presentation";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { Info } from "lucide-react";
import { Box, Layers } from "lucide-react";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useHosts } from "@/data/queries";
import type { Host } from "@/domain/model";
import type { UpdateSettings } from "@/domain/settings";
import { UPDATE_CHECK_ANCHOR, UPDATE_POLICIES_ANCHOR } from "@/domain/job-links";
import { PolicyControl } from "@/shared/components/updates/policy-control";
import { useUpdateAction } from "@/shared/components/updates/use-update-action";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewTable, OverviewRowView } from "@/shared/components/overview/overview-table";
import { QueryError } from "@/shared/components/page";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Switch } from "@/shared/ui/switch";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Field, FieldLabel, FieldDescription, FieldGroup } from "@/shared/ui/field";
import { SettingsForm } from "../SettingsForm";
import { useSectionDraft } from "../use-section-draft";
import { NotificationsPointer } from "../notifications/NotificationsPointer";
import { HostUpdateSettings } from "./LiveUpdatesSection";

function PolicyHelp({ label, text }: { label: string; text: string }) {
  return <span className="inline-flex items-center gap-1">{label}<Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon-sm" aria-label={label}><Info aria-hidden className="size-3" /></Button></TooltipTrigger><TooltipContent className="max-w-80">{text}</TooltipContent></Tooltip></span>;
}
function PoliciesTable({ hosts, timeZone }: { hosts: Host[]; timeZone: string }) {
  const adapter = useAdapter(), t = useTranslations("settings.updates"), common = useTranslations("common.stackUpdates");
  const queries = useQueries({ queries: hosts.map((host) => ({ queryKey: ["stack", "update-policies", host.id], queryFn: async () => {
    const cap = await adapter.getCapability("listUpdatePolicies", { kind: "host", id: host.id });
    if (!cap.available) throw new Error(cap.disabledReasonCode);
    return adapter.listUpdatePolicies!(host.id);
  } })) });
  return <>
    {queries.map((q, i) => q.isError ? <div key={hosts[i]!.id}><HostChip host={hosts[i]!} /><QueryError error={q.error} onRetry={() => void q.refetch()} /></div> : null)}
    <OverviewTable fitKey="update-policies" columns={() => [
      { id: "name", label: t("target"), className: "min-w-40" },
      { id: "host", label: t("host") },
      { id: "mode", label: t("mode"), wide: true, head: <PolicyHelp label={t("mode")} text={common("updateWithCheckHelp")} /> },
      { id: "window", label: t("window"), head: <PolicyHelp label={t("window")} text={t("windowHelp")} /> },
    ]} selecting={false} actionsLabel="" listLabel={t("table")} dense>
      {(view, columns) => queries.flatMap((q, i) => (q.data ?? []).map((p) => <PolicyControl key={`${hosts[i]!.id}:${p.targetKind}:${p.targetId}`} policy={p} timeZone={timeZone} saved={() => { void q.refetch(); }} render={(mode, window) => <OverviewRowView view={view} columns={columns} pad="py-3" hostColor={hosts[i]!.color} actions={null} cells={{
        name: <span className="inline-flex min-w-0 items-center gap-2 break-all font-medium">{p.targetKind === "stack" ? <Layers className="size-4 shrink-0" /> : <Box className="size-4 shrink-0" />}{p.targetName}</span>, host: <HostChip host={hosts[i]!} />, mode, window,
      }} />} />))}
    </OverviewTable>
  </>;
}
function UpdateForm({ value, refresh }: { value: UpdateSettings & { timeZone: string; nextRun?: string; projectionUnavailable?: boolean }; refresh: () => Promise<unknown> }) {
  const adapter = useAdapter(), t = useTranslations("settings.updates"), common = useTranslations("common.stackUpdates"), locale = useLocale();
  const draft = useSectionDraft<UpdateSettings>({ interval: value.interval, notify: value.notify }), d = draft.draft!;
  // Interval and notify are settings writes; only "Check all now" needs the check job grant.
  const cap = useCapability("saveSettings"), check = useCapability("checkAllUpdates"), action = useUpdateAction(), hosts = useHosts();
  return <SettingsForm title={t("title")} dirty={draft.dirty} valid={cap.available} onDiscard={draft.reset} onSave={async () => { await adapter.saveSettings("updates", d); draft.commit(d); await refresh(); }}>
    <FieldGroup><div id={UPDATE_CHECK_ANCHOR} tabIndex={-1} className="scroll-mt-24 rounded-xl border bg-muted/20 p-4">
      <Field><FieldLabel htmlFor="update-interval">{t("interval")}</FieldLabel><Select value={d.interval} disabled={!cap.available} onValueChange={(interval) => draft.patch({ interval: interval as UpdateSettings["interval"] })}><SelectTrigger id="update-interval" className="w-auto"><SelectValue /></SelectTrigger><SelectContent><SelectGroup>{(["6h", "12h", "24h"] as const).map((v) => <SelectItem value={v} key={v}>{t(`intervals.${v}`)}</SelectItem>)}</SelectGroup></SelectContent></Select><FieldDescription>{t("defaultMode")}</FieldDescription></Field>
      <p className="my-3 text-xs">{common("next")}: {value.projectionUnavailable ? common("unavailable") : value.nextRun ? new Intl.DateTimeFormat(locale, { timeZone: value.timeZone, dateStyle: "medium", timeStyle: "short" }).format(new Date(value.nextRun)) : common("unknown")} · {value.timeZone}</p>
      <Button type="button" variant="outline" disabled={!check.available || action.busy} title={check.reason} onClick={() => void action.run(() => startPresented({ stackId: "", kinds: new Set(["check-updates"]) }, "toast", () => adapter.checkAllUpdates!()), () => { void refresh(); })}>{t("checkNow")}</Button>
      {action.error && <p role="alert">{action.error}</p>}
    </div>
    <Alert><AlertDescription>{t("noRollback")}</AlertDescription></Alert>
    <Field orientation="horizontal"><Switch id="update-notify" checked={d.notify} disabled={!cap.available} onCheckedChange={(notify) => draft.patch({ notify })} /><div><FieldLabel htmlFor="update-notify">{t("notify")}</FieldLabel><FieldDescription>{t("notifyHint")}</FieldDescription></div></Field>
    <section id={UPDATE_POLICIES_ANCHOR} tabIndex={-1} className="flex min-w-0 scroll-mt-24 flex-col gap-4"><h3 className="text-sm font-semibold">{t("table")}</h3>{hosts.isError && <QueryError error={hosts.error} onRetry={() => void hosts.refetch()} />}{hosts.data && <PoliciesTable hosts={hosts.data} timeZone={value.timeZone} />}<p className="text-xs text-muted-foreground">{t("excluded")}</p></section>
    <NotificationsPointer />
    {hosts.data?.map((h) => <HostUpdateSettings key={h.id} hostId={h.id} name={h.name} />)}
    </FieldGroup>
  </SettingsForm>;
}
export function ConnectedUpdatesSection() {
  const adapter = useAdapter(), cap = useCapability("saveSettings"), t = useTranslations("common.stackUpdates");
  const query = useQuery({ queryKey: ["settings", "updates"], queryFn: () => adapter.getUpdateSettings!(), enabled: cap.available && !!adapter.getUpdateSettings });
  if (!cap.available) return <p role="status">{cap.reason}</p>;
  if (query.isError) return <QueryError error={query.error} onRetry={() => void query.refetch()} />;
  if (!query.data) return <p aria-busy="true">{t("loading")}</p>;
  return <UpdateForm value={query.data} refresh={query.refetch} />;
}
