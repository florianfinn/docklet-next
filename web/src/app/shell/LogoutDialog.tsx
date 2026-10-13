import { useTranslations } from "use-intl";

import { useSettings } from "@/data/queries";
import { AlertDialog, AlertDialogAction, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";

/** Simulated logout explanation that follows the hub's login mode; the prototype has no session. */
export function LogoutDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("shell.user");
  const settings = useSettings();
  const auth = settings.data?.auth;
  const body =
    auth?.mode === "none"
      ? t("logoutNone")
      : auth?.mode === "proxy"
        ? t("logoutProxy", { url: auth.proxy.logoutUrl || t("logoutProxyDefault") })
        : auth?.mode === "local"
          ? t("logoutLocal", { user: auth.local.username })
          : t("logoutBody");
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("logoutTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogAction>{t("logoutConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
