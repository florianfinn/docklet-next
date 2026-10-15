import { useTranslations } from "use-intl";

import { ActionPresentationField } from "@/features/activity/ActionPresentationSetting";
import type { HostDefaults, HubSettings } from "@/domain/settings";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { HelpLabel, HelpLegend } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldSet } from "@/shared/ui/field";
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

export function HostDefaultsSection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.hostDefaults);
  const errors = useHostDefaultErrors(draft ?? settings.hostDefaults);
  if (!draft) return null;
  return (
    <SettingsForm
      title={t("sections.hosts")}
      dirty={dirty}
      valid={!Object.values(errors).some(Boolean)}
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

export function ActivitySection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const ta = useTranslations("settings.activity");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.activity);
  if (!draft) return null;
  return (
    <SettingsForm
      title={t("sections.activity")}
      dirty={dirty}
      valid
      onDiscard={reset}
      onSave={async () => {
        await save("activity", draft);
        commit(draft);
      }}
    >
      <FieldGroup>
        <p className="text-xs text-muted-foreground">{ta("historyNote")}</p>
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
      </FieldGroup>
    </SettingsForm>
  );
}
