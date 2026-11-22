import type { StatStripProps } from "@/shared/component-set/types";
import { cn } from "@/shared/lib/utils";

/**
 * Free metric strip: a band between two hairlines with centred totals and
 * vertical separators (two columns on phones, one row from md). Each metric is
 * one dt/dd pair inside its own group so the description list stays valid.
 */
export function MetricStrip({ items, className, compact }: StatStripProps) {
  // Compact: below md the same list reads as one line of counts ("9 Stacks  2 Problems").
  return (
    <dl data-slot="metric-strip" data-compact={compact || undefined} className={cn("mb-6 grid grid-cols-2 border-y border-border/60 md:grid-flow-col md:auto-cols-fr md:grid-cols-none", compact && "max-md:mb-3 max-md:flex max-md:flex-wrap max-md:gap-x-4 max-md:gap-y-1 max-md:border-0", className)}>
      {items.map((item) => (
        <div
          key={item.label}
          role="group"
          data-tone={item.tone ?? "gray"}
          className={cn(
            "flex min-w-0 flex-col items-center justify-center gap-0.5 px-3 py-3.5 text-center",
            compact ? "border-border/60 max-md:flex-row-reverse max-md:gap-1 max-md:p-0 md:not-first:border-l" : "border-border/60 max-md:odd:border-r max-md:[&:nth-child(n+3)]:border-t md:not-first:border-l",
          )}
        >
          <dt className={cn("flex min-w-0 flex-wrap items-center justify-center gap-1.5 text-[13px] text-muted-foreground [&_svg]:size-3.5 [&_svg]:opacity-80", compact && "max-md:text-sm max-md:[&_svg]:hidden")}>
            {item.icon}
            <span className="truncate">{item.label}</span>
          </dt>
          <dd className="flex flex-col items-center">
            <span className={cn("text-[26px] leading-tight font-semibold tracking-tight tabular-nums", compact && "max-md:text-sm", item.tone && "text-(--tone)")}>{item.value}</span>
            {item.sub && <span className={cn("text-[11px] text-muted-foreground", compact && "max-md:hidden")}>{item.sub}</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}
