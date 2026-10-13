import { Archive, CalendarClock, Play } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { validateRetention, type Retention } from "@/domain/retention";
import { validateSchedule } from "@/domain/schedule";
import type { BackupTarget } from "@/domain/settings";
import type { BackupPolicy, Volume } from "@/domain/volumes";
import { RetentionEditor } from "@/shared/components/retention-editor";
import { ScheduleEditor } from "@/shared/components/schedule/schedule-editor";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { Button } from "@/shared/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

import { useVolumeCommands } from "../use-volume-commands";

const PLAN_MODES = ["manual", "daily", "weekly", "once"] as const;

/** Per-volume backup plan with its own save/cancel; the run button uses the saved plan. */
export function BackupPolicyCard({ volume, targets, defaultRetention, hostOnline }: { volume: Volume; targets: BackupTarget[]; defaultRetention: Retention; hostOnline: boolean }) {
  const t = useTranslations("volumes.backups");
  const tc = useTranslations("common.actions");
  const ts = useTranslations("common.schedule");
  const cmd = useVolumeCommands();
  const timeZone = useHubTimeZone();
  const text = useScheduleText();
  const [now] = useState(() => new Date());
  const saved = volume.backup;
  const initial: BackupPolicy = saved ?? { enabled: true, targetId: targets[0]?.id ?? "", schedule: { mode: "daily", time: "03:00" }, retention: defaultRetention, stopContainers: volume.consumers.some((c) => c.running) };
  const [draft, setDraft] = useState<BackupPolicy>(initial);
  const [editing, setEditing] = useState(!!saved);
  const [busy, setBusy] = useState(false);
  const dirty = JSON.stringify(draft) !== JSON.stringify(initial) || !saved;
  const next = saved?.enabled ? text.next(saved.schedule, now, timeZone) : undefined;
  const patch = (p: Partial<BackupPolicy>) => setDraft((d) => ({ ...d, ...p }));
  const invalid = !!validateRetention(draft.retention) || (draft.schedule.mode !== "manual" && !!validateSchedule(draft.schedule, new Date(), timeZone));

  const save = async () => {
    setBusy(true);
    const ok = await cmd.setPolicy(volume.id, draft);
    setBusy(false);
    if (ok) toast.success(t("saved"));
  };

  return (
    <Card className="gap-4 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2">
          <CalendarClock className="size-4 text-primary-text" aria-hidden />
          {t("policy")}
        </CardTitle>
        <CardDescription>
          {saved ? `${text.summary(saved.schedule)} · ` : ""}
          {next ? ts("next", { when: next.when, relative: next.relative, zone: next.zone }) : saved ? `${t("next")}: ${t("nextManual")}` : t("noPolicy")}
        </CardDescription>
        <CardAction>
          <Button size="sm" disabled={!saved || !hostOnline || busy} onClick={() => void cmd.backup(volume.id)}>
            <Play data-icon="inline-start" />
            {t("runNow")}
          </Button>
        </CardAction>
      </CardHeader>
      {!editing ? (
        <CardContent className="px-4">
          <Button variant="outline" onClick={() => setEditing(true)} disabled={!targets.length}>
            <Archive data-icon="inline-start" />
            {t("createPolicy")}
          </Button>
        </CardContent>
      ) : (
        <CardContent className="flex flex-col gap-4 px-4">
          <Field orientation="horizontal">
            <Switch id="bk-enabled" checked={draft.enabled} onCheckedChange={(enabled) => patch({ enabled })} />
            <FieldLabel htmlFor="bk-enabled">{t("enabled")}</FieldLabel>
          </Field>
          <Field>
            <FieldLabel htmlFor="bk-target">{t("target")}</FieldLabel>
            <Select value={draft.targetId} onValueChange={(targetId) => patch({ targetId })}>
              <SelectTrigger id="bk-target" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {targets.map((tg) => (
                    <SelectItem key={tg.id} value={tg.id}>
                      {tg.name} · {t(`targetKinds.${tg.kind}`)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription>
              {t("targetHint")}{" "}
              <Link to="/settings/backups" className="text-primary-text hover:underline">
                {t("manageTargets")}
              </Link>
            </FieldDescription>
          </Field>
          <ScheduleEditor idPrefix="bk-schedule" value={draft.schedule} onChange={(schedule) => patch({ schedule })} modes={PLAN_MODES} timeZone={timeZone} now={now} />
          <RetentionEditor idPrefix="bk-retention" legend={t("retention")} value={draft.retention} onChange={(retention) => patch({ retention })} />
          <Field orientation="horizontal">
            <Switch id="bk-stop" checked={draft.stopContainers} onCheckedChange={(stopContainers) => patch({ stopContainers })} />
            <FieldContent>
              <FieldLabel htmlFor="bk-stop">{t("stop")}</FieldLabel>
              <FieldDescription>{draft.stopContainers ? t("stopHint") : t("consistency")}</FieldDescription>
            </FieldContent>
          </Field>
          <div className="flex flex-wrap gap-2 border-t pt-3">
            <Button onClick={() => void save()} disabled={!dirty || busy || !draft.targetId || invalid}>
              {busy && <Spinner data-icon="inline-start" />}
              {t("save")}
            </Button>
            {saved && dirty && (
              <Button variant="outline" onClick={() => setDraft(initial)}>
                {tc("cancel")}
              </Button>
            )}
            {saved && (
              <Button
                variant="ghost"
                className="ml-auto text-destructive hover:text-destructive"
                onClick={async () => {
                  if (await cmd.setPolicy(volume.id, null)) {
                    toast.success(t("removed"));
                    setEditing(false);
                  }
                }}
              >
                {t("remove")}
              </Button>
            )}
          </div>
        </CardContent>
      )}
    </Card>
  );
}
