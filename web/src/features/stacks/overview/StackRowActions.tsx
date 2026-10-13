import { useTranslations } from "use-intl";

import type { Stack } from "@/domain/model";
import { ActionsMenu, type MenuGroups } from "@/shared/components/action-menu";

export function StackRowActions({ stack, menu }: { stack: Stack; menu: MenuGroups }) {
  const tc = useTranslations("common.actions");
  return <ActionsMenu groups={menu} label={`${tc("more")}: ${stack.name}`} />;
}
