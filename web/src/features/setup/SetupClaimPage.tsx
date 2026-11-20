// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run identity claim: sign-in mode and first administrator (M30: no setup code).

import { KeyRound, Lock, ShieldOff, Waypoints } from "lucide-react";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";

import type { components } from "@/api/generated/schema";
import { AdapterOutcomeError } from "@/data/capability";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { AuthMode } from "@/domain/settings";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Spinner } from "@/shared/ui/spinner";

import { SetupFrame } from "./SetupFrame";
import { patchSetup } from "./setup-draft";

type Configuration = components["schemas"]["AuthSignInConfiguration"];

const CLAIM_ERRORS = ["setup.not-pending", "auth.mode-forced", "auth.proxy-unverified", "auth.password-required", "user.username-taken"] as const;
type ClaimError = (typeof CLAIM_ERRORS)[number];

function claimError(cause: unknown): ClaimError | "invalid" | "unavailable" {
  if (cause instanceof AdapterOutcomeError && cause.outcome.kind === "error") {
    const code = cause.outcome.code;
    if ((CLAIM_ERRORS as readonly string[]).includes(code)) return code as ClaimError;
    if (cause.outcome.status === 400 || cause.outcome.status === 422) return "invalid";
  }
  return "unavailable";
}

/** Catalogue keys cannot contain dots: setup.not-pending → setupNotPending. */
function errorKey(code: string) {
  return code.replace(/[.-](\w)/g, (_, c: string) => c.toUpperCase()) as "setupNotPending";
}

/** First step of a pending hub: chooses the sign-in mode and creates the first administrator. */
export default function SetupClaimPage({ configuration }: { configuration: Configuration }) {
  const t = useTranslations("setup.claim");
  const ta = useTranslations("settings.auth");
  const adapter = useAdapter();
  const client = useQueryClient();
  const [mode, setMode] = useState<AuthMode>(configuration.mode);
  const [proxyHeader, setProxyHeader] = useState(configuration.proxyHeader || "Remote-User");
  const [trustedProxies, setTrustedProxies] = useState(configuration.trustedProxies ?? "");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ReturnType<typeof claimError> | null>(null);
  const forced = configuration.forced;
  const effective = forced ? configuration.mode : mode;
  const errors = {
    username: username.trim() ? undefined : t("usernameRequired"),
    password: effective !== "local" ? undefined : password.length < 12 ? (password ? ta("passwordShort") : t("passwordRequired")) : undefined,
    confirm: effective === "local" && confirm !== password ? ta("passwordMismatch") : undefined,
    trustedProxies: effective === "proxy" && !forced && !trustedProxies.trim() ? t("trustedRequired") : undefined,
  };
  const show = (key: keyof typeof errors) => (tried ? errors[key] : undefined);

  const submit = async () => {
    setTried(true);
    if (Object.values(errors).some(Boolean) || busy) return;
    setBusy(true);
    setError(null);
    try {
      const signedIn = await adapter.claimSetup({
        mode: effective,
        username: username.trim(),
        ...(effective === "proxy" && !forced ? { proxyHeader: proxyHeader.trim(), trustedProxies: trustedProxies.trim() } : {}),
        ...(effective === "local" ? { password } : {}),
      });
      // The form stays mounted until the gates refetch; cleared fields must not show validation errors.
      setTried(false);
      setPassword("");
      setConfirm("");
      patchSetup({ reached: 1 });
      client.setQueryData(queryKeys.currentUser, signedIn);
      await Promise.all([queryKeys.authConfiguration, queryKeys.authMode, queryKeys.setupStatus, queryKeys.currentUser].map((queryKey) => client.invalidateQueries({ queryKey })));
    } catch (cause) {
      setError(claimError(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <SetupFrame connected current="access" reachable={0} onSelect={() => {}}>
      <Card className="gap-0 overflow-hidden p-0">
        <div className="border-b px-5 py-4">
          <h2 className="text-lg font-semibold tracking-tight">{t("title")}</h2>
          <p className="text-sm text-muted-foreground">{t("lead")}</p>
        </div>
        <form className="flex flex-col gap-5 p-5" noValidate onSubmit={(event) => { event.preventDefault(); void submit(); }}>
          {forced ? (
            <Alert>
              <Lock />
              <AlertDescription>{t("forced", { mode: ta(effective) })}</AlertDescription>
            </Alert>
          ) : (
            <ChoiceCards
              label={t("mode")}
              value={mode}
              onChange={setMode}
              columns={3}
              options={[
                { value: "local", title: ta("local"), description: ta("localDesc"), icon: <KeyRound /> },
                { value: "proxy", title: ta("proxy"), description: ta("proxyDesc"), icon: <Waypoints /> },
                { value: "none", title: ta("none"), description: ta("noneDesc"), icon: <ShieldOff /> },
              ]}
            />
          )}
          {effective === "none" && (
            <Alert className="border-warning/40 bg-warning/8">
              <ShieldOff className="text-warning" />
              <AlertDescription>{ta("noneWarning")}</AlertDescription>
            </Alert>
          )}
          {effective === "proxy" && (
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField id="setup-proxy-header" label={ta("userHeader")} value={proxyHeader} onChange={setProxyHeader} disabled={forced} mono />
              <TextField id="setup-trusted" label={ta("trustedProxies")} value={trustedProxies} onChange={setTrustedProxies} description={ta("trustedProxiesHint")} error={show("trustedProxies")} disabled={forced} mono />
              <p className="text-xs text-muted-foreground sm:col-span-2">{t("proxyCheck")}</p>
            </div>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField id="setup-username" label={t("username")} value={username} onChange={setUsername} description={t(effective === "proxy" ? "usernameProxyHint" : "usernameHint")} error={show("username")} autoComplete="username" className="sm:col-span-2 sm:max-w-sm" />
            {effective === "local" && <>
              <TextField id="setup-password" type="password" label={ta("password")} value={password} onChange={setPassword} description={ta("passwordHint")} error={show("password")} autoComplete="new-password" />
              <TextField id="setup-confirm" type="password" label={ta("passwordConfirm")} value={confirm} onChange={setConfirm} error={show("confirm")} autoComplete="new-password" />
            </>}
          </div>
          {error && <p role="alert" className="text-sm text-destructive">{t(`errors.${errorKey(error)}`)}</p>}
          <div className="flex justify-end border-t pt-4">
            <Button type="submit" disabled={busy}>
              {busy && <Spinner data-icon="inline-start" />}
              {t(busy ? "creating" : "create")}
            </Button>
          </div>
        </form>
      </Card>
    </SetupFrame>
  );
}
