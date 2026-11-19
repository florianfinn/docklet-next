import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Input } from "@/shared/ui/input";
import { Button } from "@/shared/ui/button";
import { useCapability } from "@/data/use-capability";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { useAdapter } from "@/data/use-adapter";
import { discoveryAppUrl } from "@/domain/hub-discovery";
import { hubAddress, ipFromLegacyUrl, normalizeDomainUrl } from "@/domain/validation";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";

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
interface GeneralFieldsProps {
  value: GeneralSettings;
  saved?: Pick<GeneralSettings, "timeZone" | "configuredTimeZone" | "defaultTimeZone">;
  onChange: (patch: Partial<GeneralSettings>) => void;
  showErrors: boolean;
  idPrefix?: string;
  showLanguage?: boolean;
  appUrl?: string;
  preserveAddress?: boolean;
  domainPending?: boolean;
}

export function GeneralFields({ value, saved, onChange, showErrors, idPrefix = "gen", showLanguage = true, appUrl, preserveAddress = false, domainPending = false }: GeneralFieldsProps) {
  // The hub reports TZ beside an override; without that value the effective zone equals TZ
  // only while no override is saved. The draft never decides it.
  const savedZone = saved ?? { timeZone: value.timeZone };
  const processZone = savedZone.defaultTimeZone ?? (savedZone.configuredTimeZone ? undefined : savedZone.timeZone);
  const t = useTranslations("settings.general");
  const tl = useTranslations("locale.switcher");
  const e = useGeneralErrors(value, { preserveAddress, appUrl });
  const [replacement, setReplacement] = useState<string | null>(null);
  const legacy = !!value.hubAddress.trim() && !!hubAddress(value.hubAddress);
  const strippedIp = ipFromLegacyUrl(value.hubAddress);
  const movedDomain = normalizeDomainUrl(value.hubAddress, appUrl);
  const move = (domainUrl: string) => onChange({ hubAddress: "", domainUrl });
  const links = notificationBaseUrl({ ...value, domainUrl: normalizeDomainUrl(value.domainUrl, appUrl) ?? value.domainUrl });
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
      {legacy && strippedIp && <div className="flex flex-col items-start gap-2 text-sm">
        <p>{t("legacyIpUrl")}</p>
        <Button type="button" variant="outline" onClick={() => onChange({ hubAddress: strippedIp })}>{t("stripScheme", { address: strippedIp })}</Button>
      </div>}
      {legacy && !strippedIp && <div className="flex flex-col items-start gap-2 text-sm">
        <p>{t("legacyAddress")}</p>
        <Button type="button" variant="outline" disabled={domainPending || movedDomain === undefined} onClick={() => {
          if (movedDomain === undefined) return;
          if (value.domainUrl.trim() && normalizeDomainUrl(value.domainUrl, appUrl) !== movedDomain) setReplacement(movedDomain);
          else move(movedDomain);
        }}>{t("moveToDomain")}</Button>
      </div>}
      <ConfirmDialog open={replacement !== null} onOpenChange={(open) => { if (!open) setReplacement(null); }}
        title={t("replaceDomainTitle")} description={t("replaceDomainBody", { domain: replacement ?? "" })}
        confirmLabel={t("replaceDomain")} onConfirm={() => { if (replacement !== null) move(replacement); setReplacement(null); }} />
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
  const adapter = useAdapter();
  const connected = adapter.kind === "http";
  const discovery = useQuery({ queryKey: ["hub-discovery"], queryFn: () => adapter.discoverHub(), staleTime: Infinity, retry: false });
  const appUrl = discoveryAppUrl(discovery.data);
  const [addressEdited, setAddressEdited] = useState(false);
  const addressRevision = useRef(0);
  const t = useTranslations("settings");
  const save = useSaveSettings();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(settings.general);
  const preserveAddress = !addressEdited && draft?.hubAddress === settings.general.hubAddress;
  const errors = useGeneralErrors(draft ?? settings.general, { preserveAddress, appUrl });
  if (!draft) return null;
  const valid = (!discovery.isPending || !draft.domainUrl.trim() || /^https?:\/\//i.test(draft.domainUrl.trim())) && !errors.hubName && !errors.hubAddress && !errors.domainUrl;
  return (
    <SettingsForm
      title={t("sections.general")}
      dirty={dirty}
      valid={valid}
      onDiscard={() => { reset(); addressRevision.current++; setAddressEdited(false); }}
      onSave={async () => {
        const normalized = { ...draft, hubAddress: preserveAddress ? draft.hubAddress : draft.hubAddress.trim(), domainUrl: normalizeDomainUrl(draft.domainUrl, appUrl)! };
        const submittedRevision = addressRevision.current;
        await save("general", normalized);
        commit(normalized, draft);
        if (addressRevision.current === submittedRevision) setAddressEdited(false);
      }}
    >
      <GeneralFields value={draft} saved={settings.general} onChange={(p) => { if (p.hubAddress !== undefined) { addressRevision.current++; setAddressEdited(true); } patch(p); }} showErrors={dirty} showLanguage={!connected} appUrl={appUrl} domainPending={discovery.isPending} preserveAddress={preserveAddress} />
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
