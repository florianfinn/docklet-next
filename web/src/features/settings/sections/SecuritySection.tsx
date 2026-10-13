import { HeartPulse, ShieldAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { useStacks } from "@/data/queries";
import { SCAN_MODES, SCAN_SCHEDULE_ANCHOR, validateScanSchedule, type ScanSchedule } from "@/domain/scan-schedule";
import type { HubSettings } from "@/domain/settings";
import { ScheduleEditor } from "@/shared/components/schedule/schedule-editor";
import { HelpLabel, HelpTooltip } from "@/shared/help/help-tooltip";
import { cn } from "@/shared/lib/utils";
import { Checkbox } from "@/shared/ui/checkbox";
import { Field, FieldContent, FieldDescription, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

/** Optional vulnerability scans with a typed schedule, and limited Auto Heal for unhealthy containers. */
export function SecuritySection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const ts = useTranslations("settings.security");
  const save = useSaveSettings();
  const stacks = useStacks();
  const { hash } = useLocation();
  const scheduleRef = useRef<HTMLDivElement>(null);
  const [now] = useState(() => new Date());
  const timeZone = settings.general.timeZone;
  const section = useSectionDraft({ security: settings.security, autoHeal: settings.autoHeal });
  const d = section.draft;

  // The security page links here; bring the schedule into view and focus it once per link.
  useEffect(() => {
    if (hash !== `#${SCAN_SCHEDULE_ANCHOR}`) return;
    const el = scheduleRef.current;
    el?.scrollIntoView({ block: "center" });
    el?.focus({ preventScroll: true });
  }, [hash]);

  if (!d) return null;
  const heal = d.autoHeal;
  const setHeal = (p: Partial<typeof heal>) => section.patch({ autoHeal: { ...heal, ...p } });
  const setSchedule = (schedule: ScanSchedule) => section.patch({ security: { ...d.security, schedule } });
  const scheduleError = d.security.scanEnabled ? validateScanSchedule(d.security.schedule, now, timeZone) : null;
  return (
    <SettingsForm
      title={t("sections.security")}
      dirty={section.dirty}
      valid={!scheduleError}
      onDiscard={section.reset}
      onSave={async () => {
        await save("security", d.security);
        await save("autoHeal", d.autoHeal);
        section.commit(d);
      }}
    >
      <section aria-labelledby="sec-scan" className="flex flex-col gap-4">
        <h3 id="sec-scan" className="flex items-center gap-2 text-sm font-semibold">
          <ShieldAlert className="size-4 text-orange" aria-hidden />
          {ts("scanTitle")}
        </h3>
        <Field orientation="horizontal">
          <Switch id="sec-enabled" checked={d.security.scanEnabled} onCheckedChange={(scanEnabled) => section.patch({ security: { ...d.security, scanEnabled } })} />
          <FieldContent>
            <HelpLabel htmlFor="sec-enabled" topic="settingsScans.enabled">
              {ts("scanEnabled")}
            </HelpLabel>
            <FieldDescription>{ts("scanHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <div
          ref={scheduleRef}
          id={SCAN_SCHEDULE_ANCHOR}
          tabIndex={-1}
          role="group"
          aria-labelledby="sec-schedule-label"
          className="flex scroll-mt-24 flex-col gap-3 rounded-xl border p-4 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
        >
          <div className="flex items-center gap-1">
            <span id="sec-schedule-label" className="text-sm font-medium">
              {ts("schedule")}
            </span>
            <HelpTooltip topic="settingsScans.schedule" />
          </div>
          <fieldset disabled={!d.security.scanEnabled} aria-labelledby="sec-schedule-label" className="min-w-0 disabled:opacity-60">
            <ScheduleEditor idPrefix="scan-schedule" value={d.security.schedule} onChange={(s) => s.mode !== "once" && setSchedule(s)} modes={SCAN_MODES} timeZone={timeZone} now={now} />
          </fieldset>
          <p className="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
            {!d.security.scanEnabled ? ts("scheduleOff") : d.security.schedule.mode === "manual" ? ts("scheduleManual") : ts("scheduleHint", { zone: timeZone })}
            <HelpTooltip topic="settingsGeneral.timeZone" />
            <Link to="/settings/general" className="text-primary-text underline-offset-4 hover:underline">
              {ts("scheduleZoneEdit")}
            </Link>
          </p>
        </div>
      </section>
      <section aria-labelledby="sec-heal" className="flex flex-col gap-4 border-t pt-5">
        <h3 id="sec-heal" className="flex items-center gap-2 text-sm font-semibold">
          <HeartPulse className="size-4 text-destructive" aria-hidden />
          {ts("healTitle")}
        </h3>
        <Field orientation="horizontal">
          <Switch id="heal-enabled" checked={heal.enabled} onCheckedChange={(enabled) => setHeal({ enabled })} />
          <FieldContent>
            <HelpLabel htmlFor="heal-enabled" topic="settingsHeal.enabled">
              {ts("healEnabled")}
            </HelpLabel>
            <FieldDescription>{ts("healHint")}</FieldDescription>
          </FieldContent>
        </Field>
        {/* Controls disable themselves; the info buttons stay usable while Auto Heal is off. */}
        <fieldset className={cn("flex flex-col gap-4", !heal.enabled && "opacity-60")}>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field>
              <div className="flex items-center justify-between">
                <HelpLabel htmlFor="heal-max" topic="settingsHeal.maxRestarts">
                  {ts("maxRestarts")}
                </HelpLabel>
                <span className="text-xs text-muted-foreground tabular">{ts("maxRestartsValue", { count: heal.maxRestarts })}</span>
              </div>
              <Slider id="heal-max" value={[heal.maxRestarts]} min={1} max={10} step={1} onValueChange={([v]) => setHeal({ maxRestarts: v ?? heal.maxRestarts })} aria-label={ts("maxRestarts")} disabled={!heal.enabled} />
            </Field>
            <Field>
              <HelpLabel htmlFor="heal-window" topic="settingsHeal.window">
                {ts("window")}
              </HelpLabel>
              <Select value={String(heal.windowMinutes)} onValueChange={(v) => setHeal({ windowMinutes: Number(v) })} disabled={!heal.enabled}>
                <SelectTrigger id="heal-window" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {(["30", "60", "120"] as const).map((w) => (
                      <SelectItem key={w} value={w}>
                        {ts(`windows.${w}`)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <div className="flex items-center justify-between">
                <HelpLabel htmlFor="heal-cool" topic="settingsHeal.cooldown">
                  {ts("cooldown")}
                </HelpLabel>
                <span className="text-xs text-muted-foreground tabular">{ts("cooldownValue", { count: heal.cooldownMinutes })}</span>
              </div>
              <Slider id="heal-cool" value={[heal.cooldownMinutes]} min={1} max={60} step={1} onValueChange={([v]) => setHeal({ cooldownMinutes: v ?? heal.cooldownMinutes })} aria-label={ts("cooldown")} disabled={!heal.enabled} />
            </Field>
          </div>
          <FieldSet className="gap-2">
            <FieldLegend variant="label" className="mb-1 flex items-center gap-1">
              {ts("excluded")}
              <HelpTooltip topic="settingsHeal.excluded" />
            </FieldLegend>
            <p className="text-xs text-muted-foreground">{ts("excludedHint")}</p>
            <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4">
              {(stacks.data ?? []).map((s) => (
                <Label key={s.id} className="flex items-center gap-2 rounded-lg border px-2 py-1.5 text-sm font-normal">
                  <Checkbox
                    checked={heal.excludedStacks.includes(s.id)}
                    onCheckedChange={(v) => setHeal({ excludedStacks: v === true ? [...heal.excludedStacks, s.id] : heal.excludedStacks.filter((x) => x !== s.id) })}
                    disabled={!heal.enabled}
                  />
                  {s.name}
                </Label>
              ))}
            </div>
          </FieldSet>
          <p className="text-xs text-muted-foreground">{ts("limitNote")}</p>
        </fieldset>
      </section>
    </SettingsForm>
  );
}
