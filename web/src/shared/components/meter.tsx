import type { ReactNode } from "react";

import { withPercent } from "@/domain/locale";
import { cn } from "@/shared/lib/utils";

/**
 * Arcane metric tile meter: uppercase label, value, tick marks at quarters and
 * a gradient that turns towards destructive near saturation.
 */
export function Meter({
  label,
  value,
  icon,
  sub,
  className,
  size = "md",
  color,
}: {
  label: string;
  value: number;
  icon?: ReactNode;
  sub?: ReactNode;
  className?: string;
  size?: "sm" | "md";
  color?: string;
}) {
  const v = Math.max(0, Math.min(100, value));
  const hot = v >= 90;
  const warm = v >= 75;
  return (
    <div className={cn("min-w-0", className)}>
      <div className="flex items-center justify-between gap-1.5 text-[10.5px] font-semibold tracking-wide whitespace-nowrap text-foreground/70 uppercase">
        <span className="inline-flex items-center gap-1 [&_svg]:size-3">
          {icon}
          {label}
        </span>
        <b className={cn("tabular text-foreground", size === "md" ? "text-[13px]" : "text-[11.5px]", hot && "text-destructive", !hot && warm && "text-warning")}>{withPercent(Math.round(v))}</b>
      </div>
      <div
        role="meter"
        aria-label={label}
        aria-valuenow={Math.round(v)}
        aria-valuemin={0}
        aria-valuemax={100}
        className={cn("relative mt-1.5 overflow-hidden rounded-full bg-muted/60", size === "md" ? "h-1.5" : "h-1")}
      >
        {[25, 50, 75].map((t) => (
          <i key={t} className="absolute top-0 h-full w-px bg-foreground/15" style={{ left: `${t}%` }} />
        ))}
        <span
          className="absolute inset-y-0 left-0 rounded-full transition-[width] duration-(--dur-slow) ease-(--ease-out)"
          style={{
            width: `${v}%`,
            background: hot
              ? "linear-gradient(90deg, var(--primary), var(--destructive))"
              : warm
                ? "linear-gradient(90deg, var(--primary), var(--warning))"
                : `linear-gradient(90deg, color-mix(in oklch, ${color ?? "var(--primary)"} 60%, transparent), ${color ?? "var(--primary)"})`,
          }}
        />
      </div>
      {sub && <div className="mt-1 truncate text-[11px] text-muted-foreground">{sub}</div>}
    </div>
  );
}

/** Slim inline bar for table cells. */
export function MiniBar({ value, tone = "var(--primary)", className }: { value: number; tone?: string; className?: string }) {
  return (
    <span className={cn("relative inline-block h-1 w-14 overflow-hidden rounded-full bg-muted", className)} aria-hidden>
      <span className="absolute inset-y-0 left-0 rounded-full transition-[width] duration-(--dur-slow)" style={{ width: `${Math.min(100, value)}%`, background: tone }} />
    </span>
  );
}
