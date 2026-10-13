import { useLayoutEffect, useRef, type ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { ToggleGroup as ToggleGroupPrimitive } from "radix-ui";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export interface SegmentedOption<T extends string> {
  value: T;
  label: ReactNode;
  icon?: ReactNode;
  hint?: string;
  /** Overrides the accessible name, e.g. when the label is an abbreviation. */
  ariaLabel?: string;
}

interface SegmentedToggleProps<T extends string> {
  value: T;
  onChange: (value: T) => void;
  options: SegmentedOption<T>[];
  label: string;
  size?: "xs" | "sm" | "md";
  /**
   * "equal": equal-width segments (view switches). "content": each segment as
   * wide as its label (filters); with `wrap` the segments wrap onto further
   * lines instead of overflowing narrow screens.
   */
  fit?: "equal" | "content";
  wrap?: boolean;
  className?: string;
}

/**
 * Single-choice toggle with a sliding indicator. The active segment is marked
 * by aria-checked (Radix radio semantics), so a tooltip wrapper cannot hide it.
 * The indicator is placed from the measured active segment, which also works
 * for content-sized and wrapping segments; without layout (tests, first paint)
 * the index-based CSS position applies. It slides only when the value changes
 * after mount: the first placement, a new page and resizes (fonts, wrapping)
 * set it in place without a transition. Style variants come from
 * html[data-toggle-style].
 */
export function SegmentedToggle<T extends string>({ value, onChange, options, label, size = "sm", fit = "equal", wrap, className }: SegmentedToggleProps<T>) {
  const rootRef = useRef<HTMLDivElement>(null);
  const placed = useRef<{ value: T; box: string } | null>(null);
  const index = Math.max(
    0,
    options.findIndex((o) => o.value === value),
  );

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const measure = () => {
      const item = root.querySelector<HTMLElement>('.seg-item[aria-checked="true"]');
      if (!item || !item.offsetWidth) {
        delete root.dataset.measured;
        placed.current = null;
        return;
      }
      const box = `${item.offsetLeft} ${item.offsetTop} ${item.offsetWidth} ${item.offsetHeight}`;
      const prev = placed.current;
      if (prev?.box === box && prev.value === value) return;
      placed.current = { value, box };
      // Slides only from an already placed segment to a newly chosen value.
      const slide = prev !== null && prev.value !== value;
      if (!slide) root.dataset.instant = "";
      root.style.setProperty("--seg-x", `${item.offsetLeft}px`);
      root.style.setProperty("--seg-y", `${item.offsetTop}px`);
      root.style.setProperty("--seg-w", `${item.offsetWidth}px`);
      root.style.setProperty("--seg-h", `${item.offsetHeight}px`);
      root.dataset.measured = "";
      if (!slide) {
        // Commits the new position while transitions are off, then turns them back on.
        void root.offsetWidth;
        delete root.dataset.instant;
      }
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(root);
    root.querySelectorAll(".seg-item").forEach((el) => ro.observe(el));
    return () => ro.disconnect();
  }, [value, options.length]);

  return (
    <ToggleGroupPrimitive.Root
      ref={rootRef}
      type="single"
      value={value}
      onValueChange={(v) => v && onChange(v as T)}
      aria-label={label}
      data-slot="segmented-toggle"
      data-size={size}
      data-fit={fit}
      data-wrap={wrap || undefined}
      style={{ "--seg-count": options.length, "--seg-index": index } as React.CSSProperties}
      className={cn("seg", className)}
    >
      <span aria-hidden className="seg-indicator" />
      {options.map((o) => {
        const item = (
          <ToggleGroupPrimitive.Item key={o.value} value={o.value} className="seg-item" aria-label={o.ariaLabel ?? (typeof o.label === "string" ? undefined : o.hint)}>
            {o.icon}
            <span className="seg-label" data-text={typeof o.label === "string" ? o.label : undefined}>
              {o.label}
            </span>
          </ToggleGroupPrimitive.Item>
        );
        if (!o.hint) return item;
        return (
          <Tooltip key={o.value}>
            <TooltipTrigger asChild>{item}</TooltipTrigger>
            <TooltipContent>{o.hint}</TooltipContent>
          </Tooltip>
        );
      })}
    </ToggleGroupPrimitive.Root>
  );
}
