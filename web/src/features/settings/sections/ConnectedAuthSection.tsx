// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original read-only startup sign-in mode presentation.

import { useTranslations } from "use-intl";

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { UsersSection } from "../accounts/UsersSection";
import { useAuthMode } from "@/data/queries";
import { Card } from "@/shared/ui/card";
import { Button } from "@/shared/ui/button";

export function ConnectedAuthSection() {
  const t = useTranslations("settings");
  const mode = useAuthMode();
  const adapter = useAdapter();
  const configuration = useQuery({ queryKey: ["auth-configuration"], queryFn: () => adapter.getAuthConfiguration() });
  return <Card className="max-w-5xl p-5">
    <h2 className="text-lg font-semibold">{t("sections.auth")}</h2>
    {mode.isError ? <Button onClick={() => void mode.refetch()}>{t("auth.retryMode")}</Button> : mode.data ? <>
      <p className="text-sm font-medium">{t(`auth.${mode.data}`)}</p>
      <p className="text-sm text-muted-foreground">{t("auth.startupModeInfo")}</p>
    </> : <p aria-busy="true">{t("auth.loadingMode")}</p>}
    {mode.data === "proxy" && <p className="text-sm">{t("liveUsers.proxyLogout")}: {configuration.data?.logoutUrl || t("liveUsers.noProxyLogout")}</p>}
    {mode.data && <UsersSection mode={mode.data} />}
  </Card>;
}
