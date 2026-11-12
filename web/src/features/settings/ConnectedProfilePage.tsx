// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original connected administrator language and password profile.

import { PersonalApiKeys } from "./PersonalApiKeys";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { setAppLocale, useAppLocale } from "@/app/i18n/locale-store";
import { LogoutDialog } from "@/app/shell/LogoutDialog";
import { queryKeys } from "@/data/query-keys";
import { useAuthMode, useCurrentUser } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { isLocale, type AppLocale } from "@/domain/locale";
import { TextField } from "@/shared/components/form-field";
import { PageHeader, QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

export function ConnectedProfilePage() {
  const t = useTranslations("settings.profile");
  const tu = useTranslations("shell.user");
  const tm = useTranslations("settings.auth");
  const adapter = useAdapter();
  const client = useQueryClient();
  const user = useCurrentUser();
  const isUser = user.data?.isAdmin === false;
  const mode = useAuthMode();
  const locale = useAppLocale();
  const [savingLocale, setSavingLocale] = useState(false);
  const [localeError, setLocaleError] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordBusy, setPasswordBusy] = useState(false);
  const [passwordError, setPasswordError] = useState(false);
  const [passwordSaved, setPasswordSaved] = useState(false);
  const [logout, setLogout] = useState(false);

  const changeLocale = (next: AppLocale) => {
    if (savingLocale || next === locale) return;
    const previous = locale;
    setAppLocale(next);
    setSavingLocale(true);
    setLocaleError(false);
    void adapter.updateMyLocale(next).then((updated) => {
      client.setQueryData(queryKeys.currentUser, updated);
    }).catch(() => {
      setAppLocale(isLocale(user.data?.locale) ? user.data.locale : previous);
      setLocaleError(true);
    }).finally(() => setSavingLocale(false));
  };

  const changePassword = () => {
    if (passwordBusy || !currentPassword || newPassword.length < 12 || newPassword !== confirmPassword) {
      setPasswordError(true);
      return;
    }
    setPasswordBusy(true);
    setPasswordError(false);
    setPasswordSaved(false);
    void adapter.changePassword(currentPassword, newPassword).then(() => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setPasswordSaved(true);
    }).catch(() => setPasswordError(true)).finally(() => setPasswordBusy(false));
  };

  return <>
    <PageHeader title={t("title")} description={t("description")} />
    {user.isError || mode.isError ? <QueryError error={user.error ?? mode.error} onRetry={() => { void user.refetch(); void mode.refetch(); }} /> : !user.data || !mode.data ? <Skeleton className="h-80 rounded-2xl" /> : <div className="grid max-w-6xl gap-4 lg:grid-cols-2">
      <Card className="gap-4 py-5">
        <CardHeader className="px-5"><CardTitle>{t("account")}</CardTitle></CardHeader>
        <CardContent className="flex flex-col gap-4 px-5">
          <p className="font-semibold">{user.data.username}</p>
          <p className="text-sm text-muted-foreground">{tu(user.data.isAdmin === false ? "userRole" : "role")}</p>
          <p className="text-sm text-muted-foreground">{isUser && mode.data === "local" ? t("localSignIn") : tm(mode.data)}</p>
          <div>
            <h2 className="mb-2 text-sm font-medium">{t("language")}</h2>
            <fieldset disabled={savingLocale}><LanguageSwitch onChange={changeLocale} /></fieldset>
            <p className="mt-2 text-xs text-muted-foreground">{t(isUser ? "languageHintUser" : "languageHint")}</p>
            {savingLocale && <p role="status">{t("languageSaving")}</p>}
            {localeError && <p role="alert">{t("languageError")}</p>}
          </div>
          <div className="flex flex-wrap gap-2 border-t pt-3">
            {user.data.isAdmin !== false && <Button asChild variant="outline" size="sm"><Link to="/settings/appearance">{t("appearanceLink")}</Link></Button>}
            {mode.data !== "none" && <Button variant="ghost" size="sm" onClick={() => setLogout(true)}>{t("logout")}</Button>}
          </div>
        </CardContent>
      </Card>
      {mode.data === "local" && <Card className="gap-4 py-5">
        <CardHeader className="px-5"><CardTitle>{t("password")}</CardTitle></CardHeader>
        <CardContent className="px-5">
          <form className="flex flex-col gap-4" onSubmit={(event) => { event.preventDefault(); changePassword(); }}>
            <TextField id="pf-current" type="password" label={t("currentPassword")} value={currentPassword} onChange={setCurrentPassword} autoComplete="current-password" />
            <TextField id="pf-new" type="password" label={tm("password")} value={newPassword} onChange={setNewPassword} autoComplete="new-password" />
            <TextField id="pf-confirm" type="password" label={tm("passwordConfirm")} value={confirmPassword} onChange={setConfirmPassword} autoComplete="new-password" />
            <p className="text-xs text-muted-foreground">{tm("passwordHint")}</p>
            {passwordError && <p role="alert">{t("passwordError")}</p>}
            {passwordSaved && <p role="status">{t("passwordChanged")}</p>}
            <Button type="submit" className="self-start" disabled={passwordBusy || !currentPassword || !newPassword}>{passwordBusy ? t("passwordSaving") : t("password")}</Button>
          </form>
        </CardContent>
      </Card>}
      <PersonalApiKeys key={user.data.id} mode={mode.data} />
    </div>}
    <LogoutDialog open={logout} onOpenChange={setLogout} />
  </>;
}
