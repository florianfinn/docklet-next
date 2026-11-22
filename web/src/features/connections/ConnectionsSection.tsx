// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { useTranslations } from "use-intl";
import { TailnetConnection } from "./TailnetConnection";

export function ConnectionsSection() {
  const t = useTranslations("connections");
  return <section className="min-w-0 max-w-5xl overflow-hidden rounded-xl border bg-card"><header className="border-b p-5"><h2 className="text-lg font-semibold">{t("title")}</h2><p className="mt-1 text-sm text-muted-foreground">{t("lead")}</p></header><div className="min-w-0 p-4 sm:p-5"><TailnetConnection /></div></section>;
}
