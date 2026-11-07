// Source: docklet-next/issues/359
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original production coming-soon page with no adapter reads. M20 #360: localised labels, neutral notation and keyboard log navigation.

import { Gamepad2, Waypoints } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { IconTile, ToneBadge } from "@/shared/components/tone";

export default function ComingSoonPage({ area }: { area: "proxy" | "games" }) {
  const t = useTranslations("comingSoon");
  const nav = useTranslations("shell.nav");
  const Icon = area === "proxy" ? Waypoints : Gamepad2;
  return <section className="mx-auto flex max-w-2xl flex-col items-start gap-4 rounded-xl border bg-card p-6">
    <IconTile tone="accent" size="lg"><Icon /></IconTile>
    <ToneBadge tone="violet">{t("badge")}</ToneBadge>
    <h1 className="text-xl font-semibold">{nav(area)}</h1>
    <p className="text-muted-foreground">{t("body", { area: nav(area) })}</p>
    <Link to="/dashboard" className="text-primary-text underline">{t("back")}</Link>
  </section>;
}
