// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Connected local login and proxy rejection screen; the sign-in form sits on the shared dot background; M29 pending setup opens setup.

import { useState } from "react";
import { Navigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { useAuthMode, useCurrentUser, useSignInConfiguration } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { useHubRetry } from "@/shared/components/hub-update/gate-retry";
import { GateScreen, UnreachableGate } from "@/shared/components/gate-screen";
import { AdapterOutcomeError } from "@/data/capability";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";

export default function ProductLoginPage() {
  const t = useTranslations("login");
  const mode = useAuthMode();
  const configuration = useSignInConfiguration();
  const currentUser = useCurrentUser();
  const retry = useHubRetry([mode, configuration, currentUser]);
  const adapter = useAdapter();
  const client = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (retry.unreachable) return <UnreachableGate {...retry} />;
  if (configuration.isError) return <GateScreen message={t("settingsError")} action={<Button onClick={() => void configuration.refetch()}>{t("retry")}</Button>} />;
  if (configuration.data?.setupPending) return <Navigate to="/setup" replace />;
  if (mode.isError) return <GateScreen message={t("settingsError")} action={<Button onClick={() => void mode.refetch()}>{t("retry")}</Button>} />;
  const rejected = currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401;
  if (currentUser.isError && !rejected) return <GateScreen message={t("sessionError")} action={<Button onClick={() => void currentUser.refetch()}>{t("retry")}</Button>} />;
  if (mode.isPending || configuration.isPending) return <GateScreen busy message={t("loading")} />;
  if (currentUser.data?.isAdmin === false || mode.data === "none" || (currentUser.isSuccess && !!currentUser.data && mode.data === "local")) return <Navigate to="/dashboard" replace />;
  if (mode.data === "proxy") return <GateScreen busy={currentUser.isPending} message={currentUser.isError || (currentUser.isSuccess && !currentUser.data) ? t("proxyRejected") : t("loading")} />;
  return <GateScreen slot="login-page" title={t("title", { hub: "Docklet" })}>
    <form className="flex flex-col gap-4" onSubmit={(event) => {
      event.preventDefault();
      if (!username.trim() || !password || busy) return;
      setBusy(true);
      setError("");
      void adapter.login(username.trim(), password).then(async (user) => {
        setPassword("");
        client.setQueryData(queryKeys.currentUser, user);
        await client.invalidateQueries({ queryKey: queryKeys.currentUser });
        await client.invalidateQueries({ queryKey: queryKeys.connectedSettings });
      }).catch((cause: unknown) => {
        const invalid = cause instanceof AdapterOutcomeError && cause.outcome.kind === "error" && cause.outcome.status === 401;
        setError(t(invalid ? "errors.invalid" : "errors.unavailable"));
      }).finally(() => setBusy(false));
    }}>
      <TextField id="login-user" label={t("username")} value={username} onChange={setUsername} autoComplete="username" />
      <TextField id="login-password" type="password" label={t("password")} value={password} onChange={setPassword} autoComplete="current-password" />
      {error && <p role="alert">{error}</p>}
      <Button type="submit" disabled={busy || !username.trim() || !password}>{busy ? t("submitting") : t("submit")}</Button>
    </form>
  </GateScreen>;
}
