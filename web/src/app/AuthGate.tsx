// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Mode-aware route guard and administrator locale sync; M29 pending setup opens setup before sign-in.

import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { setAppLocale } from "@/app/i18n/locale-store";
import { useHubRetry } from "@/shared/components/hub-update/gate-retry";
import { GateScreen, UnreachableGate } from "@/shared/components/gate-screen";
import { AdapterOutcomeError } from "@/data/capability";
import { useAuthMode, useCurrentUser, useSetupStatus, useSignInConfiguration } from "@/data/queries";
import { operationReasonKey } from "@/data/operation-reason";
import { isLocale } from "@/domain/locale";
import { Button } from "@/shared/ui/button";

export function AuthGate() {
  const t = useTranslations("login");
  const reasons = useTranslations("containers.operationReasons");
  const mode = useAuthMode();
  const configuration = useSignInConfiguration();
  const currentUser = useCurrentUser();
  const setup = useSetupStatus(currentUser.isSuccess && !!currentUser.data);
  const location = useLocation();
  const [unauthorized, setUnauthorized] = useState(false);
  const retry = useHubRetry([mode, configuration, currentUser, setup]);
  const savedLocale = currentUser.data?.locale;
  useEffect(() => { if (isLocale(savedLocale)) setAppLocale(savedLocale); }, [savedLocale]);
  useEffect(() => {
    const reject = () => setUnauthorized(true);
    window.addEventListener("docklet:unauthorized", reject);
    return () => window.removeEventListener("docklet:unauthorized", reject);
  }, []);
  if (retry.unreachable) return <UnreachableGate {...retry} />;
  if (configuration.isError) return <GateScreen message={t("settingsError")} action={<Button onClick={() => void configuration.refetch()}>{t("retry")}</Button>} />;
  if (configuration.data?.setupPending) return <Navigate to="/setup" replace />;
  if (mode.isError) return <GateScreen message={t("settingsError")} action={<Button onClick={() => void mode.refetch()}>{t("retry")}</Button>} />;
  if (currentUser.isError || unauthorized || (currentUser.isSuccess && !currentUser.data)) {
    const rejected = (currentUser.isSuccess && !currentUser.data) || unauthorized || (currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401);
    if (rejected && mode.data === "local") return <Navigate to="/login" state={{ from: location.pathname }} replace />;
    const code = currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" ? currentUser.error.outcome.code : undefined;
    const refusal = code === "access.not-registered" || code === "access.disabled" ? operationReasonKey(code) : undefined;
    if (refusal) return <GateScreen message={reasons(refusal)} />;
    if (rejected && mode.data === "proxy") return <GateScreen message={t("proxyRejected")} />;
    return <GateScreen message={t("sessionError")} action={<Button onClick={() => void currentUser.refetch()}>{t("retry")}</Button>} />;
  }
  if (setup.isError) return <GateScreen message={t("setupStatusError")} action={<Button onClick={() => void setup.refetch()}>{t("retry")}</Button>} />;
  if (configuration.isPending || mode.isPending || (mode.data && currentUser.isPending)) return <GateScreen busy message={t("loading")} />;
  if (mode.data === "local" && currentUser.data?.requiresPasswordChange) return <Navigate to="/setup" replace />;
  if (setup.isPending) return <GateScreen busy message={t("loading")} />;
  if (!setup.data) return <Navigate to="/setup" replace />;
  return <Outlet />;
}
