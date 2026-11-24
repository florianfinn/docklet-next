// Source: docklet-next/issues/615
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original installed/target version badge pair for Settings › Updates and host cards.

import { Link } from "react-router";
import { useTranslations } from "use-intl";
import type { VersionBadgeModel } from "@/domain/self-update";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";

/** Installed version (success tone when current) and, when an update is possible, the highlighted target. */
export function VersionBadges({ model, variant, link, className }: { model: VersionBadgeModel; variant: "panel" | "card"; link?: { href: string; name: string }; className?: string }) {
  const t = useTranslations("common.versionBadge");
  const installed = model.installed === "dev" ? t("development") : model.installed;
  const label = installed ? model.tone === "current" ? t("current", { version: installed }) : installed : t("unknown");
  const size = variant === "card" ? "h-5 px-1.5 text-[11px]" : "h-6";
  return <span data-slot="version-badges" data-tone={model.tone} className={cn("inline-flex min-w-0 flex-wrap items-center gap-1.5", className)}>
    <ToneBadge tone={model.tone === "current" ? "ok" : "gray"} className={cn(size, "font-mono")} data-slot="installed-version">{label}</ToneBadge>
    {model.target && (link
      ? <ToneBadge tone="orange" className={cn(size, "font-mono font-semibold")} asChild><Link to={link.href} aria-label={t("targetLabel", { name: link.name, version: model.target })} data-slot="target-version">{t(variant === "card" ? "target" : "available", { version: model.target })}</Link></ToneBadge>
      : <ToneBadge tone="orange" className={cn(size, "font-mono font-semibold")} data-slot="target-version">{t(variant === "card" ? "target" : "available", { version: model.target })}</ToneBadge>)}
    {/* The panel states its reason in its own status line. */}
    {variant === "card" && model.reason && <span className="sr-only" data-slot="version-reason">{t(`reasons.${model.reason}`)}</span>}
  </span>;
}
