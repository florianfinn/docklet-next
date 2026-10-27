// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { useTranslations } from "use-intl";

export function UpdateUnavailable({ reason }: { reason?: string }) {
  const t = useTranslations("activity.capability");
  return <p role="status" className="text-sm text-muted-foreground">{reason ?? t("unavailable")}</p>;
}
