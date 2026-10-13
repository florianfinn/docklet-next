import { BadgeCheck, Box, CircleArrowUp, CircleDashed, CircleFadingArrowUp, TriangleAlert, type LucideIcon } from "lucide-react";
import { useTranslations } from "use-intl";

import type { ImageVersion, UpdateInfo } from "@/domain/model";
import { UPDATE_TONE, updateVisual, type UpdateVisual } from "@/domain/updates";
import { useAdapterText } from "@/data/use-adapter-text";

export const UPDATE_ICON: Record<UpdateVisual, LucideIcon> = {
  current: BadgeCheck,
  major: CircleArrowUp,
  minor: CircleArrowUp,
  patch: CircleArrowUp,
  rebuild: CircleFadingArrowUp,
  error: TriangleAlert,
  local: Box,
  unchecked: CircleDashed,
};

export function latestOf(update: UpdateInfo): ImageVersion | undefined {
  return update.state === "version" || update.state === "rebuild" ? update.latest : undefined;
}

/** Title/description pair for an update result; never derives fix types. */
export function useUpdateCopy() {
  const t = useTranslations("common.updates");
  const dt = useAdapterText();
  return (update: UpdateInfo) => {
    const visual = updateVisual(update);
    const version = latestOf(update)?.version ?? "";
    switch (visual) {
      case "current":
        return { visual, title: t("titleCurrent"), desc: t("descCurrent"), note: t("noteCurrent") };
      case "major":
        return { visual, title: t("titleMajor"), desc: t("descMajor", { version }), note: t("noteMajor") };
      case "minor":
        return { visual, title: t("titleMinor"), desc: t("descMinor", { version }), note: t("noteMinor") };
      case "patch":
        return { visual, title: t("titlePatch"), desc: t("descPatch", { version }), note: t("notePatch") };
      case "rebuild":
        return {
          visual,
          title: t("titleRebuild"),
          desc: t("descRebuild"),
          note: version ? t("noteRebuildVersion", { version }) : t("noteRebuild"),
        };
      case "error":
        return { visual, title: t("titleError"), desc: update.state === "error" ? dt(update.message) : t("descError"), note: "" };
      case "local":
        return { visual, title: t("titleLocal"), desc: t("descLocal"), note: "" };
      default:
        return { visual, title: t("titleUnchecked"), desc: t("descUnchecked"), note: "" };
    }
  };
}

export { UPDATE_TONE };
