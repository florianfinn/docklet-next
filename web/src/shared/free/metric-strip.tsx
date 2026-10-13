import type { StatStripProps } from "@/shared/component-set/types";
import { cn } from "@/shared/lib/utils";

/**
 * Free metric strip: a band between two hairlines with centred totals and
 * vertical separators (two columns on phones, one row from md). Each metric is
 * one dt/dd pair inside its own group so the description list stays valid.
 */
export function MetricStrip({ items, className }: StatStripProps) {
  return (
    <dl data-slot="metric-strip" className={cn("mb-6 grid grid-cols-2 border-y border-border/60 md:grid-flow-col md:auto-cols-fr md:grid-cols-none", className)}>
      {items.map((item) => (
        <div
          key={item.label}
          role="group"
          data-tone={item.tone ?? "gray"}
          className={cn(
            "flex min-w-0 flex-col items-center justify-center gap-0.5 px-3 py-3.5 text-center",
            "border-border/60 max-md:odd:border-r max-md:[&:nth-child(n+3)]:border-t md:not-first:border-l",
          )}
        >
          <dt className="flex items-center gap-1.5 text-[13px] text-muted-foreground [&_svg]:size-3.5 [&_svg]:opacity-80">
            {item.icon}
            <span className="truncate">{item.label}</span>
          </dt>
          <dd className="flex flex-col items-center">
            <span className={cn("text-[26px] leading-tight font-semibold tracking-tight tabular-nums", item.tone && "text-(--tone)")}>{item.value}</span>
            {item.sub && <span className="text-[11px] text-muted-foreground">{item.sub}</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}
