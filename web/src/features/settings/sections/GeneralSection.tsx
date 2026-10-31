import { useCapability } from "@/data/use-capability";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { useAdapter } from "@/data/use-adapter";
import { notificationBaseUrl, type GeneralSettings, type HubSettings, type LinkBase } from "@/domain/settings";
import { TextField } from "@/shared/components/form-field";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet } from "@/shared/ui/field";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { HelpLabel, HelpLegend } from "@/shared/help/help-tooltip";
import { Label } from "@/shared/ui/label";

import { useGeneralErrors } from "../field-rules";
import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

/** Hub address, link policy, and time zone shared by settings and first-run setup. */
export function GeneralFields({ value, onChange, showErrors, idPrefix = "gen", showLanguage = true }: { value: GeneralSettings; onChange: (p: Partial<GeneralSettings>) => void; showErrors: boolean; idPrefix?: string; showLanguage?: boolean }) {
  const t = useTranslations("settings.general");
  const tl = useTranslations("locale.switcher");
  const e = useGeneralErrors(value);
  const links = notificationBaseUrl(value);
  const hasDomain = !!value.domainUrl.trim() && !e.domainUrl;
  return (
    <FieldGroup>
      <TextField id={`${idPrefix}-name`} label={t("hubName")} help="settingsGeneral.hubName" value={value.hubName} onChange={(hubName) => onChange({ hubName })} description={t("hubNameHint")} error={showErrors ? e.hubName : undefined} />
      <TextField
        id={`${idPrefix}-address`}
        label={t("hubAddress")}
        help="settingsGeneral.hubAddress"
        value={value.hubAddress}
        onChange={(hubAddress) => onChange({ hubAddress })}
        description={t("hubAddressHint")}
        error={showErrors ? e.hubAddress : undefined}
        placeholder="192.0.2.10:8080"
        mono
      />
      <TextField
        id={`${idPrefix}-domain`}
        label={t("domainUrl")}
        help="settingsGeneral.domainUrl"
        value={value.domainUrl}
        onChange={(domainUrl) => onChange({ domainUrl })}
        description={t("domainUrlHint")}
        error={showErrors ? e.domainUrl : undefined}
        placeholder="https://docklet.example.com"
        mono
      />
      <FieldSet>
        <HelpLegend topic="settingsGeneral.linkBase">{t("linkBase")}</HelpLegend>
        <RadioGroup value={hasDomain ? value.linkBase : "internal"} onValueChange={(v) => onChange({ linkBase: v as LinkBase })} aria-label={t("linkBase")} className="gap-2">
          {(["auto", "internal", "domain"] as const).map((k) => (
            <div key={k} className="flex items-center gap-2">
              <RadioGroupItem id={`${idPrefix}-link-${k}`} value={k} disabled={!hasDomain && k !== "internal"} />
              <Label htmlFor={`${idPrefix}-link-${k}`} className="text-sm font-normal">
                {t(`linkBases.${k}`)}
              </Label>
            </div>
          ))}
        </RadioGroup>
        <FieldDescription>
          {t("linkPreview", { url: links.url || "–", source: links.source })} {!hasDomain && t("linkNoDomain")}
        </FieldDescription>
      </FieldSet>
      <Field>
        <HelpLabel htmlFor={`${idPrefix}-tz`} topic="settingsGeneral.timeZone">
          {t("timeZone")}
        </HelpLabel>
        <input id={`${idPrefix}-tz`} className="font-mono text-sm" readOnly value={value.timeZone} />
        <FieldDescription>{t("timeZoneHint")}</FieldDescription>
      </Field>
      {showLanguage && <Field>
        <FieldLabel>{t("language")}</FieldLabel>
        <LanguageSwitch className="self-start" />
        <FieldDescription>{tl("hint")}</FieldDescription>
      </Field>}
    </FieldGroup>
  );
}

export function GeneralSection({ settings }: { settings: Pick<HubSettings, "general"> }) {
  const stop = useTranslations("settings.stackStopMode");
  const saveCapability = useCapability("saveSettings");
  const connected = useAdapter().kind === "http";
  const t = useTranslations("settings");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.general);
  const errors = useGeneralErrors(draft ?? settings.general);
  if (!draft) return null;
  const valid = !errors.hubName && !errors.hubAddress && !errors.domainUrl;
  return (
    <SettingsForm
      title={t("sections.general")}
      dirty={dirty}
      valid={valid}
      onDiscard={reset}
      onSave={async () => {
        await save("general", draft);
        commit(draft);
      }}
    >
      <GeneralFields value={draft} onChange={patch} showErrors={dirty} showLanguage={!connected} />
      <FieldGroup><Field data-disabled={!saveCapability.available}>
        <FieldLabel htmlFor="stack-stop-mode">{stop("title")}</FieldLabel>
        <Select value={draft.stackStopMode ?? "down"} disabled={!saveCapability.available} onValueChange={(value) => patch({ stackStopMode: value as "down" | "stop" })}>
          <SelectTrigger id="stack-stop-mode" title={saveCapability.reason}><SelectValue /></SelectTrigger>
          <SelectContent><SelectGroup><SelectItem value="down">{stop("down")}</SelectItem><SelectItem value="stop">{stop("stop")}</SelectItem></SelectGroup></SelectContent>
        </Select>
        <FieldDescription>{stop("hint")}</FieldDescription>
      </Field></FieldGroup>
    </SettingsForm>
  );
}
