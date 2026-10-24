import { useQueryClient } from "@tanstack/react-query";
import { CalendarClock } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { queryKeys } from "@/data/query-keys";
import { useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { SecurityOverview } from "@/domain/model";
import { nextScan, SCAN_SCHEDULE_ANCHOR, type ScanSchedule } from "@/domain/scan-schedule";
import { DEFAULT_TIME_ZONE } from "@/domain/schedule";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { useCommandError } from "@/data/use-command-error";
import { Button } from "@/shared/ui/button";
import { Label } from "@/shared/ui/label";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

/**
 * "Scans aktiv": the switch flips at once (optimistic), stays disabled with a
 * spinner until the adapter answers, and restores the previous state with an
 * error toast when the save fails. No artificial delay.
 */
export function ScanToggle({ enabled }: { enabled: boolean }) {
  const t = useTranslations("security");
  const commandError = useCommandError();
  const adapter = useAdapter();
  const capability = useCapability("setScanEnabled");
  const client = useQueryClient();
  const [pending, setPending] = useState(false);

  const change = async (next: boolean) => {
    void client.cancelQueries({ queryKey: queryKeys.security });
    const previous = client.getQueryData<SecurityOverview>(queryKeys.security);
    client.setQueryData<SecurityOverview>(queryKeys.security, (s) => s && { ...s, enabled: next });
    setPending(true);
    try {
      await adapter.setScanEnabled(next);
      // Settings › Scans shows the same switch; keep its cached copy in step.
      // No success toast: the switch is the feedback, and a toast would cover the header controls.
      void client.invalidateQueries({ queryKey: queryKeys.settings });
    } catch (e) {
      client.setQueryData(queryKeys.security, previous);
      toast.error(t("toggle.failed"), { description: `${commandError(e)} · ${t("toggle.restored")}` });
    } finally {
      setPending(false);
    }
  };

  return (
    <HelpTarget topic="settingsScans.enabled" side="bottom">
      <div data-slot="scan-toggle" aria-busy={pending} className="flex h-9 items-center gap-2 rounded-md border bg-background px-3 shadow-xs dark:border-input dark:bg-input/30">
        <Switch id="scan-on" checked={enabled} disabled={pending || !capability.available} title={capability.reason} onCheckedChange={(v) => void change(v)} />
        <Label htmlFor="scan-on" className="text-sm whitespace-nowrap">
          {t("enabled")}
        </Label>
        {pending && <Spinner className="size-3.5 text-muted-foreground" aria-label={t("toggle.saving")} />}
      </div>
    </HelpTarget>
  );
}

/** Planned time of the scans from the saved settings; opens the schedule in Settings › Scans. */
export function ScanScheduleLink({ schedule, enabled }: { schedule: ScanSchedule; enabled: boolean }) {
  const t = useTranslations("security.planned");
  const tc = useTranslations("common.schedule");
  const text = useScheduleText();
  const settings = useSettings();
  const [now] = useState(() => new Date());
  const timeZone = settings.data?.general.timeZone ?? DEFAULT_TIME_ZONE;
  const raw = schedule.mode === "manual" ? t("manual") : schedule.mode === "daily" ? t("daily", { time: schedule.time }) : t("weekly", { day: tc(`daysPlural.${schedule.day}`), time: schedule.time });
  const summary = raw.charAt(0).toUpperCase() + raw.slice(1);
  const at = nextScan(schedule, enabled, now, timeZone);
  const next = at && text.next(schedule, now, timeZone);
  return (
    <Button asChild variant="secondary">
      <Link to={`/settings/security#${SCAN_SCHEDULE_ANCHOR}`} aria-label={t("label", { summary, next: next ? t("next", next) : t("noNext") })} title={next ? t("next", next) : undefined}>
        <CalendarClock data-icon="inline-start" />
        {summary}
      </Link>
    </Button>
  );
}
