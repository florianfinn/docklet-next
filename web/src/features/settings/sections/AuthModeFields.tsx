import { KeyRound, ShieldOff, Waypoints, Wand2 } from "lucide-react";
import { useTranslations } from "use-intl";

import type { AuthSettings } from "@/domain/settings";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { HelpLegend } from "@/shared/help/help-tooltip";
import { FieldGroup, FieldSet } from "@/shared/ui/field";

import { AUTHENTIK_EXAMPLE, useAuthErrors, type PasswordDraft } from "../field-rules";

interface AuthModeFieldsProps {
  value: AuthSettings;
  onChange: (next: AuthSettings) => void;
  password: PasswordDraft;
  onPassword: (p: PasswordDraft) => void;
  showErrors: boolean;
  idPrefix?: string;
}

/** Login mode cards with the fields of the chosen mode; used by settings and first-run setup. */
export function AuthModeFields({ value, onChange, password, onPassword, showErrors, idPrefix = "auth" }: AuthModeFieldsProps) {
  const t = useTranslations("settings.auth");
  const e = useAuthErrors(value, password);
  const err = (k: keyof typeof e) => (showErrors ? e[k] : undefined);
  return (
    <FieldGroup>
      <FieldSet>
        <HelpLegend topic="settingsAuth.mode">{t("mode")}</HelpLegend>
        <ChoiceCards
          label={t("mode")}
          value={value.mode}
          onChange={(mode) => onChange({ ...value, mode })}
          columns={3}
          options={[
            { value: "none", title: t("none"), description: t("noneDesc"), icon: <ShieldOff /> },
            { value: "local", title: t("local"), description: t("localDesc"), icon: <KeyRound /> },
            { value: "proxy", title: t("proxy"), description: t("proxyDesc"), icon: <Waypoints /> },
          ]}
        />
      </FieldSet>
      {value.mode === "none" && (
        <Alert className="border-warning/40 bg-warning/8">
          <ShieldOff className="text-warning" />
          <AlertDescription>{t("noneWarning")}</AlertDescription>
        </Alert>
      )}
      {value.mode === "local" && (
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField id={`${idPrefix}-user`} label={t("username")} value={value.local.username} onChange={(username) => onChange({ ...value, local: { ...value.local, username } })} error={err("username")} autoComplete="username" className="sm:col-span-2 sm:max-w-sm" />
          <TextField
            id={`${idPrefix}-pw`}
            type="password"
            label={t("password")}
            value={password.password}
            onChange={(p) => onPassword({ ...password, password: p })}
            description={value.local.passwordSet ? t("passwordSet") : t("passwordHint")}
            error={err("password")}
            autoComplete="new-password"
          />
          <TextField id={`${idPrefix}-pw2`} type="password" label={t("passwordConfirm")} value={password.confirm} onChange={(c) => onPassword({ ...password, confirm: c })} error={err("confirm")} autoComplete="new-password" />
        </div>
      )}
      {value.mode === "proxy" && (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3">
            <Button type="button" variant="outline" size="sm" onClick={() => onChange({ ...value, proxy: { ...AUTHENTIK_EXAMPLE } })}>
              <Wand2 data-icon="inline-start" />
              {t("authentik")}
            </Button>
            <span className="text-xs text-muted-foreground">{t("authentikHint")}</span>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <TextField id={`${idPrefix}-hu`} label={t("userHeader")} help="settingsAuth.userHeader" value={value.proxy.userHeader} onChange={(userHeader) => onChange({ ...value, proxy: { ...value.proxy, userHeader } })} error={err("userHeader")} mono />
            <TextField id={`${idPrefix}-he`} label={t("emailHeader")} value={value.proxy.emailHeader} onChange={(emailHeader) => onChange({ ...value, proxy: { ...value.proxy, emailHeader } })} error={err("emailHeader")} mono />
            <TextField id={`${idPrefix}-hg`} label={t("groupsHeader")} value={value.proxy.groupsHeader} onChange={(groupsHeader) => onChange({ ...value, proxy: { ...value.proxy, groupsHeader } })} mono />
          </div>
          <TextField id={`${idPrefix}-tp`} label={t("trustedProxies")} help="settingsAuth.trustedProxies" value={value.proxy.trustedProxies} onChange={(trustedProxies) => onChange({ ...value, proxy: { ...value.proxy, trustedProxies } })} description={t("trustedProxiesHint")} mono />
          <TextField id={`${idPrefix}-lo`} label={t("logoutUrl")} value={value.proxy.logoutUrl} onChange={(logoutUrl) => onChange({ ...value, proxy: { ...value.proxy, logoutUrl } })} description={t("logoutUrlHint")} error={err("logoutUrl")} mono placeholder="https://auth.example.com/…" />
          <p className="text-xs text-muted-foreground">{t("proxyNote")}</p>
        </div>
      )}
    </FieldGroup>
  );
}
