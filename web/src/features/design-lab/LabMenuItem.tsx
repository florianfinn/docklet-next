import { FlaskConical } from "lucide-react";
import { useTranslations } from "use-intl";

import { DropdownMenuItem, DropdownMenuShortcut } from "@/shared/ui/dropdown-menu";

import { labOpenStore } from "./lab-open-store";

export function LabMenuItem() {
  const t = useTranslations("shell.user");
  return (
    <DropdownMenuItem onSelect={() => labOpenStore.set(true)}>
      <FlaskConical />
      {t("lab")}
      <DropdownMenuShortcut>{t("labShortcut")}</DropdownMenuShortcut>
    </DropdownMenuItem>
  );
}
