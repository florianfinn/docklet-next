import { CheckSquare, X } from "lucide-react";
import { useEffect } from "react";
import { createPortal } from "react-dom";

import type { BulkActionBarProps } from "@/shared/component-set/types";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Kbd } from "@/shared/ui/kbd";

/**
 * Free bulk toolbar docked to the bottom edge: live selection status left,
 * actions in the middle, a labelled clear button with its Escape hint right.
 * Escape is ignored when an overlay already handled it (Radix prevents default).
 */
export function BulkActionBar({ count, selectedLabel, clearLabel, onClear, children, className }: BulkActionBarProps) {
  useEffect(() => {
    if (count === 0) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !event.defaultPrevented && !event.isComposing) onClear();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [count, onClear]);

  if (count === 0) return null;

  return createPortal(
    <div
      role="toolbar"
      aria-label={selectedLabel}
      data-slot="bulk-action-bar"
     
      className={cn(
        "animate-fade-in fixed bottom-4 left-1/2 z-50 flex w-max max-w-[calc(100vw-1.5rem)] -translate-x-1/2 flex-wrap items-center gap-x-3 gap-y-2 rounded-2xl border border-primary/30 bg-popover/95 p-2 pl-3.5 text-popover-foreground shadow-xl shadow-black/25 backdrop-blur-md",
        className,
      )}
    >
      <p role="status" className="flex items-center gap-2 text-sm font-medium whitespace-nowrap">
        <CheckSquare className="size-4 text-primary-text" aria-hidden />
        {selectedLabel}
      </p>
      {/* Narrow screens: status and clear share the first line, actions take the second. */}
      <div className="order-last flex basis-full flex-wrap items-center gap-1.5 sm:order-none sm:basis-auto">{children}</div>
      <Button variant="ghost" size="sm" className="ml-auto h-8 gap-1.5 text-muted-foreground" onClick={onClear} aria-label={clearLabel} aria-keyshortcuts="Escape">
        <X data-icon="inline-start" />
        <span className="hidden sm:inline">{clearLabel}</span>
        <Kbd className="hidden sm:inline-flex">Esc</Kbd>
      </Button>
    </div>,
    document.body,
  );
}
