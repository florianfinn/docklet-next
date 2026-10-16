import { useTranslations } from "use-intl";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";

import { useAuthMode, useSettings } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { AlertDialog, AlertDialogAction, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";

/** Simulated logout explanation that follows the hub's login mode; the prototype has no session. */
export function LogoutDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("shell.user");
  const settings = useSettings();
  const mode = useAuthMode();
  const adapter = useAdapter();
  const client = useQueryClient();
  const navigate = useNavigate();
  const auth = settings.data?.auth;
  const activeMode = adapter.kind === "demo" ? auth?.mode : mode.data;
  const body =
    activeMode === "none"
      ? t("logoutNone")
      : activeMode === "proxy"
        ? t("logoutProxy", { url: auth?.proxy.logoutUrl || t("logoutProxyDefault") })
        : activeMode === "local"
          ? t("logoutLocal", { user: auth?.local.username ?? "admin" })
          : t("logoutBody");
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("logoutTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogAction onClick={() => {
            if (adapter.kind === "demo" || activeMode !== "local") return;
            void adapter.logout().then(() => {
              client.removeQueries({ queryKey: queryKeys.currentUser });
              void navigate("/login", { replace: true });
            });
          }}>{t("logoutConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
