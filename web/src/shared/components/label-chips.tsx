import { labelTone } from "@/domain/hosts";
import { cn } from "@/shared/lib/utils";

import { ToneBadge } from "./tone";

/** Dynamic label chips; renders nothing for stacks without labels. */
export function LabelChips({ labels, max = 4, className, size = "md" }: { labels: string[]; max?: number; className?: string; size?: "sm" | "md" }) {
  if (!labels.length) return null;
  const shown = labels.slice(0, max);
  const rest = labels.length - shown.length;
  return (
    <span className={cn("flex min-w-0 flex-wrap items-center gap-1", className)}>
      {shown.map((l) => (
        <ToneBadge key={l} tone={labelTone(l)} className={cn(size === "sm" && "h-5 px-1.5 text-[11px]")}>
          {l}
        </ToneBadge>
      ))}
      {rest > 0 && (
        <ToneBadge tone="gray" className={cn(size === "sm" && "h-5 px-1.5 text-[11px]")} title={labels.slice(max).join(", ")}>
          +{rest}
        </ToneBadge>
      )}
    </span>
  );
}
