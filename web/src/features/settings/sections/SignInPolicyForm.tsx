// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original runtime sign-in mode change in Settings › Sign-in (docs/design/first-run-setup.md).

import { KeyRound, Lock, ShieldOff, Waypoints } from "lucide-react";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { components } from "@/api/generated/schema";
import { AdapterOutcomeError } from "@/data/capability";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { AuthMode } from "@/domain/settings";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";

type Policy = components["schemas"]["AuthSignInConfiguration"];
const POLICY_ERRORS = ["auth.mode-forced", "auth.proxy-unverified", "auth.password-required"] as const;

function policyError(cause: unknown): (typeof POLICY_ERRORS)[number] | "invalid" | "unavailable" {
  if (cause instanceof AdapterOutcomeError && cause.outcome.kind === "error") {
    if ((POLICY_ERRORS as readonly string[]).includes(cause.outcome.code)) return cause.outcome.code as (typeof POLICY_ERRORS)[number];
    if (cause.outcome.status === 400 || cause.outcome.status === 422) return "invalid";
  }
  return "unavailable";
}

/** Changes the stored sign-in mode; a forced AUTH_MODE shows it locked. */
export function SignInPolicyForm({ policy }: { policy: Policy }) {
  const t = useTranslations("settings.auth");
  const adapter = useAdapter();
  const client = useQueryClient();
  const [mode, setMode] = useState<AuthMode>(policy.mode);
  const [proxyHeader, setProxyHeader] = useState(policy.proxyHeader || "Remote-User");
  const [trustedProxies, setTrustedProxies] = useState(policy.trustedProxies ?? "");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ReturnType<typeof policyError> | null>(null);
  if (policy.forced) {
    return <Alert><Lock /><AlertDescription>{t("forcedInfo", { mode: t(policy.mode) })}</AlertDescription></Alert>;
  }
  const changed = mode !== policy.mode || (mode === "proxy" && (proxyHeader !== (policy.proxyHeader || "Remote-User") || trustedProxies !== (policy.trustedProxies ?? "")));
  const apply = async () => {
    if (!changed || busy) return;
    setBusy(true);
    setError(null);
    try {
      const user = await adapter.updateSignIn({ mode, ...(mode === "proxy" ? { proxyHeader: proxyHeader.trim(), trustedProxies: trustedProxies.trim() } : {}), ...(mode === "local" && password ? { password } : {}) });
      setPassword("");
      client.setQueryData(queryKeys.currentUser, user);
      // Every session was revoked: reread the policy and identity before anything else.
      await Promise.all([queryKeys.authConfiguration, queryKeys.authMode, queryKeys.currentUser, ["sign-in-policy"]].map((queryKey) => client.invalidateQueries({ queryKey })));
      toast.success(t("policy.saved"));
    } catch (cause) {
      setError(policyError(cause));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="flex flex-col gap-4" noValidate onSubmit={(event) => { event.preventDefault(); void apply(); }}>
      <ChoiceCards
        label={t("mode")}
        value={mode}
        onChange={setMode}
        columns={3}
        options={[
          { value: "local", title: t("local"), description: t("localDesc"), icon: <KeyRound /> },
          { value: "proxy", title: t("proxy"), description: t("proxyDesc"), icon: <Waypoints /> },
          { value: "none", title: t("none"), description: t("noneDesc"), icon: <ShieldOff /> },
        ]}
      />
      {mode === "none" && <Alert className="border-warning/40 bg-warning/8"><ShieldOff className="text-warning" /><AlertDescription>{t("noneWarning")}</AlertDescription></Alert>}
      {mode === "proxy" && <div className="grid gap-4 sm:grid-cols-2">
        <TextField id="policy-proxy-header" label={t("userHeader")} value={proxyHeader} onChange={setProxyHeader} mono />
        <TextField id="policy-trusted" label={t("trustedProxies")} value={trustedProxies} onChange={setTrustedProxies} description={t("trustedProxiesHint")} mono />
        <p className="text-xs text-muted-foreground sm:col-span-2">{t("policy.proxyCheck")}</p>
      </div>}
      {mode === "local" && policy.mode !== "local" && <TextField id="policy-password" type="password" label={t("policy.password")} value={password} onChange={setPassword} description={t("policy.passwordHint")} autoComplete="new-password" className="sm:max-w-sm" />}
      <p className="text-xs text-muted-foreground">{t("startupModeInfo")}</p>
      {error && <p role="alert" className="text-sm text-destructive">{t(`policy.errors.${error.replace(/[.-](\w)/g, (_, c: string) => c.toUpperCase()) as "invalid"}`)}</p>}
      <div className="flex justify-end">
        <Button type="submit" disabled={!changed || busy}>
          {busy && <Spinner data-icon="inline-start" />}
          {t(busy ? "policy.saving" : "policy.save")}
        </Button>
      </div>
    </form>
  );
}
