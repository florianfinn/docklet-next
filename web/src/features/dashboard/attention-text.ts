import { useTranslations } from "use-intl";

import type { AttentionItem } from "./attention";

/** Headline text for an attention item. */
export function useAttentionText() {
  const t = useTranslations("dashboard.attention");
  return (item: AttentionItem) => {
    const v = item.values;
    switch (item.kind) {
      case "unhealthy":
        return t("unhealthy", { name: String(v.name) });
      case "offline":
        return t("offline", { name: String(v.name) });
      case "failed":
        return String(v.title);
      case "critical":
        return t("critical", { image: String(v.image) });
      case "major":
        return t("major", { name: String(v.name) });
      case "disk":
        return t("disk", { host: String(v.host) });
      default:
        return t("unknownOrigin", { name: String(v.name) });
    }
  };
}
