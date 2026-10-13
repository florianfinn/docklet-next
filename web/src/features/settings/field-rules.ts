import { useTranslations } from "use-intl";

import type { AuthSettings, GeneralSettings, HostDefaults, ProxyAuth } from "@/domain/settings";
import { absolutePath, headerName, hubAddress, installDirProblem, optionalId, required, url } from "@/domain/validation";

export const TIME_ZONES = ["Europe/Berlin", "Europe/Vienna", "Europe/Zurich", "Europe/London", "UTC", "America/New_York"];

/** Synthetic Authentik proxy-outpost values; example domains and RFC 5737 addresses only. */
export const AUTHENTIK_EXAMPLE: ProxyAuth = {
  userHeader: "X-authentik-username",
  emailHeader: "X-authentik-email",
  groupsHeader: "X-authentik-groups",
  trustedProxies: "192.0.2.10/32",
  logoutUrl: "https://auth.example.com/outpost.goauthentik.io/sign_out",
};

/** Write-only password fields of the local admin; never part of saved settings. */
export interface PasswordDraft {
  password: string;
  confirm: string;
}

export function useGeneralErrors(g: GeneralSettings) {
  const tv = useTranslations("hosts.validation");
  const tg = useTranslations("settings.general");
  const name = required(g.hubName);
  const address = hubAddress(g.hubAddress);
  const domain = g.domainUrl.trim() ? url(g.domainUrl) : undefined;
  return {
    hubName: name ? tv(name as "required") : undefined,
    hubAddress: address === "required" ? tv("required") : address ? tg("hubAddressInvalid") : undefined,
    domainUrl: domain ? tg("urlInvalid") : undefined,
  };
}

/** Password is required for local mode only when none is set yet. */
export function useAuthErrors(a: AuthSettings, pw: PasswordDraft) {
  const tv = useTranslations("hosts.validation");
  const t = useTranslations("settings.auth");
  const e: Partial<Record<"username" | "password" | "confirm" | "userHeader" | "emailHeader" | "logoutUrl", string>> = {};
  if (a.mode === "local") {
    if (required(a.local.username)) e.username = tv("required");
    const needs = !a.local.passwordSet || pw.password.length > 0;
    if (needs && pw.password.length < 12) e.password = pw.password ? t("passwordShort") : tv("required");
    if (needs && pw.confirm !== pw.password) e.confirm = t("passwordMismatch");
  }
  if (a.mode === "proxy") {
    const uh = headerName(a.proxy.userHeader);
    if (uh) e.userHeader = uh === "required" ? tv("required") : t("headerInvalid");
    if (a.proxy.emailHeader && headerName(a.proxy.emailHeader)) e.emailHeader = t("headerInvalid");
    if (a.proxy.logoutUrl && url(a.proxy.logoutUrl)) e.logoutUrl = t("logoutUrlHint");
  }
  return e;
}

export function useHostDefaultErrors(h: HostDefaults) {
  const tv = useTranslations("hosts.validation");
  const msg = (k: string | undefined) => (k ? tv(k as "required") : undefined);
  const projectDir = absolutePath(h.projectDir);
  const installDir = absolutePath(h.installDir) ?? (projectDir ? undefined : installDirProblem(h.projectDir, h.installDir));
  return {
    projectDir: msg(projectDir),
    installDir: msg(installDir),
    puid: msg(optionalId(h.puid)),
    pgid: msg(optionalId(h.pgid)),
  };
}
