import { HISTORY_LIMITS, validateHistoryLimit } from "@/domain/activity-history";
import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { queryKeys } from "@/data/query-keys";
import { QueryError } from "@/shared/components/page";
import { Skeleton } from "@/shared/ui/skeleton";
import { useCapability } from "@/data/use-capability";
import { useTranslations } from "use-intl";

import { ActionPresentationField } from "@/features/activity/ActionPresentationSetting";
import { LOAD_ALERT_LIMITS, loadAlertValueValid, type HostDefaults, type HubSettings, type LoadAlertSettings } from "@/domain/settings";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { HelpLabel, HelpLegend } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldSet } from "@/shared/ui/field";
import { Switch } from "@/shared/ui/switch";

import { useHostDefaultErrors } from "../field-rules";
import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

export function HostDefaultsFields({ value, onChange, showErrors, idPrefix = "hd", compact }: { value: HostDefaults; onChange: (p: Partial<HostDefaults>) => void; showErrors: boolean; idPrefix?: string; compact?: boolean }) {
  const t = useTranslations("settings.hosts");
  const th = useTranslations("hosts.onboarding");
  const e = useHostDefaultErrors(value);
  return (
    <FieldGroup>
      <TextField id={`${idPrefix}-proj`} label={t("projectDir")} help="settingsHosts.projectDir" value={value.projectDir} onChange={(projectDir) => onChange({ projectDir })} description={t("projectDirHint")} error={showErrors ? e.projectDir : undefined} mono />
      {!compact && <TextField id={`${idPrefix}-inst`} label={t("installDir")} help="settingsHosts.installDir" value={value.installDir} onChange={(installDir) => onChange({ installDir })} description={t("installDirHint")} error={showErrors ? e.installDir : undefined} mono />}
      {!compact && (
        <FieldSet>
          <HelpLegend topic="settingsHosts.connection">{t("connection")}</HelpLegend>
          <ChoiceCards
            label={t("connection")}
            value={value.connection}
            onChange={(connection) => onChange({ connection })}
            options={[
              { value: "direct", title: t("direct"), description: th("directDesc") },
              { value: "edge", title: t("edge"), description: th("edgeDesc") },
            ]}
          />
        </FieldSet>
      )}
      <FieldSet className="gap-3">
        <HelpLegend topic="settingsHosts.ids" className="mb-1">
          {t("ids")}
        </HelpLegend>
        <p className="text-xs text-muted-foreground">{t("idsHint")}</p>
        <div className="grid max-w-xs grid-cols-2 gap-3">
          <TextField id={`${idPrefix}-puid`} label={t("puid")} value={value.puid} onChange={(puid) => onChange({ puid })} error={showErrors ? e.puid : undefined} mono placeholder="auto" />
          <TextField id={`${idPrefix}-pgid`} label={t("pgid")} value={value.pgid} onChange={(pgid) => onChange({ pgid })} error={showErrors ? e.pgid : undefined} mono placeholder="auto" />
        </div>
      </FieldSet>
      <Field orientation="horizontal">
        <Switch id={`${idPrefix}-sock`} checked={value.detectSocketGroup} onCheckedChange={(detectSocketGroup) => onChange({ detectSocketGroup })} />
        <FieldContent>
          <HelpLabel htmlFor={`${idPrefix}-sock`} topic="settingsHosts.socket">
            {t("socket")}
          </HelpLabel>
          <FieldDescription>{t("socketHint")}</FieldDescription>
        </FieldContent>
      </Field>
      <p className="text-xs text-muted-foreground">{t("noChown")}</p>
    </FieldGroup>
  );
}

export function HostDefaultsSection({ settings }: { settings: Pick<HubSettings, "hostDefaults"> }) {
  const t = useTranslations("settings");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.hostDefaults);
  const capability = useCapability("saveSettings");
  const errors = useHostDefaultErrors(draft ?? settings.hostDefaults);
  if (!draft) return null;
  return (
    <SettingsForm
      title={t("sections.hosts")}
      dirty={dirty}
      valid={capability.available && !Object.values(errors).some(Boolean)}
      note={capability.reason}
      onDiscard={reset}
      onSave={async () => {
        await save("hostDefaults", draft);
        commit(draft);
      }}
    >
      <HostDefaultsFields value={draft} onChange={patch} showErrors={dirty} />
    </SettingsForm>
  );
}

export function ActivitySection({ settings }: { settings: Pick<HubSettings, "activity"> }) {
  const t = useTranslations("settings");
  const ta = useTranslations("settings.activity");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.activity);
  if (!draft) return null;
  const load = draft.loadAlerts;
  const loadValid = loadAlertValueValid("percent", load.cpuPercent) && loadAlertValueValid("percent", load.memoryPercent) && loadAlertValueValid("minutes", load.minutes);
  return (
    <SettingsForm
      title={t("sections.activity")}
      dirty={dirty}
      valid={loadValid && !validateHistoryLimit("events", draft.maxEvents) && !validateHistoryLimit("actions", draft.maxFinishedActions)}
      onDiscard={reset}
      onSave={async () => {
        await save("activity", draft);
        commit(draft);
      }}
    >
      <FieldGroup>
        <p className="text-xs text-muted-foreground">{ta("historyNote")}</p>
        <div className="grid gap-4 sm:grid-cols-2">{(["events", "actions"] as const).map((kind) => {
          const key = kind === "events" ? "maxEvents" : "maxFinishedActions";
          const limits = HISTORY_LIMITS[kind];
          const error = validateHistoryLimit(kind, draft[key]);
          return <TextField key={kind} id={`act-${kind}`} type="number" label={ta(kind)} value={Number.isNaN(draft[key]) ? "" : String(draft[key])} onChange={(text) => patch({ [key]: text.trim() ? Number(text) : NaN })} description={ta(`${kind}Hint`, { ...limits, fallback: limits.default })} error={error ? ta(`errors.${error}`, limits) : undefined} />;
        })}</div>
        <Field orientation="horizontal">
          <Switch id="act-output" checked={draft.keepActionOutput} onCheckedChange={(keepActionOutput) => patch({ keepActionOutput })} />
          <FieldContent>
            <HelpLabel htmlFor="act-output" topic="settingsActivity.output">
              {ta("output")}
            </HelpLabel>
            <FieldDescription>{ta("outputHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <ActionPresentationField value={draft.actionPresentation} onChange={(actionPresentation) => patch({ actionPresentation })} />
        <LoadAlertFields value={load} onChange={(next) => patch({ loadAlerts: { ...load, ...next } })} />
      </FieldGroup>
    </SettingsForm>
  );
}

function LoadAlertFields({ value, onChange }: { value: LoadAlertSettings; onChange: (p: Partial<LoadAlertSettings>) => void }) {
  const ta = useTranslations("settings.activity");
  const error = (kind: keyof typeof LOAD_ALERT_LIMITS, n: number) => (loadAlertValueValid(kind, n) ? undefined : Number.isInteger(n) ? ta("errors.range", LOAD_ALERT_LIMITS[kind]) : ta("errors.integer"));
  const number = (text: string) => (text.trim() === "" ? Number.NaN : Number(text));
  return (
    <FieldSet data-load-alerts>
      <Field orientation="horizontal">
        <Switch id="act-load" checked={value.enabled} onCheckedChange={(enabled) => onChange({ enabled })} />
        <FieldContent>
          <FieldLabel htmlFor="act-load">{ta("load")}</FieldLabel>
          <FieldDescription>{ta("loadHint")}</FieldDescription>
        </FieldContent>
      </Field>
      <div className="grid gap-4 sm:grid-cols-3">
        <TextField id="act-load-cpu" type="number" label={ta("loadCpu")} value={Number.isNaN(value.cpuPercent) ? "" : String(value.cpuPercent)} onChange={(v) => onChange({ cpuPercent: number(v) })} error={error("percent", value.cpuPercent)} disabled={!value.enabled} />
        <TextField id="act-load-memory" type="number" label={ta("loadMemory")} value={Number.isNaN(value.memoryPercent) ? "" : String(value.memoryPercent)} onChange={(v) => onChange({ memoryPercent: number(v) })} error={error("percent", value.memoryPercent)} disabled={!value.enabled} />
        <TextField id="act-load-minutes" type="number" label={ta("loadMinutes")} value={Number.isNaN(value.minutes) ? "" : String(value.minutes)} onChange={(v) => onChange({ minutes: number(v) })} error={error("minutes", value.minutes)} disabled={!value.enabled} />
      </div>
    </FieldSet>
  );
}

/** This control does not parse unrelated general or activity settings. */
export function ConnectedHostDefaultsSection() {
  const adapter = useAdapter();
  const settings = useQuery({ queryKey: [...queryKeys.connectedSettings, "hostDefaults"], queryFn: () => adapter.getConnectedSettings("hostDefaults") });
  if (settings.isError) return <QueryError error={settings.error} onRetry={() => void settings.refetch()} />;
  if (!settings.data) return <Skeleton className="h-80 rounded-xl" />;
  return <HostDefaultsSection settings={settings.data} />;
}
