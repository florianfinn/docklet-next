import { ShieldAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { SCAN_MODES, SCAN_SCHEDULE_ANCHOR, validateScanSchedule, type ScanSchedule } from "@/domain/scan-schedule";
import type { HubSettings } from "@/domain/settings";
import { ScheduleEditor } from "@/shared/components/schedule/schedule-editor";
import { HelpLabel, HelpTooltip } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription } from "@/shared/ui/field";
import { Switch } from "@/shared/ui/switch";

import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";
import { NotificationsPointer } from "../notifications/NotificationsPointer";

/** Optional vulnerability scans with a typed schedule. */
export function SecuritySection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const ts = useTranslations("settings.security");
  const save = useSaveSettings();
  const { hash } = useLocation();
  const scheduleRef = useRef<HTMLDivElement>(null);
  const [now] = useState(() => new Date());
  const timeZone = settings.general.timeZone;
  const section = useSectionDraft({ security: settings.security });
  const d = section.draft;

  // The security page links here; bring the schedule into view and focus it once per link.
  useEffect(() => {
    if (hash !== `#${SCAN_SCHEDULE_ANCHOR}`) return;
    const el = scheduleRef.current;
    el?.scrollIntoView({ block: "center" });
    el?.focus({ preventScroll: true });
  }, [hash]);

  if (!d) return null;
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
        <NotificationsPointer />
      </section>
    </SettingsForm>
  );
}
