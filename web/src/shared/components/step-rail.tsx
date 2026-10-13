import { Check } from "lucide-react";

import { cn } from "@/shared/lib/utils";

export interface RailStep {
  id: string;
  title: string;
  description?: string;
}

interface StepRailProps {
  steps: RailStep[];
  current: string;
  /** Steps up to this index may be revisited by clicking. */
  reachable: number;
  onSelect: (id: string) => void;
  label: string;
}

/** Wizard progress: vertical on wide screens, compact horizontal strip on phones. */
export function StepRail({ steps, current, reachable, onSelect, label }: StepRailProps) {
  const index = steps.findIndex((s) => s.id === current);
  return (
    <nav aria-label={label}>
      <ol className="flex gap-1.5 overflow-x-auto pb-1 lg:flex-col lg:gap-1 lg:overflow-visible lg:pb-0">
        {steps.map((s, i) => {
          const done = i < index;
          const active = i === index;
          const enabled = i <= reachable && !active;
          return (
            <li key={s.id} className="min-w-0 shrink-0 lg:shrink">
              <button
                type="button"
                disabled={!enabled}
                aria-current={active ? "step" : undefined}
                onClick={() => onSelect(s.id)}
                className={cn(
                  "group flex w-full items-center gap-3 rounded-xl border border-transparent px-2.5 py-2 text-left outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50",
                  active && "border-primary/40 bg-primary/10",
                  enabled && "hover:bg-muted/50",
                  !enabled && !active && "cursor-default",
                )}
              >
                <span
                  className={cn(
                    "grid size-7 shrink-0 place-items-center rounded-full border text-xs font-semibold tabular transition-[background,border-color,color] duration-(--dur)",
                    done && "border-success/50 bg-success/15 text-success",
                    active && "border-primary bg-primary text-primary-foreground shadow-md shadow-highlight/30",
                    !done && !active && "text-muted-foreground",
                  )}
                >
                  {done ? <Check className="size-3.5 animate-pop-in" strokeWidth={3} /> : i + 1}
                </span>
                <span className="hidden min-w-0 sm:grid">
                  <span className={cn("truncate text-sm", active ? "font-semibold" : "text-muted-foreground")}>{s.title}</span>
                  {s.description && <span className="hidden truncate text-xs text-muted-foreground lg:block">{s.description}</span>}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
