// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original read-only startup sign-in mode presentation; M29 runtime sign-in mode change.

import { useTranslations } from "use-intl";

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { UsersSection } from "../accounts/UsersSection";
import { useAuthMode } from "@/data/queries";
import { Card } from "@/shared/ui/card";
import { Button } from "@/shared/ui/button";

import { SignInPolicyForm } from "./SignInPolicyForm";

export function ConnectedAuthSection() {
  const t = useTranslations("settings");
  const mode = useAuthMode();
  const adapter = useAdapter();
  const configuration = useQuery({ queryKey: ["auth-configuration"], queryFn: () => adapter.getAuthConfiguration() });
  const policy = useQuery({ queryKey: ["sign-in-policy"], queryFn: () => adapter.getSignInPolicy() });
  return <Card className="max-w-5xl p-5">
    <h2 className="text-lg font-semibold">{t("sections.auth")}</h2>
    {mode.isError || policy.isError ? <Button onClick={() => { void mode.refetch(); void policy.refetch(); }}>{t("auth.retryMode")}</Button> : mode.data && policy.data ? <>
      <SignInPolicyForm key={`${policy.data.mode}:${policy.dataUpdatedAt}`} policy={policy.data} />
    </> : <p aria-busy="true">{t("auth.loadingMode")}</p>}
    {mode.data === "proxy" && <p className="text-sm">{t("liveUsers.proxyLogout")}: {configuration.data?.logoutUrl || t("liveUsers.noProxyLogout")}</p>}
    {mode.data && <UsersSection mode={mode.data} />}
  </Card>;
}
