// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Mode-aware route guard and administrator locale sync.

import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { setAppLocale } from "@/app/i18n/locale-store";
import { AdapterOutcomeError } from "@/data/capability";
import { useAuthMode, useCurrentUser, useSetupStatus } from "@/data/queries";
import { operationReasonKey } from "@/data/operation-reason";
import { isLocale } from "@/domain/locale";
import { Button } from "@/shared/ui/button";

export function AuthGate() {
  const t = useTranslations("login");
  const reasons = useTranslations("containers.operationReasons");
  const mode = useAuthMode();
  const currentUser = useCurrentUser();
  const setup = useSetupStatus(currentUser.isSuccess && !!currentUser.data);
  const location = useLocation();
  const [unauthorized, setUnauthorized] = useState(false);
  const savedLocale = currentUser.data?.locale;
  useEffect(() => { if (isLocale(savedLocale)) setAppLocale(savedLocale); }, [savedLocale]);
  useEffect(() => {
    const reject = () => setUnauthorized(true);
    window.addEventListener("docklet:unauthorized", reject);
    return () => window.removeEventListener("docklet:unauthorized", reject);
  }, []);
  if (mode.isPending || (mode.data && currentUser.isPending)) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (mode.isError) return <main className="p-6"><p>{t("settingsError")}</p><Button onClick={() => void mode.refetch()}>{t("retry")}</Button></main>;
  if (currentUser.isError || unauthorized || (currentUser.isSuccess && !currentUser.data)) {
    const rejected = (currentUser.isSuccess && !currentUser.data) || unauthorized || (currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401);
    if (rejected && mode.data === "local") return <Navigate to="/login" state={{ from: location.pathname }} replace />;
    const code = currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" ? currentUser.error.outcome.code : undefined;
    const refusal = code === "access.not-registered" || code === "access.disabled" ? operationReasonKey(code) : undefined;
    if (refusal) return <main className="p-6" role="alert">{reasons(refusal)}</main>;
    if (rejected && mode.data === "proxy") return <main className="p-6" role="alert">{t("proxyRejected")}</main>;
    return <main className="p-6"><p>{t("sessionError")}</p><Button onClick={() => void currentUser.refetch()}>{t("retry")}</Button></main>;
  }
  if (mode.data === "local" && currentUser.data?.requiresPasswordChange) return <Navigate to="/setup" replace />;
  if (setup.isPending) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (setup.isError) return <main className="p-6"><p>{t("setupStatusError")}</p><Button onClick={() => void setup.refetch()}>{t("retry")}</Button></main>;
  if (!setup.data) return <Navigate to="/setup" replace />;
  return <Outlet />;
}
