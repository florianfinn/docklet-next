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
  const savedLocale = currentUser.data?.locale;
  useEffect(() => { if (isLocale(savedLocale)) setAppLocale(savedLocale); }, [savedLocale]);
  if (configuration.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  // A pending hub has no account yet: setup itself creates the first administrator, without a sign-in screen.
  if (configuration.data?.setupPending) return <Suspense fallback={<main aria-busy="true" className="p-6">{t("loading")}</main>}><SetupClaimPage configuration={configuration.data} /></Suspense>;
  if (mode.isPending || currentUser.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (currentUser.isError || (currentUser.isSuccess && !currentUser.data)) {
    const rejected = (currentUser.isSuccess && !currentUser.data) || currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401;
    if (rejected && mode.data === "local") return <Navigate to="/login" replace />;
    if (rejected && mode.data === "proxy") return <main role="alert" className="p-6">{t("proxyRejected")}</main>;
  }
  if (mode.isError || currentUser.isError || currentUser.data === null || setup.isError) return <main className="p-6"><p>{t("setupStatusError")}</p><Button onClick={() => { void mode.refetch(); void currentUser.refetch(); void setup.refetch(); }}>{t("retry")}</Button></main>;
  if (setup.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (setup.data && mode.data === "local" && currentUser.data?.requiresPasswordChange) return <InitialPasswordPage />;
  if (currentUser.data?.isAdmin === false) return <Navigate to="/dashboard" replace />;
  return <SetupSession completed={setup.data} />;
}

function SetupSession({ completed }: { completed: boolean }) {
  const t = useTranslations("login");
  // Keep the Done screen mounted when its link publishes the completed setup status.
  const [startedHere] = useState(!completed);
  if (completed && !startedHere) return <Navigate to="/dashboard" replace />;
  return <Suspense fallback={<main aria-busy="true" className="p-6">{t("loading")}</main>}><SetupPage /></Suspense>;
}
