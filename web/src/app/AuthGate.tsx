// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Mode-aware route guard for authenticated hub routes.

import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { useAuthMode, useCurrentUser } from "@/data/queries";
import { Button } from "@/shared/ui/button";

export function AuthGate() {
  const t = useTranslations("login");
  const mode = useAuthMode();
  const currentUser = useCurrentUser();
  const location = useLocation();
  const [unauthorized, setUnauthorized] = useState(false);
  useEffect(() => {
    const reject = () => setUnauthorized(true);
    window.addEventListener("docklet:unauthorized", reject);
    return () => window.removeEventListener("docklet:unauthorized", reject);
  }, []);
  if (mode.isPending || (mode.data && currentUser.isPending)) return <main aria-busy="true" className="p-6">{t("loading")}</main>;
  if (mode.isError) return <main className="p-6"><p>{t("settingsError")}</p><Button onClick={() => void mode.refetch()}>{t("retry")}</Button></main>;
  if (currentUser.isError || unauthorized) {
    const rejected = unauthorized || (currentUser.error instanceof AdapterOutcomeError && currentUser.error.outcome.kind === "error" && currentUser.error.outcome.status === 401);
    if (rejected && mode.data === "local") return <Navigate to="/login" state={{ from: location.pathname }} replace />;
    if (rejected && mode.data === "proxy") return <main className="p-6" role="alert">{t("proxyRejected")}</main>;
    return <main className="p-6"><p>{t("sessionError")}</p><Button onClick={() => void currentUser.refetch()}>{t("retry")}</Button></main>;
  }
  return <Outlet />;
}
