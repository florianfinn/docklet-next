import { useId } from "react";

import type { SparklineProps } from "@/shared/component-set/types";
import { cn } from "@/shared/lib/utils";

import { areaPath, linePath, sparkPoints } from "./sparkline-geometry";

/**
 * Free sparkline: plain SVG with straight segments, soft fill and a marker on
 * the latest value. The marker is HTML so the stretched viewBox cannot distort it.
 */
export function Sparkline({ data, type = "line", color = "var(--chart-2)", domain, animate = false, label, className }: SparklineProps) {
  const gradientId = `spark-${useId().replace(/:/g, "")}`;
  const points = sparkPoints(data, domain);
  const last = points.at(-1);
  const barWidth = data.length ? 100 / data.length : 0;

  return (
    <span role="img" aria-label={label} data-slot="sparkline" className={cn("relative block h-10 w-24", className)}>
      <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden className="absolute inset-0 size-full overflow-visible">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" style={{ stopColor: color, stopOpacity: 0.28 }} />
            <stop offset="100%" style={{ stopColor: color, stopOpacity: 0 }} />
          </linearGradient>
        </defs>
        <line x1="0" x2="100" y1="100" y2="100" vectorEffect="non-scaling-stroke" className="stroke-border" strokeWidth={1} />
        {type === "bar" ? (
          points.map((p, i) => (
            <rect key={i} x={i * barWidth + barWidth * 0.15} width={barWidth * 0.7} y={p.y} height={100 - p.y} rx={1} style={{ fill: color }} opacity={i === points.length - 1 ? 1 : 0.55} />
          ))
        ) : (
          <>
            {type === "area" && <path d={areaPath(points)} fill={`url(#${gradientId})`} />}
            <path d={linePath(points)} fill="none" style={{ stroke: color }} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" vectorEffect="non-scaling-stroke" />
          </>
        )}
      </svg>
      {last && type !== "bar" && (
        <span
          aria-hidden
          className={cn("absolute size-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2 ring-card", animate && "transition-[top] duration-(--dur) ease-(--ease-out)")}
          style={{ left: `${last.x}%`, top: `${last.y}%`, background: color }}
        />
      )}
    </span>
  );
}
