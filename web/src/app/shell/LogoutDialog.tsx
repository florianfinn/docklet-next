// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/app/shell/LogoutDialog.tsx
// Revision: bea7ad34a5332b1690d2b2982def959232e362dea40b5e85f0c3178fbb148337
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: #69 connected identity and hub settings. Issue #77: session-bound closure and identity generation cleanup. Simple-user management, assigned-stack access and mode-aware self-service.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";

import { useAuthMode, useCurrentUser } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { AlertDialog, AlertDialogAction, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";

/** Sign-out confirmation for the active startup sign-in mode. */
export function LogoutDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("shell.user");
  const adapter = useAdapter();
  const mode = useAuthMode();
  const configuration = useQuery({ queryKey: ["auth-configuration"], queryFn: () => adapter.getAuthConfiguration(), enabled: open && mode.data === "proxy" });
  const currentUser = useCurrentUser(adapter.kind === "http");
  const client = useQueryClient();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const logoutUrl = configuration.data?.logoutUrl;
  const body = mode.data === "proxy" ? logoutUrl ? t("logoutProxy", { url: logoutUrl }) : t("proxyHint") : t("logoutLocal", { user: currentUser.data?.username ?? t("unknownName") });
  if (mode.data === "none") return null;
  return <AlertDialog open={open} onOpenChange={(next) => { if (!busy) { setError(false); onOpenChange(next); } }}>
    <AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>{t("logoutTitle")}</AlertDialogTitle><AlertDialogDescription>{body}</AlertDialogDescription></AlertDialogHeader>
      {error && <p role="alert">{t("logoutError")}</p>}
      <AlertDialogFooter><AlertDialogAction disabled={busy || (mode.data === "proxy" && configuration.isPending)} onClick={(event) => {
        if (adapter.kind === "demo" || mode.data === "proxy" && !logoutUrl) return;
        event.preventDefault();
        setBusy(true); setError(false);
        void adapter.logout().then(() => {
          if (mode.data === "proxy" && logoutUrl) window.location.assign(logoutUrl);
          else { client.setQueryData(queryKeys.currentUser, null); void navigate("/login", { replace: true }); }
          onOpenChange(false);
        }, () => setError(true)).finally(() => setBusy(false));
      }}>{t("logoutConfirm")}</AlertDialogAction></AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>;
}
