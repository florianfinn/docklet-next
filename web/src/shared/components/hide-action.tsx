import { Eye, EyeOff } from "lucide-react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { setHidden, type HiddenKind } from "@/shared/preferences/hidden-store";

import type { MenuAction } from "./action-menu";

/** Menu entry that hides an entry from its overview or shows it again, with an undo toast. */
export function useHideAction(kind: HiddenKind) {
  const t = useTranslations("common.hidden");
  return (id: string, name: string, hidden: boolean): MenuAction => ({
    id: hidden ? "restore" : "hide",
    label: hidden ? t("restore") : t("hide"),
    icon: hidden ? <Eye /> : <EyeOff />,
    onSelect: () => {
      setHidden(kind, id, !hidden);
      if (hidden) toast.success(t("restoredToast", { name }));
      else toast(t("hiddenToast", { name }), { description: t("hiddenToastHint"), action: { label: t("undo"), onClick: () => setHidden(kind, id, false) } });
    },
  });
}
