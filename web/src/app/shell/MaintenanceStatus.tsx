// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original compact hub maintenance status next to the live indicator.

import { Wrench } from "lucide-react";
import { useTranslations } from "use-intl";

import { useMaintenance } from "@/data/use-maintenance";

/** Cached content stays visible while the hub is in maintenance; this status explains paused updates. */
export function MaintenanceStatus() {
  const t = useTranslations("shell.maintenance");
  const state = useMaintenance();
  if (!state.active) return null;
  const label = state.overdue ? t("overdue") : t(state.operation ?? "active");
  return (
    <span role="status" data-hub-maintenance={state.operation ?? "active"} className="flex h-8 items-center gap-1.5 rounded-md px-2 text-sm text-warning">
      <Wrench className="size-4" aria-hidden />
      <span className="hidden sm:inline">{label}</span>
      <span className="sr-only sm:hidden">{label}</span>
    </span>
  );
}
