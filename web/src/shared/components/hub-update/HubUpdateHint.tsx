// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original accessible hub update badge shared by both navigation surfaces.

import { useTranslations } from "use-intl";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export function HubUpdateHint({ version }: { version?: string }) {
  const t = useTranslations("common.hubUpdate");
  if (!version) return null;
  return <Tooltip><TooltipTrigger asChild><span className="inline-flex items-center" tabIndex={0}><span aria-hidden className="size-2 rounded-full bg-primary" /><span className="sr-only">{t("available", { version })}</span></span></TooltipTrigger><TooltipContent>{t("availableTooltip", { version })}</TooltipContent></Tooltip>;
}
