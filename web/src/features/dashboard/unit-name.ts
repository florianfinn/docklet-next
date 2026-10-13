import { useTranslations } from "use-intl";

import type { FleetUnit } from "./fleet-status";

/** Display name of a unit: the stack name, or the origin group with its size. */
export function useUnitName() {
  const t = useTranslations("dashboard.ring.group");
  return (u: FleetUnit) => (u.kind === "stack" ? u.name : t(u.origin ?? "standalone", { count: u.containers.length }));
}
