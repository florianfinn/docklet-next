// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Connected local login and proxy rejection screen.

import { useState } from "react";
import { Navigate, useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import { useAuthMode, useCurrentUser } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";

export default function ProductLoginPage() {
  const t = useTranslations("login");
  const mode = useAuthMode();
  const currentUser = useCurrentUser();
  const adapter = useAdapter();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (mode.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (mode.isError) return <main className="p-6"><p>{t("settingsError")}</p><Button onClick={() => void mode.refetch()}>{t("retry")}</Button></main>;
  if (mode.data === "none" || (currentUser.isSuccess && mode.data === "local")) return <Navigate to="/dashboard" replace />;
  if (mode.data === "proxy") return <main className="mx-auto max-w-md p-6" role="alert">{currentUser.isError ? t("proxyRejected") : t("loading")}</main>;
  return <main className="grid min-h-svh place-items-center p-4"><Card className="w-full max-w-md p-6">
    <h1 className="text-xl font-semibold">{t("title", { hub: "Docklet" })}</h1>
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
        void navigate("/dashboard", { replace: true });
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
  </Card></main>;
}
