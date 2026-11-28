// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original time-bin strip with hover hint for the M47 log views.

import { useState } from "react";

export interface StripBin { tone: string; height?: number; label: string; onSelect?: () => void; selected?: boolean }
/** One row of time bins. Hovering or focusing a bin shows its label above it; selectable bins are buttons. */
export function BinStrip({ bins, label, className = "h-4" }: { bins: StripBin[]; label: string; className?: string }) {
  const [hover, setHover] = useState<number>();
  const shown = hover === undefined ? undefined : bins[hover];
  const at = hover === undefined ? 0 : (hover + 0.5) / bins.length;
  return <div role="group" aria-label={label} className={`relative flex flex-1 items-end gap-px ${className}`} onPointerLeave={() => setHover(undefined)}>
    {shown && <span aria-hidden="true" className={`pointer-events-none absolute bottom-full z-10 mb-1 whitespace-nowrap rounded-md border bg-popover px-2 py-0.5 text-xs text-popover-foreground shadow-sm ${at < 0.25 ? "" : at > 0.75 ? "-translate-x-full" : "-translate-x-1/2"}`} style={{ left: `${at * 100}%` }}>{shown.label}</span>}
    {bins.map((bin, i) => {
      const style = { height: `${Math.max(bin.height ?? 1, 0.08) * 100}%` };
      const common = { className: `min-w-0 flex-1 ${bin.tone}${bin.selected ? " outline outline-2 outline-ring" : ""}`, style, onPointerEnter: () => setHover(i) };
      return bin.onSelect
        ? <button key={i} type="button" aria-label={bin.label} aria-pressed={bin.selected} {...common} onFocus={() => setHover(i)} onBlur={() => setHover(undefined)} onClick={bin.onSelect} />
        : <span key={i} {...common} />;
    })}
  </div>;
}
