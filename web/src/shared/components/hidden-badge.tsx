import { Eye, EyeOff } from "lucide-react";
import { useTranslations } from "use-intl";

import { setHidden, useHiddenIds, type HiddenKind } from "@/shared/preferences/hidden-store";
import { Button } from "@/shared/ui/button";

/** Shown on a detail page whose entry is hidden from its overview; one click shows it again. */
export function HiddenBadge({ kind, id, name }: { kind: HiddenKind; id: string; name: string }) {
  const t = useTranslations("common.hidden");
  const hidden = useHiddenIds(kind).includes(id);
  if (!hidden) return null;
  return (
    <span data-slot="hidden-badge" className="inline-flex items-center gap-1 rounded-md border border-dashed px-1.5 py-0.5 text-xs text-muted-foreground">
      <EyeOff className="size-3.5" aria-hidden />
      {t("badge")}
      <Button type="button" variant="link" size="sm" className="h-auto px-1 text-xs" onClick={() => setHidden(kind, id, false)} aria-label={t("restoreItem", { name })}>
        <Eye data-icon="inline-start" />
        {t("restore")}
      </Button>
    </span>
  );
}
