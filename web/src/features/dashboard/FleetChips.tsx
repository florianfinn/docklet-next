// Source: docklet-next/issues/371
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original fleet ring filter chips.

import { useTranslations } from "use-intl";

import type { Tone } from "@/domain/model";
import { ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";

import { matchesFleetFilter, type FleetFilter, type FleetUnit } from "./fleet-status";

const CHIPS: readonly [FleetFilter, Tone][] = [
  ["down", "err"],
  ["unhealthy", "warn"],
  ["stopped", "gray"],
  ["running", "ok"],
  ["update", "orange"],
];

/**
 * Hovering or focusing a chip highlights its units in the ring; a click pins the
 * filter until clicked again. Chips without units are hidden unless pinned.
 */
export function FleetChips({ units, pinned, onPin, onHover }: { units: readonly FleetUnit[]; pinned: FleetFilter | null; onPin: (f: FleetFilter | null) => void; onHover: (f: FleetFilter | null) => void }) {
  const t = useTranslations("dashboard.ring.chips");
  return (
    <div role="group" aria-label={t("label")} className="flex flex-wrap justify-center gap-1.5" data-slot="fleet-chips">
      {CHIPS.map(([key, tone]) => {
        const count = units.filter((u) => matchesFleetFilter(u, key)).length;
        if (!count && pinned !== key) return null;
        const on = pinned === key;
        return (
          <button
            key={key}
            type="button"
            data-tone={tone}
            data-chip={key}
            aria-pressed={on}
            className={cn("inline-flex h-7 items-center gap-1.5 rounded-full border border-(--tone)/40 bg-(--tone)/10 px-2.5 text-xs font-medium tabular-nums transition-colors hover:bg-(--tone)/25", on && "bg-(--tone)/25 ring-1 ring-(--tone)")}
            onClick={() => onPin(on ? null : key)}
            onPointerEnter={() => onHover(key)}
            onPointerLeave={() => onHover(null)}
            onFocus={() => onHover(key)}
            onBlur={() => onHover(null)}
          >
            <ToneDot tone={tone} />
            {t(key, { count })}
          </button>
        );
      })}
    </div>
  );
}
