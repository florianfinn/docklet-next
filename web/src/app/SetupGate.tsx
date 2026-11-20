// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original first-run route gate and administrator locale sync; M29 first-run claim before any sign-in.

import { lazy, Suspense, useEffect, useState } from "react";
import { Navigate } from "react-router";
import { useTranslations } from "use-intl";

import { InitialPasswordPage } from "@/features/setup/InitialPasswordPage";

import { setAppLocale } from "@/app/i18n/locale-store";
import { useHubRetry } from "@/shared/components/hub-update/gate-retry";
import { GateScreen, UnreachableGate } from "@/shared/components/gate-screen";
import { AdapterOutcomeError } from "@/data/capability";
import { useAuthMode, useCurrentUser, useSetupStatus, useSignInConfiguration } from "@/data/queries";
import { isLocale } from "@/domain/locale";
import { Button } from "@/shared/ui/button";

const SetupPage = lazy(() => import("@/features/setup/SetupPage"));
const SetupClaimPage = lazy(() => import("@/features/setup/SetupClaimPage"));

export function SetupGate() {
  const t = useTranslations("login");
  const mode = useAuthMode();
  const configuration = useSignInConfiguration();
  const currentUser = useCurrentUser();
  const setup = useSetupStatus(currentUser.isSuccess && !!currentUser.data);
  const retry = useHubRetry([mode, configuration, currentUser, setup]);
  const savedLocale = currentUser.data?.locale;
  useEffect(() => { if (isLocale(savedLocale)) setAppLocale(savedLocale); }, [savedLocale]);
  if (retry.unreachable) return <UnreachableGate {...retry} />;
  if (configuration.isError) return <GateScreen message={t("settingsError")} action={<Button onClick={() => void configuration.refetch()}>{t("retry")}</Button>} />;
  // A pending hub has no account yet: setup itself creates the first administrator, without a sign-in screen.
  if (configuration.data?.setupPending) return <Suspense fallback={<GateScreen busy message={t("loading")} />}><SetupClaimPage configuration={configuration.data} /></Suspense>;
  if (currentUser.isError || (currentUser.isSuccess && !currentUser.data)) {
    const rejected = (currentUser.isSuccess && !currentUser.data) || currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401;
    if (rejected && mode.data === "local") return <Navigate to="/login" replace />;
    if (rejected && mode.data === "proxy") return <GateScreen message={t("proxyRejected")} />;
  }
  if (mode.isError || currentUser.isError || currentUser.data === null || setup.isError) return <GateScreen message={t("setupStatusError")} action={<Button onClick={() => { void mode.refetch(); void currentUser.refetch(); void setup.refetch(); }}>{t("retry")}</Button>} />;
  if (configuration.isPending || mode.isPending || currentUser.isPending || setup.isPending) return <GateScreen busy message={t("loading")} />;
  if (setup.data && mode.data === "local" && currentUser.data?.requiresPasswordChange) return <InitialPasswordPage />;
  if (currentUser.data?.isAdmin === false) return <Navigate to="/dashboard" replace />;
  return <SetupSession completed={setup.data} />;
}

function SetupSession({ completed }: { completed: boolean }) {
  const t = useTranslations("login");
  // Keep the Done screen mounted when its link publishes the completed setup status.
  const [startedHere] = useState(!completed);
  if (completed && !startedHere) return <Navigate to="/dashboard" replace />;
  return <Suspense fallback={<GateScreen busy message={t("loading")} />}><SetupPage /></Suspense>;
}
