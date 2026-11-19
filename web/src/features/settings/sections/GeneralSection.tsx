import { Input } from "@/shared/ui/input";
import { Button } from "@/shared/ui/button";
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
/** `saved` is the stored zone state; without it (setup) no override is stored yet. */
export function GeneralFields({ value, saved, onChange, showErrors, idPrefix = "gen", showLanguage = true }: { value: GeneralSettings; saved?: Pick<GeneralSettings, "timeZone" | "configuredTimeZone" | "defaultTimeZone">; onChange: (p: Partial<GeneralSettings>) => void; showErrors: boolean; idPrefix?: string; showLanguage?: boolean }) {
  // The hub reports TZ beside an override; without that value the effective zone equals TZ
  // only while no override is saved. The draft never decides it.
  const savedZone = saved ?? { timeZone: value.timeZone };
  const processZone = savedZone.defaultTimeZone ?? (savedZone.configuredTimeZone ? undefined : savedZone.timeZone);
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
        {/* Shown as the backend resolves it: auto and domain both use the domain once one is set. */}
        <RadioGroup value={hasDomain && value.linkBase !== "internal" ? "domain" : "internal"} onValueChange={(v) => onChange({ linkBase: v as LinkBase })} aria-label={t("linkBase")} className="gap-2">
          {(["domain", "internal"] as const).map((k) => (
            <div key={k} className="flex flex-wrap items-center gap-x-2">
              <RadioGroupItem id={`${idPrefix}-link-${k}`} value={k} disabled={k === "domain" && !hasDomain} aria-describedby={k === "domain" && !hasDomain ? `${idPrefix}-link-domain-reason` : undefined} />
              <Label htmlFor={`${idPrefix}-link-${k}`} className="text-sm font-normal">
                {t(`linkBases.${k}`)}
              </Label>
              {k === "domain" && !hasDomain && <span id={`${idPrefix}-link-domain-reason`} className="text-xs text-muted-foreground">{t("linkDomainFirst")}</span>}
            </div>
          ))}
        </RadioGroup>
        <FieldDescription>
          {t("linkPreview", { url: links.url || "–", source: links.source })}
        </FieldDescription>
      </FieldSet>
      <Field>
        <HelpLabel htmlFor={`${idPrefix}-tz`} topic="settingsGeneral.timeZone">
          {t("timeZone")}
        </HelpLabel>
        <Input id={`${idPrefix}-tz`} className="font-mono" list={`${idPrefix}-zones`} value={value.configuredTimeZone ?? value.timeZone} placeholder={processZone ?? t("systemTZ")} onChange={(e) => onChange({ configuredTimeZone: e.target.value })} />
        <datalist id={`${idPrefix}-zones`}>{[...new Set(["UTC", value.timeZone, ...(value.configuredTimeZone ? [value.configuredTimeZone] : []), ...Intl.supportedValuesOf("timeZone")])].map((zone) => <option key={zone} value={zone} />)}</datalist>
        {/* Field stretches its direct children; the wrapper takes that width so the link keeps its text width. */}
        <div><Button type="button" variant="link" className="w-fit px-0" onClick={() => onChange({ configuredTimeZone: "" })}>{t("useTZ", { zone: processZone ?? t("systemTZ") })}</Button></div>
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
      <GeneralFields value={draft} saved={settings.general} onChange={patch} showErrors={dirty} showLanguage={!connected} />
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
