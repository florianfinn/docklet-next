// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original restore-point projection consumer for issue #231.

import { useTranslations } from "use-intl";
import type { UpdatePoint } from "@/domain/stack-updates";

export function RestorePointState({ point }: { point: UpdatePoint }) {
  const t = useTranslations("common.restorePoints");
  const held = point.holdState !== "none";
  return <div className="flex flex-col gap-1 text-sm">
    <p>{held ? t(`hold.${point.holdState}`) : point.protectionReason ? t(`protection.${point.protectionReason}`) : t("noRetentionAnchor")}</p>
    {held && point.protectionReason && <p>{t(`protection.${point.protectionReason}`)}</p>}
    <p className="text-muted-foreground">{t(point.failedAttempt ? "kind.failedAttempt" : "kind.point")}</p>
  </div>;
}
