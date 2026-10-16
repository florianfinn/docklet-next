// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original first-run route gate and administrator locale sync.

import { useEffect } from "react";
import { Navigate } from "react-router";
import { useTranslations } from "use-intl";

import { setAppLocale } from "@/app/i18n/locale-store";
import { AdapterOutcomeError } from "@/data/capability";
import { useAuthMode, useCurrentUser, useSetupStatus } from "@/data/queries";
import { isLocale } from "@/domain/locale";
import { Button } from "@/shared/ui/button";

import SetupPage from "@/features/setup/SetupPage";

export function SetupGate() {
  const t = useTranslations("login");
  const mode = useAuthMode();
  const currentUser = useCurrentUser();
  const setup = useSetupStatus(currentUser.isSuccess);
  const savedLocale = currentUser.data?.locale;
  useEffect(() => { if (isLocale(savedLocale)) setAppLocale(savedLocale); }, [savedLocale]);
  if (mode.isPending || currentUser.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (currentUser.isError) {
    const rejected = currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401;
    if (rejected && mode.data === "local") return <Navigate to="/login" replace />;
    if (rejected && mode.data === "proxy") return <main role="alert" className="p-6">{t("proxyRejected")}</main>;
  }
  if (mode.isError || currentUser.isError || setup.isError) return <main className="p-6"><p>{t("setupStatusError")}</p><Button onClick={() => { void mode.refetch(); void currentUser.refetch(); void setup.refetch(); }}>{t("retry")}</Button></main>;
  if (setup.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (setup.data) return <Navigate to="/dashboard" replace />;
  return <SetupPage />;
}
