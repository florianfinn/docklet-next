// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original pointer from event-producing sections to the notification targets.

import { Link } from "react-router";
import { useTranslations } from "use-intl";

export function NotificationsPointer() {
  const t = useTranslations("settings.notifications");
  return (
    <p className="text-xs text-muted-foreground">
      {t("pointer")}{" "}
      <Link to="/settings/notifications" className="text-primary-text underline-offset-4 hover:underline">{t("pointerLink")}</Link>
    </p>
  );
}
