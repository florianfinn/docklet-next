// Source: docklet-next/issues/581
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original hint for Unraid containers left out per host (M37 #581).

import { EyeOff } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

/** Explains counts that leave out Unraid-managed containers; the choice lives on each host card. */
export function UnraidHiddenHint({ count }: { count: number }) {
  const t = useTranslations("common.hidden");
  if (count <= 0) return null;
  return (
    <p data-slot="unraid-hidden-hint" className="inline-flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
      <EyeOff className="size-3.5" aria-hidden />
      {t("unraid", { count })}
      <Link to="/hosts" className="underline underline-offset-2">{t("unraidLink")}</Link>
    </p>
  );
}
